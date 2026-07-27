package runtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/rootobservation"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type dispatchPool interface {
	Dispatch(string, work.WorkDispatch) bool
	ResultCh() <-chan workerexecution.WorkResult
	Start()
	Stop()
}

// workstationExecutionBoundary is the exact canonical Workers role used by
// one Factory Runtime. Runtime plans identities and observes results; Workers
// owns route admission, capacity, executor invocation, and cancellation.
type workstationExecutionBoundary interface {
	StartWorkstationPool(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error)
	StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error)
	DispatchWorkstation(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)
}

type runtimeWorkersBoundary struct {
	service  workstationExecutionBoundary
	bindings []workers.AssembledRuntimeBinding
	async    bool
	started  bool
	stopped  bool
	mu       sync.Mutex
}

type workerExecutorRequestAdapter struct {
	executors map[string]workers.WorkerExecutor
}

func (a workerExecutorRequestAdapter) Execute(
	ctx context.Context,
	request workers.WorkstationExecutionRequest,
) (result workerexecution.WorkResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = workerexecution.WorkResult{
				DispatchID: request.Dispatch.DispatchID, TransitionID: request.Dispatch.TransitionID,
				Outcome: workerexecution.OutcomeFailed,
				Error:   fmt.Sprintf("executor panic: %v", recovered),
			}
			err = nil
		}
	}()
	workerType := request.WorkerType
	if workerType == "" {
		workerType = request.Dispatch.WorkerType
	}
	executor := a.executors[workerType]
	if executor == nil {
		return workerexecution.WorkResult{}, fmt.Errorf(
			"no executor registered for worker type %q",
			workerType,
		)
	}
	return executor.Execute(ctx, request.Dispatch)
}

func runtimeWorkersBoundaryRouteNames(
	net *state.Net,
	executors map[string]workers.WorkerExecutor,
) []string {
	routes := make(map[string]struct{}, len(executors))
	for name := range executors {
		if name != "" {
			routes[name] = struct{}{}
		}
	}
	if net != nil {
		for id, transition := range net.Transitions {
			if id != "" {
				routes[id] = struct{}{}
			}
			if transition == nil {
				continue
			}
			if transition.Name != "" {
				routes[transition.Name] = struct{}{}
			}
			if transition.WorkerType != "" {
				routes[transition.WorkerType] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func newRuntimeWorkersBoundary(
	service workstationExecutionBoundary,
	net *state.Net,
	executors map[string]workers.WorkerExecutor,
	async bool,
) *runtimeWorkersBoundary {
	adapter := workerExecutorRequestAdapter{executors: executors}
	names := runtimeWorkersBoundaryRouteNames(net, executors)
	bindings := make([]workers.AssembledRuntimeBinding, 0, len(names))
	for _, name := range names {
		bindings = append(bindings, workers.AssembledRuntimeBinding{
			RoleName:      name,
			RoleKind:      workers.RuntimeBuildRoleKindWorkstation,
			Executor:      adapter,
			Capacity:      defaultRuntimeBufferSize,
			QueueCapacity: defaultRuntimeBufferSize,
		})
	}
	return &runtimeWorkersBoundary{
		service: service, bindings: bindings, async: async,
	}
}

func (b *runtimeWorkersBoundary) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return nil
	}
	if b.stopped {
		return workers.ErrWorkstationPoolStopped
	}
	if b.service == nil {
		return workers.ErrWorkstationPoolUnavailable
	}
	if _, err := b.service.StartWorkstationPool(
		ctx,
		workers.WorkstationPoolStartRequest{
			Bindings: append([]workers.AssembledRuntimeBinding(nil), b.bindings...),
		},
	); err != nil {
		return err
	}
	b.started = true
	return nil
}

func (b *runtimeWorkersBoundary) Publish(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	accept func(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchResult, error),
) error {
	if err := b.Start(ctx); err != nil {
		return err
	}
	execute := func() {
		result, err := b.service.DispatchWorkstation(context.WithoutCancel(ctx), request)
		accept(context.Background(), request, result, err)
	}
	if b.async {
		go execute()
		return nil
	}
	execute()
	return nil
}

func (b *runtimeWorkersBoundary) Cancel(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if b.service == nil {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return b.service.CancelWorkstationDispatch(ctx, request)
}

func (b *runtimeWorkersBoundary) Stop(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped || !b.started {
		b.stopped = true
		return nil
	}
	_, err := b.service.StopWorkstationPool(ctx)
	if err == nil {
		b.stopped = true
	}
	return err
}

func (f *factoryImpl) ControlPause(ctx context.Context, _ factory.PauseRequest) (factory.PauseResult, error) {
	outcome, _, err := f.applyPauseControl()
	if err != nil {
		return factory.PauseResult{}, err
	}
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Pause(ctx); err != nil {
			return factory.PauseResult{}, err
		}
	}
	return factory.PauseResult{Outcome: outcome}, nil
}

func (f *factoryImpl) ControlResume(ctx context.Context, _ factory.ResumeRequest) (factory.ResumeResult, error) {
	outcome, _, err := f.applyResumeControl()
	if err != nil {
		return factory.ResumeResult{}, err
	}
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Resume(ctx); err != nil {
			return factory.ResumeResult{}, err
		}
	}
	return factory.ResumeResult{Outcome: outcome}, nil
}

func (f *factoryImpl) ControlTerminate(ctx context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
	f.mu.Lock()
	state := f.state
	switch state {
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrAlreadyStopped
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStateCompleted
		cancel := f.runCancel
		f.mu.Unlock()
		f.recordStateChange(state, interfaces.FactoryStateCompleted, req.Reason)
		if cancel != nil {
			cancel()
		}
		stopErr := f.stopDispatchRuntime(ctx, dispatchplanning.RuntimeStopReasonTerminated)
		if cancel == nil {
			f.completeOnce.Do(func() { close(f.completeCh) })
		}
		return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, stopErr
	default:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrNotRunning
	}
}

func (f *factoryImpl) stopDispatchRuntime(
	ctx context.Context,
	reason dispatchplanning.RuntimeStopReason,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var stopErr error
	if f.dispatchPlan != nil {
		stopErr = f.dispatchPlan.Stop(stopCtx, reason)
	}
	if f.workers != nil {
		stopErr = errors.Join(stopErr, f.workers.Stop(stopCtx))
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

func (f *factoryImpl) Observe(ctx context.Context, req factory.ObserveRequest) (factory.ObserveResult, error) {
	if !validObservationScope(req.Scope) {
		return factory.ObserveResult{}, factory.ErrInvalidObservationScope
	}
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle,
		interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
	default:
		return factory.ObserveResult{}, factory.ErrNotRunning
	}
	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		return factory.ObserveResult{}, err
	}
	return factory.ObserveResult{Observation: rootobservation.Project(snapshot, req.Scope)}, nil
}

func (f *factoryImpl) PlanDispatch(
	ctx context.Context,
	req factory.PlanDispatchRequest,
) (factory.PlanDispatchResult, error) {
	if err := f.requireActiveDispatchRuntime(); err != nil {
		return factory.PlanDispatchResult{}, err
	}
	if f.dispatchPlan == nil {
		return factory.PlanDispatchResult{}, factory.ErrCapabilityUnavailable
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
				WorkerType:   req.WorkerType,
				InputPayload: make([]any, 0),
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
	if f.dispatchPlan == nil {
		return factory.AcceptDispatchResultResult{}, factory.ErrCapabilityUnavailable
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
	if f.dispatchFlow == nil {
		return factory.AcceptDispatchResultResult{}, factory.ErrCapabilityUnavailable
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

func (f *factoryImpl) CaptureCheckpoint(
	_ context.Context,
	_ factory.CaptureCheckpointRequest,
) (factory.CaptureCheckpointResult, error) {
	return factory.CaptureCheckpointResult{}, f.unavailableRootCapability(false, nil, false)
}

func (f *factoryImpl) LoadCheckpoint(_ context.Context, req factory.LoadCheckpointRequest) (factory.LoadCheckpointResult, error) {
	return factory.LoadCheckpointResult{}, f.unavailableRootCapability(
		req.CheckpointID == "",
		factory.ErrCheckpointNotFound,
		true,
	)
}

func (f *factoryImpl) RestoreCheckpoint(
	_ context.Context,
	req factory.RestoreCheckpointRequest,
) (factory.RestoreCheckpointResult, error) {
	var validationErr error
	switch {
	case req.Checkpoint.CheckpointID == "":
		validationErr = factory.ErrCheckpointNotFound
	case req.Checkpoint.SchemaVersion <= 0 || len(req.Checkpoint.Payload) == 0:
		validationErr = factory.ErrCorruptCheckpoint
	case req.Checkpoint.SchemaVersion != 1:
		validationErr = factory.ErrIncompatibleCheckpoint
	}
	return factory.RestoreCheckpointResult{}, f.unavailableRootCapability(validationErr != nil, validationErr, false)
}

func (f *factoryImpl) unavailableRootCapability(invalid bool, invalidErr error, allowStopped bool) error {
	f.mu.RLock()
	state := f.state
	f.mu.RUnlock()
	switch state {
	case interfaces.FactoryStateRunning, interfaces.FactoryStatePaused, interfaces.FactoryStateIdle:
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		if !allowStopped {
			return factory.ErrNotRunning
		}
	default:
		return factory.ErrNotRunning
	}
	if invalid {
		return invalidErr
	}
	return factory.ErrCapabilityUnavailable
}

func validObservationScope(scope factory.ObservationScope) bool {
	switch scope {
	case "", factory.ObservationScopeFull, factory.ObservationScopeStatus, factory.ObservationScopeProgress,
		factory.ObservationScopeDispatches, factory.ObservationScopeResults, factory.ObservationScopeResources,
		factory.ObservationScopeHealth:
		return true
	default:
		return false
	}
}

type workerPool struct {
	runners  map[string]*workerRunner
	resultCh chan workerexecution.WorkResult
	logger   logging.Logger
	mu       sync.RWMutex
	clock    factory.Clock
}

func newWorkerPool(logger logging.Logger, clock factory.Clock) *workerPool {
	if clock == nil {
		panic("Factory Runtime worker-pool clock is required")
	}
	return &workerPool{
		runners:  make(map[string]*workerRunner),
		resultCh: make(chan workerexecution.WorkResult, defaultRuntimeBufferSize),
		logger:   logging.EnsureLogger(logger),
		clock:    clock,
	}
}

func (p *workerPool) Register(workerType string, executor workers.WorkerExecutor) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.runners[workerType] = newWorkerRunner(workerType, executor, p.resultCh, p.logger, p.clock)
	p.logger.Debug("pool: worker registered", "worker_type", workerType)
}

func (p *workerPool) Dispatch(workerType string, dispatch work.WorkDispatch) bool {
	p.mu.RLock()
	runner, ok := p.runners[workerType]
	p.mu.RUnlock()
	if !ok {
		p.logger.Error("pool: no runner for worker type", "worker_type", workerType)
		return false
	}
	p.logger.Info("pool: dispatch submitted",
		runtimeWorkLogFields(dispatch.Execution,
			"event_name", "worker_pool.submitted",
			"status", "submitted",
			"worker_type", workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID)...)
	runner.dispatchCh <- dispatch
	return true
}

func (p *workerPool) ResultCh() <-chan workerexecution.WorkResult { return p.resultCh }

func (p *workerPool) Start() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, runner := range p.runners {
		runner.Start()
	}
}

func (p *workerPool) Stop() {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for _, runner := range p.runners {
		runner.Stop()
	}
}

type workerRunner struct {
	workerType string
	executor   workers.WorkerExecutor
	logger     logging.Logger
	dispatchCh chan work.WorkDispatch
	resultCh   chan<- workerexecution.WorkResult
	stopOnce   sync.Once
	clock      factory.Clock
}

func newWorkerRunner(
	workerType string,
	executor workers.WorkerExecutor,
	resultCh chan<- workerexecution.WorkResult,
	logger logging.Logger,
	clock factory.Clock,
) *workerRunner {
	return &workerRunner{
		workerType: workerType,
		executor:   executor,
		logger:     logging.EnsureLogger(logger),
		dispatchCh: make(chan work.WorkDispatch, 16),
		resultCh:   resultCh,
		clock:      clock,
	}
}

func (r *workerRunner) Start() { go r.run() }

func (r *workerRunner) Stop() {
	r.stopOnce.Do(func() { close(r.dispatchCh) })
}

func (r *workerRunner) run() {
	var wait sync.WaitGroup
	for dispatch := range r.dispatchCh {
		wait.Add(1)
		go func(d work.WorkDispatch) {
			defer wait.Done()
			result := r.execute(d)
			r.resultCh <- result
			r.logger.Info("runner: response submitted",
				runtimeWorkLogFields(d.Execution,
					"event_name", "worker_pool.response_submitted",
					"status", "response_submitted",
					"worker_type", r.workerType,
					"transition_id", d.TransitionID,
					"dispatch_id", d.DispatchID,
					"outcome", result.Outcome)...)
		}(dispatch)
	}
	wait.Wait()
}

func (r *workerRunner) execute(dispatch work.WorkDispatch) (result workerexecution.WorkResult) {
	start := r.clock.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			result = panicAsFailedResult(dispatch, recovered, r.clock.Now().Sub(start))
			r.logger.Error("runner: execution panic recovered",
				runtimeWorkLogFields(dispatch.Execution,
					"worker_type", r.workerType,
					"transition_id", dispatch.TransitionID,
					"dispatch_id", dispatch.DispatchID,
					"panic", recovered)...)
		}
	}()

	r.logger.Info("runner: execution started",
		runtimeWorkLogFields(dispatch.Execution,
			"event_name", "worker_pool.executor_entered",
			"status", "entered_executor",
			"worker_type", r.workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID)...)
	result, err := r.executor.Execute(context.Background(), dispatch)
	elapsed := r.clock.Now().Sub(start)
	if err != nil {
		r.logger.Error("runner: execution error",
			runtimeWorkLogFields(dispatch.Execution,
				"worker_type", r.workerType,
				"transition_id", dispatch.TransitionID,
				"dispatch_id", dispatch.DispatchID,
				"error", err)...)
		return workerexecution.WorkResult{
			DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
			Outcome: workerexecution.OutcomeFailed, Error: err.Error(),
			Metrics: workerexecution.WorkMetrics{Duration: elapsed},
		}
	}
	if result.Metrics.Duration == 0 {
		result.Metrics.Duration = elapsed
	}
	r.logger.Info("runner: execution completed",
		runtimeWorkLogFields(dispatch.Execution,
			"worker_type", r.workerType,
			"transition_id", dispatch.TransitionID,
			"dispatch_id", dispatch.DispatchID,
			"outcome", result.Outcome,
			"elapsed_ms", elapsed.Milliseconds())...)
	return result
}

func panicAsFailedResult(
	dispatch work.WorkDispatch,
	recovered any,
	duration time.Duration,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID: dispatch.DispatchID, TransitionID: dispatch.TransitionID,
		Outcome: workerexecution.OutcomeFailed,
		Error:   fmt.Sprintf("executor panic: %v", recovered),
		Metrics: workerexecution.WorkMetrics{Duration: duration},
	}
}

func runtimeWorkLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	workIDs := append([]string(nil), metadata.WorkIDs...)
	primaryWorkID := ""
	for _, workID := range workIDs {
		if workID != "" {
			primaryWorkID = workID
			break
		}
	}
	fields := []any{
		"request_id", metadata.RequestID,
		"trace_id", metadata.TraceID,
		"work_id", primaryWorkID,
		"work_ids", workIDs,
	}
	return append(fields, keysAndValues...)
}
