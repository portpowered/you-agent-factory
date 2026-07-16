// Package runtime provides the concrete Factory implementation that wires
// together the engine, workers, and subsystems.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/work"

	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
)

const defaultRuntimeBufferSize = 64

// TickableFactory extends Factory with synchronous tick control.
// Used by the test harness to drive the engine step-by-step without
// starting the async Run loop.
type TickableFactory interface {
	factory.Factory
	Tick(ctx context.Context) error
	TickN(ctx context.Context, n int) error
	TickUntil(ctx context.Context, pred func(*petri.MarkingSnapshot) bool, maxTicks int) error
}

// factoryImpl is the concrete Factory implementation.
type factoryImpl struct {
	engine       *engine.FactoryEngine
	pool         *workers.WorkerPool
	cfg          *factory.FactoryConfig
	topology     *state.Net
	logger       logging.Logger
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult]
	dispatchHook *workerPoolDispatchResultHook
	eventHistory *factoryevents.FactoryEventHistory
	state        interfaces.FactoryState
	startedAt    time.Time
	clock        factory.Clock
	mu           sync.RWMutex
	// completeCh is closed when Run() returns (either by termination or error).
	// WaitToComplete() returns this channel.
	completeCh           chan struct{}
	usePool              bool
	operatorMoveRequests map[string]appliedOperatorMove
	resumeDrainPending   bool
}

type appliedOperatorMove struct {
	workID string
	result work.OperatorMoveResult
}

// Compile-time checks.
var _ factory.Factory = (*factoryImpl)(nil)
var _ TickableFactory = (*factoryImpl)(nil)

// New constructs a Factory from functional options. It wires the engine,
// worker pool, and subsystems together. Returns an error if required
// options (WithNet) are missing.
func New(opts ...factory.FactoryOption) (factory.Factory, error) {
	cfg := &factory.FactoryConfig{
		RuntimeMode: interfaces.RuntimeModeBatch,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	cfg.Clock = factory.EnsureClock(cfg.Clock)

	if cfg.GetNet() == nil {
		return nil, fmt.Errorf("a factory specification is required")
	}

	sched := buildRuntimeScheduler(cfg)
	logger := logging.EnsureLogger(cfg.Logger)
	sharedTransformer, subs := buildRuntimeSubsystems(cfg, sched, logger)
	marking := buildRuntimeMarking(cfg)
	resultBuffer := buffers.NewTypedBuffer[workerexecution.WorkResult](defaultRuntimeBufferSize)
	eventHistory := ensureEventHistory(cfg)
	engineOpts := buildRuntimeEngineOptions(cfg, logger, sharedTransformer, resultBuffer, eventHistory)
	usePool := !cfg.IsInlineDispatch()
	pool, dispatchHook, engineOpts := configureRuntimeDispatch(cfg, logger, resultBuffer, usePool, engineOpts)
	impl := newFactoryImpl(cfg, nil, pool, logger, resultBuffer, dispatchHook, eventHistory, usePool)
	engineOpts = append(engineOpts,
		engine.WithAutomaticTicksPaused(impl.automaticTicksPaused),
		engine.WithResultBufferDrainObserver(impl.observePostResumeBufferedDrain),
	)
	impl.engine = engine.NewFactoryEngine(cfg.GetNet(), marking, subs, engineOpts...)
	return impl, nil
}

func buildRuntimeScheduler(cfg *factory.FactoryConfig) scheduler.Scheduler {
	if cfg.Scheduler != nil {
		scheduler.ApplyRuntimeConfig(cfg.Scheduler, cfg.RuntimeConfig)
		return &schedulerAdapter{inner: cfg.Scheduler}
	}
	return scheduler.NewWorkInQueueScheduler(50, scheduler.WithRuntimeConfig(cfg.RuntimeConfig))
}

func buildRuntimeSubsystems(cfg *factory.FactoryConfig, sched scheduler.Scheduler, logger logging.Logger) (*token_transformer.Transformer, []subsystems.Subsystem) {
	workIDGen := petri.NewWorkIDGenerator()
	sharedTransformer := token_transformer.New(
		cfg.GetNet().Places,
		cfg.GetNet().WorkTypes,
		token_transformer.WithWorkIDGenerator(workIDGen),
	)
	return sharedTransformer, []subsystems.Subsystem{
		subsystems.NewCircuitBreakerWithClock(
			cfg.GetNet(),
			cfg.Clock.Now,
			logger,
			subsystems.WithCircuitBreakerRuntimeConfig(cfg.RuntimeConfig),
		),
		subsystems.NewDispatcher(
			cfg.GetNet(),
			sched,
			cfg.WorkflowContext,
			logger,
			subsystems.WithDispatcherRuntimeConfig(cfg.RuntimeConfig),
			subsystems.WithDispatcherClock(cfg.Clock.Now),
		),
		subsystems.NewHistory(logger),
		subsystems.NewTransitioner(
			cfg.GetNet(),
			logger,
			subsystems.WithTokenTransformer(sharedTransformer),
			subsystems.WithTransitionerClock(cfg.Clock.Now),
			subsystems.WithTransitionerRuntimeConfig(cfg.RuntimeConfig),
		),
		subsystems.NewCascadingFailure(cfg.GetNet(), logger),
		subsystems.NewTerminationCheck(cfg.GetNet(), logger, cfg.RuntimeMode),
	}
}

func buildRuntimeMarking(cfg *factory.FactoryConfig) *petri.Marking {
	marking := petri.NewMarking(cfg.GetNet().ID)
	for _, rd := range cfg.GetNet().Resources {
		_, tokens := state.GenerateResourcePlaces(rd)
		for _, tok := range tokens {
			marking.AddToken(tok)
		}
	}
	return marking
}

func ensureEventHistory(cfg *factory.FactoryConfig) *factoryevents.FactoryEventHistory {
	eventHistory := cfg.EventHistory
	if eventHistory == nil {
		eventHistory = factoryevents.NewFactoryEventHistory(cfg.GetNet(), cfg.Clock.Now, cfg.RuntimeConfig)
	}
	eventHistory.RecordRunRequest()
	eventHistory.AddEventRecorder(cfg.FactoryEventRecorder)
	eventHistory.RecordInitialStructure()
	recordSessionStartedFromFactoryConfig(cfg, eventHistory)
	return eventHistory
}

func sessionIDFromFactoryConfig(cfg *factory.FactoryConfig) string {
	if cfg != nil && cfg.WorkflowContext != nil {
		if sessionID := strings.TrimSpace(cfg.WorkflowContext.SessionID); sessionID != "" {
			return sessionID
		}
	}
	return factory_context.DefaultSessionID
}

func factoryConfigFromFactoryConfig(cfg *factory.FactoryConfig) *interfaces.FactoryConfig {
	if cfg == nil {
		return nil
	}
	provider, ok := cfg.RuntimeConfig.(interfaces.RuntimeFactoryConfigLookup)
	if !ok {
		return nil
	}
	return provider.FactoryConfig()
}

func recordSessionStartedFromFactoryConfig(cfg *factory.FactoryConfig, eventHistory *factoryevents.FactoryEventHistory) {
	if cfg == nil || eventHistory == nil {
		return
	}
	eventHistory.RecordSessionLifecycleFromFactoryConfig(
		sessionIDFromFactoryConfig(cfg),
		factoryConfigFromFactoryConfig(cfg),
		0,
		cfg.Clock.Now(),
	)
}

func recordSessionLifecycleCompletionFromFactory(
	f *factoryImpl,
	tick int,
	factoryState interfaces.FactoryState,
	reason string,
	eventTime time.Time,
) {
	if f == nil || f.eventHistory == nil || f.cfg == nil {
		return
	}
	f.eventHistory.RecordSessionLifecycleCompletion(
		sessionIDFromFactoryConfig(f.cfg),
		factoryConfigFromFactoryConfig(f.cfg),
		tick,
		factoryState,
		reason,
		eventTime,
	)
}

func (f *factoryImpl) recordSessionLifecyclePause() {
	if f == nil || f.eventHistory == nil || f.cfg == nil {
		return
	}
	tick := 0
	if f.engine != nil {
		tick = f.engine.GetRuntimeStateSnapshot().TickCount
	}
	f.eventHistory.RecordSessionPaused(factoryevents.SessionLifecycleControlInput{
		SessionID:        sessionIDFromFactoryConfig(f.cfg),
		OrchestratorKind: interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryConfigFromFactoryConfig(f.cfg))),
		Source:           "runtime",
		Tick:             tick,
	}, f.clock.Now())
}

func (f *factoryImpl) recordSessionLifecycleResume() {
	if f == nil || f.eventHistory == nil || f.cfg == nil {
		return
	}
	tick := 0
	if f.engine != nil {
		tick = f.engine.GetRuntimeStateSnapshot().TickCount
	}
	f.eventHistory.RecordSessionResumed(factoryevents.SessionLifecycleControlInput{
		SessionID:        sessionIDFromFactoryConfig(f.cfg),
		OrchestratorKind: interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryConfigFromFactoryConfig(f.cfg))),
		Source:           "runtime",
		Tick:             tick,
	}, f.clock.Now())
}

func buildRuntimeEngineOptions(cfg *factory.FactoryConfig, logger logging.Logger, sharedTransformer *token_transformer.Transformer, resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult], eventHistory *factoryevents.FactoryEventHistory) []engine.Option {
	engineOpts := []engine.Option{
		engine.WithLogger(logger),
		engine.WithClock(cfg.Clock),
		engine.WithTokenTransformer(sharedTransformer),
		engine.WithResultBuffer(resultBuffer),
		engine.WithWorkRequestRecorder(func(tick int, record work.WorkRequestRecord) {
			eventHistory.RecordWorkRequest(tick, record, cfg.Clock.Now())
		}),
		engine.WithWorkInputRecorder(func(tick int, req work.SubmitRequest, token factorytoken.Token) {
			eventHistory.RecordWorkInput(tick, req, token, cfg.Clock.Now())
		}),
		engine.WithWorkstationResponseRecorder(func(tick int, result workerexecution.WorkResult, completed interfaces.CompletedDispatch) {
			eventHistory.RecordWorkstationResponse(tick, result, completed)
		}),
		engine.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			eventHistory.RecordWorkstationRequest(record.Dispatch.Execution.DispatchCreatedTick, record, cfg.Clock.Now())
			if cfg.DispatchRecorder != nil {
				cfg.DispatchRecorder(record)
			}
		}),
	}
	if cfg.PetriMutationRecorder != nil {
		engineOpts = append(engineOpts, engine.WithPetriMutationRecorder(func(mutations []interfaces.TokenMutationRecord) error {
			return cfg.PetriMutationRecorder(sessionIDFromFactoryConfig(cfg), mutations)
		}))
	}
	if cfg.SubmissionRecorder != nil {
		engineOpts = append(engineOpts, engine.WithSubmissionRecorder(cfg.SubmissionRecorder))
	}
	if cfg.CompletionRecorder != nil {
		engineOpts = append(engineOpts, engine.WithCompletionRecorder(cfg.CompletionRecorder))
	}
	for _, hook := range cfg.SubmissionHooks {
		engineOpts = append(engineOpts, engine.WithSubmissionHook(hook))
	}
	return engineOpts
}

func configureRuntimeDispatch(cfg *factory.FactoryConfig, logger logging.Logger, resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult], usePool bool, engineOpts []engine.Option) (*workers.WorkerPool, *workerPoolDispatchResultHook, []engine.Option) {
	if !usePool {
		return nil, nil, append(engineOpts, engine.WithDispatchHandler(inlineDispatchHandler(cfg, resultBuffer)))
	}

	pool := workerexecutor.NewWorkerPool(logger)
	for typ, exec := range cfg.WorkerExecutors {
		pool.Register(typ, exec)
	}
	dispatchHook := newWorkerPoolDispatchResultHook(
		cfg.GetNet(),
		pool,
		cfg.WorkerExecutors,
		logger,
		defaultRuntimeBufferSize,
		cfg.CompletionDeliveryPlanner,
	)
	return pool, dispatchHook, append(engineOpts, engine.WithDispatchResultHook(dispatchHook))
}

func inlineDispatchHandler(cfg *factory.FactoryConfig, resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult]) func(work.WorkDispatch) {
	executors := cfg.WorkerExecutors
	net := cfg.GetNet()
	return func(d work.WorkDispatch) {
		tr := net.Transitions[d.TransitionID]
		workerType := dispatchRunnerKey(tr, d)
		result := executeDispatchSynchronously(context.Background(), d, workerType, executors)
		resultBuffer.Write(context.Background(), result)
	}
}

func newFactoryImpl(cfg *factory.FactoryConfig, eng *engine.FactoryEngine, pool *workers.WorkerPool, logger logging.Logger, resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult], dispatchHook *workerPoolDispatchResultHook, eventHistory *factoryevents.FactoryEventHistory, usePool bool) *factoryImpl {
	return &factoryImpl{
		engine:               eng,
		pool:                 pool,
		cfg:                  cfg,
		topology:             cfg.GetNet(),
		logger:               logger,
		resultBuffer:         resultBuffer,
		dispatchHook:         dispatchHook,
		eventHistory:         eventHistory,
		state:                interfaces.FactoryStateIdle,
		clock:                cfg.Clock,
		completeCh:           make(chan struct{}),
		usePool:              usePool,
		operatorMoveRequests: make(map[string]appliedOperatorMove),
	}
}

// Run starts the factory. Blocks until ctx is cancelled or the engine
// terminates (all tokens terminal/failed, or deadlock detected).
// Closes completeCh when Run returns so WaitToComplete() unblocks.
func (f *factoryImpl) Run(ctx context.Context) error {
	f.mu.Lock()
	previousState := f.state
	f.state = interfaces.FactoryStateRunning
	f.startedAt = f.clock.Now()
	f.mu.Unlock()
	f.recordStateChange(previousState, interfaces.FactoryStateRunning, "run started")

	defer close(f.completeCh)

	// Use a derived context for the engine so we can stop the engine before
	// stopping the pool (prevents send-on-closed-channel panics).
	engCtx, cancelEng := context.WithCancel(ctx)
	defer cancelEng()

	if f.usePool {
		f.pool.Start()
		f.dispatchHook.Start(engCtx)
	}

	// The engine's Run returns when shouldTerminate is true (from
	// TerminationCheck) or context is cancelled. No doneCh select needed.
	err := f.engine.Run(engCtx)

	f.mu.Lock()
	previousState = f.state
	nextState := interfaces.FactoryStateCompleted
	if err == nil || errors.Is(err, context.Canceled) {
		f.state = interfaces.FactoryStateCompleted
		f.logger.Info("factory run completed")
	} else {
		f.state = interfaces.FactoryStateFailed
		nextState = interfaces.FactoryStateFailed
		f.logger.Info("factory run completed with error", "error", err)
	}
	f.mu.Unlock()
	f.recordStateChange(previousState, nextState, "run stopped")
	runStopReason := ""
	if err != nil && !errors.Is(err, context.Canceled) {
		runStopReason = err.Error()
	}
	tick := f.engine.GetRuntimeStateSnapshot().TickCount
	completedAt := f.clock.Now()
	f.eventHistory.RecordRunResponse(tick, nextState, runStopReason, completedAt)
	recordSessionLifecycleCompletionFromFactory(f, tick, nextState, runStopReason, completedAt)

	if f.usePool {
		f.pool.Stop()
	}

	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// SubmitWorkRequest injects a canonical work request batch idempotently.
func (f *factoryImpl) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	return f.engine.SubmitWorkRequest(ctx, request)
}

// MoveWork validates and applies a synchronous operator relocation for one work item.
func (f *factoryImpl) MoveWork(ctx context.Context, workID string, stateName string, source work.WorkStateChangeSource, requestID string) (work.OperatorMoveResult, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID != "" {
		f.mu.RLock()
		if existing, ok := f.operatorMoveRequests[requestID]; ok {
			f.mu.RUnlock()
			if existing.workID != workID {
				return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
			}
			return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
		}
		f.mu.RUnlock()
	}

	result, err := f.engine.MoveWork(ctx, workID, stateName)
	if err != nil {
		return result, err
	}

	if requestID != "" {
		f.mu.Lock()
		if f.operatorMoveRequests == nil {
			f.operatorMoveRequests = make(map[string]appliedOperatorMove)
		}
		f.operatorMoveRequests[requestID] = appliedOperatorMove{
			workID: workID,
			result: result,
		}
		f.mu.Unlock()
	}

	if source != "" {
		f.recordOperatorWorkStateChange(result, source, requestID, "", "")
	}
	return result, nil
}

func (f *factoryImpl) recordOperatorWorkStateChange(result work.OperatorMoveResult, source work.WorkStateChangeSource, requestID, triggerWorkID, reason string) {
	workTypeName := result.WorkTypeID
	if workType, ok := f.topology.WorkTypes[result.WorkTypeID]; ok && workType != nil {
		if name := strings.TrimSpace(workType.Name); name != "" {
			workTypeName = name
		}
	}
	tick := f.engine.GetRuntimeStateSnapshot().TickCount
	f.eventHistory.RecordWorkStateChange(tick, work.WorkStateChangeRecord{
		WorkID:        result.WorkID,
		WorkTypeID:    result.WorkTypeID,
		WorkTypeName:  workTypeName,
		FromState:     result.FromState,
		ToState:       result.ToState,
		FromPlaceID:   result.FromPlaceID,
		ToPlaceID:     result.ToPlaceID,
		Source:        source,
		RequestID:     requestID,
		TriggerWorkID: triggerWorkID,
		Reason:        reason,
	}, f.clock.Now())
}

// SubscribeFactoryEvents returns canonical history followed by live events.
func (f *factoryImpl) SubscribeFactoryEvents(ctx context.Context, reconnect *interfaces.FactoryEventReconnectCursor, scope interfaces.FactoryEventReconnectScope) (*interfaces.FactoryEventStream, error) {
	stream, err := f.eventHistory.Subscribe(ctx, reconnect, scope)
	if err != nil {
		return nil, err
	}
	return &stream, nil
}

// Pause pauses the factory. Repeated calls while already paused are a no-op.
func (f *factoryImpl) Pause(_ context.Context) error {
	f.mu.Lock()
	previousState := f.state
	if previousState == interfaces.FactoryStatePaused {
		f.mu.Unlock()
		return nil
	}
	if previousState == interfaces.FactoryStateCompleted || previousState == interfaces.FactoryStateFailed {
		f.mu.Unlock()
		return fmt.Errorf("pause factory: invalid state %s", previousState)
	}
	f.state = interfaces.FactoryStatePaused
	f.mu.Unlock()
	reason := "pause requested"
	f.recordStateChange(previousState, interfaces.FactoryStatePaused, reason)
	f.recordSessionLifecycleControl(previousState, interfaces.FactoryStatePaused, interfaces.FactorySessionLifecycleControlPause, reason)
	f.recordSessionLifecyclePause()
	f.logRuntimeLifecycleControl("PAUSE", previousState, interfaces.FactoryStatePaused, "ACCEPTED")
	return nil
}

// Resume resumes a paused factory.
func (f *factoryImpl) Resume(_ context.Context) error {
	f.mu.Lock()
	previousState := f.state
	if previousState == interfaces.FactoryStateRunning || previousState == interfaces.FactoryStateIdle {
		f.mu.Unlock()
		return nil
	}
	if previousState != interfaces.FactoryStatePaused {
		f.mu.Unlock()
		return fmt.Errorf("resume factory: invalid state %s", previousState)
	}
	f.state = interfaces.FactoryStateRunning
	f.mu.Unlock()
	reason := "resume requested"
	f.recordStateChange(previousState, interfaces.FactoryStateRunning, reason)
	f.recordSessionLifecycleControl(previousState, interfaces.FactoryStateRunning, interfaces.FactorySessionLifecycleControlResume, reason)
	f.recordSessionLifecycleResume()
	f.markResumeDrainPending()
	f.logRuntimeLifecycleControl("RESUME", previousState, interfaces.FactoryStateRunning, "ACCEPTED")
	f.engine.WakeForPendingProcessing()
	return nil
}

func (f *factoryImpl) automaticTicksPaused() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state == interfaces.FactoryStatePaused
}

// GetEngineStateSnapshot returns the aggregate observability snapshot for
// service-facing callers.
func (f *factoryImpl) GetEngineStateSnapshot(_ context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	runtimeSnap := f.engine.GetRuntimeStateSnapshot()
	runtimeSnap.StreamGenerationID = f.eventHistory.StreamGenerationID()

	f.mu.RLock()
	currentState := f.state
	startedAt := f.startedAt
	now := f.clock.Now()
	f.mu.RUnlock()

	worldState := f.currentWorldState(runtimeSnap.TickCount)
	runtimeSnap.RuntimeStatus = f.deriveRuntimeStatus(currentState, runtimeSnap, worldState)
	uptime := time.Duration(0)
	if !startedAt.IsZero() {
		uptime = now.Sub(startedAt)
	}

	snap := state.NewEngineStateSnapshot(runtimeSnap, string(currentState), uptime, f.topology)
	snap.LifecycleControlStatus = lifecycleControlStatusFromWorldState(worldState, string(currentState))
	return &snap, nil
}

// GetFactoryEvents returns the current-process canonical event history.
func (f *factoryImpl) GetFactoryEvents(_ context.Context) ([]interfaces.FactoryEvent, error) {
	return f.eventHistory.CanonicalEvents(), nil
}

// WaitToComplete returns a channel that is closed when Run() returns (either
// by normal termination, deadlock detection, or error). Callers can select
// on this channel to detect factory completion.
func (f *factoryImpl) WaitToComplete() <-chan struct{} {
	return f.completeCh
}

// Tick executes a single engine tick synchronously. For use by the test harness.
func (f *factoryImpl) Tick(ctx context.Context) error {
	return f.engine.Tick(ctx)
}

// TickN executes n ticks sequentially. For use by the test harness.
func (f *factoryImpl) TickN(ctx context.Context, n int) error {
	return f.engine.TickN(ctx, n)
}

// TickUntil ticks until the predicate returns true or maxTicks is exceeded.
// For use by the test harness.
func (f *factoryImpl) TickUntil(ctx context.Context, pred func(*petri.MarkingSnapshot) bool, maxTicks int) error {
	return f.engine.TickUntil(ctx, pred, maxTicks)
}

func (f *factoryImpl) recordStateChange(previous interfaces.FactoryState, next interfaces.FactoryState, reason string) {
	if f.eventHistory == nil {
		return
	}
	tick := 0
	if f.engine != nil {
		tick = f.engine.GetRuntimeStateSnapshot().TickCount
	}
	f.eventHistory.RecordFactoryStateChange(tick, previous, next, reason, f.clock.Now())
}

func (f *factoryImpl) recordSessionLifecycleControl(
	previous interfaces.FactoryState,
	next interfaces.FactoryState,
	operation interfaces.FactorySessionLifecycleControlKind,
	reason string,
) {
	if f.eventHistory == nil || f.cfg == nil {
		return
	}
	tick := 0
	if f.engine != nil {
		tick = f.engine.GetRuntimeStateSnapshot().TickCount
	}
	factoryCfg := factoryConfigFromFactoryConfig(f.cfg)
	orchestratorKind := interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryCfg))
	var orchestratorDialect string
	if factoryCfg != nil && factoryCfg.Orchestrator != nil && factoryCfg.Orchestrator.JavaScript != nil {
		orchestratorDialect = factoryCfg.Orchestrator.JavaScript.Dialect
	}
	f.eventHistory.RecordSessionLifecycleControl(factoryevents.SessionLifecycleControlInput{
		SessionID:           sessionIDFromFactoryConfig(f.cfg),
		OrchestratorKind:    orchestratorKind,
		OrchestratorDialect: orchestratorDialect,
		Source:              "runtime",
		Tick:                tick,
		Operation:           operation,
		Outcome:             interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		PreviousStatus:      factoryevents.FactoryStateToDurableLifecycleStatus(previous),
		NewStatus:           factoryevents.FactoryStateToDurableLifecycleStatus(next),
		Reason:              reason,
	}, f.clock.Now())
}

func (f *factoryImpl) logRuntimeLifecycleControl(
	operation string,
	previousState interfaces.FactoryState,
	nextState interfaces.FactoryState,
	outcome string,
) {
	if f == nil || f.logger == nil {
		return
	}
	f.logger.Info(
		"factory runtime lifecycle control",
		"session_id", sessionIDFromFactoryConfig(f.cfg),
		"operation", operation,
		"outcome", outcome,
		"previous_factory_state", string(previousState),
		"factory_state", string(nextState),
	)
}

func (f *factoryImpl) markResumeDrainPending() {
	if f == nil {
		return
	}
	pending := 0
	if f.resultBuffer != nil {
		pending = f.resultBuffer.Len()
	}
	f.mu.Lock()
	f.resumeDrainPending = pending > 0
	f.mu.Unlock()
	if pending > 0 {
		f.logger.Info(
			"factory runtime resume buffered results pending drain",
			"session_id", sessionIDFromFactoryConfig(f.cfg),
			"buffered_result_count", pending,
		)
	}
}

func (f *factoryImpl) observePostResumeBufferedDrain(drainedCount int) {
	if f == nil || drainedCount <= 0 {
		return
	}
	f.mu.Lock()
	pending := f.resumeDrainPending
	if pending {
		f.resumeDrainPending = false
	}
	f.mu.Unlock()
	if !pending {
		return
	}
	f.logger.Info(
		"factory runtime resume buffered results drained",
		"session_id", sessionIDFromFactoryConfig(f.cfg),
		"drained_result_count", drainedCount,
	)
}

func (f *factoryImpl) currentWorldState(tick int) *interfaces.FactoryWorldState {
	if f.eventHistory == nil {
		return nil
	}
	state, err := projections.ReconstructCanonicalFactoryWorldState(f.eventHistory.CanonicalEvents(), tick)
	if err != nil {
		f.logger.Warn("factory world-state reconstruction failed; falling back to runtime snapshot", "error", err)
		return nil
	}
	return &state
}

func lifecycleControlStatusFromWorldState(worldState *interfaces.FactoryWorldState, factoryState string) string {
	_ = factoryState
	if worldState != nil && worldState.SessionBracket != nil {
		return strings.TrimSpace(worldState.SessionBracket.LifecycleControlStatus)
	}
	return ""
}

// schedulerAdapter adapts factory.TransitionScheduler to scheduler.Scheduler.
type schedulerAdapter struct {
	inner scheduler.Scheduler
}

func (a *schedulerAdapter) Select(enabled []interfaces.EnabledTransition, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) []interfaces.FiringDecision {
	results := a.inner.Select(enabled, snapshot)
	decisions := make([]interfaces.FiringDecision, len(results))
	for i, r := range results {
		decisions[i] = interfaces.FiringDecision{
			TransitionID:  r.TransitionID,
			ConsumeTokens: r.ConsumeTokens,
			WorkerType:    r.WorkerType,
			InputBindings: r.InputBindings,
		}
	}
	return decisions
}

func (a *schedulerAdapter) SupportsRepeatedTransitionBindings() bool {
	if a == nil {
		return false
	}
	return scheduler.SupportsRepeatedTransitionBindings(a.inner)
}

func (f *factoryImpl) deriveRuntimeStatus(currentState interfaces.FactoryState, snapshot interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], worldState *interfaces.FactoryWorldState) interfaces.RuntimeStatus {
	if currentState == interfaces.FactoryStateCompleted || currentState == interfaces.FactoryStateFailed {
		return interfaces.RuntimeStatusFinished
	}

	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 || hasNonTerminalWork(snapshot.Marking, f.topology) {
		return interfaces.RuntimeStatusActive
	}

	return interfaces.RuntimeStatusIdle
}

func hasNonTerminalWork(marking petri.MarkingSnapshot, topology *state.Net) bool {
	if topology == nil {
		return false
	}

	for _, token := range marking.Tokens {
		if token == nil || token.Color.DataType == factorytoken.DataTypeResource || token.Color.WorkTypeID == "" {
			continue
		}

		category := topology.StateCategoryForPlace(token.PlaceID)
		if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
			return true
		}
	}

	return false
}

// WorkflowContext returns the workflow context wired at factory construction.
func WorkflowContext(f factory.Factory) *factory_context.FactoryContext {
	impl, ok := f.(*factoryImpl)
	if !ok || impl == nil || impl.cfg == nil {
		return nil
	}
	return impl.cfg.WorkflowContext
}
