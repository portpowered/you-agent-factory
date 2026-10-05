package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// openRuntimeWithOptions opens session-owned state using the collaborators already
// injected into this owner. Only invocation selections cross this boundary.
func (r *Root) openRuntimeWithOptions(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	runtime factoryruntime.RuntimeSelection,
	session *factorysessions.SessionStartRequest,
	canonicalSessionIDGenerated bool,
	worker workers.RuntimeSelection,
	recording recordings.RuntimeSelection,
	modelCacheDirectory string,
	operatorDefaults operatorsettings.ResolvedDefaults,
	baseLogger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (products runtimeProducts, err error) {
	opening, err := r.prepareRuntimeOpening(ctx, definition, runtime, session,
		canonicalSessionIDGenerated, worker, recording, modelCacheDirectory,
		operatorDefaults, baseLogger, definitionSnapshot, replayInput)
	if err != nil {
		return runtimeProducts{}, err
	}
	if opening.load.HistoricalReplay != nil {
		return r.openHistoricalSessionRuntime(opening)
	}
	cleanup := &runtimeOpeningCleanup{}
	defer func() {
		if err != nil {
			if cleanupErr := cleanup.Close(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
				products.closeArtifacts = cleanup.Close
			}
		}
	}()
	if err = r.openSessionDurableScopes(ctx, opening, cleanup); err != nil {
		return runtimeProducts{}, err
	}
	if err = r.openSessionEngine(ctx, opening, cleanup); err != nil {
		return runtimeProducts{}, err
	}
	return r.completeSessionOpening(ctx, opening, cleanup)
}

// sessionRuntimeOpening retains one opening's selections and partial results.
// Its lifetime ends with the opening; peer openings never share this state.
type sessionRuntimeOpening struct {
	configured                  preparedRuntime
	root                        RuntimeRoot
	load                        RuntimeLoad
	clock                       factoryruntime.Clock
	logger                      *zap.Logger
	sessionID                   string
	metricsSessionID            string
	providedCanonicalSessionID  string
	canonicalSessionIDGenerated bool
	sessionSelection            *factorysessions.SessionRuntimeSelection
	operatorSettingsPath        string
	recordingProjections        recordings.ProjectionService
	providerForDurable          providers.Service
	durableExecution            DurableExecution
	modelsBind                  modelsRuntimeBind
	resumeInput                 *recordings.LoadResumeInputResult
	restoredWorldState          *factorydefinitions.FactoryWorldState
	restoredEventHistory        []factorydefinitions.FactoryEvent
	boardHistoryOpening         currentBoardHistoryOpening
	runtimebuildService         runtimeports.RuntimeReplacementBuilder
	startupRuntime              runtimeports.RuntimeInstance
	startupSpec                 factoryruntime.SessionBuildSpec
	runtimeLifecycle            runtimeports.RuntimeLifecycle
	runtimeSidecars             runtimeports.RuntimeSidecarService
}

func (r *Root) prepareRuntimeOpening(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	runtime factoryruntime.RuntimeSelection,
	session *factorysessions.SessionStartRequest,
	canonicalSessionIDGenerated bool,
	worker workers.RuntimeSelection,
	recording recordings.RuntimeSelection,
	modelCacheDirectory string,
	operatorDefaults operatorsettings.ResolvedDefaults,
	baseLogger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (*sessionRuntimeOpening, error) {
	if session == nil {
		return nil, fmt.Errorf("Factory Session runtime selection is required")
	}
	if r.recordingsService == nil {
		return nil, fmt.Errorf("construct runtime scope: Recordings service is required")
	}
	if r.recordingsRuntime == nil {
		return nil, fmt.Errorf("construct runtime scope: Recordings runtime scope is required")
	}
	selection := sessionRuntimeSelection(session)
	providedCanonicalSessionID := strings.TrimSpace(selection.CanonicalSessionID)
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	session.SessionID = sessionID
	configured, root, load, clock, logger, err := r.prepareRuntime(
		ctx,
		definition,
		runtime,
		*session,
		canonicalSessionIDGenerated,
		worker,
		recording,
		modelCacheDirectory,
		operatorDefaults,
		baseLogger,
		definitionSnapshot,
		replayInput,
	)
	if err != nil {
		return nil, err
	}
	// Resolve and validate the Factory Definition before allocating the
	// canonical metrics identity. Invalid Current Factory or working-directory
	// input must not consume a Factory Session identity or create a product
	// lifecycle effect.
	if err := ensureDefaultCanonicalSessionID(session, recording.ReplayPath, r.generateRuntimeInstanceID); err != nil {
		return nil, err
	}
	canonicalSessionIDGenerated = canonicalSessionIDGenerated ||
		(providedCanonicalSessionID == "" && strings.TrimSpace(selection.CanonicalSessionID) != "")
	configured.CanonicalSessionIDGenerated = canonicalSessionIDGenerated
	sessionSelection := sessionRuntimeSelection(&configured.Session)
	sessionSelection.CanonicalSessionID = selection.CanonicalSessionID
	metricsSessionID := strings.TrimSpace(sessionSelection.CanonicalSessionID)
	if metricsSessionID == "" {
		metricsSessionID = sessionID
	}
	if err := applyRuntimeWorkerReasoningEffort(configured, load); err != nil {
		return nil, err
	}
	return &sessionRuntimeOpening{
		configured:                  configured,
		root:                        root,
		load:                        load,
		clock:                       clock,
		logger:                      logger,
		sessionID:                   sessionID,
		metricsSessionID:            metricsSessionID,
		providedCanonicalSessionID:  providedCanonicalSessionID,
		canonicalSessionIDGenerated: canonicalSessionIDGenerated,
		sessionSelection:            sessionSelection,
	}, nil
}

func applyRuntimeWorkerReasoningEffort(configured preparedRuntime, load RuntimeLoad) error {
	if effort := strings.TrimSpace(configured.Workers.WorkerReasoningEffort); effort != "" &&
		load.LoadedFactoryCfg != nil {
		if err := load.LoadedFactoryCfg.MutateWorkers(func(worker *factorydefinitions.FactoryWorkerConfig) error {
			if worker != nil {
				worker.ReasoningEffort = effort
			}
			return nil
		}); err != nil {
			return fmt.Errorf("apply worker reasoning effort override: %w", err)
		}
	}
	return nil
}

func (r *Root) openHistoricalSessionRuntime(opening *sessionRuntimeOpening) (runtimeProducts, error) {
	var err error
	var liveOwner durableexecution.Service
	var replayClose func() error
	if opening.load.HistoricalReplay.Checkpoint != nil {
		liveOwner, replayClose, err = openPortableReplayDurableOwner(
			opening.configured,
			opening.root,
			opening.logger,
			r.clock,
			r.providerOverride,
			r.providerCommandRunner,
			r.scriptCommandRunner,
			r.workerService,
			r.workersMockCommandRunnerFactory,
			r.providerFromCommandRunnerFactory,
			r.durableExecutionFactory,
			r.factorySessionExecutionFactory,
			r.providerIdentities,
			r.resolveClock,
			r.factoryRuntimeAssembler,
			r.recordingsRuntime,
			r.initialFactorySnapshotFactory,
			r.loadFactory,
			r.automationService,
			r.submissionRecorder,
			r.dispatchRecorder,
		)
		if err != nil {
			return runtimeProducts{}, err
		}
	}
	historicalProducts := historicalReplayRuntimeProducts(
		opening.logger,
		*opening.load.HistoricalReplay,
		liveOwner,
		replayClose,
	)
	historicalProducts.replayMetadataWarnings = append(
		[]recordings.MetadataMismatchWarning(nil),
		opening.load.ReplayMetadataWarnings...,
	)
	return historicalProducts, nil
}

func (r *Root) openSessionDurableScopes(ctx context.Context, opening *sessionRuntimeOpening, cleanup *runtimeOpeningCleanup) error {
	var err error
	opening.operatorSettingsPath, err = operatorConfigPath(opening.sessionSelection.SystemConfigPath, opening.sessionSelection.SystemConfigHome)
	if err != nil {
		return fmt.Errorf("resolve operator settings path for runtime transport: %w", err)
	}
	if opening.clock == nil {
		return fmt.Errorf("construct runtime scope: Factory Runtime clock is required")
	}
	opening.recordingProjections = r.recordingsRuntime.Projection()
	if opening.recordingProjections == nil {
		return fmt.Errorf("construct runtime scope: Recordings projection is unavailable")
	}
	if r.durableExecutionFactory == nil {
		return fmt.Errorf("construct runtime scope: durable execution operation is required")
	}
	opening.providerForDurable, err = resolveDurableExecutionProvider(
		r.providerOverride,
		opening.configured.Workers.MockWorkers,
		opening.load.LoadedFactoryCfg,
		r.providerCommandRunner,
		r.workersMockCommandRunnerFactory,
		r.providerFromCommandRunnerFactory,
	)
	if err != nil {
		return err
	}
	opening.durableExecution, err = r.durableExecutionFactory(
		opening.configured.Definition,
		opening.configured.Session.Persistence,
		opening.sessionSelection.SystemConfigHome,
		opening.sessionSelection.SystemConfigPath,
		opening.configured.OperatorDefaults,
		opening.root,
		opening.clock,
		opening.providerForDurable,
		opening.configured.Workers.MockWorkers,
		r.factorySessionExecutionFactory,
		r.providerIdentities,
	)
	if closer, ok := opening.durableExecution.Service.(interface{ Close() error }); ok {
		cleanup.Add(func() error {
			if err := closer.Close(); err != nil {
				return fmt.Errorf("close durable Factory Session execution: %w", err)
			}
			return nil
		})
	}
	if err != nil {
		return err
	}
	setPersistenceWarningLogger(opening.durableExecution.Service, opening.logger)
	if r.factorySessionsRuntimeAssembly == nil {
		return fmt.Errorf("construct runtime scope: Factory Sessions runtime assembly is required")
	}
	currentRuntimeConfig := func() *models.RuntimeConfig {
		// The Models scope must snapshot the Factory Definition selected by this
		// opening. CurrentRuntime is process-global and can belong to another
		// concurrently opening Factory Session, which would bind this session's
		// host launcher to the other session's worker endpoint.
		return modelinvocation.ProjectModelsRuntimeConfig(opening.load.LoadedFactoryCfg)
	}
	opening.modelsBind, err = bindModelsRuntimeScope(
		ctx,
		r.modelService,
		opening.configured.ModelCacheDirectory,
		currentRuntimeConfig,
		opening.durableExecution.OperatorModels,
	)
	cleanup.OwnModelsScope(context.WithoutCancel(ctx), opening.modelsBind)
	if err != nil {
		return err
	}
	if r.workService == nil {
		return fmt.Errorf("construct runtime scope: Work service is required")
	}
	if r.workerService == nil {
		return fmt.Errorf("construct runtime scope: Workers service is required")
	}
	if r.automationService == nil {
		return fmt.Errorf("construct runtime scope: Automations service is required")
	}
	return nil
}

func (r *Root) restoreSessionOpeningHistory(ctx context.Context, opening *sessionRuntimeOpening) error {
	var err error
	if strings.TrimSpace(opening.configured.Recordings.ResumePath) != "" {
		input := opening.configured.Recordings.ResumeInput
		opening.resumeInput = &input
	}
	// A portable resume input owns its selected-history reconstruction. Only a
	// direct live opening should restore the current board recording; applying
	// that restart-only probe to an explicit resume artifact would reject valid
	// replay fixtures that intentionally have no current-board recording.
	canonicalSessionIDWasProvided := opening.providedCanonicalSessionID != "" && !opening.canonicalSessionIDGenerated
	if opening.load.ReplayArtifact == nil && opening.resumeInput == nil && !canonicalSessionIDWasProvided {
		if strings.TrimSpace(opening.configured.Recordings.RecordPath) != "" {
			opening.boardHistoryOpening, err = inspectCurrentBoardHistory(
				ctx,
				opening.durableExecution.Service,
				opening.sessionID,
			)
			if err != nil {
				return err
			}
		}
		var restoredBoard *currentBoardHistory
		restoredBoard, err = restoreCurrentBoardHistory(
			r.recordingsService,
			opening.configured.Recordings.RecordPath,
			opening.sessionID,
			opening.boardHistoryOpening.allowMissingHistory,
		)
		if err != nil {
			logCurrentBoardHistoryFailure(
				opening.logger,
				opening.sessionID,
				factoryruntime.RecordingPath(opening.configured.Recordings.RecordPath).ForSession(opening.sessionID),
				err,
			)
			return err
		}
		if restoredBoard != nil {
			opening.restoredWorldState = restoredBoard.state
			opening.restoredEventHistory = restoredBoard.events
		}
	}
	return nil
}

func (r *Root) openSessionEngine(ctx context.Context, opening *sessionRuntimeOpening, cleanup *runtimeOpeningCleanup) error {
	var err error
	mutationOwner, ok := opening.durableExecution.Service.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return fmt.Errorf(
			"compose runtime: durable execution owner does not record Petri mutations",
		)
	}
	if r.factoryRuntimeAssembler == nil {
		return fmt.Errorf("construct runtime scope: Factory Runtime assembler is required")
	}
	if err := r.restoreSessionOpeningHistory(ctx, opening); err != nil {
		return err
	}

	opening.runtimebuildService, opening.startupRuntime, opening.startupSpec, opening.runtimeLifecycle, opening.runtimeSidecars, err =
		r.factoryRuntimeAssembler.Assemble(
			ctx,
			opening.configured.OperatorDefaults.WorkerModelProvider,
			opening.configured.OperatorDefaults.WorkerModel,
			opening.configured.Recordings.ReplayPath == "",
			opening.configured.Recordings.RecordPath,
			opening.configured.Recordings.WorkflowID,
			opening.sessionID,
			opening.metricsSessionID,
			nil,
			r.loadFactory,
			r.providerOverride,
			r.providerCommandRunner,
			r.scriptCommandRunner,
			opening.configured.Workers.MockWorkers,
			opening.configured.Runtime.Mode,
			factoryruntime.Scheduler(nil),
			false,
			r.submissionRecorder,
			r.dispatchRecorder,
			opening.configured.Runtime.LogDirectory,
			opening.configured.Runtime.LogConfig,
			factoryruntime.RuntimeFileLoggingPolicy(opening.configured.Runtime.FileLoggingPolicy),
			factoryruntime.RuntimeMetricsPolicy(opening.configured.Runtime.MetricsPolicy),
			opening.configured.Runtime.MetricsDirectory,
			opening.configured.Runtime.MetricsConfig,
			opening.configured.Recordings.FlushInterval,
			opening.sessionSelection.BackendScopeID,
			opening.configured.Workers.RunnerID,
			opening.configured.Runtime.Verbose,
			opening.configured.Workers.SkipBuiltInPrerequisiteValidation,
			opening.configured.Workers.InvocationSkipPermissionsOverride,
			opening.clock,
			opening.logger,
			r.workersMockCommandRunnerFactory,
			fanOutWorkerProgress(
				r.factorySessionsRuntimeAssembly.InferenceProgressPublisherFactory(opening.logger),
				opening.durableExecution.Service,
			),
			r.factorySessionsRuntimeAssembly.DispatchCompletionObserverFactory(),
			mutationOwner.RecordPetriTokenMutations,
			opening.recordingProjections.ReconstructFactoryWorldState,
			r.recordingsRuntime,
			r.initialFactorySnapshotFactory,
			opening.configured.Definition.Directory,
			opening.root.FactoryRootDir,
			opening.configured.Definition.ExecutionBaseDir,
			opening.load.LoadedFactoryCfg,
			opening.configured.Runtime.RuntimeInstanceID,
			opening.load.ReplayArtifact,
			opening.resumeInput,
			opening.restoredWorldState,
			opening.restoredEventHistory,
			r.automationService,
			opening.configured.Runtime.Mode == factorydefinitions.RuntimeModeService,
		)
	cleanup.OwnRuntimeRecord(opening.startupRuntime, opening.clock)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opening.startupSpec.SessionID) != opening.sessionID {
		return fmt.Errorf(
			"construct runtime scope: built Factory Session ID %q does not match requested ID %q",
			opening.startupSpec.SessionID,
			opening.sessionID,
		)
	}
	opening.startupSpec.CanonicalSessionIDGenerated = opening.canonicalSessionIDGenerated &&
		opening.sessionID == factorysessions.DefaultSessionID &&
		opening.metricsSessionID != factorysessions.DefaultSessionID
	opening.warnMissingBoardHistory()
	return nil
}

func (opening *sessionRuntimeOpening) warnMissingBoardHistory() {
	if opening.boardHistoryOpening.hasDurableState && opening.restoredWorldState == nil {
		if runtimeLogger := opening.startupRuntime.RuntimeLogger(); runtimeLogger != nil {
			runtimeLogger.Warn(
				"current Factory Session board recording is absent after durable state was preserved; board contents were lost, an empty board was initialized, and preserved durable state was not deleted",
				zap.String("session_id", opening.sessionID),
				zap.String(
					"recording_path",
					factoryruntime.RecordingPath(opening.configured.Recordings.RecordPath).ForSession(opening.sessionID),
				),
				zap.String("recovery", "missing_board_recording_after_durable_state"),
			)
		}
	}
}

func (r *Root) completeSessionOpening(ctx context.Context, opening *sessionRuntimeOpening, cleanup *runtimeOpeningCleanup) (runtimeProducts, error) {
	webhookSubscription, err := startFactoryWebhookSubscription(
		ctx,
		r.webhooksService,
		opening.startupRuntime.RecordingLedger(),
		opening.load.LoadedFactoryCfg,
		opening.load.ReplayArtifact == nil,
		opening.sessionID,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if webhookSubscription != nil {
		cleanup.Add(func() error {
			return webhookSubscription(context.WithoutCancel(ctx))
		})
	}
	if r.factoryDefinitions == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions service is required")
	}
	if r.definitionRuntimeRouter == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions runtime router is required")
	}
	sessionRuntime, service4, invocationDomain, definitionHost, definitionActivationGateway, err := r.factorySessionsRuntimeAssembly.Complete(
		opening.root.FactoryRootDir,
		opening.clock,
		opening.logger,
		opening.startupRuntime.RuntimeLogger(),
		opening.runtimebuildService,
		opening.startupRuntime,
		opening.modelsBind.Scope,
		opening.startupSpec,
		opening.runtimeLifecycle,
		opening.runtimeSidecars,
		opening.durableExecution.Service,
		r.factoryDefinitions,
		opening.sessionID,
		opening.configured.Definition.Directory,
		opening.configured.Definition.ExecutionBaseDir,
		opening.configured.Runtime.Mode,
		opening.sessionSelection.BackendScopeID,
		opening.sessionSelection.WorkFile,
		opening.configured.Recordings.WorkflowID,
		nil,
		r.loadFactory,
		r.factoryScaffoldInitializer,
		r.editableFactoryValidator,
		func(
			recorded []factorydefinitions.FactoryEvent,
			cursor factorydefinitions.FactoryEventReconnectCursor,
			scope factorydefinitions.FactoryEventReconnectScope,
		) error {
			return opening.recordingProjections.ValidateReconnectReplay(recorded, cursor, scope)
		},
		opening.recordingProjections.ReconstructFactoryWorldState,
		r.invocationMetricsRecorder,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if bound := runtimebinding.SessionStateFrom(r.factorySessionsRuntimeAssembly.Resolve(opening.sessionID)); bound != nil {
		bound.SetMockWorkers(opening.configured.Workers.MockWorkers)
	}
	if binder, ok := opening.startupRuntime.(interface {
		BindModelsRuntimeScope(models.RuntimeScopeRef) error
	}); ok {
		if err := binder.BindModelsRuntimeScope(opening.modelsBind.Scope); err != nil {
			return runtimeProducts{}, fmt.Errorf("bind Models runtime scope to Factory Runtime: %w", err)
		}
	}
	if err := r.definitionRuntimeRouter.Bind(
		opening.sessionID,
		definitionHost,
		definitionActivationGateway,
	); err != nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: bind Factory Definitions runtime: %w", err)
	}
	cleanup.Add(func() error {
		r.definitionRuntimeRouter.Unbind(opening.sessionID)
		return nil
	})
	if r.processRuntimeFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions process runtime factory is required")
	}
	processRuntime, err := r.processRuntimeFactory.Bind(
		sessionRuntime,
		factorysessions.RuntimeHostRequest{
			Directory: opening.configured.Definition.Directory, RuntimeMode: opening.configured.Runtime.Mode,
			WorkFile: opening.sessionSelection.WorkFile, MockWorkers: opening.configured.Workers.MockWorkers != nil,
			Host: opening.sessionSelection.Host.Host, Port: opening.sessionSelection.Host.Port,
			AutoPort: opening.sessionSelection.Host.AutoPort, Pprof: opening.sessionSelection.Host.Pprof,
		},
		opening.startupRuntime.RuntimeLogger(),
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	return r.bindSessionOpeningProducts(ctx, opening, cleanup, sessionRuntime, service4, invocationDomain, processRuntime)
}

func (r *Root) bindSessionOpeningProducts(
	ctx context.Context,
	opening *sessionRuntimeOpening,
	cleanup *runtimeOpeningCleanup,
	sessionRuntime roles.ApplicationRuntime,
	service4 roles.SessionGateway,
	invocationDomain roles.SessionInvoker,
	processRuntime roles.ProcessRuntime,
) (runtimeProducts, error) {
	rootRuntime, ok := sessionRuntime.(factoryruntime.Service)
	if !ok {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: session runtime does not implement Factory Runtime root Service")
	}
	// A JavaScript workflow's children are detached Workers. The Workers root is
	// already composed before opening; Runtime contributes only the identity and
	// resource-admission capability that the child request needs. The existing
	// live-change Runtime bind remains separate and is not an execution route.
	var resourceLeaseAdmission factoryruntime.ResourceCapacityLeaseAdmission
	if admission, ok := rootRuntime.(factoryruntime.ResourceCapacityLeaseAdmission); ok {
		resourceLeaseAdmission = admission
	}
	if err := bindDurableExecutionCapabilities(
		opening.sessionID,
		opening.durableExecution.Service,
		r.workerService,
		rootRuntime,
		resourceLeaseAdmission,
		opening.configured.Runtime.RuntimeInstanceID,
		opening.startupRuntime.StreamGeneration(),
		opening.startupRuntime.RecordingLedger(),
		opening.providerForDurable,
		opening.configured.Workers.MockWorkers,
		r.providerCommandRunner,
		runtimeProgressPublisher(opening.startupRuntime),
		runtimeWorkerAttemptStarter(opening.startupRuntime),
	); err != nil {
		return runtimeProducts{}, err
	}
	opened := assembleRuntimeProducts(
		ctx,
		r.factoryDefinitions,
		service4,
		invocationDomain,
		rootRuntime,
		r.factoryWorkflows,
		r.workflowPreview,
		r.workService,
		r.workerService,
		opening.modelsBind,
		r.providerSessions,
		opening.startupRuntime,
		sessionRuntime,
		processRuntime,
		r.factorySessionsRuntimeAssembly,
		opening.recordingProjections,
		opening.configured.Definition.Directory,
		opening.configured.Runtime.RuntimeInstanceID,
		opening.sessionSelection.BackendScopeID,
		cleanup.Close,
		opening.sessionID,
	)
	opened.engine = opening.startupRuntime.RuntimeService()
	opened.clock = opening.clock
	opened.recordings = r.recordingsService
	opened.orderlyStop = newOrderlyRecordingFlush(
		r.recordingsService,
		opened.runtimeInstanceID,
		opening.configured.Recordings.RecordPath,
	)
	opened.operatorSettingsPath = opening.operatorSettingsPath
	opened.workerSettings = opening.durableExecution.WorkerSettings
	opened.replayMetadataWarnings = append(
		[]recordings.MetadataMismatchWarning(nil),
		opening.load.ReplayMetadataWarnings...,
	)
	opened.recordings = r.recordingsService
	return opened, nil
}

func startFactoryWebhookSubscription(
	ctx context.Context,
	webhooksService webhooks.Service,
	ledger recordings.Ledger,
	loaded factorydefinitions.MutableLoadedFactorySource,
	active bool,
	sessionID string,
) (webhooks.Subscription, error) {
	if !active || loaded == nil || !hasEnabledWebhooks(loaded.FactoryConfig()) {
		return nil, nil
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	return webhooksService.Start(ctx, webhooks.StartRequest{
		Definitions:      loaded.FactoryConfig().Webhooks,
		Scope:            scope,
		ActivationCursor: lastCanonicalCursor(ledger, scope),
		RuntimeSource:    loaded,
		DeadLetterPath:   factoryWebhookDeadLetterPath(loaded),
	})
}

func factoryWebhookDeadLetterPath(loaded factorydefinitions.LoadedFactorySource) string {
	if loaded == nil {
		return ""
	}
	baseDir := strings.TrimSpace(loaded.RuntimeBaseDir())
	if baseDir == "" {
		baseDir = strings.TrimSpace(loaded.FactoryDir())
	}
	if baseDir == "" {
		return ""
	}
	return filepath.Join(baseDir, filepath.FromSlash(webhooks.DeadLetterRelativePath))
}

func hasEnabledWebhooks(config *factorydefinitions.FactoryConfig) bool {
	if config == nil {
		return false
	}
	for _, webhook := range config.Webhooks {
		if webhook.Enabled {
			return true
		}
	}
	return false
}

func lastCanonicalCursor(
	ledger recordings.Ledger,
	scope recordings.CanonicalEventScope,
) *recordings.CanonicalEventCursor {
	if ledger == nil {
		return nil
	}
	events := ledger.CanonicalEvents()
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if scope.FactorySessionID != "" &&
			(event.Context.SessionID == nil || *event.Context.SessionID != scope.FactorySessionID) {
			continue
		}
		return &recordings.CanonicalEventCursor{
			StreamGenerationID: ledger.StreamGenerationID(),
			Sequence:           recordings.CanonicalEventSequence(event.Context.Sequence),
		}
	}
	return nil
}

// workerProgressObserver is the narrow capability a durable execution service
// exposes when the Workers its orchestrator starts produce output that session
// must record.
type workerProgressObserver interface {
	PublishWorkerProgress(workers.ProgressFragment)
}

// fanOutWorkerProgress adds the durable execution service to one runtime's
// Worker progress publication.
//
// A Worker's output reaches its runtime, which routes it to the live session's
// response stream. A JavaScript workflow child is a Worker of that runtime but
// belongs to a durable session, whose response-event store is its own; without
// this the child's output would reach the runtime and stop there, and the
// dashboard, the SSE feed, and the CLI's NDJSON contract would all show a
// session that produced nothing. The durable service ignores any dispatch it
// does not own, so a Petri Worker's progress still goes only where it went
// before.
func fanOutWorkerProgress(
	publishers func(string) workers.ProgressPublisher,
	execution any,
) func(string) workers.ProgressPublisher {
	observer, ok := execution.(workerProgressObserver)
	if !ok {
		return publishers
	}
	return func(sessionID string) workers.ProgressPublisher {
		var next workers.ProgressPublisher
		if publishers != nil {
			next = publishers(sessionID)
		}
		return func(fragment workers.ProgressFragment) {
			if next != nil {
				next(fragment)
			}
			observer.PublishWorkerProgress(fragment)
		}
	}
}

// workerInvokerBinder is the narrow capability a durable execution service
// exposes when its orchestrator runs Workers of its own.
type workerInvokerSetter interface {
	SetWorkerInvoker(factoryruntime.Service)
}

// setWorkerInvoker hands one session's opaque Factory Runtime capability to its
// execution service. An execution backend with no Workers of its own does not
// implement the setter, and skipping it is correct rather than a missing wire.
func setWorkerInvoker(execution any, runtime factoryruntime.Service) {
	setter, ok := execution.(workerInvokerSetter)
	if !ok || runtime == nil {
		return
	}
	setter.SetWorkerInvoker(runtime)
}

func setPersistenceWarningLogger(execution durableexecution.Service, logger *zap.Logger) {
	if execution == nil {
		return
	}
	setter, ok := execution.(interface {
		SetPersistenceWarningLogger(*zap.Logger)
	})
	if ok {
		setter.SetPersistenceWarningLogger(logger)
	}
}

// workerExecutionSetter is the narrow live-session child capability. The
// Workers service is already composed by process Wire; only its Execute method
// crosses into the child projection, while Runtime contributes the separate
// resource-lease admission and identity metadata.
type workerExecutionSetter interface {
	SetWorkerExecution(
		interface {
			Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
		},
		factoryruntime.ResourceCapacityLeaseAdmission,
		string,
		string,
		providers.Service,
		*workers.MockWorkersConfig,
		platformprocess.CommandRunner,
	)
}

func setWorkerExecution(
	sessionID string,
	execution any,
	workerService workers.Service,
	admission factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunnerOverride platformprocess.CommandRunner,
) error {
	setter, ok := execution.(workerExecutionSetter)
	if !ok {
		return fmt.Errorf(
			"bind Workers Execute for Factory Session %q: live child execution setter is required",
			strings.TrimSpace(sessionID),
		)
	}
	if missingPortDependency(workerService) {
		return fmt.Errorf(
			"bind Workers Execute for Factory Session %q: Workers service is required",
			strings.TrimSpace(sessionID),
		)
	}
	setter.SetWorkerExecution(workerService, admission, runtimeID, generationID, providerOverride, mockWorkers, commandRunnerOverride)
	return nil
}

type runtimeProgressPublisherProvider interface {
	RuntimeProgressPublisher() workers.ProgressPublisher
}

func runtimeProgressPublisher(runtime runtimeports.RuntimeInstance) workers.ProgressPublisher {
	if runtime == nil {
		return nil
	}
	if provider, ok := runtime.(runtimeProgressPublisherProvider); ok {
		return provider.RuntimeProgressPublisher()
	}
	if service := runtime.RuntimeService(); service != nil {
		if provider, ok := service.(runtimeProgressPublisherProvider); ok {
			return provider.RuntimeProgressPublisher()
		}
	}
	return nil
}

func setWorkerProgressPublisher(execution any, publisher workers.ProgressPublisher) {
	if publisher == nil {
		return
	}
	setter, ok := execution.(interface {
		SetWorkerProgressPublisher(workers.ProgressPublisher)
	})
	if !ok {
		return
	}
	setter.SetWorkerProgressPublisher(publisher)
}

type runtimeWorkerAttemptStarterProvider interface {
	BeginWorkerAttempt(
		context.Context,
		workers.ExecuteRequest,
	) (func(context.Context, workers.ExecuteResult, error) error, error)
}

func runtimeWorkerAttemptStarter(
	runtime runtimeports.RuntimeInstance,
) func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error) {
	if runtime == nil {
		return nil
	}
	if provider, ok := runtime.(runtimeWorkerAttemptStarterProvider); ok {
		return provider.BeginWorkerAttempt
	}
	if service := runtime.RuntimeService(); service != nil {
		if provider, ok := service.(runtimeWorkerAttemptStarterProvider); ok {
			return provider.BeginWorkerAttempt
		}
	}
	return nil
}

func setWorkerAttemptStarter(
	execution any,
	starter func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
) {
	if starter == nil {
		return
	}
	setter, ok := execution.(interface {
		SetWorkerAttemptStarter(
			func(context.Context, workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) error, error),
		)
	})
	if !ok {
		return
	}
	setter.SetWorkerAttemptStarter(starter)
}

type historicalRecordingReader interface {
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
}
