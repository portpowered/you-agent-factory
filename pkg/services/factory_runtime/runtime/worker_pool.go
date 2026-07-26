package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/rootobservation"
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

func (f *factoryImpl) ControlPause(_ context.Context, _ factory.PauseRequest) (factory.PauseResult, error) {
	outcome, _, err := f.applyPauseControl()
	if err != nil {
		return factory.PauseResult{}, err
	}
	return factory.PauseResult{Outcome: outcome}, nil
}

func (f *factoryImpl) ControlResume(_ context.Context, _ factory.ResumeRequest) (factory.ResumeResult, error) {
	outcome, _, err := f.applyResumeControl()
	if err != nil {
		return factory.ResumeResult{}, err
	}
	return factory.ResumeResult{Outcome: outcome}, nil
}

func (f *factoryImpl) ControlTerminate(_ context.Context, req factory.TerminateRequest) (factory.TerminateResult, error) {
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
		} else {
			f.completeOnce.Do(func() { close(f.completeCh) })
		}
		return factory.TerminateResult{Outcome: factory.ControlOutcomeAccepted}, nil
	default:
		f.mu.Unlock()
		return factory.TerminateResult{}, factory.ErrNotRunning
	}
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

func (f *factoryImpl) PlanDispatch(_ context.Context, req factory.PlanDispatchRequest) (factory.PlanDispatchResult, error) {
	return factory.PlanDispatchResult{}, f.unavailableRootCapability(
		req.DispatchID == "" || req.CorrelationID == "",
		factory.ErrInvalidDispatchResultBoundary,
		false,
	)
}

func (f *factoryImpl) AcceptDispatchResult(
	_ context.Context,
	req factory.AcceptDispatchResultRequest,
) (factory.AcceptDispatchResultResult, error) {
	var validationErr error
	if req.ResultOutcome != "" &&
		req.ResultOutcome != factory.DispatchResultOutcomeSuccess &&
		req.ResultOutcome != factory.DispatchResultOutcomeFailure &&
		req.ResultOutcome != factory.DispatchResultOutcomeCancelled {
		validationErr = factory.ErrInvalidDispatchResultBoundary
	} else if req.DispatchID == "" || req.CorrelationID == "" {
		validationErr = factory.ErrUnknownDispatchCorrelation
	}
	return factory.AcceptDispatchResultResult{}, f.unavailableRootCapability(
		validationErr != nil,
		validationErr,
		false,
	)
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
