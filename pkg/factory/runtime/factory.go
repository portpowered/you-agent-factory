// Package runtime provides the concrete Factory implementation that wires
// together the engine, workers, and subsystems.
package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/buffers"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/engine"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/factory/token_transformer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
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
	eventHistory *factory.FactoryEventHistory
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
// portos:func-length-exception owner=agent-factory reason=legacy-runtime-constructor-wiring review=2026-07-18 removal=split-subsystem-engine-and-dispatch-mode-builders-before-next-runtime-wiring-change
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

	// Default scheduler.
	var sched scheduler.Scheduler
	if cfg.Scheduler != nil {
		scheduler.ApplyRuntimeConfig(cfg.Scheduler, cfg.RuntimeConfig)
		sched = &schedulerAdapter{inner: cfg.Scheduler}
	} else {
		sched = scheduler.NewWorkInQueueScheduler(50, scheduler.WithRuntimeConfig(cfg.RuntimeConfig))
	}

	// Resolve logger — use NoopLogger if none provided.
	logger := logging.EnsureLogger(cfg.Logger)

	// Build subsystems.
	workIDGen := petri.NewWorkIDGenerator()
	sharedTransformer := token_transformer.New(
		cfg.GetNet().Places,
		cfg.GetNet().WorkTypes,
		token_transformer.WithWorkIDGenerator(workIDGen),
	)
	historySubsystem := subsystems.NewHistory(logger)
	transitionerSubsystem := subsystems.NewTransitioner(
		cfg.GetNet(),
		logger,
		subsystems.WithTokenTransformer(sharedTransformer),
		subsystems.WithTransitionerClock(cfg.Clock.Now),
		subsystems.WithTransitionerRuntimeConfig(cfg.RuntimeConfig),
	)
	dispatcher := subsystems.NewDispatcher(
		cfg.GetNet(),
		sched,
		cfg.WorkflowContext,
		logger,
		subsystems.WithDispatcherRuntimeConfig(cfg.RuntimeConfig),
		subsystems.WithDispatcherClock(cfg.Clock.Now),
		subsystems.WithDispatcherThrottlePauseDuration(cfg.ProviderThrottlePauseDuration),
	)

	circuitBreaker := subsystems.NewCircuitBreakerWithClock(
		cfg.GetNet(),
		cfg.Clock.Now,
		logger,
		subsystems.WithCircuitBreakerRuntimeConfig(cfg.RuntimeConfig),
	)
	cascadingFailure := subsystems.NewCascadingFailure(cfg.GetNet(), logger)
	termSub := subsystems.NewTerminationCheck(cfg.GetNet(), logger, cfg.RuntimeMode)

	subs := []subsystems.Subsystem{
		circuitBreaker,
		dispatcher,
		historySubsystem,
		transitionerSubsystem,
		cascadingFailure,
	}

	subs = append(subs, termSub)

	// Create marking and pre-load resource tokens.
	marking := petri.NewMarking(cfg.GetNet().ID)
	for _, rd := range cfg.GetNet().Resources {
		_, tokens := state.GenerateResourcePlaces(rd)
		for _, tok := range tokens {
			marking.AddToken(tok)
		}
	}

	// Build engine options.
	resultBuffer := buffers.NewTypedBuffer[interfaces.WorkResult](defaultRuntimeBufferSize)
	eventHistory := cfg.EventHistory
	if eventHistory == nil {
		eventHistory = factory.NewFactoryEventHistory(cfg.GetNet(), cfg.Clock.Now, cfg.RuntimeConfig)
	}
	eventHistory.RecordRunRequest()
	eventHistory.AddGeneratedRecorder(cfg.FactoryEventRecorder)
	eventHistory.RecordInitialStructure()
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
	}
	if cfg.SubmissionRecorder != nil {
		engineOpts = append(engineOpts, engine.WithSubmissionRecorder(cfg.SubmissionRecorder))
	}
	engineOpts = append(engineOpts, engine.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
		eventHistory.RecordWorkstationRequest(record.Dispatch.Execution.DispatchCreatedTick, record, cfg.Clock.Now())
		if cfg.DispatchRecorder != nil {
			cfg.DispatchRecorder(record)
		}
	}))
	if cfg.CompletionRecorder != nil {
		engineOpts = append(engineOpts, engine.WithCompletionRecorder(cfg.CompletionRecorder))
	}
	for _, hook := range cfg.SubmissionHooks {
		engineOpts = append(engineOpts, engine.WithSubmissionHook(hook))
	}

	// Determine whether to use the real worker pool or synchronous inline dispatch.
	usePool := !cfg.IsInlineDispatch()

	var pool *workers.WorkerPool
	var dispatchHook *workerPoolDispatchResultHook
	if usePool {
		// Build worker pool.
		pool = workers.NewWorkerPool(logger)
		for typ, exec := range cfg.WorkerExecutors {
			pool.Register(typ, exec)
		}

		dispatchHook = newWorkerPoolDispatchResultHook(
			cfg.GetNet(),
			pool,
			cfg.WorkerExecutors,
			logger,
			defaultRuntimeBufferSize,
			cfg.CompletionDeliveryPlanner,
		)
		engineOpts = append(engineOpts, engine.WithDispatchResultHook(dispatchHook))
	} else {
		// Inline dispatch mode: execute worker executors synchronously and enqueue
		// results to the engine so they are processed in the same tick (engine
		// drains pending results after dispatches are forwarded).
		executors := cfg.WorkerExecutors
		net := cfg.GetNet()
		engineOpts = append(engineOpts, engine.WithDispatchHandler(func(d interfaces.WorkDispatch) {
			tr := net.Transitions[d.TransitionID]
			workerType := dispatchRunnerKey(tr, d)
			result := executeDispatchSynchronously(d, workerType, executors)
			resultBuffer.Write(context.Background(), result)
		}))

		builtEng := engine.NewFactoryEngine(
			cfg.GetNet(),
			marking,
			subs,
			engineOpts...,
		)
		return &factoryImpl{
			engine:       builtEng,
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
		}, nil
	}

	eng := engine.NewFactoryEngine(
		cfg.GetNet(),
		marking,
		subs,
		engineOpts...,
	)

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
	}, nil
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
	if err == nil || err == context.Canceled {
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
	if err != nil && err != context.Canceled {
		runStopReason = err.Error()
	}
	f.eventHistory.RecordRunResponse(f.engine.GetRuntimeStateSnapshot().TickCount, nextState, runStopReason, f.clock.Now())

	if f.usePool {
		f.pool.Stop()
	}

	if err == context.Canceled {
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
