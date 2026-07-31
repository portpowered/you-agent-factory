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

	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	checkpointrecoverywire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/wire"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/engine"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime/buffers"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
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
	engine             *engine.FactoryEngine
	cfg                *runtimeConfig
	topology           *state.Net
	logger             logging.Logger
	resultBuffer       *buffers.TypedBuffer[workerexecution.WorkResult]
	dispatchFlow       *dispatchPlanningResultHook
	dispatchPlan       dispatchplanning.Service
	checkpointRecovery checkpointrecovery.Service
	workers            workers.WorkstationPoolBoundary
	eventHistory       recordings.RuntimeLedger
	state              interfaces.FactoryState
	startedAt          time.Time
	clock              factory.Clock
	mu                 sync.RWMutex
	// completeCh is closed when Run() returns (either by termination or error).
	// WaitToComplete() returns this channel.
	completeCh           chan struct{}
	completeOnce         sync.Once
	runCancel            context.CancelFunc
	operatorMoveRequests map[string]appliedOperatorMove
	resumeDrainPending   bool
}

type appliedOperatorMove struct {
	workID string
	result work.OperatorMoveResult
}

type runtimeConfig struct {
	net                       *state.Net
	scheduler                 scheduler.Scheduler
	workerExecutors           map[string]workers.WorkerExecutor
	workerService             workers.WorkstationExecutionService
	runtimeConfig             interfaces.RuntimeDefinitionLookup
	workflowContext           *factory_context.FactoryContext
	runtimeMode               interfaces.RuntimeMode
	logger                    logging.Logger
	clock                     factory.Clock
	workRequestIDs            work.RequestIDGenerator
	eventHistory              recordings.RuntimeLedger
	worldStateProjector       factory.WorldStateProjector
	submissionRecorder        recordings.SubmissionRecorder
	factoryEventRecorder      factory.FactoryEventRecorder
	submissionHooks           []factory.SubmissionHook
	dispatchRecorder          recordings.DispatchRecorder
	completionRecorder        factory.CompletionRecorder
	petriMutationRecorder     factory.PetriMutationRecorder
	completionDeliveryPlanner factory.CompletionDeliveryPlanner
	inlineDispatch            bool
	quorumPolicy              interfaces.QuorumPolicyService
	outputShaping             interfaces.InvocationOutputShapingService
	workPropagation           interfaces.WorkPropagationPolicyService
	decisionEnvelopes         interfaces.DecisionEnvelopeService
}

// Compile-time checks.
var _ factory.Factory = (*factoryImpl)(nil)
var _ factory.Service = (*factoryImpl)(nil)
var _ TickableFactory = (*factoryImpl)(nil)

// New constructs a Factory from explicit runtime collaborators.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func New(
	net *state.Net,
	runtimeScheduler scheduler.Scheduler,
	workerExecutors map[string]workers.WorkerExecutor,
	workerService workers.WorkstationExecutionService,
	runtimeDefinitions interfaces.RuntimeDefinitionLookup,
	workflowContext *factory_context.FactoryContext,
	runtimeMode interfaces.RuntimeMode,
	logger logging.Logger,
	clock factory.Clock,
	inlineDispatch bool,
	eventHistory recordings.RuntimeLedger,
	worldStateProjector factory.WorldStateProjector,
	submissionRecorder recordings.SubmissionRecorder,
	factoryEventRecorder factory.FactoryEventRecorder,
	submissionHooks []factory.SubmissionHook,
	dispatchRecorder recordings.DispatchRecorder,
	completionRecorder factory.CompletionRecorder,
	petriMutationRecorder factory.PetriMutationRecorder,
	completionDeliveryPlanner factory.CompletionDeliveryPlanner,
	quorumPolicy interfaces.QuorumPolicyService,
	outputShaping interfaces.InvocationOutputShapingService,
	workPropagation interfaces.WorkPropagationPolicyService,
	workRequestIDs work.RequestIDGenerator,
	newID factory.IDGenerator,
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) (factory.Factory, error) {
	if net == nil {
		return nil, fmt.Errorf("a factory specification is required")
	}
	if eventHistory == nil {
		return nil, fmt.Errorf("a Recordings runtime ledger is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("a Factory Runtime clock is required")
	}
	if workRequestIDs == nil {
		return nil, fmt.Errorf("a Work Request ID generator is required")
	}
	if newID == nil {
		return nil, fmt.Errorf("a Factory Runtime ID generator is required")
	}
	if workerService == nil {
		return nil, fmt.Errorf("a canonical Workers workstation service is required")
	}
	if runtimeMode == "" {
		runtimeMode = interfaces.RuntimeModeBatch
	}
	cfg := &runtimeConfig{
		net:                       net,
		scheduler:                 runtimeScheduler,
		workerExecutors:           workerExecutors,
		workerService:             workerService,
		runtimeConfig:             runtimeDefinitions,
		workflowContext:           workflowContext,
		runtimeMode:               runtimeMode,
		logger:                    logger,
		clock:                     clock,
		workRequestIDs:            workRequestIDs,
		inlineDispatch:            inlineDispatch,
		eventHistory:              eventHistory,
		worldStateProjector:       worldStateProjector,
		submissionRecorder:        submissionRecorder,
		factoryEventRecorder:      factoryEventRecorder,
		submissionHooks:           append([]factory.SubmissionHook(nil), submissionHooks...),
		dispatchRecorder:          dispatchRecorder,
		completionRecorder:        completionRecorder,
		petriMutationRecorder:     petriMutationRecorder,
		completionDeliveryPlanner: completionDeliveryPlanner,
		quorumPolicy:              quorumPolicy,
		outputShaping:             outputShaping,
		workPropagation:           workPropagation,
		decisionEnvelopes:         firstDecisionEnvelopeService(decisionEnvelopes),
	}

	sched := buildRuntimeScheduler(cfg)
	effectiveLogger := logging.EnsureLogger(cfg.logger)
	sharedTransformer, subs := buildRuntimeSubsystems(cfg, sched, effectiveLogger, newID)
	marking := buildRuntimeMarking(cfg)
	resultBuffer := buffers.NewTypedBuffer[workerexecution.WorkResult](defaultRuntimeBufferSize)
	effectiveEventHistory := ensureEventHistory(cfg)
	dispatchResultHook, dispatchPlan, workersBoundary := configureRuntimeDispatch(
		cfg, resultBuffer,
	)
	impl := newFactoryImpl(
		cfg, nil, effectiveLogger, resultBuffer,
		dispatchResultHook, dispatchPlan, checkpointrecoverywire.New(), workersBoundary, effectiveEventHistory,
	)
	var recordPetriMutations func([]interfaces.TokenMutationRecord) error
	if cfg.petriMutationRecorder != nil {
		recordPetriMutations = func(mutations []interfaces.TokenMutationRecord) error {
			return cfg.petriMutationRecorder(sessionIDFromFactoryConfig(cfg), mutations)
		}
	}
	runtimeEngine, err := engine.NewFactoryEngine(
		cfg.net,
		marking,
		subs,
		effectiveLogger,
		cfg.clock,
		cfg.workRequestIDs,
		nil,
		dispatchResultHook,
		sharedTransformer,
		resultBuffer,
		cfg.submissionHooks,
		cfg.submissionRecorder,
		func(tick int, record work.WorkRequestRecord) {
			effectiveEventHistory.RecordWorkRequest(tick, record, cfg.clock.Now())
		},
		func(tick int, req work.SubmitRequest, token factorytoken.Token) {
			effectiveEventHistory.RecordWorkInput(tick, req, token, cfg.clock.Now())
		},
		func(record interfaces.FactoryDispatchRecord) {
			effectiveEventHistory.RecordWorkstationRequest(
				record.Dispatch.Execution.DispatchCreatedTick, record, cfg.clock.Now(),
			)
			if cfg.dispatchRecorder != nil {
				cfg.dispatchRecorder(record)
			}
		},
		cfg.completionRecorder,
		func(tick int, result workerexecution.WorkResult, completed interfaces.CompletedDispatch) {
			effectiveEventHistory.RecordWorkstationResponse(tick, result, completed)
		},
		recordPetriMutations,
		impl.automaticTicksPaused,
		impl.observePostResumeBufferedDrain,
	)
	if err != nil {
		return nil, fmt.Errorf("create Factory Runtime engine: %w", err)
	}
	impl.engine = runtimeEngine
	return impl, nil
}

func buildRuntimeScheduler(cfg *runtimeConfig) scheduler.Scheduler {
	if cfg.scheduler != nil {
		scheduler.ApplyRuntimeConfig(cfg.scheduler, cfg.runtimeConfig)
		return &schedulerAdapter{inner: cfg.scheduler}
	}
	return scheduler.NewWorkInQueueScheduler(50, cfg.runtimeConfig)
}

func buildRuntimeSubsystems(cfg *runtimeConfig, sched scheduler.Scheduler, logger logging.Logger, newID factory.IDGenerator) (*token_transformer.Transformer, []subsystems.Subsystem) {
	workIDGen := petri.NewWorkIDGenerator()
	sharedTransformer := token_transformer.New(
		cfg.net.Places,
		cfg.net.WorkTypes,
		workIDGen,
	)
	return sharedTransformer, []subsystems.Subsystem{
		subsystems.NewCircuitBreakerWithClock(
			cfg.net,
			cfg.clock.Now,
			logger,
			cfg.runtimeConfig),

		subsystems.NewDispatcher(
			cfg.net,
			sched,
			cfg.workflowContext,
			logger,
			cfg.runtimeConfig,
			cfg.clock.Now,
			newID),

		subsystems.NewHistory(logger),
		subsystems.NewTransitioner(
			cfg.net,
			logger,
			cfg.clock.Now,
			sharedTransformer,
			cfg.runtimeConfig,
			cfg.quorumPolicy,
			cfg.outputShaping,
			cfg.workPropagation,
			cfg.decisionEnvelopes,
		),
		subsystems.NewCascadingFailure(cfg.net, logger, cfg.clock.Now),
		subsystems.NewTerminationCheck(cfg.net, logger, cfg.runtimeMode),
	}
}

func firstDecisionEnvelopeService(
	services []interfaces.DecisionEnvelopeService,
) interfaces.DecisionEnvelopeService {
	if len(services) == 0 {
		return nil
	}
	return services[0]
}

func buildRuntimeMarking(cfg *runtimeConfig) *petri.Marking {
	marking := petri.NewMarking(cfg.net.ID)
	for _, rd := range cfg.net.Resources {
		_, tokens := state.GenerateResourcePlaces(rd, cfg.clock.Now())
		for _, tok := range tokens {
			marking.AddToken(tok)
		}
	}
	return marking
}

func ensureEventHistory(cfg *runtimeConfig) recordings.RuntimeLedger {
	eventHistory := cfg.eventHistory
	eventHistory.RecordRunRequest()
	eventHistory.AddEventRecorder(cfg.factoryEventRecorder)
	eventHistory.RecordInitialStructure()
	recordSessionStartedFromFactoryConfig(cfg, eventHistory)
	return eventHistory
}

func sessionIDFromFactoryConfig(cfg *runtimeConfig) string {
	if cfg != nil && cfg.workflowContext != nil {
		if sessionID := strings.TrimSpace(cfg.workflowContext.SessionID); sessionID != "" {
			return sessionID
		}
	}
	return factory_context.DefaultSessionID
}

func factoryConfigFromFactoryConfig(cfg *runtimeConfig) *interfaces.FactoryConfig {
	if cfg == nil {
		return nil
	}
	provider, ok := cfg.runtimeConfig.(interfaces.RuntimeFactoryConfigLookup)
	if !ok {
		return nil
	}
	return provider.FactoryConfig()
}

func recordSessionStartedFromFactoryConfig(cfg *runtimeConfig, eventHistory recordings.RuntimeLedger) {
	if cfg == nil || eventHistory == nil {
		return
	}
	eventHistory.RecordSessionLifecycleFromFactoryConfig(
		sessionIDFromFactoryConfig(cfg),
		factoryConfigFromFactoryConfig(cfg),
		0,
		cfg.clock.Now(),
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
	f.eventHistory.RecordSessionPaused(recordings.SessionLifecycleControlInput{
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
	f.eventHistory.RecordSessionResumed(recordings.SessionLifecycleControlInput{
		SessionID:        sessionIDFromFactoryConfig(f.cfg),
		OrchestratorKind: interfaces.StrictPublicFactoryOrchestratorKind(interfaces.EffectiveOrchestratorKind(factoryConfigFromFactoryConfig(f.cfg))),
		Source:           "runtime",
		Tick:             tick,
	}, f.clock.Now())
}

func configureRuntimeDispatch(
	cfg *runtimeConfig,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
) (
	*dispatchPlanningResultHook,
	dispatchplanning.Service,
	workers.WorkstationPoolBoundary,
) {
	workersBoundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    cfg.workerService,
		Executors:  cfg.workerExecutors,
		RouteNames: runtimeWorkstationRouteNames(cfg.net, cfg.workerExecutors),
		Async:      !cfg.inlineDispatch && cfg.completionDeliveryPlanner == nil,
	})
	var resultHook *dispatchPlanningResultHook
	planner := dispatchplanningwire.New(
		func(ctx context.Context, request workers.WorkstationDispatchRequest) error {
			return workersBoundary.Publish(ctx, request, resultHook.acceptWorkersResult)
		},
		workersBoundary.Cancel,
	)
	resultHook = newCanonicalDispatchPlanningResultHook(
		planner,
		cfg.net,
		resultBuffer,
		cfg.completionDeliveryPlanner,
		cfg.workRequestIDs,
		sessionIDFromFactoryConfig(cfg),
	)
	return resultHook, planner, workersBoundary
}

func newFactoryImpl(
	cfg *runtimeConfig,
	eng *engine.FactoryEngine,
	logger logging.Logger,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
	dispatchFlow *dispatchPlanningResultHook,
	dispatchPlan dispatchplanning.Service,
	checkpointRecovery checkpointrecovery.Service,
	workersBoundary workers.WorkstationPoolBoundary,
	eventHistory recordings.RuntimeLedger,
) *factoryImpl {
	return &factoryImpl{
		engine:               eng,
		cfg:                  cfg,
		topology:             cfg.net,
		logger:               logger,
		resultBuffer:         resultBuffer,
		dispatchFlow:         dispatchFlow,
		dispatchPlan:         dispatchPlan,
		checkpointRecovery:   checkpointRecovery,
		workers:              workersBoundary,
		eventHistory:         eventHistory,
		state:                interfaces.FactoryStateIdle,
		clock:                cfg.clock,
		completeCh:           make(chan struct{}),
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

	defer f.completeOnce.Do(func() { close(f.completeCh) })

	// Use a derived context for the engine so we can stop the engine before
	// stopping the pool (prevents send-on-closed-channel panics).
	engCtx, cancelEng := context.WithCancel(ctx)
	defer cancelEng()
	f.mu.Lock()
	f.runCancel = cancelEng
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.runCancel = nil
		f.mu.Unlock()
	}()

	// The engine's Run returns when shouldTerminate is true (from
	// TerminationCheck) or context is cancelled. No doneCh select needed.
	runErr := f.engine.Run(engCtx)
	stopReason := dispatchplanning.RuntimeStopReasonTerminated
	if errors.Is(runErr, context.Canceled) || ctx.Err() != nil {
		stopReason = dispatchplanning.RuntimeStopReasonCancelled
	}
	stopErr := f.stopDispatchRuntime(ctx, stopReason)

	f.mu.Lock()
	previousState = f.state
	nextState := interfaces.FactoryStateCompleted
	if (runErr == nil || errors.Is(runErr, context.Canceled)) && stopErr == nil {
		f.state = interfaces.FactoryStateCompleted
		f.logger.Info("factory run completed")
	} else {
		f.state = interfaces.FactoryStateFailed
		nextState = interfaces.FactoryStateFailed
		f.logger.Info("factory run completed with error", "error", errors.Join(runErr, stopErr))
	}
	f.mu.Unlock()
	f.recordStateChange(previousState, nextState, "run stopped")
	runStopReason := ""
	finalErr := errors.Join(runErr, stopErr)
	if finalErr != nil && !(errors.Is(runErr, context.Canceled) && stopErr == nil) {
		runStopReason = finalErr.Error()
	}
	tick := f.engine.GetRuntimeStateSnapshot().TickCount
	completedAt := f.clock.Now()
	f.eventHistory.RecordRunResponse(tick, nextState, runStopReason, completedAt)
	recordSessionLifecycleCompletionFromFactory(f, tick, nextState, runStopReason, completedAt)
	closeRuntimeEventSubscriptions(f.eventHistory)

	if errors.Is(runErr, context.Canceled) && stopErr == nil {
		return nil
	}
	return finalErr
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
func (f *factoryImpl) Pause(ctx context.Context) error {
	_, previousState, err := f.applyPauseControl()
	if err != nil {
		return fmt.Errorf("pause factory: invalid state %s", previousState)
	}
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Pause(ctx); err != nil {
			return fmt.Errorf("pause Factory Runtime dispatch outbox: %w", err)
		}
	}
	return nil
}

func (f *factoryImpl) applyPauseControl() (factory.ControlOutcome, interfaces.FactoryState, error) {
	f.mu.Lock()
	previousState := f.state
	switch previousState {
	case interfaces.FactoryStatePaused:
		f.mu.Unlock()
		return factory.ControlOutcomeNoOp, previousState, nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		return "", previousState, factory.ErrNotRunning
	case interfaces.FactoryStateRunning, interfaces.FactoryStateIdle:
		f.state = interfaces.FactoryStatePaused
	default:
		f.mu.Unlock()
		return "", previousState, factory.ErrInvalidLifecycleTransition
	}
	f.mu.Unlock()
	reason := "pause requested"
	f.recordStateChange(previousState, interfaces.FactoryStatePaused, reason)
	f.recordSessionLifecycleControl(previousState, interfaces.FactoryStatePaused, interfaces.FactorySessionLifecycleControlPause, reason)
	f.recordSessionLifecyclePause()
	f.logRuntimeLifecycleControl("PAUSE", previousState, interfaces.FactoryStatePaused, "ACCEPTED")
	return factory.ControlOutcomeAccepted, previousState, nil
}

// Resume resumes a paused factory.
func (f *factoryImpl) Resume(ctx context.Context) error {
	_, previousState, err := f.applyResumeControl()
	if err != nil {
		return fmt.Errorf("resume factory: invalid state %s", previousState)
	}
	if f.dispatchPlan != nil {
		if err := f.dispatchPlan.Resume(ctx); err != nil {
			return fmt.Errorf("resume Factory Runtime dispatch outbox: %w", err)
		}
	}
	return nil
}

func (f *factoryImpl) applyResumeControl() (factory.ControlOutcome, interfaces.FactoryState, error) {
	f.mu.Lock()
	previousState := f.state
	switch previousState {
	case interfaces.FactoryStateRunning, interfaces.FactoryStateIdle:
		f.mu.Unlock()
		return factory.ControlOutcomeNoOp, previousState, nil
	case interfaces.FactoryStateCompleted, interfaces.FactoryStateFailed:
		f.mu.Unlock()
		return "", previousState, factory.ErrNotRunning
	case interfaces.FactoryStatePaused:
		f.state = interfaces.FactoryStateRunning
	default:
		f.mu.Unlock()
		return "", previousState, factory.ErrInvalidLifecycleTransition
	}
	f.mu.Unlock()
	reason := "resume requested"
	f.recordStateChange(previousState, interfaces.FactoryStateRunning, reason)
	f.recordSessionLifecycleControl(previousState, interfaces.FactoryStateRunning, interfaces.FactorySessionLifecycleControlResume, reason)
	f.recordSessionLifecycleResume()
	f.markResumeDrainPending()
	f.logRuntimeLifecycleControl("RESUME", previousState, interfaces.FactoryStateRunning, "ACCEPTED")
	f.engine.WakeForPendingProcessing()
	return factory.ControlOutcomeAccepted, previousState, nil
}

func (f *factoryImpl) automaticTicksPaused() bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.state == interfaces.FactoryStatePaused
}

// GetEngineStateSnapshot returns the aggregate observability snapshot for
// service-facing callers.
func (f *factoryImpl) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
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
	snap.EnabledTransitions = scheduler.NewEnablementEvaluator(
		f.logger,
		f.clock.Now,
		f.cfg.runtimeConfig,
	).FindEnabledTransitions(ctx, f.topology, &snap.Marking)
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
	f.eventHistory.RecordSessionLifecycleControl(recordings.SessionLifecycleControlInput{
		SessionID:           sessionIDFromFactoryConfig(f.cfg),
		OrchestratorKind:    orchestratorKind,
		OrchestratorDialect: orchestratorDialect,
		Source:              "runtime",
		Tick:                tick,
		Operation:           operation,
		Outcome:             interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		PreviousStatus:      durableLifecycleStatus(previous),
		NewStatus:           durableLifecycleStatus(next),
		Reason:              reason,
	}, f.clock.Now())
}

func durableLifecycleStatus(state interfaces.FactoryState) interfaces.FactorySessionLifecycleStatus {
	switch state {
	case interfaces.FactoryStatePaused:
		return interfaces.FactorySessionLifecycleStatusPaused
	case interfaces.FactoryStateCompleted:
		return interfaces.FactorySessionLifecycleStatusSucceeded
	case interfaces.FactoryStateFailed:
		return interfaces.FactorySessionLifecycleStatusFailed
	default:
		return interfaces.FactorySessionLifecycleStatusRunning
	}
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
	if f.cfg == nil || f.cfg.worldStateProjector == nil {
		return nil
	}
	state, err := f.cfg.worldStateProjector(f.eventHistory.CanonicalEvents(), tick)
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
			InputTokens:   r.InputTokens,
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

func (f *factoryImpl) WorkflowContext() *factory_context.FactoryContext {
	if f == nil || f.cfg == nil {
		return nil
	}
	return f.cfg.workflowContext
}

func closeRuntimeEventSubscriptions(ledger recordings.RuntimeLedger) {
	if ledger == nil {
		return
	}
	closer, ok := ledger.(interface{ CloseLiveSubscriptions() })
	if !ok {
		return
	}
	closer.CloseLiveSubscriptions()
}
