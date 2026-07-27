package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// dispatchPlanningResultHook is the Runtime-owned bridge between scheduler
// dispatches and the canonical Workers request boundary. The delegate retains
// the existing live/replay delivery mechanics; the planner owns acceptance,
// publication, correlation, and duplicate retirement.
type dispatchPlanningResultHook struct {
	planner  dispatchplanning.Service
	delegate factory.DispatchResultHook
	net      *state.Net
}

type outOfBandDispatchResults interface {
	takeOutOfBandResults() []workerexecution.WorkResult
}

type dispatchHookSnapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

var _ factory.DispatchResultHook = (*dispatchPlanningResultHook)(nil)
var _ factory.DispatchResultHookWakeSignaler = (*dispatchPlanningResultHook)(nil)

func newDispatchPlanningResultHook(
	planner dispatchplanning.Service,
	delegate factory.DispatchResultHook,
	net *state.Net,
) *dispatchPlanningResultHook {
	return &dispatchPlanningResultHook{planner: planner, delegate: delegate, net: net}
}

func (h *dispatchPlanningResultHook) SubmitDispatch(
	ctx context.Context,
	dispatch work.WorkDispatch,
) error {
	decision := h.runnableDecision(dispatch)
	planned, err := h.planner.Plan(ctx, dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{decision},
	})
	if err != nil {
		return err
	}
	if len(planned.Actions) != 1 {
		return fmt.Errorf("dispatch planning produced %d actions for dispatch %q", len(planned.Actions), dispatch.DispatchID)
	}
	if _, err = h.planner.Publish(ctx, planned.Actions[0]); err != nil {
		return err
	}
	if forwarded, ok := h.delegate.(outOfBandDispatchResults); ok {
		for _, result := range forwarded.takeOutOfBandResults() {
			if _, err := h.retireResult(ctx, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *dispatchPlanningResultHook) OnTick(
	ctx context.Context,
	snapshot dispatchHookSnapshot,
) ([]workerexecution.WorkResult, error) {
	results, err := h.delegate.OnTick(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	retired := make([]workerexecution.WorkResult, 0, len(results))
	for _, result := range results {
		observation, err := h.retireResult(ctx, result)
		if err != nil {
			return nil, err
		}
		if observation.Outcome == dispatchplanning.RetirementOutcomeRetired {
			retired = append(retired, result)
		}
	}
	return retired, nil
}

func (h *dispatchPlanningResultHook) retireResult(
	ctx context.Context,
	result workerexecution.WorkResult,
) (dispatchplanning.RetirementResult, error) {
	intent, ok := h.planner.Intent(result.DispatchID)
	if !ok {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch ID %q is not present",
			dispatchplanning.ErrUnknownDispatchCorrelation,
			result.DispatchID,
		)
	}
	workIDs := intent.Action.Request.Execution.Dispatch.Execution.WorkIDs
	if len(workIDs) == 0 {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch %q has no Work lineage",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			result.DispatchID,
		)
	}
	outcome, err := terminalResultOutcome(result.Outcome)
	if err != nil {
		return dispatchplanning.RetirementResult{}, err
	}
	return h.planner.Retire(ctx, dispatchplanning.TerminalResult{
		DispatchID:    result.DispatchID,
		CorrelationID: intent.Action.CorrelationID,
		WorkID:        workIDs[0],
		Outcome:       outcome,
	})
}

func (h *dispatchPlanningResultHook) WaitCh() <-chan struct{} {
	return h.delegate.WaitCh()
}

func (h *dispatchPlanningResultHook) HasPendingResults() bool {
	return h.delegate.HasPendingResults()
}

func (h *dispatchPlanningResultHook) HasBufferedResults() bool {
	if signaler, ok := h.delegate.(factory.DispatchResultHookWakeSignaler); ok {
		return signaler.HasBufferedResults()
	}
	return h.delegate.HasPendingResults()
}

func (h *dispatchPlanningResultHook) SignalBufferedResults() {
	if signaler, ok := h.delegate.(factory.DispatchResultHookWakeSignaler); ok {
		signaler.SignalBufferedResults()
	}
}

func (h *dispatchPlanningResultHook) runnableDecision(
	dispatch work.WorkDispatch,
) dispatchplanning.RunnableDecision {
	workerType := dispatch.WorkerType
	if transition := h.net.Transitions[dispatch.TransitionID]; workerType == "" && transition != nil {
		workerType = transition.WorkerType
	}
	if workerType == "" {
		workerType = dispatch.WorkstationName
	}
	return dispatchplanning.RunnableDecision{
		CorrelationID: dispatch.DispatchID,
		Dispatch:      dispatch,
		Execution: dispatchplanning.ExecutionFacts{
			WorkerType:       workerType,
			ProjectID:        dispatch.ProjectID,
			InputPayload:     cloneDispatchInput(dispatch.InputTokens),
			FactorySessionID: "",
		},
	}
}

func cloneDispatchInput(input []any) []any {
	cloned := make([]any, len(input))
	copy(cloned, input)
	return cloned
}

func terminalResultOutcome(outcome workerexecution.WorkOutcome) (dispatchplanning.TerminalResultOutcome, error) {
	switch outcome {
	case workerexecution.OutcomeAccepted,
		workerexecution.OutcomeContinue,
		workerexecution.OutcomeRejected:
		return dispatchplanning.TerminalResultOutcomeSuccess, nil
	case workerexecution.OutcomeFailed:
		return dispatchplanning.TerminalResultOutcomeFailure, nil
	default:
		return "", fmt.Errorf(
			"%w: Workers outcome %q is not terminal",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			outcome,
		)
	}
}

// inlineDispatchResultHook preserves synchronous test/replay execution while
// presenting the same dispatch/result hook boundary used by live worker pools.
type inlineDispatchResultHook struct {
	net          *state.Net
	executors    map[string]workers.WorkerExecutor
	clock        factory.Clock
	waitCh       chan struct{}
	results      []workerexecution.WorkResult
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult]
	mu           sync.Mutex
}

var _ factory.DispatchResultHook = (*inlineDispatchResultHook)(nil)
var _ factory.DispatchResultHookWakeSignaler = (*inlineDispatchResultHook)(nil)

func newInlineDispatchResultHook(
	net *state.Net,
	executors map[string]workers.WorkerExecutor,
	clock factory.Clock,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
) *inlineDispatchResultHook {
	return &inlineDispatchResultHook{
		net:          net,
		executors:    executors,
		clock:        clock,
		waitCh:       make(chan struct{}, 1),
		resultBuffer: resultBuffer,
	}
}

func (h *inlineDispatchResultHook) SubmitDispatch(ctx context.Context, dispatch work.WorkDispatch) error {
	transition, ok := h.net.Transitions[dispatch.TransitionID]
	if !ok {
		return fmt.Errorf("unknown transition %q", dispatch.TransitionID)
	}
	result := executeDispatchSynchronously(
		ctx,
		dispatch,
		dispatchRunnerKey(transition, dispatch),
		h.executors,
		h.clock,
	)
	h.resultBuffer.Write(ctx, result)
	h.mu.Lock()
	h.results = append(h.results, result)
	h.signalLocked()
	h.mu.Unlock()
	return nil
}

func (h *inlineDispatchResultHook) OnTick(
	_ context.Context,
	_ dispatchHookSnapshot,
) ([]workerexecution.WorkResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	results := append([]workerexecution.WorkResult(nil), h.results...)
	h.results = nil
	return results, nil
}

func (h *inlineDispatchResultHook) WaitCh() <-chan struct{} { return h.waitCh }

func (h *inlineDispatchResultHook) HasPendingResults() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.results) > 0
}

func (h *inlineDispatchResultHook) HasBufferedResults() bool { return h.HasPendingResults() }

func (h *inlineDispatchResultHook) takeOutOfBandResults() []workerexecution.WorkResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	results := append([]workerexecution.WorkResult(nil), h.results...)
	h.results = nil
	return results
}

func (h *inlineDispatchResultHook) SignalBufferedResults() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.results) > 0 {
		h.signalLocked()
	}
}

func (h *inlineDispatchResultHook) signalLocked() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

type workerPoolDispatchResultHook struct {
	net           *state.Net
	pool          dispatchPool
	executors     map[string]workers.WorkerExecutor
	logger        logging.Logger
	waitCh        chan struct{}
	results       []workerexecution.WorkResult
	deliveryTicks map[string]int
	planner       factory.CompletionDeliveryPlanner
	clock         factory.Clock
	onResult      func()
	mu            sync.Mutex
}

// SetOnBufferedResult installs an owner-local observation hook. It must be
// configured before the dispatch hook starts receiving results.
func (h *workerPoolDispatchResultHook) SetOnBufferedResult(fn func()) {
	h.onResult = fn
}

var _ factory.DispatchResultHook = (*workerPoolDispatchResultHook)(nil)

type replayTickValidator interface {
	ValidateReplayTick(currentTick int) error
}

type plannedCompletionResultProvider interface {
	PlannedResultForDispatch(dispatch work.WorkDispatch) (workerexecution.WorkResult, bool, error)
}

func newWorkerPoolDispatchResultHook(
	net *state.Net,
	pool dispatchPool,
	executors map[string]workers.WorkerExecutor,
	logger logging.Logger,
	buffer int,
	planner factory.CompletionDeliveryPlanner,
	clock factory.Clock,
) *workerPoolDispatchResultHook {
	if clock == nil {
		panic("Factory Runtime dispatch-result clock is required")
	}
	if buffer <= 0 {
		buffer = 1
	}
	return &workerPoolDispatchResultHook{
		net:           net,
		pool:          pool,
		executors:     executors,
		logger:        logging.EnsureLogger(logger),
		waitCh:        make(chan struct{}, buffer),
		deliveryTicks: make(map[string]int),
		planner:       planner,
		clock:         clock,
	}
}

func (h *workerPoolDispatchResultHook) SubmitDispatch(ctx context.Context, dispatch work.WorkDispatch) error {
	tr, ok := h.net.Transitions[dispatch.TransitionID]
	if !ok {
		return fmt.Errorf("unknown transition %q", dispatch.TransitionID)
	}
	runnerKey := dispatchRunnerKey(tr, dispatch)
	deliveryTick, hasDeliveryTick := 0, false
	if h.planner != nil {
		tick, ok, err := h.planner.DeliveryTickForDispatch(dispatch)
		if err != nil {
			return err
		}
		if ok {
			deliveryTick, hasDeliveryTick = tick, true
		}
	}
	if hasDeliveryTick {
		h.mu.Lock()
		h.deliveryTicks[dispatch.DispatchID] = deliveryTick
		h.mu.Unlock()
	}
	if h.planner != nil {
		result := executeDispatchSynchronously(ctx, dispatch, runnerKey, h.executors, h.clock)
		if provider, ok := h.planner.(plannedCompletionResultProvider); ok {
			planned, hasPlanned, err := provider.PlannedResultForDispatch(dispatch)
			if err != nil {
				return err
			}
			if hasPlanned && (result.Outcome != workerexecution.OutcomeFailed ||
				planned.Outcome == workerexecution.OutcomeFailed) {
				result = planned
			}
		}
		h.mu.Lock()
		h.results = append(h.results, result)
		h.signalWaitLocked()
		h.notifyResultLocked()
		h.mu.Unlock()
		return nil
	}
	if !h.pool.Dispatch(runnerKey, dispatch) {
		if hasDeliveryTick {
			h.mu.Lock()
			delete(h.deliveryTicks, dispatch.DispatchID)
			h.mu.Unlock()
		}
		return fmt.Errorf("no worker pool runner for worker type %q", runnerKey)
	}
	return nil
}

func (h *workerPoolDispatchResultHook) OnTick(_ context.Context, snapshot dispatchHookSnapshot) ([]workerexecution.WorkResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if validator, ok := h.planner.(replayTickValidator); ok {
		if err := validator.ValidateReplayTick(snapshot.TickCount); err != nil {
			return nil, err
		}
	}
	if len(h.results) == 0 {
		return nil, nil
	}

	results := h.takeDueResults(snapshot.TickCount)
	if len(h.results) > 0 {
		h.signalWaitLocked()
	}
	if len(results) == 0 {
		return nil, nil
	}

	return results, nil
}

func (h *workerPoolDispatchResultHook) takeDueResults(currentTick int) []workerexecution.WorkResult {
	results := make([]workerexecution.WorkResult, 0, len(h.results))
	pending := h.results[:0]
	for _, result := range h.results {
		deliveryTick, delayed := h.deliveryTicks[result.DispatchID]
		if delayed && deliveryTick > currentTick {
			pending = append(pending, result)
			continue
		}
		if delayed {
			delete(h.deliveryTicks, result.DispatchID)
		}
		results = append(results, result)
	}
	h.results = pending
	return results
}

func (h *workerPoolDispatchResultHook) WaitCh() <-chan struct{} {
	return h.waitCh
}

func (h *workerPoolDispatchResultHook) HasPendingResults() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.results) > 0
}

func (h *workerPoolDispatchResultHook) HasBufferedResults() bool {
	return h.HasPendingResults()
}

func (h *workerPoolDispatchResultHook) SignalBufferedResults() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.results) > 0 {
		h.signalWaitLocked()
	}
}

func (h *workerPoolDispatchResultHook) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case result := <-h.pool.ResultCh():
				h.mu.Lock()
				h.results = append(h.results, result)
				h.signalWaitLocked()
				h.notifyResultLocked()
				h.mu.Unlock()
			case <-ctx.Done():
				h.logger.Info("factory worker pool dispatch/result hook completed", "reason", ctx.Err())
				return
			}
		}
	}()
}

func (h *workerPoolDispatchResultHook) notifyResultLocked() {
	if h.onResult != nil {
		h.onResult()
	}
}

func (h *workerPoolDispatchResultHook) signalWaitLocked() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

func executeDispatchSynchronously(
	ctx context.Context,
	dispatch work.WorkDispatch,
	runnerKey string,
	executors map[string]workers.WorkerExecutor,
	clock factory.Clock,
) workerexecution.WorkResult {
	if exec, ok := executors[runnerKey]; ok {
		var (
			result workerexecution.WorkResult
			err    error
		)
		start := clock.Now()
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					result = panicAsFailedResult(dispatch, recovered, clock.Now().Sub(start))
					err = nil
				}
			}()
			result, err = exec.Execute(ctx, dispatch)
		}()
		if err == nil {
			return result
		}
		return workerexecution.WorkResult{
			DispatchID:   dispatch.DispatchID,
			TransitionID: dispatch.TransitionID,
			Outcome:      workerexecution.OutcomeFailed,
			Error:        err.Error(),
		}
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        fmt.Sprintf("no executor registered for worker type %q (transition %s)", runnerKey, dispatch.TransitionID),
	}
}

func dispatchRunnerKey(tr *petri.Transition, dispatch work.WorkDispatch) string {
	if tr != nil && tr.WorkerType != "" {
		return tr.WorkerType
	}
	if dispatch.WorkstationName != "" {
		return dispatch.WorkstationName
	}
	return dispatch.TransitionID
}
