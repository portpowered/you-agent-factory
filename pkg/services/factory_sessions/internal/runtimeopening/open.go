package runtimeopening

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
	edges ExternalEffects,
	baseLogger *zap.Logger,
	durableExecutionFactory DurableExecutionFactory,
	workerExecutionFactory WorkerExecutionFactory,
	modelService models.Service,
	workFactory WorkFactory,
	automationFactory AutomationFactory,
	factorySessionsService factorysessions.Service,
	factorySessionExecutionFactory FactorySessionExecutionFactory,
	recordingsProjectionFactory RecordingsProjectionFactory,
	recordingsFactory RecordingsFactory,
	runtimeLedgerFactory RuntimeLedgerFactory,
	runtimeRecorderFactory recordings.RuntimeRecorderFactory,
	replayClockFactory ReplayClockFactory,
	replayExecutionFactory recordings.ReplayExecutionFactory,
	workersRuntimeFactory WorkersRuntimeFactory,
	workersRuntimeExecutorsFactory factoryruntime.WorkersRuntimeExecutorsFactory,
	workersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory,
	workerHostedPollersFactory WorkerHostedPollersFactory,
	workersLocalRuntimeHooksFactory WorkersLocalRuntimeHooksFactory,
	factoryDefinitionsFactory FactoryDefinitionsFactory,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	initialFactorySnapshotFactory factorydefinitions.InitialFactorySnapshotFactory,
	factoryRuntimeAssembler FactoryRuntimeAssembler,
	contentMaterializer work.ContentMaterializer,
	providerSessions providersessions.Service,
	factoryDefinitionValidator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	loadReplay recordings.ReplayArtifactLoader,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	resolveClock factoryruntime.ClockResolver,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	adaptWorkerCommandRunner WorkerCommandRunnerAdapter,
	processRuntimeFactory roles.ProcessRuntimeFactory,
	ensureOperatorBackendScope operatorsettings.BackendScopeEnsurer,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	replayFiles fileeffects.ReplayRecordingReader,
	providerIdentities factorysessions.ProviderIdentityResolver,
) (runtimeProducts, error) {
	if request == nil {
		return runtimeProducts{}, fmt.Errorf("runtime opening request is required")
	}
	definitionRequest := request.FactoryDefinition
	runtimeRequest := request.FactoryRuntime
	sessionRequest := request.FactorySession
	workerRequest := request.Workers
	recordingRequest := request.Recordings
	modelRequest := request.Models
	operatorDefaults := request.OperatorDefaults
	configured, root, load, clock, logger, hostedPollers, err := PrepareRuntime(
		ctx,
		definitionRequest,
		runtimeRequest,
		sessionRequest,
		workerRequest,
		recordingRequest,
		modelRequest,
		operatorDefaults,
		baseLogger,
		edges,
		factoryDefinitionValidator,
		namedPaths,
		loadFactory,
		newLoadedFactory,
		decodeReplayConfig,
		loadReplay,
		replayClockFactory,
		workerHostedPollersFactory,
		factoryScaffoldInitializer,
		editableFactoryValidator,
		captureLoadedFactorySnapshot,
		resolveClock,
		newSessionLogger,
		ensureOperatorBackendScope,
		generateRuntimeInstanceID,
		resolveHome,
		replayFiles,
		providerIdentities,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if clock == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Runtime clock is required")
	}
	if recordingsProjectionFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings projection factory is required")
	}
	recordingProjections := recordingsProjectionFactory()
	if recordingProjections == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings projection factory returned nil service")
	}
	if runtimeLedgerFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings runtime ledger factory is required")
	}
	newRuntimeLedger := runtimeLedgerFactory()
	if newRuntimeLedger == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings runtime ledger factory returned nil")
	}
	if runtimeRecorderFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings runtime recorder factory is required")
	}
	if durableExecutionFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: durable execution operation is required")
	}
	factorysessionexecutionService, err := durableExecutionFactory(
		configured.Definition,
		configured.Session,
		root,
		clock,
		edges.ProviderOverride,
		factorySessionExecutionFactory,
		providerIdentities,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if factorySessionsService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions service is required")
	}
	boundService, err := factorySessionsService.ForRuntime(factorysessions.RuntimeBinding{Clock: clock})
	if err != nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions service: %w", err)
	}
	if boundService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions service returned nil runtime view")
	}
	runtimeService, ok := boundService.(roles.RuntimeAssembly)
	if !ok {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions runtime view does not expose its private assembly")
	}
	currentRuntimeConfig := func() *models.RuntimeConfig {
		runtime := runtimeService.CurrentRuntime()
		if runtime == nil {
			return nil
		}
		return ProjectModelsRuntimeConfig(runtime.RuntimeConfig)
	}
	if modelService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Models service is required")
	}
	modelDomain, err := modelService.ForRuntime(models.RuntimeBinding{
		CacheDirectory: configured.Models.CacheDirectory,
		RuntimeConfig:  currentRuntimeConfig,
	})
	if err != nil {
		return runtimeProducts{}, err
	}
	if modelDomain == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Models service returned nil runtime view")
	}
	selectedModels := modelDomain
	if contentMaterializer == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Work content materializer is required")
	}
	if workerExecutionFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: worker execution operation is required")
	}
	providerCommandRunner := adaptWorkerCommandRunner(edges.ProviderCommandRunner)
	scriptCommandRunner := adaptWorkerCommandRunner(edges.ScriptCommandRunner)
	serviceService, err := workerExecutionFactory(
		configured.Runtime,
		configured.Workers,
		clock,
		logger,
		providerCommandRunner,
		scriptCommandRunner,
		nil,
		edges.ProviderOverride,
		runtimeService,
		selectedModels, contentMaterializer,
		workersRuntimeFactory,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if automationFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Automations factory is required")
	}
	service2 := automationFactory(
		logger,
		clock,
		scriptCommandRunner,
		configured.Recordings.WorkflowID,
		configured.Definition.Directory,
		hostedPollers,
	)
	if service2 == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Automations factory returned nil service")
	}
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
			factorysessions.DefaultSessionID,
			nil,
			loadFactory,
			edges.ProviderOverride,
			providerCommandRunner,
			scriptCommandRunner,
			configured.Workers.MockWorkers,
			configured.Runtime.Mode,
			nil,
			nil,
			nil,
			false,
			edges.SubmissionRecorder,
			edges.DispatchRecorder,
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
			workersRuntimeExecutorsFactory,
			workersMockCommandRunnerFactory,
			runtimeService.InferenceProgressPublisherFactory(logger),
			runtimeService.DispatchCompletionObserverFactory(),
			mutationOwner.RecordPetriTokenMutations,
			recordingProjections.ReconstructFactoryWorldState,
			newRuntimeLedger,
			runtimeRecorderFactory,
			initialFactorySnapshotFactory,
			configured.Definition.Directory,
			root.FactoryRootDir,
			configured.Definition.ExecutionBaseDir,
			load.LoadedFactoryCfg,
			configured.Runtime.RuntimeInstanceID,
			load.ReplayArtifact,
			replayExecutionFactory,
			service2,
			configured.Runtime.Mode == factorydefinitions.RuntimeModeService,
		)
	if err != nil {
		return runtimeProducts{}, err
	}
	sessionRuntime, service4, invocationDomain, definitionHost, err := runtimeService.Complete(
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
		edges.InvocationMetricsRecorder,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if factoryDefinitionsFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions factory is required")
	}
	factoryDefinitionOwner := factoryDefinitionsFactory(definitionHost, factoryDefinitionValidator)
	if factoryDefinitionOwner == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions factory returned nil service")
	}
	if workFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Work factory is required")
	}
	workDomain := workFactory(runtimeService)
	if workDomain == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Work factory returned nil service")
	}
	if recordingsFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings factory is required")
	}
	recordingService := recordingsFactory(startupRuntime.RecordingLedger(), recordingProjections)
	if recordingService == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Recordings factory returned nil service")
	}
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
		edges.RuntimeHostObserver,
		startupRuntime.RuntimeLogger(),
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	rootRuntime, ok := sessionRuntime.(factoryruntime.Service)
	if !ok {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: session runtime does not implement Factory Runtime root Service")
	}
	opened := assembleRuntimeProducts(
		factoryDefinitionOwner,
		service4,
		invocationDomain,
		rootRuntime,
		factoryWorkflows,
		workflowPreview,
		workDomain,
		serviceService,
		selectedModels,
		providerSessions,
		startupRuntime,
		sessionRuntime,
		processRuntime,
		runtimeService,
		recordingProjections,
		configured.Definition.Directory,
		configured.Runtime.RuntimeInstanceID,
		configured.Session.BackendScopeID,
	)
	return opened, nil
}
