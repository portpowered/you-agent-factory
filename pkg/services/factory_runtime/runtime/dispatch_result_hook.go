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
	planner           dispatchplanning.Service
	net               *state.Net
	resultBuffer      *buffers.TypedBuffer[workerexecution.WorkResult]
	completionPlanner factory.CompletionDeliveryPlanner
	factorySessionID  string
	waitCh            chan struct{}
	scheduled         []scheduledDispatchResult
	asyncErr          error
	onResult          func()
	mu                sync.Mutex
}

func (h *dispatchPlanningResultHook) SetOnBufferedResult(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onResult = fn
}

type scheduledDispatchResult struct {
	deliveryTick int
	result       workerexecution.WorkResult
}

type dispatchHookSnapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

var _ factory.DispatchResultHook = (*dispatchPlanningResultHook)(nil)
var _ factory.DispatchResultHookWakeSignaler = (*dispatchPlanningResultHook)(nil)

func newCanonicalDispatchPlanningResultHook(
	planner dispatchplanning.Service,
	net *state.Net,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
	completionPlanner factory.CompletionDeliveryPlanner,
	factorySessionID string,
) *dispatchPlanningResultHook {
	return &dispatchPlanningResultHook{
		planner: planner, net: net, resultBuffer: resultBuffer,
		completionPlanner: completionPlanner, factorySessionID: factorySessionID,
		waitCh: make(chan struct{}, 1),
	}
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
	return nil
}

func (h *dispatchPlanningResultHook) OnTick(
	ctx context.Context,
	snapshot dispatchHookSnapshot,
) ([]workerexecution.WorkResult, error) {
	_ = ctx
	return h.takeCanonicalResults(snapshot.TickCount)
}

func (h *dispatchPlanningResultHook) takeCanonicalResults(
	currentTick int,
) ([]workerexecution.WorkResult, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.asyncErr != nil {
		err := h.asyncErr
		h.asyncErr = nil
		return nil, err
	}
	due := make([]workerexecution.WorkResult, 0, len(h.scheduled))
	pending := h.scheduled[:0]
	for _, scheduled := range h.scheduled {
		if scheduled.deliveryTick <= currentTick {
			due = append(due, scheduled.result)
		} else {
			pending = append(pending, scheduled)
		}
	}
	h.scheduled = pending
	if len(h.scheduled) > 0 {
		h.signalCanonicalLocked()
	}
	return due, nil
}

func (h *dispatchPlanningResultHook) WaitCh() <-chan struct{} {
	return h.waitCh
}

func (h *dispatchPlanningResultHook) HasPendingResults() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.asyncErr != nil || len(h.scheduled) > 0 ||
		(h.resultBuffer != nil && h.resultBuffer.HasData())
}

func (h *dispatchPlanningResultHook) HasBufferedResults() bool {
	return h.HasPendingResults()
}

func (h *dispatchPlanningResultHook) SignalBufferedResults() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.asyncErr != nil || len(h.scheduled) > 0 ||
		(h.resultBuffer != nil && h.resultBuffer.HasData()) {
		h.signalCanonicalLocked()
	}
}

func (h *dispatchPlanningResultHook) acceptWorkersResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) {
	workResult := canonicalWorkResult(request, result, dispatchErr)
	if provider, ok := h.completionPlanner.(plannedCompletionResultProvider); ok {
		planned, hasPlanned, err := provider.PlannedResultForDispatch(request.Execution.Dispatch)
		if err != nil {
			h.recordCanonicalError(err)
			return
		}
		if hasPlanned && (workResult.Outcome != workerexecution.OutcomeFailed ||
			planned.Outcome == workerexecution.OutcomeFailed) {
			planned.DispatchID = request.Execution.Dispatch.DispatchID
			planned.TransitionID = request.Execution.Dispatch.TransitionID
			workResult = planned
		}
	}
	outcome := workstationTerminalResultOutcome(result.TerminalOutcome, workResult.Outcome)
	if _, err := h.acceptCanonicalResult(ctx, request, workResult, "", outcome); err != nil {
		h.recordCanonicalError(err)
	}
}

func canonicalWorkResult(
	request workers.WorkstationDispatchRequest,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) workerexecution.WorkResult {
	workResult := result.Result
	if workResult.DispatchID == "" {
		workResult.DispatchID = request.Execution.Dispatch.DispatchID
	}
	if workResult.TransitionID == "" {
		workResult.TransitionID = request.Execution.Dispatch.TransitionID
	}
	if dispatchErr != nil || result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeFailed {
		workResult.Outcome = workerexecution.OutcomeFailed
		if workResult.Error == "" && dispatchErr != nil {
			workResult.Error = dispatchErr.Error()
		}
	}
	if result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled {
		workResult.Outcome = workerexecution.OutcomeFailed
		if workResult.Error == "" {
			workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
		}
	}
	return workResult
}

func workstationTerminalResultOutcome(
	terminal workers.WorkstationDispatchTerminalOutcome,
	outcome workerexecution.WorkOutcome,
) dispatchplanning.TerminalResultOutcome {
	switch terminal {
	case workers.WorkstationDispatchTerminalOutcomeCanceled:
		return dispatchplanning.TerminalResultOutcomeCancelled
	case workers.WorkstationDispatchTerminalOutcomeFailed:
		return dispatchplanning.TerminalResultOutcomeFailure
	default:
		mapped, err := terminalResultOutcome(outcome)
		if err != nil {
			return dispatchplanning.TerminalResultOutcomeFailure
		}
		return mapped
	}
}

func (h *dispatchPlanningResultHook) acceptRootResult(
	ctx context.Context,
	req factory.AcceptDispatchResultRequest,
	outcome dispatchplanning.TerminalResultOutcome,
) (dispatchplanning.RetirementResult, error) {
	intent, ok := h.planner.Intent(req.DispatchID)
	if !ok {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch ID %q is not present",
			dispatchplanning.ErrUnknownDispatchCorrelation,
			req.DispatchID,
		)
	}
	workResult := workerexecution.WorkResult{
		DispatchID:   req.DispatchID,
		TransitionID: intent.Action.Request.Execution.Dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}
	switch outcome {
	case dispatchplanning.TerminalResultOutcomeFailure:
		workResult.Outcome = workerexecution.OutcomeFailed
		workResult.Error = "Workers dispatch failed"
	case dispatchplanning.TerminalResultOutcomeCancelled:
		workResult.Outcome = workerexecution.OutcomeFailed
		workResult.Error = workers.ErrWorkstationDispatchCanceled.Error()
	}
	return h.acceptCanonicalResult(ctx, intent.Action.Request, workResult, req.WorkID, outcome)
}

func (h *dispatchPlanningResultHook) acceptCanonicalResult(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	result workerexecution.WorkResult,
	workID string,
	outcome dispatchplanning.TerminalResultOutcome,
) (dispatchplanning.RetirementResult, error) {
	intent, ok := h.planner.Intent(result.DispatchID)
	if !ok {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch ID %q is not present",
			dispatchplanning.ErrUnknownDispatchCorrelation,
			result.DispatchID,
		)
	}
	workIDs := request.Execution.Dispatch.Execution.WorkIDs
	if len(workIDs) == 0 {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: dispatch %q has no Work lineage",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
			result.DispatchID,
		)
	}
	if workID == "" {
		workID = workIDs[0]
	}
	terminal := dispatchplanning.TerminalResult{
		DispatchID: result.DispatchID, CorrelationID: intent.Action.CorrelationID,
		WorkID: workID, Outcome: outcome,
	}
	if intent.Result != nil {
		return h.planner.Retire(ctx, terminal)
	}
	deliveryTick, scheduled := 0, false
	if h.completionPlanner != nil {
		var scheduleErr error
		deliveryTick, scheduled, scheduleErr = h.completionPlanner.DeliveryTickForDispatch(
			request.Execution.Dispatch,
		)
		if scheduleErr != nil {
			return dispatchplanning.RetirementResult{}, scheduleErr
		}
	}
	if !scheduled && h.resultBuffer == nil {
		return dispatchplanning.RetirementResult{}, fmt.Errorf(
			"%w: Runtime result buffer is unavailable",
			dispatchplanning.ErrInvalidDispatchResultBoundary,
		)
	}
	retired, err := h.planner.Retire(ctx, terminal)
	if err != nil || retired.Outcome != dispatchplanning.RetirementOutcomeRetired {
		return retired, err
	}
	if scheduled {
		h.mu.Lock()
		h.scheduled = append(h.scheduled, scheduledDispatchResult{
			deliveryTick: deliveryTick,
			result:       result,
		})
		h.notifyCanonicalResultLocked()
		h.signalCanonicalLocked()
		h.mu.Unlock()
		return retired, nil
	}
	if !h.resultBuffer.Write(ctx, result) {
		h.mu.Lock()
		h.scheduled = append(h.scheduled, scheduledDispatchResult{result: result})
		h.notifyCanonicalResultLocked()
		h.signalCanonicalLocked()
		h.mu.Unlock()
		return retired, nil
	}
	h.mu.Lock()
	h.notifyCanonicalResultLocked()
	h.signalCanonicalLocked()
	h.mu.Unlock()
	return retired, nil
}

func (h *dispatchPlanningResultHook) recordCanonicalError(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.asyncErr == nil {
		h.asyncErr = err
	}
	h.signalCanonicalLocked()
}

func (h *dispatchPlanningResultHook) signalCanonicalLocked() {
	select {
	case h.waitCh <- struct{}{}:
	default:
	}
}

func (h *dispatchPlanningResultHook) notifyCanonicalResultLocked() {
	if h.onResult != nil {
		h.onResult()
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
			FactorySessionID: h.factorySessionID,
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
