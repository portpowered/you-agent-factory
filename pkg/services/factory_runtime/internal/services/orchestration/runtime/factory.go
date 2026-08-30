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

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
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
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const defaultRuntimeBufferSize = 64

// TickableFactory extends Factory with synchronous tick control.
// Used by the test harness to drive the engine step-by-step without
// starting the async Run loop.
type TickableFactory interface {
	factoryhost.Engine
	Tick(ctx context.Context) error
	TickN(ctx context.Context, n int) error
	TickUntil(ctx context.Context, pred func(*petri.MarkingSnapshot) bool, maxTicks int) error
}

// factoryImpl is the concrete Factory implementation.
type factoryImpl struct {
	engine       *engine.FactoryEngine
	cfg          *runtimeConfig
	topology     *state.Net
	logger       logging.Logger
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult]
	dispatchFlow *dispatchPlanningResultHook
	dispatchPlan dispatchplanning.Service
	eventHistory recordings.RuntimeLedger
	state        interfaces.FactoryState
	startedAt    time.Time
	clock        factory.Clock
	mu           sync.RWMutex
	// completeCh is closed when Run() returns (either by termination or error).
	// WaitToComplete() returns this channel.
	completeCh                  chan struct{}
	completeOnce                sync.Once
	runCancel                   context.CancelFunc
	operatorMoveRequests        map[string]appliedOperatorMove
	resumeDrainPending          bool
	workerSessionControlMu      sync.Mutex
	workerSessionControlResults map[workerSessionControlKey]factory.WorkerSessionControlResult
	capacitySnapshotMu          sync.Mutex
	effectiveFactoryConfig      *interfaces.FactoryConfig
}
type appliedOperatorMove struct {
	workID string
	result work.OperatorMoveResult
}
type runtimePromptRenderer interface {
	RenderPrompt(string, []workers.Token, *workers.Context) (string, error)
}
type runtimeConfig struct {
	net                                *state.Net
	scheduler                          scheduler.Scheduler
	executeService                     executeCapability
	promptRenderer                     runtimePromptRenderer
	templateFieldResolver              runtimeTemplateFieldResolver
	promptSourceReader                 func(string) ([]byte, error)
	attempts                           *attemptLifecycle
	attemptCapacity                    int
	newID                              factory.IDGenerator
	workerSessions                     workersessions.Service
	runtimeConfig                      interfaces.RuntimeDefinitionLookup
	invocationInterpolation            interfaces.InvocationInterpolationService
	invocationFileReader               interfaces.FileReader
	workflowContext                    *factory_context.FactoryContext
	runtimeMode                        interfaces.RuntimeMode
	logger                             logging.Logger
	clock                              factory.Clock
	workRequestIDs                     work.RequestIDGenerator
	eventHistory                       recordings.RuntimeLedger
	recordingID                        string // optional Worker recording target propagated to Worker Sessions
	runtimeID                          string // stable runtime-instance identity used for attempt correlation
	worldStateProjector                factory.WorldStateProjector
	restoredWorldState                 *interfaces.FactoryWorldState
	skipRestoredDispatchReconciliation bool
	providerSessions                   providersessions.Service
	submissionRecorder                 recordings.SubmissionRecorder
	factoryEventRecorder               factory.FactoryEventRecorder
	submissionHooks                    []factory.SubmissionHook
	dispatchRecorder                   recordings.DispatchRecorder
	completionRecorder                 factory.CompletionRecorder
	petriMutationRecorder              factory.PetriMutationRecorder
	completionDeliveryPlanner          factory.CompletionDeliveryPlanner
	replayEvents                       []interfaces.FactoryEvent
	restoredEventPrefix                []interfaces.FactoryEvent
	inlineDispatch                     bool
	quorumPolicy                       interfaces.QuorumPolicyService
	outputShaping                      interfaces.InvocationOutputShapingService
	workPropagation                    interfaces.WorkPropagationPolicyService
	workService                        work.Service
	decisionEnvelopes                  interfaces.DecisionEnvelopeService
	expectedArtifactFileSystem         expectedArtifactFileSystem
	mockWorkersConfig                  *workers.MockWorkersConfig
	progressPublisher                  workers.ProgressPublisher
	modelRuntimeScope                  modelprovider.RuntimeScopeRef
}

var _ factoryhost.Engine = (*factoryImpl)(nil)
var _ factory.Service = (*factoryImpl)(nil)
var _ TickableFactory = (*factoryImpl)(nil)

// New constructs a Factory from explicit runtime collaborators.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func New(
	net *state.Net,
	runtimeScheduler scheduler.Scheduler,
	statelessService executeCapability,
	workerSessionsService workersessions.Service,
	runtimeDefinitions interfaces.RuntimeDefinitionLookup,
	invocationInterpolation interfaces.InvocationInterpolationService,
	invocationFileReader interfaces.FileReader,
	workflowContext *factory_context.FactoryContext,
	runtimeMode interfaces.RuntimeMode,
	logger logging.Logger,
	clock factory.Clock,
	inlineDispatch bool,
	eventHistory recordings.RuntimeLedger,
	recordingID string,
	runtimeID string,
	worldStateProjector factory.WorldStateProjector,
	restoredWorldState *interfaces.FactoryWorldState,
	skipRestoredDispatchReconciliation bool,
	providerSessions providersessions.Service,
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
	workService work.Service,
	workRequestIDs work.RequestIDGenerator,
	newID factory.IDGenerator,
	expectedArtifactFileSystemValue any,
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) (factoryhost.Engine, error) {
	if err := validateFactoryRuntimeDependencies(net, eventHistory, clock, workRequestIDs, newID, statelessService, workerSessionsService); err != nil {
		return nil, err
	}
	runtimeMode = normalizeRuntimeMode(runtimeMode)
	promptRenderer, _ := statelessService.(runtimePromptRenderer)
	templateFieldResolver, _ := statelessService.(runtimeTemplateFieldResolver)
	cfg := &runtimeConfig{
		net:                                net,
		scheduler:                          runtimeScheduler,
		executeService:                     statelessService,
		promptRenderer:                     promptRenderer,
		templateFieldResolver:              templateFieldResolver,
		workerSessions:                     workerSessionsService,
		attemptCapacity:                    defaultRuntimeAttemptCapacity,
		newID:                              newID,
		runtimeConfig:                      runtimeDefinitions,
		invocationInterpolation:            invocationInterpolation,
		invocationFileReader:               invocationFileReader,
		workflowContext:                    workflowContext.Clone(),
		runtimeMode:                        runtimeMode,
		logger:                             logger,
		clock:                              clock,
		workRequestIDs:                     workRequestIDs,
		inlineDispatch:                     inlineDispatch,
		eventHistory:                       eventHistory,
		recordingID:                        strings.TrimSpace(recordingID),
		runtimeID:                          strings.TrimSpace(runtimeID),
		worldStateProjector:                worldStateProjector,
		restoredWorldState:                 restoredWorldState,
		skipRestoredDispatchReconciliation: skipRestoredDispatchReconciliation,
		providerSessions:                   providerSessions,
		submissionRecorder:                 submissionRecorder,
		factoryEventRecorder:               factoryEventRecorder,
		submissionHooks:                    append([]factory.SubmissionHook(nil), submissionHooks...),
		dispatchRecorder:                   dispatchRecorder,
		completionRecorder:                 completionRecorder,
		petriMutationRecorder:              petriMutationRecorder,
		completionDeliveryPlanner:          completionDeliveryPlanner,
		quorumPolicy:                       quorumPolicy,
		outputShaping:                      outputShaping,
		workPropagation:                    workPropagation,
		workService:                        workService,
		expectedArtifactFileSystem:         expectedArtifactFileSystemFrom(expectedArtifactFileSystemValue),
		decisionEnvelopes:                  firstDecisionEnvelopeService(decisionEnvelopes),
	}
	if restoredWorldState != nil && eventHistory != nil {
		cfg.restoredEventPrefix = cloneFactoryEventsInOrder(eventHistory.CanonicalEvents())
	}
	if cfg.executeService != nil {
		cfg.attempts = newAttemptLifecycle(cfg.executeService, cfg.newID, cfg.attemptCapacity)
	}

	sched := buildRuntimeScheduler(cfg)
	effectiveLogger := logging.EnsureLogger(cfg.logger)
	marking, seededRestoredWorkIDs, seededReplayWorkIDsWithRecordedDispatch, err := buildRuntimeMarking(cfg)
	if err != nil {
		return nil, fmt.Errorf("restore Factory Runtime Work board: %w", err)
	}
	sharedTransformer, subs := buildRuntimeSubsystems(cfg, sched, effectiveLogger, newID, seededRestoredWorkIDs)
	resultBuffer := buffers.NewTypedBuffer[workerexecution.WorkResult](defaultRuntimeBufferSize)
	effectiveEventHistory := ensureEventHistory(cfg)
	if !cfg.skipRestoredDispatchReconciliation {
		if err := reconcileRestoredDispatches(cfg, effectiveEventHistory); err != nil {
			return nil, fmt.Errorf("reconcile restored Factory Runtime dispatches: %w", err)
		}
	}
	dispatchResultHook, dispatchPlan, err := configureRuntimeDispatch(
		cfg, resultBuffer, effectiveEventHistory,
	)
	if err != nil {
		return nil, err
	}
	impl := newFactoryImpl(
		cfg, nil, effectiveLogger, resultBuffer,
		dispatchResultHook, dispatchPlan, effectiveEventHistory,
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
			effectiveEventHistory.RecordWorkInput(tick, req, factorytoken.ToWorker(token), cfg.clock.Now())
		},
		func(record interfaces.FactoryDispatchRecord) {
			effectiveEventHistory.RecordWorkstationRequest(
				record.Dispatch.Execution.DispatchCreatedTick, record, cfg.clock.Now(),
			)
			if record.HumanApproval {
				if approvalRecorder, ok := effectiveEventHistory.(recordings.HumanApprovalRequestRecorder); ok {
					approvalRecorder.RecordHumanApprovalRequested(
						record.Dispatch.Execution.DispatchCreatedTick, record, cfg.clock.Now(),
					)
				}
			}
			if cfg.dispatchRecorder != nil {
				cfg.dispatchRecorder(record)
			}
		},
		cfg.completionRecorder,
		func(tick int, result workerexecution.WorkResult, completed interfaces.CompletedDispatch) {
			if ignored := completed.IgnoredResult; ignored != nil {
				if ignoredRecorder, ok := effectiveEventHistory.(recordings.DispatchResultIgnoredRecorder); ok {
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
			effectiveEventHistory.RecordWorkstationResponse(tick, result, completed)
		},
		recordPetriMutations,
		impl.automaticTicksPaused,
		impl.observePostResumeBufferedDrain,
		seededRestoredWorkIDs,
		seededReplayWorkIDsWithRecordedDispatch,
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

func buildRuntimeSubsystems(cfg *runtimeConfig, sched scheduler.Scheduler, logger logging.Logger, newID factory.IDGenerator, seededRestoredWorkIDs map[string]struct{}) (*token_transformer.Transformer, []subsystems.Subsystem) {
	workIDGen := petri.NewWorkIDGenerator()
	var replayIDs factory.ReplayDispatchIDResolver
	if resolver, ok := cfg.completionDeliveryPlanner.(factory.ReplayDispatchIDResolver); ok {
		replayIDs = resolver
	}
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

		subsystems.NewDispatcherWithSeededReplay(
			cfg.net,
			sched,
			cfg.workflowContext,
			logger,
			cfg.runtimeConfig,
			cfg.clock.Now,
			newID,
			replayIDs,
			seededRestoredWorkIDs),

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
		subsystems.NewTerminationCheckWithRuntime(cfg.net, logger, cfg.runtimeMode, cfg.runtimeConfig, cfg.clock.Now),
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

// buildRuntimeMarking returns the fresh marking, the exact restored Work
// identities inserted into it, and the restored Work identities with recorded
// dispatch facts. Invalid or unoccupied historical Work is deliberately absent
// from the first returned set.
func buildRuntimeMarking(cfg *runtimeConfig) (*petri.Marking, map[string]struct{}, map[string]struct{}, error) {
	marking := petri.NewMarking(cfg.net.ID)
	seededRestoredWorkIDs := make(map[string]struct{})
	seededReplayWorkIDsWithRecordedDispatch := restoredWorkIDsWithRecordedDispatch(cfg.restoredWorldState)
	resourcePlaceIDs := make(map[string]struct{}, len(cfg.net.Resources))
	var constructionNow time.Time
	hasConstructionNow := false
	for _, rd := range cfg.net.Resources {
		now := cfg.clock.Now()
		if !hasConstructionNow {
			constructionNow = now
			hasConstructionNow = true
		}
		place, tokens := state.GenerateResourcePlaces(rd, now)
		resourcePlaceIDs[place.ID] = struct{}{}
		for _, tok := range tokens {
			marking.AddToken(tok)
		}
	}
	if cfg.restoredWorldState != nil {
		if !hasConstructionNow {
			constructionNow = cfg.clock.Now()
		}
		var err error
		seededRestoredWorkIDs, err = restoreRestoredWorkMarking(
			cfg,
			marking,
			constructionNow,
			resourcePlaceIDs,
			seededReplayWorkIDsWithRecordedDispatch,
		)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	return marking, seededRestoredWorkIDs, seededReplayWorkIDsWithRecordedDispatch, nil
}
func ensureEventHistory(cfg *runtimeConfig) recordings.RuntimeLedger {
	eventHistory := cfg.eventHistory
	eventHistory.RecordRunRequest()
	eventHistory.AddEventRecorder(cfg.factoryEventRecorder)
	eventHistory.RecordInitialStructure()
	recordSessionStartedFromFactoryConfig(cfg, eventHistory)
	if !cfg.skipRestoredDispatchReconciliation {
		recordRestoredWorkRequests(cfg, eventHistory)
	}
	return eventHistory
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
	eventHistory recordings.RuntimeLedger,
) (
	*dispatchPlanningResultHook,
	dispatchplanning.Service,
	error,
) {
	var resultHook *dispatchPlanningResultHook
	publisher := func(ctx context.Context, request workers.WorkstationDispatchRequest) error {
		return startThroughStatelessWorkers(ctx, cfg, request, resultHook.acceptWorkersResult)
	}
	canceler := func(ctx context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		return cancelStatelessAttempt(ctx, cfg, request)
	}
	planner := dispatchplanningwire.New(
		publisher,
		canceler,
	)
	resultHook = newCanonicalDispatchPlanningResultHook(
		planner,
		cfg.net,
		resultBuffer,
		cfg.completionDeliveryPlanner,
		cfg.workService,
		cfg.workRequestIDs,
		sessionIDFromFactoryConfig(cfg),
	)
	return resultHook, planner, nil
}

func newFactoryImpl(
	cfg *runtimeConfig,
	eng *engine.FactoryEngine,
	logger logging.Logger,
	resultBuffer *buffers.TypedBuffer[workerexecution.WorkResult],
	dispatchFlow *dispatchPlanningResultHook,
	dispatchPlan dispatchplanning.Service,
	eventHistory recordings.RuntimeLedger,
) *factoryImpl {
	return &factoryImpl{
		engine:                      eng,
		cfg:                         cfg,
		topology:                    cfg.net,
		logger:                      logger,
		resultBuffer:                resultBuffer,
		dispatchFlow:                dispatchFlow,
		dispatchPlan:                dispatchPlan,
		eventHistory:                eventHistory,
		state:                       interfaces.FactoryStateIdle,
		clock:                       cfg.clock,
		completeCh:                  make(chan struct{}),
		operatorMoveRequests:        make(map[string]appliedOperatorMove),
		workerSessionControlResults: make(map[workerSessionControlKey]factory.WorkerSessionControlResult),
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
		Source:        source,
		RequestID:     requestID,
		TriggerWorkID: triggerWorkID,
		Reason:        reason,
		SessionID:     sessionIDFromFactoryConfig(f.cfg),
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
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
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
	f.workerSessionControlMu.Lock()
	defer f.workerSessionControlMu.Unlock()
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

func lifecycleControlStatusFromWorldState(worldState *interfaces.FactoryWorldState, factoryState string) string {
	_ = factoryState
	if worldState != nil && worldState.SessionBracket != nil {
		return strings.TrimSpace(worldState.SessionBracket.LifecycleControlStatus)
	}
	return ""
}

// SetProgressPublisher installs the Runtime-owned observation bridge.
func (f *factoryImpl) SetProgressPublisher(publisher workers.ProgressPublisher) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.progressPublisher = publisher
}

// RuntimeProgressPublisher exposes the already-bound runtime observation
// bridge to the Factory Sessions child Execute seam. The concrete publisher
// remains runtime-owned; callers receive only its detached function value.
func (f *factoryImpl) RuntimeProgressPublisher() workers.ProgressPublisher {
	if f == nil || f.cfg == nil {
		return nil
	}
	return f.cfg.progressPublisher
}

// SetMockWorkersConfig installs a cloned request-scoped testing override.
func (f *factoryImpl) SetMockWorkersConfig(config *workers.MockWorkersConfig) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.mockWorkersConfig = config.Clone()
}

// SetPromptSourceReader installs the read-only prompt-source filesystem edge.
func (f *factoryImpl) SetPromptSourceReader(reader func(string) ([]byte, error)) {
	if f == nil || f.cfg == nil {
		return
	}
	f.cfg.promptSourceReader = reader
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

func (f *factoryImpl) WorkflowContext() *factory_context.FactoryContext {
	if f == nil || f.cfg == nil {
		return nil
	}
	return f.cfg.workflowContext.Clone()
}
