// Package runtime provides the concrete Factory implementation that wires
// together the engine, workers, and subsystems.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	factoryevents "github.com/portpowered/infinite-you/pkg/factory/events"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
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
	resultBuffer *buffers.TypedBuffer[interfaces.WorkResult]
	dispatchHook *workerPoolDispatchResultHook
	eventHistory *factoryevents.FactoryEventHistory
	state        interfaces.FactoryState
	startedAt    time.Time
	clock        factory.Clock
	mu           sync.RWMutex
	// completeCh is closed when Run() returns (either by termination or error).
	// WaitToComplete() returns this channel.
	completeCh chan struct{}
	usePool    bool
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
	resultBuffer := buffers.NewTypedBuffer[interfaces.WorkResult](defaultRuntimeBufferSize)
	eventHistory := ensureEventHistory(cfg)
	engineOpts := buildRuntimeEngineOptions(cfg, logger, sharedTransformer, resultBuffer, eventHistory)
	usePool := !cfg.IsInlineDispatch()
	pool, dispatchHook, engineOpts := configureRuntimeDispatch(cfg, logger, resultBuffer, usePool, engineOpts)
	eng := engine.NewFactoryEngine(cfg.GetNet(), marking, subs, engineOpts...)
	return newFactoryImpl(cfg, eng, pool, logger, resultBuffer, dispatchHook, eventHistory, usePool), nil
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
	eventHistory.AddGeneratedRecorder(cfg.FactoryEventRecorder)
	eventHistory.RecordInitialStructure()
	return eventHistory
}

func buildRuntimeEngineOptions(cfg *factory.FactoryConfig, logger logging.Logger, sharedTransformer *token_transformer.Transformer, resultBuffer *buffers.TypedBuffer[interfaces.WorkResult], eventHistory *factoryevents.FactoryEventHistory) []engine.Option {
	engineOpts := []engine.Option{
		engine.WithLogger(logger),
		engine.WithClock(cfg.Clock),
		engine.WithTokenTransformer(sharedTransformer),
		engine.WithResultBuffer(resultBuffer),
		engine.WithWorkRequestRecorder(func(tick int, record interfaces.WorkRequestRecord) {
			eventHistory.RecordWorkRequest(tick, record, cfg.Clock.Now())
		}),
		engine.WithWorkInputRecorder(func(tick int, req interfaces.SubmitRequest, token interfaces.Token) {
			eventHistory.RecordWorkInput(tick, req, token, cfg.Clock.Now())
		}),
		engine.WithWorkstationResponseRecorder(func(tick int, result interfaces.WorkResult, completed interfaces.CompletedDispatch) {
			eventHistory.RecordWorkstationResponse(tick, result, completed)
		}),
		engine.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			eventHistory.RecordWorkstationRequest(record.Dispatch.Execution.DispatchCreatedTick, record, cfg.Clock.Now())
			if cfg.DispatchRecorder != nil {
				cfg.DispatchRecorder(record)
			}
		}),
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

func configureRuntimeDispatch(cfg *factory.FactoryConfig, logger logging.Logger, resultBuffer *buffers.TypedBuffer[interfaces.WorkResult], usePool bool, engineOpts []engine.Option) (*workers.WorkerPool, *workerPoolDispatchResultHook, []engine.Option) {
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

func inlineDispatchHandler(cfg *factory.FactoryConfig, resultBuffer *buffers.TypedBuffer[interfaces.WorkResult]) func(interfaces.WorkDispatch) {
	executors := cfg.WorkerExecutors
	net := cfg.GetNet()
	return func(d interfaces.WorkDispatch) {
		tr := net.Transitions[d.TransitionID]
		workerType := dispatchRunnerKey(tr, d)
		result := executeDispatchSynchronously(context.Background(), d, workerType, executors)
		resultBuffer.Write(context.Background(), result)
	}
}

func newFactoryImpl(cfg *factory.FactoryConfig, eng *engine.FactoryEngine, pool *workers.WorkerPool, logger logging.Logger, resultBuffer *buffers.TypedBuffer[interfaces.WorkResult], dispatchHook *workerPoolDispatchResultHook, eventHistory *factoryevents.FactoryEventHistory, usePool bool) *factoryImpl {
	return &factoryImpl{
		engine:       eng,
		pool:         pool,
		cfg:          cfg,
		topology:     cfg.GetNet(),
		logger:       logger,
		resultBuffer: resultBuffer,
		dispatchHook: dispatchHook,
		eventHistory: eventHistory,
		state:        interfaces.FactoryStateIdle,
		clock:        cfg.Clock,
		completeCh:   make(chan struct{}),
		usePool:      usePool,
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
	f.eventHistory.RecordRunResponse(f.engine.GetRuntimeStateSnapshot().TickCount, nextState, runStopReason, f.clock.Now())

	if f.usePool {
		f.pool.Stop()
	}

	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// SubmitWorkRequest injects a canonical work request batch idempotently.
func (f *factoryImpl) SubmitWorkRequest(ctx context.Context, request interfaces.WorkRequest) (interfaces.WorkRequestSubmitResult, error) {
	return f.engine.SubmitWorkRequest(ctx, request)
}

// SubscribeFactoryEvents returns canonical history followed by live events.
func (f *factoryImpl) SubscribeFactoryEvents(ctx context.Context) (*interfaces.FactoryEventStream, error) {
	stream := f.eventHistory.Subscribe(ctx)
	return &stream, nil
}

// Pause pauses the factory.
func (f *factoryImpl) Pause(_ context.Context) error {
	f.mu.Lock()
	previousState := f.state
	f.state = interfaces.FactoryStatePaused
	f.mu.Unlock()
	f.recordStateChange(previousState, interfaces.FactoryStatePaused, "pause requested")
	return nil
}

// GetEngineStateSnapshot returns the aggregate observability snapshot for
// service-facing callers.
func (f *factoryImpl) GetEngineStateSnapshot(_ context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	runtimeSnap := f.engine.GetRuntimeStateSnapshot()

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
	return &snap, nil
}

// GetFactoryEvents returns the current-process canonical event history.
func (f *factoryImpl) GetFactoryEvents(_ context.Context) ([]factoryapi.FactoryEvent, error) {
	return f.eventHistory.Events(), nil
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

func (f *factoryImpl) currentWorldState(tick int) *interfaces.FactoryWorldState {
	if f.eventHistory == nil {
		return nil
	}
	state, err := projections.ReconstructFactoryWorldState(f.eventHistory.Events(), tick)
	if err != nil {
		f.logger.Warn("factory world-state reconstruction failed; falling back to runtime snapshot", "error", err)
		return nil
	}
	return &state
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

	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 || hasNonTerminalWorkInWorldState(worldState) || hasNonTerminalWork(snapshot.Marking, f.topology) {
		return interfaces.RuntimeStatusActive
	}

	return interfaces.RuntimeStatusIdle
}

func hasNonTerminalWorkInWorldState(worldState *interfaces.FactoryWorldState) bool {
	return worldState != nil && len(worldState.ActiveWorkItemsByID) > 0
}

func hasNonTerminalWork(marking petri.MarkingSnapshot, topology *state.Net) bool {
	if topology == nil {
		return false
	}

	for _, token := range marking.Tokens {
		if token == nil || token.Color.DataType == interfaces.DataTypeResource || token.Color.WorkTypeID == "" {
			continue
		}

		category := topology.StateCategoryForPlace(token.PlaceID)
		if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
			return true
		}
	}

	return false
}
