package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

type lifecycleState uint8

const (
	stateConstructed lifecycleState = iota
	stateRunning
	stateStopping
	stateStopped
)

// Pool owns the explicit lifecycle and immutable route snapshot for one
// Workers workstation capability.
type Pool struct {
	mu     sync.RWMutex
	state  lifecycleState
	routes map[string]*routePool

	dispatches map[string]*dispatchRecord
	active     sync.WaitGroup
	stopped    chan struct{}
}

type routePool struct {
	binding workstations.Route
	mu      sync.Mutex
	running int
	waiting []*admission
}

type admission struct {
	ready    chan struct{}
	running  bool
	released bool
}

type dispatchRecord struct {
	mu              sync.Mutex
	dispatchID      string
	workstationName string
	transitionID    string
	cancel          context.CancelFunc
	terminal        bool
	canceled        bool
	result          workers.WorkstationDispatchResult
	err             error
}

var _ workstations.Service = (*Pool)(nil)

// New constructs an inert pool without starting background activity.
func New() *Pool {
	return &Pool{
		state:      stateConstructed,
		dispatches: make(map[string]*dispatchRecord),
		stopped:    make(chan struct{}),
	}
}

// Start atomically activates the supplied route snapshot.
func (p *Pool) Start(
	ctx context.Context,
	routes []workstations.Route,
) (workers.WorkstationPoolLifecycleOutcome, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	snapshot, err := routeSnapshot(routes)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.state {
	case stateConstructed:
		p.routes = snapshot
		p.state = stateRunning
		return workers.WorkstationPoolLifecycleOutcomeStarted, nil
	case stateRunning:
		return workers.WorkstationPoolLifecycleOutcomeAlreadyRunning, nil
	case stateStopping, stateStopped:
		return "", workers.ErrWorkstationPoolStopped
	default:
		return "", workers.ErrWorkstationPoolUnavailable
	}
}

// Stop closes admission and converges concurrent callers on a terminal state.
func (p *Pool) Stop(ctx context.Context) (workers.WorkstationPoolLifecycleOutcome, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}

	p.mu.Lock()
	switch p.state {
	case stateStopped:
		p.mu.Unlock()
		return workers.WorkstationPoolLifecycleOutcomeAlreadyStopped, nil
	case stateStopping:
		stopped := p.stopped
		p.mu.Unlock()
		<-stopped
		return workers.WorkstationPoolLifecycleOutcomeAlreadyStopped, nil
	}

	p.state = stateStopping
	active := make([]*dispatchRecord, 0, len(p.dispatches))
	for _, record := range p.dispatches {
		active = append(active, record)
	}
	p.mu.Unlock()

	for _, record := range active {
		record.commitCancellation(context.Canceled)
	}
	p.active.Wait()

	p.mu.Lock()
	p.routes = nil
	p.state = stateStopped
	close(p.stopped)
	p.mu.Unlock()
	return workers.WorkstationPoolLifecycleOutcomeStopped, nil
}

// Route verifies availability against the active immutable snapshot.
func (p *Pool) Route(ctx context.Context, workstationName string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	name := strings.TrimSpace(workstationName)
	if name == "" {
		return workers.ErrUnknownWorkstationRoute
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	switch p.state {
	case stateConstructed:
		return workers.ErrWorkstationPoolUnavailable
	case stateStopping, stateStopped:
		return workers.ErrWorkstationPoolStopped
	case stateRunning:
		if _, ok := p.routes[name]; !ok {
			return workers.ErrUnknownWorkstationRoute
		}
		return nil
	default:
		return workers.ErrWorkstationPoolUnavailable
	}
}

// Dispatch resolves one immutable route binding before invoking its executor.
func (p *Pool) Dispatch(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	if err := contextError(ctx); err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	name, err := validDispatch(request)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}

	executionCtx, cancelExecution := context.WithCancel(ctx)
	record := newDispatchRecord(request, name, cancelExecution)
	route, entry, err := p.accept(record)
	if err != nil {
		cancelExecution()
		return workers.WorkstationDispatchResult{}, err
	}
	defer p.active.Done()
	defer cancelExecution()

	if err := route.await(executionCtx, entry); err != nil {
		return record.commitCancellation(err)
	}
	defer route.release(entry)

	execution := workers.CloneWorkstationExecutionRequest(request.Execution)
	execution.RunnerID = route.binding.RunnerSelection.RunnerID
	execution.RunnerSelectionSource = route.binding.RunnerSelection.Source
	defer func() {
		if panicValue := recover(); panicValue != nil {
			record.commitFailure(fmt.Errorf("workstation executor panic: %v", panicValue))
			panic(panicValue)
		}
	}()
	result, executeErr := route.binding.Executor.Execute(executionCtx, execution)
	result.DispatchID = execution.Dispatch.DispatchID
	result.TransitionID = execution.Dispatch.TransitionID
	dispatchResult := workers.WorkstationDispatchResult{
		DispatchID:      execution.Dispatch.DispatchID,
		WorkstationName: name,
		TerminalOutcome: terminalOutcome(executeErr),
		Result:          result,
	}
	if executionCtx.Err() != nil {
		return record.commitCancellation(executionCtx.Err())
	}
	return record.commit(dispatchResult, executeErr)
}

// Cancel commits cancellation before signaling the execution context, so a
// concurrent executor completion cannot replace the canonical terminal result.
func (p *Pool) Cancel(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if err := contextError(ctx); err != nil {
		return workers.WorkstationDispatchCancelResult{}, err
	}
	dispatchID := strings.TrimSpace(request.DispatchID)
	if dispatchID == "" {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrInvalidWorkstationCancellation
	}

	p.mu.RLock()
	record, ok := p.dispatches[dispatchID]
	p.mu.RUnlock()
	if !ok {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrUnknownWorkstationDispatch
	}
	return record.cancelOutcome()
}

func (p *Pool) accept(
	record *dispatchRecord,
) (*routePool, *admission, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.state {
	case stateConstructed:
		return nil, nil, workers.ErrWorkstationPoolUnavailable
	case stateStopping, stateStopped:
		return nil, nil, workers.ErrWorkstationPoolStopped
	case stateRunning:
		route, ok := p.routes[record.workstationName]
		if !ok {
			return nil, nil, workers.ErrUnknownWorkstationRoute
		}
		if route.binding.Executor == nil {
			return nil, nil, workers.ErrMissingWorkstationBinding
		}
		if _, duplicate := p.dispatches[record.dispatchID]; duplicate {
			return nil, nil, workers.ErrInvalidWorkstationDispatch
		}
		entry, err := route.reserve()
		if err != nil {
			return nil, nil, err
		}
		p.dispatches[record.dispatchID] = record
		p.active.Add(1)
		return route, entry, nil
	default:
		return nil, nil, workers.ErrWorkstationPoolUnavailable
	}
}

func (route *routePool) reserve() (*admission, error) {
	route.mu.Lock()
	defer route.mu.Unlock()
	if route.running < route.binding.Capacity && len(route.waiting) == 0 {
		route.running++
		entry := &admission{ready: make(chan struct{}), running: true}
		close(entry.ready)
		return entry, nil
	}
	if len(route.waiting) >= route.binding.QueueCapacity {
		return nil, workers.ErrWorkstationSaturated
	}
	entry := &admission{ready: make(chan struct{})}
	route.waiting = append(route.waiting, entry)
	return entry, nil
}

func (route *routePool) await(ctx context.Context, entry *admission) error {
	select {
	case <-entry.ready:
		if err := ctx.Err(); err != nil {
			route.release(entry)
			return err
		}
		return nil
	case <-ctx.Done():
		route.cancel(entry)
		return ctx.Err()
	}
}

func (route *routePool) cancel(entry *admission) {
	route.mu.Lock()
	defer route.mu.Unlock()
	if entry.running {
		route.releaseLocked(entry)
		return
	}
	for index, queued := range route.waiting {
		if queued != entry {
			continue
		}
		route.waiting = append(route.waiting[:index], route.waiting[index+1:]...)
		entry.released = true
		return
	}
}

func (route *routePool) release(entry *admission) {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.releaseLocked(entry)
}

func (route *routePool) releaseLocked(entry *admission) {
	if entry.released {
		return
	}
	entry.released = true
	if !entry.running {
		return
	}
	route.running--
	if len(route.waiting) == 0 {
		return
	}
	next := route.waiting[0]
	route.waiting = route.waiting[1:]
	route.running++
	next.running = true
	close(next.ready)
}

func newDispatchRecord(
	request workers.WorkstationDispatchRequest,
	workstationName string,
	cancel context.CancelFunc,
) *dispatchRecord {
	return &dispatchRecord{
		dispatchID:      request.Execution.Dispatch.DispatchID,
		workstationName: workstationName,
		transitionID:    request.Execution.Dispatch.TransitionID,
		cancel:          cancel,
	}
}

func (record *dispatchRecord) commit(
	result workers.WorkstationDispatchResult,
	err error,
) (workers.WorkstationDispatchResult, error) {
	record.mu.Lock()
	defer record.mu.Unlock()
	if record.terminal {
		return record.result, record.err
	}
	record.terminal = true
	record.result = result
	record.err = err
	return result, err
}

func (record *dispatchRecord) commitFailure(err error) {
	result := record.baseResult()
	result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
	result.Result.Outcome = workers.OutcomeFailed
	result.Result.Error = err.Error()
	_, _ = record.commit(result, err)
}

func (record *dispatchRecord) commitCancellation(
	cause error,
) (workers.WorkstationDispatchResult, error) {
	record.mu.Lock()
	if record.terminal {
		result, err := record.result, record.err
		record.mu.Unlock()
		return result, err
	}
	result, err, cancel := record.commitCancellationLocked(cause)
	record.mu.Unlock()
	cancel()
	return result, err
}

func (record *dispatchRecord) commitCancellationLocked(
	cause error,
) (workers.WorkstationDispatchResult, error, context.CancelFunc) {
	record.terminal = true
	record.canceled = true
	record.result = record.baseResult()
	record.result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeCanceled
	record.result.Result.Outcome = workers.OutcomeFailed
	record.result.Result.Error = workers.ErrWorkstationDispatchCanceled.Error()
	record.err = canceledDispatchError(cause)
	return record.result, record.err, record.cancel
}

func (record *dispatchRecord) cancelOutcome() (
	workers.WorkstationDispatchCancelResult,
	error,
) {
	record.mu.Lock()
	if record.terminal {
		outcome := workers.WorkstationDispatchCancelOutcomeAlreadyTerminal
		var err error = workers.ErrWorkstationDispatchAlreadyTerminal
		if record.canceled {
			outcome = workers.WorkstationDispatchCancelOutcomeAlreadyCanceled
			err = nil
		}
		record.mu.Unlock()
		return workers.WorkstationDispatchCancelResult{
			DispatchID: record.dispatchID,
			Outcome:    outcome,
		}, err
	}
	_, _, cancel := record.commitCancellationLocked(context.Canceled)
	record.mu.Unlock()
	cancel()
	return workers.WorkstationDispatchCancelResult{
		DispatchID: record.dispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

func (record *dispatchRecord) baseResult() workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      record.dispatchID,
		WorkstationName: record.workstationName,
		Result: workers.WorkResult{
			DispatchID:   record.dispatchID,
			TransitionID: record.transitionID,
		},
	}
}

func canceledDispatchError(cause error) error {
	if cause == nil {
		return workers.ErrWorkstationDispatchCanceled
	}
	return fmt.Errorf("%w: %w", workers.ErrWorkstationDispatchCanceled, cause)
}

func terminalOutcome(err error) workers.WorkstationDispatchTerminalOutcome {
	if err != nil {
		return workers.WorkstationDispatchTerminalOutcomeFailed
	}
	return workers.WorkstationDispatchTerminalOutcomeCompleted
}

func validDispatch(request workers.WorkstationDispatchRequest) (string, error) {
	name := strings.TrimSpace(request.WorkstationName)
	dispatch := request.Execution.Dispatch
	if name == "" ||
		strings.TrimSpace(dispatch.DispatchID) == "" ||
		strings.TrimSpace(dispatch.WorkstationName) == "" ||
		dispatch.WorkstationName != name {
		return "", workers.ErrInvalidWorkstationDispatch
	}
	return name, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return workers.ErrWorkstationPoolUnavailable
	}
	return ctx.Err()
}

func routeSnapshot(routes []workstations.Route) (map[string]*routePool, error) {
	if len(routes) == 0 {
		return nil, workers.ErrInvalidWorkstationPoolStart
	}
	snapshot := make(map[string]*routePool, len(routes))
	for _, route := range routes {
		name := strings.TrimSpace(route.WorkstationName)
		if name == "" {
			return nil, workers.ErrInvalidWorkstationPoolStart
		}
		if _, duplicate := snapshot[name]; duplicate {
			return nil, fmt.Errorf(
				"%w: duplicate route %q",
				workers.ErrInvalidWorkstationPoolStart,
				name,
			)
		}
		route.WorkstationName = name
		route.Capacity = normalizedLimit(route.Capacity, workers.DefaultWorkstationCapacity)
		route.QueueCapacity = normalizedLimit(
			route.QueueCapacity,
			workers.DefaultWorkstationQueueCapacity,
		)
		if route.Capacity < 0 || route.QueueCapacity < 0 {
			return nil, workers.ErrInvalidWorkstationPoolStart
		}
		snapshot[name] = &routePool{binding: route}
	}
	return snapshot, nil
}

func normalizedLimit(configured int, fallback int) int {
	if configured == 0 {
		return fallback
	}
	return configured
}
