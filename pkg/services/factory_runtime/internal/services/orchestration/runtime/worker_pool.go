package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

var (
	// ErrAttemptCapacityExceeded reports that Runtime cannot admit another
	// Execute call until an active attempt reaches a terminal boundary.
	ErrAttemptCapacityExceeded = errors.New("Factory Runtime worker-attempt capacity is full")
	// ErrAttemptLifecycleUnavailable reports an attempt lifecycle that was not
	// constructed with the required stateless execution capability.
	ErrAttemptLifecycleUnavailable = errors.New("Factory Runtime worker-attempt lifecycle is unavailable")
)

func normalizeRuntimeMode(mode interfaces.RuntimeMode) interfaces.RuntimeMode {
	if mode == "" {
		return interfaces.RuntimeModeBatch
	}
	return mode
}

const defaultRuntimeAttemptCapacity = 64

type attemptTerminalFunc func(context.Context, workers.ExecuteRequest, workers.ExecuteResult, error)

// attemptPreparation opens a Runtime-adjacent observation window after
// Runtime admits an attempt but before the detached Workers Execute call.
// The returned terminal hook runs only for the one callback that wins the
// Runtime terminal race.
type attemptPreparation func(context.Context, *workers.ExecuteRequest) (attemptTerminalFunc, error)

// executeCapability is deliberately private to Runtime. Workers' aggregate
// Service already exposes Execute, but publishing another service-root
// interface would make the public Workers contract own Runtime's lifecycle
// seam.
type executeCapability interface {
	Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
}

type activeAttempt struct {
	dispatchID   string
	attemptID    string
	ctx          context.Context
	cancel       context.CancelCauseFunc
	cancelReason workers.DispatchCancellationReason
	done         chan struct{}
	canceled     bool
	processGone  bool
}

// attemptLifecycle is the Runtime-owned lifecycle boundary for stateless
// Worker execution. It deliberately retains only cancellation, correlation,
// and process-reconciliation state; Workers receives a detached ExecuteRequest
// and returns a detached ExecuteResult.
type attemptLifecycle struct {
	mu       sync.Mutex
	service  executeCapability
	newID    factory.IDGenerator
	capacity int
	active   map[string]*activeAttempt
	terminal map[string]string
}

func newAttemptLifecycle(service executeCapability, newID factory.IDGenerator, capacity int) *attemptLifecycle {
	if capacity <= 0 {
		capacity = defaultRuntimeAttemptCapacity
	}
	return &attemptLifecycle{
		service:  service,
		newID:    newID,
		capacity: capacity,
		active:   make(map[string]*activeAttempt),
		terminal: make(map[string]string),
	}
}

// start admits one request and invokes its terminal callback exactly once.
// A full lifecycle is removed from active state before the callback is
// delivered, so result application cannot hold the admission lock.
func (l *attemptLifecycle) start(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
) error {
	return l.startWithRetry(ctx, request, async, terminal, false)
}

// startRetry admits a new Attempt ID for an already-terminal dispatch. The
// Runtime uses this only for an explicit caller-requested retry; ordinary
// duplicate dispatch publication remains idempotent through start.
func (l *attemptLifecycle) startRetry(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
) error {
	return l.startWithRetry(ctx, request, async, terminal, true)
}

func (l *attemptLifecycle) startWithRetry(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
	allowRetry bool,
) error {
	return l.startWithPreparation(ctx, request, async, terminal, allowRetry, nil)
}

func (l *attemptLifecycle) startWithPreparation(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
	allowRetry bool,
	prepare attemptPreparation,
) error {
	if l == nil || l.service == nil {
		return ErrAttemptLifecycleUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("start worker attempt: context is required")
	}
	if terminal == nil {
		return fmt.Errorf("start worker attempt: terminal callback is required")
	}
	request, err := l.prepareAttemptRequest(request)
	if err != nil {
		return err
	}
	execCtx, cancel := context.WithCancelCause(ctx)
	attempt := &activeAttempt{
		dispatchID: request.Correlation.DispatchID,
		attemptID:  request.Correlation.AttemptID,
		ctx:        execCtx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	admitted, err := l.admitAttempt(attempt, allowRetry)
	if err != nil || !admitted {
		cancel(platformprocess.NewCancellationCause(platformprocess.CancellationReasonCanceled))
		return err
	}
	request = attachAttemptProcessObserver(request, l, attempt)
	var preparedTerminal attemptTerminalFunc
	if prepare != nil {
		preparedTerminal, err = prepare(context.WithoutCancel(execCtx), &request)
		if err != nil {
			l.finish(attempt)
			cancel(platformprocess.NewCancellationCause(platformprocess.CancellationReasonCanceled))
			close(attempt.done)
			return err
		}
	}

	run := func() {
		result, err := l.executeSafely(execCtx, request)
		applied, canceled, processGone := l.finish(attempt)
		if !applied {
			close(attempt.done)
			return
		}
		defer close(attempt.done)
		if processGone {
			result = processGoneAttemptResult(request, result)
			err = workers.ErrWorkstationDispatchProcessGone
		} else if canceled {
			result = canceledAttemptResult(request, result, attempt.cancelReason)
			err = nil
		}
		if preparedTerminal != nil {
			preparedTerminal(context.Background(), request, result, err)
		}
		terminal(context.Background(), request, result, err)
	}
	if async {
		go run()
		return nil
	}
	run()
	return nil
}

func attachAttemptProcessObserver(
	request workers.ExecuteRequest,
	lifecycle *attemptLifecycle,
	attempt *activeAttempt,
) workers.ExecuteRequest {
	if request.Input.ProcessLifecycleObserver == nil {
		request.Input.ProcessLifecycleObserver = attemptProcessObserver{
			lifecycle: lifecycle,
			attempt:   attempt,
		}
	}
	return request
}

func (l *attemptLifecycle) prepareAttemptRequest(
	request workers.ExecuteRequest,
) (workers.ExecuteRequest, error) {
	request = request.Clone()
	request.Correlation.DispatchID = strings.TrimSpace(request.Correlation.DispatchID)
	request.Correlation.AttemptID = strings.TrimSpace(request.Correlation.AttemptID)
	if request.Correlation.AttemptID == "" {
		if l.newID == nil {
			return workers.ExecuteRequest{}, fmt.Errorf("start worker attempt: Attempt ID generator is required")
		}
		request.Correlation.AttemptID = strings.TrimSpace(l.newID())
		if request.Correlation.AttemptID == "" {
			return workers.ExecuteRequest{}, fmt.Errorf("start worker attempt: Attempt ID generator returned an empty ID")
		}
	}
	if err := request.Validate(); err != nil {
		return workers.ExecuteRequest{}, err
	}
	if request.Attempt.Number <= 0 {
		request.Attempt.Number = 1
	}
	return request, nil
}

func (l *attemptLifecycle) admitAttempt(
	attempt *activeAttempt,
	allowRetry bool,
) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, exists := l.active[attempt.dispatchID]; exists {
		return false, nil
	}
	terminalAttemptID, terminalAlreadyApplied := l.terminal[attempt.dispatchID]
	if terminalAlreadyApplied &&
		(!allowRetry || terminalAttemptID == attempt.attemptID) {
		return false, nil
	}
	if len(l.active) >= l.capacity {
		return false, ErrAttemptCapacityExceeded
	}
	l.active[attempt.dispatchID] = attempt
	return true, nil
}

func (l *attemptLifecycle) executeSafely(
	ctx context.Context,
	request workers.ExecuteRequest,
) (result workers.ExecuteResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = workers.ExecuteResult{
				Correlation: request.Correlation,
				Outcome:     workers.ExecutionOutcomeFailed,
				Failure: &workers.ExecutionFailure{
					Type:    workers.WorkFailureTypeUnknown,
					Family:  workers.WorkFailureFamilyTerminal,
					Message: "worker execution panicked",
				},
			}
			// Keep panic detail out of the detached customer-facing result.
			err = nil
		}
		result = normalizeAttemptResult(request, result, err, platformprocess.CancellationReasonFromContext(ctx))
		if result.Outcome == workers.ExecutionOutcomeCanceled {
			err = nil
		}
	}()
	result, err = l.service.Execute(ctx, request)
	return result, err
}

func normalizeAttemptResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	executeErr error,
	cancellationReason platformprocess.CancellationReason,
) workers.ExecuteResult {
	result, conflicts := normalizeAttemptCorrelation(request, result)
	if conflicts {
		return conflictingAttemptResult(request)
	}
	return normalizeAttemptOutcome(request, result, executeErr, cancellationReason)
}

func normalizeAttemptOutcome(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	executeErr error,
	cancellationReason platformprocess.CancellationReason,
) workers.ExecuteResult {
	if result.Cancellation != nil {
		result.Correlation = request.Correlation
		result.Outcome = workers.ExecutionOutcomeCanceled
		result.Output = workers.ProposedOutput{}
		result.StructuredResult = nil
		result.StructuredResultPresent = false
		result.Continuation = nil
		if result.Failure == nil {
			result.Failure = &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "execution canceled",
			}
		}
		return result
	}
	if executeErr == nil && result.Outcome != "" {
		return result
	}
	if cancellationReason != "" ||
		errors.Is(executeErr, workers.ErrWorkstationDispatchCanceled) ||
		result.Outcome == workers.ExecutionOutcomeCanceled {
		result.Correlation = request.Correlation
		result.Outcome = workers.ExecutionOutcomeCanceled
		result.Output = workers.ProposedOutput{}
		result.StructuredResult = nil
		result.StructuredResultPresent = false
		if result.Cancellation == nil {
			result.Cancellation = &workers.DispatchCancellation{Reason: workers.DispatchCancellationReasonCanceled}
		}
		if result.Failure == nil {
			result.Failure = &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "execution canceled",
			}
		}
		return result
	}
	result.Correlation = request.Correlation
	result.Outcome = workers.ExecutionOutcomeFailed
	if result.Failure == nil {
		message := "worker execution returned no terminal outcome"
		if executeErr != nil {
			message = executeErr.Error()
		}
		result.Failure = &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: message,
		}
	}
	return result
}

func canceledAttemptResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	reason workers.DispatchCancellationReason,
) workers.ExecuteResult {
	result.Correlation = request.Correlation
	result.Outcome = workers.ExecutionOutcomeCanceled
	if reason != workers.DispatchCancellationReasonSuperseded {
		reason = workers.DispatchCancellationReasonCanceled
	}
	result.Cancellation = &workers.DispatchCancellation{Reason: reason}
	// A provider may have returned output just as Runtime cancellation won the
	// terminal race. That output is not an eligible proposal and must not reach
	// Work materialization or downstream Runtime routing.
	result.Output = workers.ProposedOutput{}
	result.StructuredResult = nil
	result.StructuredResultPresent = false
	result.Continuation = nil
	if result.Failure == nil {
		result.Failure = &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "execution canceled",
		}
	}
	return result
}

func correlationValueConflicts(actual, expected string) bool {
	return strings.TrimSpace(actual) != "" &&
		strings.TrimSpace(expected) != "" &&
		strings.TrimSpace(actual) != strings.TrimSpace(expected)
}

func (l *attemptLifecycle) finish(attempt *activeAttempt) (bool, bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	current, exists := l.active[attempt.dispatchID]
	if !exists || current != attempt {
		return false, false, false
	}
	delete(l.active, attempt.dispatchID)
	l.terminal[attempt.dispatchID] = attempt.attemptID
	return true, attempt.canceled, attempt.processGone
}

func (l *attemptLifecycle) reconcileProcessGone(attempt *activeAttempt) {
	if l == nil || attempt == nil {
		return
	}
	l.mu.Lock()
	current, active := l.active[attempt.dispatchID]
	if !active || current != attempt || attempt.canceled {
		l.mu.Unlock()
		return
	}
	attempt.canceled = true
	processGone := true
	if attempt.ctx == nil || attempt.ctx.Err() != context.Canceled {
		attempt.processGone = true
	} else {
		processGone = false
		attempt.cancelReason = dispatchCancellationReasonFromContext(attempt.ctx)
	}
	cancel := attempt.cancel
	l.mu.Unlock()
	if processGone {
		cancel(platformprocess.NewCancellationCause(platformprocess.CancellationReasonProcessGone))
		return
	}
	cancel(platformprocess.NewCancellationCause(platformCancellationReason(attempt.cancelReason)))
}

func (l *attemptLifecycle) cancel(
	ctx context.Context,
	dispatchID string,
	reasons ...workers.WorkstationDispatchCancelReason,
) (workers.WorkstationDispatchCancelOutcome, error) {
	if l == nil {
		return "", ErrAttemptLifecycleUnavailable
	}
	if ctx == nil {
		return "", fmt.Errorf("cancel worker attempt: context is required")
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return "", fmt.Errorf("cancel worker attempt: dispatch ID is required")
	}
	l.mu.Lock()
	attempt := l.active[dispatchID]
	_, wasTerminal := l.terminal[dispatchID]
	if attempt != nil {
		if attempt.canceled {
			l.mu.Unlock()
			return workers.WorkstationDispatchCancelOutcomeAlreadyCanceled, nil
		}
		attempt.canceled = true
		attempt.cancelReason = dispatchCancellationReasonFromCancelRequest(reasons...)
	}
	l.mu.Unlock()
	if attempt == nil {
		if wasTerminal {
			return workers.WorkstationDispatchCancelOutcomeAlreadyTerminal, nil
		}
		return "", fmt.Errorf("cancel worker attempt %q: dispatch is unknown", dispatchID)
	}
	attempt.cancel(platformprocess.NewCancellationCause(platformCancellationReason(attempt.cancelReason)))
	return workers.WorkstationDispatchCancelOutcomeCanceled, nil
}

func (l *attemptLifecycle) stop(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("stop worker attempts: context is required")
	}
	l.mu.Lock()
	attempts := make([]*activeAttempt, 0, len(l.active))
	for _, attempt := range l.active {
		attempt.canceled = true
		attempt.cancelReason = workers.DispatchCancellationReasonCanceled
		attempts = append(attempts, attempt)
	}
	l.mu.Unlock()
	for _, attempt := range attempts {
		attempt.cancel(platformprocess.NewCancellationCause(platformprocess.CancellationReasonCanceled))
	}
	for _, attempt := range attempts {
		select {
		case <-attempt.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if stopper, ok := l.service.(interface{ Stop(context.Context) error }); ok {
		return stopper.Stop(ctx)
	}
	return nil
}

func (l *attemptLifecycle) activeCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.active)
}

func (l *attemptLifecycle) terminalAttemptID(dispatchID string) (string, bool) {
	if l == nil {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attemptID, ok := l.terminal[strings.TrimSpace(dispatchID)]
	return attemptID, ok
}

func (f *factoryImpl) ControlPause(ctx context.Context, req factory.PauseRequest) (factory.PauseResult, error) {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	outcome, _, err := f.applyPauseControl()
	if err != nil {
		return factory.PauseResult{}, err
	}
	workerSessionControl := f.controlAssociatedWorkerSessions(
		ctx, req.TurnID, req.ControlID, factory.WorkerSessionControlActionPause, outcome,
	)
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Pause(ctx); err != nil {
			return factory.PauseResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, err
		}
	}
	return factory.PauseResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, nil
}

func (f *factoryImpl) ControlResume(ctx context.Context, req factory.ResumeRequest) (factory.ResumeResult, error) {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	outcome, _, err := f.applyResumeControl()
	if err != nil {
		return factory.ResumeResult{}, err
	}
	workerSessionControl := f.controlAssociatedWorkerSessions(
		ctx, req.TurnID, req.ControlID, factory.WorkerSessionControlActionResume, outcome,
	)
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Resume(ctx); err != nil {
			return factory.ResumeResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, err
		}
	}
	return factory.ResumeResult{Outcome: outcome, WorkerSessionControl: workerSessionControl}, nil
}

func (f *factoryImpl) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	action, err := terminateWorkerSessionControlAction(req.WorkerSessionAction)
	if err != nil {
		return factory.TerminateResult{}, err
	}
	f.mu.Lock()
	state := f.state
	switch state {
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		if strings.TrimSpace(req.TurnID) != "" || strings.TrimSpace(req.ControlID) != "" {
			f.logRuntimeLifecycleControl(string(action), state, state, "NO_OP")
			return factory.TerminateResult{
				Outcome: factory.ControlOutcomeNoOp,
				WorkerSessionControl: f.controlAssociatedWorkerSessions(
					ctx, req.TurnID, req.ControlID, action, factory.ControlOutcomeNoOp,
				),
			}, nil
		}
		return factory.TerminateResult{}, factory.ErrAlreadyStopped
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStateCompleted
		cancel := f.runCancel
		f.mu.Unlock()
		f.recordStateChange(state, interfaces.FactoryStateCompleted, req.Reason)
		// The control has committed before child coordination starts. Canceling
		// the engine prevents new dispatch planning while the shared control
		// lock keeps Run's pool shutdown behind the exact Worker Sessions
		// cancellations below.
		if cancel != nil {
			cancel()
		}
		workerSessionControl := f.controlAssociatedWorkerSessions(
			ctx, req.TurnID, req.ControlID, action, factory.ControlOutcomeAccepted,
		)
		stopErr := f.stopDispatchRuntimeLocked(ctx, dispatchplanning.RuntimeStopReasonTerminated)
		f.logRuntimeLifecycleControl(string(action), state, interfaces.FactoryStateCompleted, "ACCEPTED")
		if cancel == nil {
			f.completeOnce.Do(func() { close(f.completeCh) })
		}
		return factory.TerminateResult{
			Outcome:              factory.ControlOutcomeAccepted,
			WorkerSessionControl: workerSessionControl,
		}, stopErr
	default:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrNotRunning
	}
}

func terminateWorkerSessionControlAction(action factory.WorkerSessionControlAction) (factory.WorkerSessionControlAction, error) {
	switch action {
	case "", factory.WorkerSessionControlActionTerminate:
		return factory.WorkerSessionControlActionTerminate, nil
	case factory.WorkerSessionControlActionCancel:
		return factory.WorkerSessionControlActionCancel, nil
	default:
		return "", fmt.Errorf("%w: stop Worker Session action %q", factory.ErrInvalidLifecycleTransition, action)
	}
}

func (f *factoryImpl) stopDispatchRuntime(
	ctx context.Context,
	reason dispatchplanning.RuntimeStopReason,
) error {
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
	return f.stopDispatchRuntimeLocked(ctx, reason)
}

func (f *factoryImpl) stopDispatchRuntimeLocked(
	ctx context.Context,
	reason dispatchplanning.RuntimeStopReason,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var stopErr error
	// Worker Sessions closes asynchronous admission and joins its server-owned
	// supervisors before Workers closes the dispatch pool. This ordering keeps
	// a shutdown race from admitting new work after the pool has stopped and
	// lets terminal callbacks publish through the still-open Events boundary.
	if f.cfg != nil && f.cfg.workerSessions != nil {
		if lifecycle, ok := f.cfg.workerSessions.(interface{ Stop(context.Context) error }); ok {
			stopErr = lifecycle.Stop(stopCtx)
		}
	}
	if f.dispatchPlan != nil {
		stopErr = errors.Join(stopErr, f.dispatchPlan.Stop(stopCtx, reason))
	}
	if f.cfg != nil && f.cfg.attempts != nil {
		stopErr = errors.Join(stopErr, f.cfg.attempts.stop(stopCtx))
	}
	return stopErr
}

func (f *factoryImpl) ControlWaitToComplete(_ factory.WaitToCompleteRequest) factory.WaitToCompleteResult {
	return factory.WaitToCompleteResult{Done: f.WaitToComplete()}
}

func (f *factoryImpl) ControlMoveWork(ctx context.Context, req factory.MoveWorkRequest) (factory.MoveWorkResult, error) {
	result, err := f.MoveWork(ctx, req.WorkID, req.StateName, work.WorkStateChangeSource(req.Source), req.RequestID)
	if err != nil {
		if errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
			return factory.MoveWorkResult{}, factory.ErrMoveWorkRequestConflict
		}
		return factory.MoveWorkResult{}, err
	}
	return factory.MoveWorkResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
}

func (f *factoryImpl) PlanDispatch(
	ctx context.Context,
	req factory.PlanDispatchRequest,
) (factory.PlanDispatchResult, error) {
	if err := f.requireActiveDispatchRuntime(); err != nil {
		return factory.PlanDispatchResult{}, err
	}
	if err := validateRootDispatchPlan(req); err != nil {
		return factory.PlanDispatchResult{}, err
	}
	dispatch := work.WorkDispatch{
		DispatchID:      req.DispatchID,
		TransitionID:    req.WorkstationName,
		WorkerType:      req.WorkerType,
		WorkstationName: req.WorkstationName,
		Execution: work.ExecutionMetadata{
			WorkIDs:   append([]string(nil), req.WorkIDs...),
			ReplayKey: req.ReplayKey,
		},
		InputTokens: make([]any, 0),
	}
	planned, err := f.dispatchPlan.Plan(ctx, dispatchplanning.PlanRequest{
		Decisions: []dispatchplanning.RunnableDecision{{
			CorrelationID: req.CorrelationID,
			Dispatch:      dispatch,
			Execution: dispatchplanning.ExecutionFacts{
				WorkerType:       req.WorkerType,
				InputPayload:     make([]any, 0),
				FactorySessionID: sessionIDFromFactoryConfig(f.cfg),
				RecordingID:      f.cfg.recordingID,
			},
		}},
	})
	if err != nil {
		return factory.PlanDispatchResult{}, mapDispatchPlanningError(err)
	}
	if len(planned.Actions) != 1 {
		return factory.PlanDispatchResult{}, fmt.Errorf(
			"%w: planner returned %d actions",
			factory.ErrInvalidDispatchResultBoundary,
			len(planned.Actions),
		)
	}
	published, err := f.dispatchPlan.Publish(ctx, planned.Actions[0])
	if err != nil {
		return factory.PlanDispatchResult{}, mapDispatchPlanningError(err)
	}
	return factory.PlanDispatchResult{
		Outcome:       factory.DispatchPlanOutcome(published.Outcome),
		DispatchID:    published.DispatchID,
		CorrelationID: published.CorrelationID,
	}, nil
}

func (f *factoryImpl) AcceptDispatchResult(
	ctx context.Context,
	req factory.AcceptDispatchResultRequest,
) (factory.AcceptDispatchResultResult, error) {
	if err := f.requireDispatchResultRuntime(); err != nil {
		return factory.AcceptDispatchResultResult{}, err
	}
	if req.CorrelationID == "" {
		return factory.AcceptDispatchResultResult{}, factory.ErrUnknownDispatchCorrelation
	}
	if req.DispatchID == "" || req.WorkID == "" {
		return factory.AcceptDispatchResultResult{}, factory.ErrInvalidDispatchResultBoundary
	}
	outcome, err := rootTerminalResultOutcome(req.ResultOutcome)
	if err != nil {
		return factory.AcceptDispatchResultResult{}, err
	}
	retired, err := f.dispatchFlow.acceptRootResult(ctx, req, outcome)
	if err != nil {
		return factory.AcceptDispatchResultResult{}, mapDispatchPlanningError(err)
	}
	return factory.AcceptDispatchResultResult{
		Outcome:       factory.DispatchPlanOutcome(retired.Outcome),
		DispatchID:    retired.DispatchID,
		CorrelationID: retired.CorrelationID,
	}, nil
}

func (f *factoryImpl) requireDispatchResultRuntime() error {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		return nil
	default:
		return factory.ErrNotRunning
	}
}

func (f *factoryImpl) requireActiveDispatchRuntime() error {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		return nil
	default:
		return factory.ErrNotRunning
	}
}

func validateRootDispatchPlan(req factory.PlanDispatchRequest) error {
	if req.DispatchID == "" || req.CorrelationID == "" || req.WorkstationName == "" ||
		req.WorkerType == "" || req.ReplayKey == "" || len(req.WorkIDs) == 0 {
		return factory.ErrInvalidDispatchResultBoundary
	}
	for _, workID := range req.WorkIDs {
		if workID == "" {
			return factory.ErrInvalidDispatchResultBoundary
		}
	}
	return nil
}

func rootTerminalResultOutcome(
	outcome factory.DispatchResultOutcome,
) (dispatchplanning.TerminalResultOutcome, error) {
	switch outcome {
	case factory.DispatchResultOutcomeSuccess:
		return dispatchplanning.TerminalResultOutcomeSuccess, nil
	case factory.DispatchResultOutcomeFailure:
		return dispatchplanning.TerminalResultOutcomeFailure, nil
	case factory.DispatchResultOutcomeCancelled:
		return dispatchplanning.TerminalResultOutcomeCancelled, nil
	default:
		return "", factory.ErrInvalidDispatchResultBoundary
	}
}

func mapDispatchPlanningError(err error) error {
	switch {
	case errors.Is(err, dispatchplanning.ErrDuplicateDispatchIntent):
		return fmt.Errorf("%w: %v", factory.ErrDuplicateDispatchIntent, err)
	case errors.Is(err, dispatchplanning.ErrUnknownDispatchCorrelation):
		return fmt.Errorf("%w: %v", factory.ErrUnknownDispatchCorrelation, err)
	case errors.Is(err, dispatchplanning.ErrInvalidDispatchResultBoundary),
		errors.Is(err, dispatchplanning.ErrInvalidRunnableDecision):
		return fmt.Errorf("%w: %v", factory.ErrInvalidDispatchResultBoundary, err)
	default:
		return err
	}
}

func interpolateRuntimeWorkerPrompt(
	cfg *runtimeConfig,
	body string,
	invocation *work.InvocationArguments,
) (string, error) {
	if cfg == nil || cfg.invocationInterpolation == nil {
		return body, nil
	}
	interpolated, err := cfg.invocationInterpolation.InterpolateWorkerConfig(
		interfaces.FactoryWorkerConfig{Body: body},
		invocation,
		cfg.invocationFileReader,
	)
	if err != nil {
		return "", err
	}
	return interpolated.Body, nil
}

func runtimeScriptEventRecorder(cfg *runtimeConfig) workers.ScriptEventRecorder {
	if cfg == nil || cfg.eventHistory == nil {
		return nil
	}
	recorder, ok := cfg.eventHistory.(recordings.WorkerEventRecorder)
	if !ok || recorder == nil {
		return nil
	}
	return recorder.RecordScriptEvent
}

func runtimeInferenceEventRecorder(cfg *runtimeConfig) workers.InferenceEventRecorder {
	if cfg == nil || cfg.eventHistory == nil {
		return nil
	}
	recorder, ok := cfg.eventHistory.(recordings.WorkerEventRecorder)
	if !ok || recorder == nil {
		return nil
	}
	return recorder.RecordInferenceEvent
}

func cloneSessionCapabilities(value *workers.Capabilities) *workers.Capabilities {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func recordRuntimeDispatchCompletion(
	cfg *runtimeConfig,
	eventHistory recordings.RuntimeLedger,
	tick int,
	result workers.WorkResult,
	completed interfaces.CompletedDispatch,
) {
	if ignored := completed.IgnoredResult; ignored != nil {
		if ignoredRecorder, ok := eventHistory.(recordings.DispatchResultIgnoredRecorder); ok {
			eventTime := completed.EndTime
			if eventTime.IsZero() {
				eventTime = cfg.clock.Now()
			}
			ignoredRecorder.RecordDispatchResultIgnored(recordings.DispatchResultIgnoredInput{
				SessionID:        sessionIDFromFactoryConfig(cfg),
				OrchestratorKind: interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryConfigFromFactoryConfig(cfg))),
				DispatchID:       result.DispatchID,
				Source:           "runtime",
				Tick:             tick,
				WorkIDs:          []string{completed.IgnoredWorkID},
				Reason:           ignored.Reason,
				ResultOutcome:    ignored.ResultOutcome,
				ObservedState:    ignored.ObservedState,
			}, eventTime)
		}
		return
	}
	eventHistory.RecordWorkstationResponse(tick, result, completed)
}

// finalizeOperatorMove fences matching Runtime intents after the engine has
// committed the move. Cancellation is best effort; the result-time guard is
// the correctness boundary when Workers cannot accept the request.
func (f *factoryImpl) finalizeOperatorMove(ctx context.Context, result work.OperatorMoveResult) {
	if f.dispatchPlan != nil {
		if _, err := f.dispatchPlan.InvalidateWork(ctx, result.WorkID); err != nil {
			f.logger.Error(
				"Factory Runtime Work move committed but dispatch invalidation/cancellation failed",
				"work_id", result.WorkID,
				"error", err,
			)
		}
	}
	// Active-dispatch moves defer the engine wake until the matching outbox
	// intents are fenced. Normal moves may already have signaled; the signal is
	// intentionally idempotent and also covers a nil dispatch planner.
	if f.engine != nil {
		f.engine.WakeForPendingProcessing()
	}
}
