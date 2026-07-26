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
	stateStopped
)

// Pool owns the explicit lifecycle and immutable route snapshot for one
// Workers workstation capability.
type Pool struct {
	mu     sync.RWMutex
	state  lifecycleState
	routes map[string]*routePool
}

type routePool struct {
	binding workstations.Route
	mu      sync.Mutex
	running int
	waiting []*admission
}

type admission struct {
	ready chan struct{}
}

var _ workstations.Service = (*Pool)(nil)

// New constructs an inert pool without starting background activity.
func New() *Pool {
	return &Pool{state: stateConstructed}
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
	case stateStopped:
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
	defer p.mu.Unlock()
	if p.state == stateStopped {
		return workers.WorkstationPoolLifecycleOutcomeAlreadyStopped, nil
	}
	p.routes = nil
	p.state = stateStopped
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
	case stateStopped:
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

	route, releaseLifecycle, err := p.acquireRoute(name)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	defer releaseLifecycle()
	if route.binding.Executor == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrMissingWorkstationBinding
	}
	if err := route.admit(ctx); err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	defer route.release()

	execution := workers.CloneWorkstationExecutionRequest(request.Execution)
	execution.RunnerID = route.binding.RunnerSelection.RunnerID
	execution.RunnerSelectionSource = route.binding.RunnerSelection.Source
	result, executeErr := route.binding.Executor.Execute(ctx, execution)
	result.DispatchID = execution.Dispatch.DispatchID
	result.TransitionID = execution.Dispatch.TransitionID
	return workers.WorkstationDispatchResult{
		DispatchID:      execution.Dispatch.DispatchID,
		WorkstationName: name,
		Result:          result,
	}, executeErr
}

func (p *Pool) acquireRoute(
	name string,
) (*routePool, func(), error) {
	p.mu.RLock()
	switch p.state {
	case stateConstructed:
		p.mu.RUnlock()
		return nil, nil, workers.ErrWorkstationPoolUnavailable
	case stateStopped:
		p.mu.RUnlock()
		return nil, nil, workers.ErrWorkstationPoolStopped
	case stateRunning:
		route, ok := p.routes[name]
		if !ok {
			p.mu.RUnlock()
			return nil, nil, workers.ErrUnknownWorkstationRoute
		}
		return route, p.mu.RUnlock, nil
	default:
		p.mu.RUnlock()
		return nil, nil, workers.ErrWorkstationPoolUnavailable
	}
}

func (route *routePool) admit(ctx context.Context) error {
	route.mu.Lock()
	if route.running < route.binding.Capacity && len(route.waiting) == 0 {
		route.running++
		route.mu.Unlock()
		return nil
	}
	if len(route.waiting) >= route.binding.QueueCapacity {
		route.mu.Unlock()
		return workers.ErrWorkstationSaturated
	}
	entry := &admission{ready: make(chan struct{})}
	route.waiting = append(route.waiting, entry)
	route.mu.Unlock()

	select {
	case <-entry.ready:
		return nil
	case <-ctx.Done():
		return route.cancelWaiting(entry, ctx.Err())
	}
}

func (route *routePool) cancelWaiting(entry *admission, cancellation error) error {
	route.mu.Lock()
	defer route.mu.Unlock()
	for index, queued := range route.waiting {
		if queued != entry {
			continue
		}
		route.waiting = append(route.waiting[:index], route.waiting[index+1:]...)
		return cancellation
	}
	// Promotion already committed a running slot. Execution observes the
	// cancelled context and release returns that slot exactly once.
	return nil
}

func (route *routePool) release() {
	route.mu.Lock()
	defer route.mu.Unlock()
	route.running--
	if len(route.waiting) == 0 {
		return
	}
	next := route.waiting[0]
	route.waiting = route.waiting[1:]
	route.running++
	close(next.ready)
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
