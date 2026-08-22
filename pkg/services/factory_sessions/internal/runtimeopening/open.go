package runtimeopening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// openRuntime constructs one Factory Session and its domain-owned runtime state from
// collaborators selected by the canonical process injector.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func openRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
	baseLogger *zap.Logger,
	clockEdge factoryruntime.Clock,
	providerOverride providers.Service,
	invocationMetricsRecorder roles.InvocationMetricsRecorder,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	durableExecutionFactory DurableExecutionFactory,
	workerService workers.Service,
	modelService models.Service,
	automationService automations.Service,
	factorySessionsRuntimeAssembly roles.RuntimeAssembly,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	recordingsService recordings.Service,
	recordingsRuntime recordings.RuntimeOpening,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	factoryDefinitions factorydefinitions.Service,
	definitionRuntimeRouter *factorysessions.DefinitionRuntimeRouter,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	workService work.Service,
	providerSessions providersessions.Service,
	factoryDefinitionValidator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	webhooksService webhooks.Service,
	resolveClock factoryruntime.ClockResolver,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	providerFromCommandRunnerFactory ProviderFromCommandRunnerFactory,
	processRuntimeFactory roles.ProcessRuntimeFactory,
	ensureOperatorBackendScope operatorsettings.BackendScopeEnsurer,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	providerIdentities factorysessions.ProviderIdentityResolver,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (products runtimeProducts, err error) {
	if request == nil {
		return runtimeProducts{}, fmt.Errorf("runtime opening request is required")
	}
	if recordingsService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings service is required")
	}
	if recordingsRuntime == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings runtime opening is required")
	}
	definitionRequest := request.FactoryDefinition
	runtimeRequest := request.FactoryRuntime
	sessionRequest := request.FactorySession
	sessionID := strings.TrimSpace(sessionRequest.FactorySessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	sessionRequest.FactorySessionID = sessionID
	workerRequest := request.Workers
	recordingRequest := request.Recordings
	modelCacheDirectory := request.ModelCacheDirectory
	operatorDefaults := request.OperatorDefaults
	configured, root, load, clock, logger, err := PrepareRuntime(
		ctx,
		definitionRequest,
		runtimeRequest,
		sessionRequest,
		workerRequest,
		recordingRequest,
		modelCacheDirectory,
		operatorDefaults,
		baseLogger,
		clockEdge,
		factoryDefinitionValidator,
		namedPaths,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		recordingsRuntime,
		recordingsRuntime.ReplayClock,
		factoryScaffoldInitializer,
		editableFactoryValidator,
		captureLoadedFactorySnapshot,
		resolveClock,
		newSessionLogger,
		ensureOperatorBackendScope,
		generateRuntimeInstanceID,
		resolveHome,
		providerIdentities,
		definitionSnapshot,
		replayInput,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if effort := strings.TrimSpace(configured.Workers.WorkerReasoningEffort); effort != "" &&
		load.LoadedFactoryCfg != nil {
		if err := load.LoadedFactoryCfg.MutateWorkers(func(worker *factorydefinitions.FactoryWorkerConfig) error {
			if worker != nil {
				worker.ReasoningEffort = effort
			}
			return nil
		}); err != nil {
			return runtimeProducts{}, fmt.Errorf("apply worker reasoning effort override: %w", err)
		}
	}
	if load.HistoricalReplay != nil {
		var liveOwner durableexecution.Service
		var replayClose func() error
		if load.HistoricalReplay.Checkpoint != nil {
			liveOwner, replayClose, err = openPortableReplayDurableOwner(
				configured,
				root,
				logger,
				clockEdge,
				providerOverride,
				providerCommandRunner,
				scriptCommandRunner,
				workerService,
				workersMockCommandRunnerFactory,
				providerFromCommandRunnerFactory,
				durableExecutionFactory,
				factorySessionExecutionFactory,
				providerIdentities,
				resolveClock,
				factoryRuntimeAssembler,
				recordingsRuntime,
				initialFactorySnapshotFactory,
				loadFactory,
				automationService,
				submissionRecorder,
				dispatchRecorder,
			)
			if err != nil {
				return runtimeProducts{}, err
			}
		}
		return historicalReplayRuntimeProducts(
			logger,
			*load.HistoricalReplay,
			liveOwner,
			replayClose,
		), nil
	}
	operatorSettingsPath, err := operatorConfigPath(configured.Session)
	if err != nil {
		return runtimeProducts{}, fmt.Errorf("resolve operator settings path for runtime transport: %w", err)
	}
	if clock == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Runtime clock is required")
	}
	recordingProjections := recordingsRuntime.Projection()
	if recordingProjections == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings projection is unavailable")
	}
	if durableExecutionFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: durable execution operation is required")
	}
	providerForDurable, err := resolveDurableExecutionProvider(
		providerOverride,
		configured.Workers.MockWorkers,
		load.LoadedFactoryCfg,
		providerCommandRunner,
		workersMockCommandRunnerFactory,
		providerFromCommandRunnerFactory,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	durableExecution, err := durableExecutionFactory(
		configured.Definition,
		configured.Session,
		configured.OperatorDefaults,
		root,
		clock,
		providerForDurable,
		configured.Workers.MockWorkers,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	factorysessionexecutionService := durableExecution.Service
	if factorySessionsRuntimeAssembly == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions runtime assembly is required")
	}
	runtimeService := factorySessionsRuntimeAssembly
	currentRuntimeConfig := func() *models.RuntimeConfig {
		runtime := runtimeService.CurrentRuntime()
		if runtime != nil {
			return ProjectModelsRuntimeConfig(runtime.RuntimeConfig)
		}
		return ProjectModelsRuntimeConfig(load.LoadedFactoryCfg)
	}
	modelsBind, err := bindModelsRuntimeScope(
		ctx,
		modelService,
		configured.ModelCacheDirectory,
		currentRuntimeConfig,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	cleanup := &runtimeOpeningCleanup{}
	cleanup.OwnModelsScope(context.WithoutCancel(ctx), modelsBind)
	defer func() {
		if err != nil {
			err = cleanup.Unwind(err)
		}
	}()
	if workService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Work service is required")
	}
	if workerService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Workers service is required")
	}
	if automationService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Automations service is required")
	}
	service2 := automationService
	mutationOwner, ok := factorysessionexecutionService.(interface {
		RecordPetriTokenMutations(string, []factorydefinitions.TokenMutationRecord) error
	})
	if !ok {
		return runtimeProducts{}, fmt.Errorf(
			"compose runtime: durable execution owner does not record Petri mutations",
		)
	}
	if factoryRuntimeAssembler == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Runtime assembler is required")
	}
	var resumeInput *recordings.LoadResumeInputResult
	if strings.TrimSpace(configured.Recordings.ResumePath) != "" {
		input := configured.Recordings.ResumeInput
		resumeInput = &input
	}
	var restoredWorldState *factorydefinitions.FactoryWorldState
	var boardHistoryOpening currentBoardHistoryOpening
	// A portable resume input owns its selected-history reconstruction. Only a
	// direct live opening should restore the current board recording; applying
	// that restart-only probe to an explicit resume artifact would reject valid
	// replay fixtures that intentionally have no current-board recording.
	if load.ReplayArtifact == nil && resumeInput == nil {
		if strings.TrimSpace(configured.Recordings.RecordPath) != "" {
			boardHistoryOpening, err = inspectCurrentBoardHistory(
				ctx,
				durableExecution.Service,
				sessionID,
			)
			if err != nil {
				return runtimeProducts{}, err
			}
		}
		restoredWorldState, err = restoreCurrentBoardState(
			recordingsService,
			configured.Recordings.RecordPath,
			sessionID,
			boardHistoryOpening.allowMissingHistory,
		)
		if err != nil {
			logCurrentBoardHistoryFailure(
				logger,
				sessionID,
				factoryruntime.SessionScopedRecordPath(configured.Recordings.RecordPath, sessionID),
				err,
			)
			return runtimeProducts{}, err
		}
	}
	runtimebuildService, startupRuntime, startupSpec, runtimeLifecycle, runtimeSidecars, err :=
		factoryRuntimeAssembler.Assemble(
			ctx,
			configured.OperatorDefaults.WorkerModelProvider,
			configured.OperatorDefaults.WorkerModel,
			configured.Recordings.ReplayPath == "",
			configured.Recordings.RecordPath,
			configured.Recordings.WorkflowID,
			sessionID,
			nil,
			loadFactory,
			providerOverride,
			providerCommandRunner,
			scriptCommandRunner,
			configured.Workers.MockWorkers,
			configured.Runtime.Mode,
			factoryruntime.Scheduler(nil),
			false,
			submissionRecorder,
			dispatchRecorder,
			configured.Runtime.LogDirectory,
			configured.Runtime.LogConfig,
			factoryruntime.RuntimeFileLoggingPolicy(configured.Runtime.FileLoggingPolicy),
			factoryruntime.RuntimeMetricsPolicy(configured.Runtime.MetricsPolicy),
			configured.Runtime.MetricsDirectory,
			configured.Runtime.MetricsConfig,
			configured.Recordings.FlushInterval,
			configured.Session.BackendScopeID,
			configured.Workers.RunnerID,
			configured.Runtime.Verbose,
			configured.Workers.SkipBuiltInPrerequisiteValidation,
			configured.Workers.InvocationSkipPermissionsOverride,
			clock,
			logger,
			workersMockCommandRunnerFactory,
			fanOutWorkerProgress(
				runtimeService.InferenceProgressPublisherFactory(logger),
				durableExecution.Service,
			),
			runtimeService.DispatchCompletionObserverFactory(),
			mutationOwner.RecordPetriTokenMutations,
			recordingProjections.ReconstructFactoryWorldState,
			recordingsRuntime,
			initialFactorySnapshotFactory,
			configured.Definition.Directory,
			root.FactoryRootDir,
			configured.Definition.ExecutionBaseDir,
			load.LoadedFactoryCfg,
			configured.Runtime.RuntimeInstanceID,
			load.ReplayArtifact,
			resumeInput,
			restoredWorldState,
			service2,
			configured.Runtime.Mode == factorydefinitions.RuntimeModeService,
		)
	if err != nil {
		return runtimeProducts{}, err
	}
	if boardHistoryOpening.hasDurableState && restoredWorldState == nil {
		if runtimeLogger := startupRuntime.RuntimeLogger(); runtimeLogger != nil {
			runtimeLogger.Warn(
				"current Factory Session board recording is absent after durable state was preserved; board contents were lost, an empty board was initialized, and preserved durable state was not deleted",
				zap.String("session_id", sessionID),
				zap.String(
					"recording_path",
					factoryruntime.SessionScopedRecordPath(configured.Recordings.RecordPath, sessionID),
				),
				zap.String("recovery", "missing_board_recording_after_durable_state"),
			)
		}
	}
	cleanup.Add(func() error {
		var finalizationErr error
		if finalizer, ok := startupRuntime.(interface {
			FinalizeRecording(time.Time) error
		}); ok {
			finalizationErr = finalizer.FinalizeRecording(clock.Now().UTC())
		}
		return errors.Join(finalizationErr, startupRuntime.CloseArtifacts())
	})
	webhookSubscription, err := startFactoryWebhookSubscription(
		ctx,
		webhooksService,
		recordingsService,
		startupRuntime.RecordingLedger(),
		load.LoadedFactoryCfg,
		load.ReplayArtifact == nil,
		sessionID,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if webhookSubscription != nil {
		cleanup.Add(func() error {
			return webhookSubscription(context.WithoutCancel(ctx))
		})
	}
	if factoryDefinitions == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions service is required")
	}
	if definitionRuntimeRouter == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions runtime router is required")
	}
	sessionRuntime, service4, invocationDomain, definitionHost, definitionActivationGateway, err := runtimeService.Complete(
		root.FactoryRootDir,
		clock,
		logger,
		startupRuntime.RuntimeLogger(),
		runtimebuildService,
		startupRuntime,
		modelsBind.Scope,
		startupSpec,
		runtimeLifecycle,
		runtimeSidecars,
		factorysessionexecutionService,
		factoryDefinitions,
		sessionID,
		configured.Definition.Directory,
		configured.Definition.ExecutionBaseDir,
		configured.Runtime.Mode,
		configured.Session.BackendScopeID,
		configured.Session.WorkFile,
		configured.Recordings.WorkflowID,
		nil,
		loadFactory,
		factoryScaffoldInitializer,
		editableFactoryValidator,
		func(
			recorded []factorydefinitions.FactoryEvent,
			cursor factorydefinitions.FactoryEventReconnectCursor,
			scope factorydefinitions.FactoryEventReconnectScope,
		) error {
			return recordingProjections.ValidateReconnectReplay(recorded, cursor, scope)
		},
		recordingProjections.ReconstructFactoryWorldState,
		invocationMetricsRecorder,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if binder, ok := startupRuntime.(interface {
		BindModelsRuntimeScope(models.RuntimeScopeRef) error
	}); ok {
		if err := binder.BindModelsRuntimeScope(modelsBind.Scope); err != nil {
			return runtimeProducts{}, fmt.Errorf("bind Models runtime scope to Factory Runtime: %w", err)
		}
	}
	if err := definitionRuntimeRouter.Bind(
		sessionID,
		definitionHost,
		definitionActivationGateway,
	); err != nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: bind Factory Definitions runtime: %w", err)
	}
	cleanup.Add(func() error {
		definitionRuntimeRouter.Unbind(sessionID)
		return nil
	})
	if processRuntimeFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions process runtime factory is required")
	}
	processRuntime, err := processRuntimeFactory.Bind(
		sessionRuntime,
		factorysessions.RuntimeHostRequest{
			Directory: configured.Definition.Directory, RuntimeMode: configured.Runtime.Mode,
			WorkFile: configured.Session.WorkFile, MockWorkers: configured.Workers.MockWorkers != nil,
			Host: configured.Session.Host.Host, Port: configured.Session.Host.Port,
			AutoPort: configured.Session.Host.AutoPort,
		},
		nil,
		startupRuntime.RuntimeLogger(),
	)
	if err != nil {
		return runtimeProducts{}, err
	}
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
		sessionID,
		durableExecution.Service,
		workerService,
		rootRuntime,
		resourceLeaseAdmission,
		configured.Runtime.RuntimeInstanceID,
		startupRuntime.StreamGeneration(),
		providerForDurable,
		configured.Workers.MockWorkers,
		providerCommandRunner,
		runtimeProgressPublisher(startupRuntime),
		runtimeWorkerAttemptStarter(startupRuntime),
	); err != nil {
		return runtimeProducts{}, err
	}
	opened := assembleRuntimeProducts(
		ctx,
		factoryDefinitions,
		service4,
		invocationDomain,
		rootRuntime,
		factoryWorkflows,
		workflowPreview,
		workService,
		workerService,
		modelsBind,
		providerSessions,
		startupRuntime,
		sessionRuntime,
		processRuntime,
		runtimeService,
		recordingProjections,
		configured.Definition.Directory,
		configured.Runtime.RuntimeInstanceID,
		configured.Session.BackendScopeID,
		cleanup.Close,
		sessionID,
	)
	opened.engine = startupRuntime.RuntimeService()
	opened.application.Resources.Clock = clock
	opened.application.Recordings = recordingsService
	opened.application.OperatorSettingsPath = operatorSettingsPath
	opened.execution.Recordings = recordingsService
	return opened, nil
}

func startFactoryWebhookSubscription(
	ctx context.Context,
	webhooksService webhooks.Service,
	recordingsService recordings.Service,
	ledger recordings.Ledger,
	loaded factorydefinitions.MutableLoadedFactorySource,
	active bool,
	sessionID string,
) (webhooks.Subscription, error) {
	if !active || loaded == nil || !hasEnabledWebhooks(loaded.FactoryConfig()) {
		return nil, nil
	}
	if webhooksService == nil {
		return nil, fmt.Errorf("construct runtime scope: Webhooks service is required")
	}
	if recordingsService == nil {
		return nil, fmt.Errorf("construct runtime scope: Recordings service is required for Webhooks")
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	return webhooksService.Start(ctx, webhooks.StartRequest{
		Definitions:      loaded.FactoryConfig().Webhooks,
		Events:           recordingsService,
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
		workers.CommandRunner,
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
	commandRunnerOverride workers.CommandRunner,
) error {
	setter, ok := execution.(workerExecutionSetter)
	if !ok {
		return fmt.Errorf(
			"bind Workers Execute for Factory Session %q: live child execution setter is required",
			strings.TrimSpace(sessionID),
		)
	}
	if missingRuntimeOpeningDependency(workerService) {
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

type durableSessionStateReader interface {
	HasDurableState(context.Context, string) (bool, error)
}

type currentBoardHistoryOpening struct {
	allowMissingHistory bool
	hasDurableState     bool
}

const (
	currentBoardRecordingFailureCode    = "CURRENT_BOARD_RECORDING_FAILURE"
	currentBoardRecordingMissingCode    = "CURRENT_BOARD_RECORDING_MISSING"
	currentBoardRecordingCorruptCode    = "CURRENT_BOARD_RECORDING_CORRUPT"
	currentBoardRecordingUnreadableCode = "CURRENT_BOARD_RECORDING_UNREADABLE"
)

// currentBoardHistoryRestoreError is also a safe CLI error. The method names
// intentionally match the transport's narrow coded-error interface without
// making runtime opening depend on the CLI package. Its Error method never
// includes the underlying cause, which keeps startup diagnostics safe while
// Unwrap retains typed failure matching for service callers.
type currentBoardHistoryRestoreError struct {
	code    string
	message string
	cause   error
}

func (err *currentBoardHistoryRestoreError) Error() string {
	if err == nil {
		return ""
	}
	return err.message
}

func (err *currentBoardHistoryRestoreError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func (err *currentBoardHistoryRestoreError) CLIErrorCode() string {
	if err == nil {
		return ""
	}
	return err.code
}

func (err *currentBoardHistoryRestoreError) CLIErrorMessage() string {
	if err == nil {
		return ""
	}
	return err.message
}

func logCurrentBoardHistoryFailure(
	logger *zap.Logger,
	sessionID string,
	recordPath string,
	err error,
) {
	if logger == nil {
		return
	}
	kind := "UNREADABLE_OR_CORRUPT_RECORDING"
	var queryErr *recordings.HistoricalRecordingQueryError
	if errors.As(err, &queryErr) && queryErr.Kind != "" {
		kind = string(queryErr.Kind)
	}
	logger.Error(
		"current Factory Session board recording could not be restored; startup aborted",
		zap.String("session_id", sessionID),
		zap.String("recording_path", recordPath),
		zap.String("failure", kind),
	)
}

// inspectCurrentBoardHistory makes the missing-history escape hatch explicit.
// A successful persistence probe is required before a missing board recording
// can be treated as either a fresh opening or an interrupted write. The latter
// is observable through hasDurableState so the caller can warn that the board
// was lost while preserving the durable snapshot.
func inspectCurrentBoardHistory(
	ctx context.Context,
	service any,
	sessionID string,
) (currentBoardHistoryOpening, error) {
	if service == nil {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history: durable session state probe is unavailable")
	}
	probe, ok := service.(durableSessionStateReader)
	if !ok {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history: durable session state probe is unavailable")
	}
	hasDurableState, err := probe.HasDurableState(ctx, sessionID)
	if err != nil {
		return currentBoardHistoryOpening{}, fmt.Errorf("inspect current Factory Session board history initialization: %w", err)
	}
	return currentBoardHistoryOpening{
		allowMissingHistory: true,
		hasDurableState:     hasDurableState,
	}, nil
}

// restoreCurrentBoardState loads a detached Factory world state through the
// public Recordings history contract for callers that explicitly request a
// current-board read.
func restoreCurrentBoardState(
	service historicalRecordingReader,
	recordPath string,
	sessionID string,
	allowMissingHistory bool,
) (*factorydefinitions.FactoryWorldState, error) {
	recordPath = strings.TrimSpace(recordPath)
	if recordPath == "" {
		return nil, nil
	}
	recordPath = factoryruntime.SessionScopedRecordPath(recordPath, sessionID)
	if service == nil {
		return nil, fmt.Errorf("restore current Factory Session board: Recordings history is unavailable")
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: strings.TrimSpace(sessionID)}
	result, err := service.QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest{
		Recording: recordings.HistoricalRecordingIdentity{
			RecordingID: recordings.RecordingID("current-board/" + scope.FactorySessionID),
			Artifact:    recordings.RecordingArtifactReference(recordPath),
			Scope:       scope,
		},
	})
	if err != nil {
		var queryErr *recordings.HistoricalRecordingQueryError
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorMissingHistory {
			if allowMissingHistory {
				return nil, nil
			}
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"MISSING_HISTORY: durable state exists but the current-board recording is missing; preserve the durable snapshot and restore the recording from a trusted backup without deleting the snapshot",
				err,
			)
		}
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorCorruptHistory {
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"CORRUPT_HISTORY: the current-board recording is corrupt or incompatible; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
				err,
			)
		}
		if errors.As(err, &queryErr) && queryErr.Kind == recordings.HistoricalRecordingQueryErrorUnavailable {
			return nil, currentBoardHistoryFailure(
				recordPath,
				sessionID,
				"UNREADABLE_RECORDING: the current-board recording is present but unreadable; preserve the artifact for investigation and repair access or replace it from a trusted backup before retrying",
				err,
			)
		}
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"UNREADABLE_OR_CORRUPT_RECORDING: the current-board recording is unreadable or corrupt; preserve the artifact for investigation and repair it or replace it from a trusted backup before retrying",
			err,
		)
	}
	view := result.WorldState
	if view.SchemaVersion != recordings.WorldStateViewSchemaV1 || strings.TrimSpace(view.Payload) == "" {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"CORRUPT_HISTORY: Recordings returned an incompatible or empty world-state view; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
			nil,
		)
	}
	if view.Scope != scope {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			fmt.Sprintf(
				"CORRUPT_HISTORY: world-state scope %#v does not match %#v; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
				view.Scope,
				scope,
			),
			nil,
		)
	}
	var state factorydefinitions.FactoryWorldState
	if err := json.Unmarshal([]byte(view.Payload), &state); err != nil {
		return nil, currentBoardHistoryFailure(
			recordPath,
			sessionID,
			"CORRUPT_HISTORY: decode world state failed; preserve the artifact for investigation and repair or replace it from a trusted backup before retrying",
			err,
		)
	}
	return &state, nil
}

func currentBoardHistoryFailure(
	recordPath string,
	sessionID string,
	diagnostic string,
	cause error,
) error {
	message := fmt.Sprintf(
		"restore current Factory Session board from %q (session %q): %s",
		recordPath,
		sessionID,
		diagnostic,
	)
	code := currentBoardHistoryFailureCode(diagnostic)
	return &currentBoardHistoryRestoreError{
		code:    code,
		message: message,
		cause:   cause,
	}
}

func currentBoardHistoryFailureCode(diagnostic string) string {
	colon := strings.IndexByte(diagnostic, ':')
	if colon < 0 {
		return currentBoardRecordingFailureCode
	}
	switch strings.TrimSpace(diagnostic[:colon]) {
	case "MISSING_HISTORY":
		return currentBoardRecordingMissingCode
	case "CORRUPT_HISTORY":
		return currentBoardRecordingCorruptCode
	case "UNREADABLE_RECORDING":
		return currentBoardRecordingUnreadableCode
	default:
		return currentBoardRecordingFailureCode
	}
}
