package runtimeopening

import (
	"context"
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
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
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
	providerOverride workers.Provider,
	invocationMetricsRecorder roles.InvocationMetricsRecorder,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	durableExecutionFactory DurableExecutionFactory,
	workerExecutionFactory WorkerExecutionFactory,
	modelService models.Service,
	automationService automations.Service,
	factorySessionsRuntimeAssembly roles.RuntimeAssembly,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	recordingsRoot recordings.Root,
	workersRuntimeFactory WorkersRuntimeFactory,
	workersRuntimeExecutorsFactory factoryruntime.WorkersRuntimeExecutorsFactory,
	providerInvocationFactory factoryruntime.ProviderInvocationExecutorFactory,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	workersLocalRuntimeHooksFactory WorkersLocalRuntimeHooksFactory,
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
	adaptWorkerCommandRunner WorkerCommandRunnerAdapter,
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
	if recordingsRoot == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings root is required")
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
		recordingsRoot,
		recordingsRoot.ReplayClock,
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
	if load.HistoricalReplay != nil {
		return historicalReplayRuntimeProducts(logger, *load.HistoricalReplay), nil
	}
	if clock == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Runtime clock is required")
	}
	recordingProjections := recordingsRoot.Projection()
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
	selectedModels := modelsBind.Root
	if workService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Work service is required")
	}
	if workerExecutionFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: worker execution operation is required")
	}
	var initialProgressPublisher workers.ProgressPublisher
	if inferenceProgressPublisherFactory := runtimeService.InferenceProgressPublisherFactory(logger); inferenceProgressPublisherFactory != nil {
		initialProgressPublisher = inferenceProgressPublisherFactory(sessionID)
	}
	sessionBuildRuntimes := &sessionBuildRuntimeSink{}
	cleanup.Add(func() error {
		return sessionBuildRuntimes.Close(context.WithoutCancel(ctx))
	})
	serviceService, sessionBuildFactory, err := workerExecutionFactory(
		configured.Runtime,
		configured.Workers,
		clock,
		logger,
		providerCommandRunner,
		scriptCommandRunner,
		initialProgressPublisher,
		nil,
		providerOverride,
		runtimeService,
		selectedModels, modelsBind.Scope, workService,
		workersRuntimeFactory,
		durableExecution.ACPIntegrations,
		sessionBuildRuntimes.Add,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	cleanup.Add(func() error {
		return serviceService.Close(context.WithoutCancel(ctx))
	})
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
			nil,
			nil,
			nil,
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
			serviceService,
			sessionBuildFactory,
			providerInvocationFactory,
			workersRuntimeExecutorsFactory,
			workersMockCommandRunnerFactory,
			fanOutWorkerProgress(
				runtimeService.InferenceProgressPublisherFactory(logger),
				durableExecution.Service,
			),
			runtimeService.DispatchCompletionObserverFactory(),
			mutationOwner.RecordPetriTokenMutations,
			recordingProjections.ReconstructFactoryWorldState,
			recordingsRoot,
			initialFactorySnapshotFactory,
			configured.Definition.Directory,
			root.FactoryRootDir,
			configured.Definition.ExecutionBaseDir,
			load.LoadedFactoryCfg,
			configured.Runtime.RuntimeInstanceID,
			load.ReplayArtifact,
			service2,
			configured.Runtime.Mode == factorydefinitions.RuntimeModeService,
		)
	if err != nil {
		return runtimeProducts{}, err
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
		recordingsRoot,
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
	// A JavaScript workflow's children are Workers, and Workers are supervised
	// by the runtime that owns this session's Worker Sessions service and
	// canonical ledger. That runtime only exists here, after the execution
	// service it must be handed to was already constructed. The eventual root
	// activation operation must preserve this dependency ordering without
	// publishing a second post-construction binding path.
	bindWorkerInvoker(durableExecution.Service, rootRuntime)
	opened := assembleRuntimeProducts(
		factoryDefinitions,
		service4,
		invocationDomain,
		rootRuntime,
		factoryWorkflows,
		workflowPreview,
		workService,
		serviceService,
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
	)
	opened.startup = startupRuntime
	opened.replacement = runtimebuildService
	opened.buildSpec = startupSpec
	opened.lifecycle = runtimeLifecycle
	opened.sidecars = runtimeSidecars
	opened.application.Resources.Clock = clock
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
type workerInvokerBinder interface {
	BindWorkerInvoker(func(sessionID string) factoryruntime.Service)
}

// bindWorkerInvoker hands one session's Factory Runtime to its execution
// service. An execution backend with no Workers of its own does not implement
// the binder, and skipping it is correct rather than a missing wire.
func bindWorkerInvoker(execution any, runtime factoryruntime.Service) {
	binder, ok := execution.(workerInvokerBinder)
	if !ok || runtime == nil {
		return
	}
	binder.BindWorkerInvoker(func(string) factoryruntime.Service { return runtime })
}
