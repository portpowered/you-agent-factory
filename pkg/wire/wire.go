//go:build wireinject

package wire

import (
	"context"

	"github.com/google/wire"
	initializerapplication "github.com/portpowered/infinite-you/pkg/initializer/application"
	processcontract "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	edges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/portableconfig"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeservice "github.com/portpowered/infinite-you/pkg/services/factory_runtime/service"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	applicationopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/applicationopening"
	sessionexecutionopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/executionopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening"
	invocationopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening/invocation"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/transports/cli"
	configinitcmd "github.com/portpowered/infinite-you/pkg/transports/cli/configinit"
	httpapplication "github.com/portpowered/infinite-you/pkg/transports/http/application"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/composition"
	factorydefinitionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition"
	factorysessionmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
	mcpstdio "github.com/portpowered/infinite-you/pkg/transports/mcp/stdio"
)

var platformSet = wire.NewSet(
	// TODO: remove this when we figure out how to appropriately inject the logging.
	logging.NewDefaultLogger,
)

var apiSet = wire.NewSet(
	composition.NewWorkAPI,
	composition.NewHTTPBinder,
	apisurface.NewRuntimeAPI,
	composition.NewLiveSessionAPI,
	factorydefinitionmapping.NewAPI,
	factorysessionmapping.NewDurableAPI,
	factorysessionmapping.NewLiveAPI,
	factorysessionmapping.NewInvocationAPI,
	mcpstdio.NewOpener,
	httpapplication.NewHandler,
)

var servicesSet = wire.NewSet(
	factorysessions.NewRequestPreparation,
	factoryruntime.NewFactoryStatusProjector,
	factoryruntime.NewSessionResultProjectionOperation,
	provideOperatorSettingsFileSystem,
	provideOperatorSettingsCreateTemporaryFile,
	provideOperatorSettingsIDGenerator,
	provideSystemInitializationInspectPath,
	provideSystemInitializationLegacyFactoryMigrationFileSystem,
	provideOperatorConfigLoader,
	provideOperatorBackendScopeEnsurer,
	provideDurableExecutionFactory,
	provideWorkerExecutionFactory,
	provideAPIServerStarter,
	provideRuntimeHostOperation,
	provideProcessRuntimeFactory,
	processlifecycle.NewLifecyclePlanOperation,
	provideFactoryVisualizationFactory,
	provideResponsePresentation,
	provideWorkContentStagingService,
	work.NewContentPreparation,
	work.NewRequestPreparationService,
	work.NewSingleWorkTargetPreparation,
	work.NewListRequestPreparation,
	work.NewFactoryRequestBatchPreparation,
	work.NewInvocationInputPreparation,
	provideWorkersMockWorkersConfigFileSystem,
	workers.NewMockWorkersConfigLoader,
	provideRuntimeArtifactClock,
	provideRuntimeArtifactIDGenerator,
	provideRuntimeArtifactPathReserver,
	provideRuntimeArtifactRootResolver,
	provideRuntimeLoggerFactory,
	provideRuntimeLogSinkFactory,
	provideRuntimeMetricsSinkFactory,
	provideManagedRunnerFactory,
	provideModelsService,
	provideFactorySessionsWorkingDirectory,
	provideFactorySessionExecutionOpeningFileSystem,
	provideFactorySessionDirectoryInspection,
	provideFactorySessionContractFixtureReader,
	provideFactorySessionInvocationInputReader,
	provideFactorySessionReplayRecordingReader,
	provideFactorySessionInitialWorkReader,
	provideFactorySessionResolveHomeDirectory,
	provideFactorySessionIDGenerator,
	provideFactorySessionRuntimeInstanceIDGenerator,
	provideFactorySessionResponseEventIDGenerator,
	provideFactorySessionCursorPersistenceFileSystem,
	provideFactorySessionCursorCreateTemporaryFile,
	provideFactorySessionCursorStoreFactory,
	provideFactorySessionRuntimePersistenceFileSystem,
	provideFactorySessionRuntimePersistenceStoreFactory,
	provideFactorySessionSyncWaitScheduler,
	provideModelInvocationArtifactExporter,
	provideModelInvocationTimeout,
	provideModelInvocationOperation,
	provideWorkFactory,
	provideWorkRequestIDGenerator,
	provideWorkSubmittedFileReader,
	provideWorkContentHostPlatform,
	provideContentMaterializer,
	provideDecisionEnvelopeService,
	provideInvocationInterpolationService,
	provideInvocationOutputShapingService,
	provideInvocationWorkTypeService,
	provideQuorumPolicyService,
	provideWorkPropagationPolicyService,
	provideTTSObservabilityService,
	provideAutomationFactory,
	provideFactorySessionsFactory,
	providePortableRecordingWriter,
	provideFactorySessionExecutionFactory,
	provideRecordingsProjectionFactory,
	provideRecordingsFactory,
	provideRuntimeLedgerFactory,
	provideReplayArtifactStorage,
	provideRuntimeRecorderFactory,
	provideReplayClockFactory,
	provideFactoryRuntimeIDGenerator,
	provideFactoryRuntimeDirectories,
	provideFactoryRuntimeInputs,
	provideFactoryRuntimeInputDirectoryWalker,
	provideFactoryRuntimeWorkflowSources,
	provideFactoryRuntimeWorkflowSourceResolveSymlinks,
	provideFactoryRuntimeWorkflowHome,
	provideFactoryDefinitionPortableFileSystem,
	provideFactoryDefinitionLoadingFileSystem,
	provideFactoryDefinitionClock,
	provideFactoryDefinitionVersionFileSystem,
	provideFactoryDefinitionPackagedGoalPromptFileSystem,
	provideFactoryDefinitionPortableBundledFileInspection,
	provideFactoryDefinitionRequiredToolPathLookup,
	provideFactoryDefinitionRequiredToolVersionProbe,
	provideFactoryDefinitionRequiredToolChecker,
	provideFactoryDefinitionPersistenceFileSystem,
	provideFactoryDefinitionDirectoryReplacementStore,
	provideFactoryDefinitionNamedPathFileSystem,
	provideFactoryDefinitionNamedPathResolver,
	provideFactoryDefinitionNamedFactoryCatalogFileSystem,
	provideFactoryDefinitionPackagedInstallationFileSystem,
	provideFactoryDefinitionAuthoredReaderFileSystem,
	provideFactoryDefinitionAuthoredWriterFileSystem,
	provideFactoryDefinitionScaffoldFileSystem,
	provideFactoryDefinitionScaffoldOutput,
	provideFactoryDefinitionInputInboxSentinelEnsurer,
	providePortableBundledFilesMaterializer,
	providePortableBundledFileWritesValidator,
	providePortableBundledFilesCopier,
	providePortableBundledFileSourceResolver,
	portableconfig.NewPortableBundledFilesApplier,
	portableconfig.NewFactoryStarterWorkApplier,
	portableconfig.NewPortableBundledDocsPruner,
	provideFactoryDefinitionLoader,
	provideFactoryRuntimeClockResolver,
	provideFactoryRuntimeSessionLoggerFactory,
	provideReplayExecutionFactory,
	provideWorkersRetryRandomSource,
	provideWorkersWorkstationFileSystem,
	provideWorkersProviderTemporaryFileSystem,
	provideAgyPTYAllocator,
	provideWorkersRuntimeFactory,
	provideWorkersRuntimeExecutorsFactory,
	provideWorkersMockCommandRunnerFactory,
	provideWorkerHostedPollersFactory,
	provideWorkersLocalRuntimeHooksFactory,
	provideWorkerCommandRunnerAdapter,
	provideFactoryDefinitionsFactory,
	provideFactoryScaffoldInitializer,
	provideEditableFactoryValidator,
	provideInitialFactorySnapshotFactory,
	factoryruntimeservice.NewRuntimeFactory,
	factoryruntimeservice.NewAssembly,
	wire.Bind(new(runtimeopening.FactoryRuntimeAssembler), new(*factoryruntimeservice.Assembly)),
	provideLoadedFactorySourceFactory,
	provideLoadedFactoryLoader,
	provideReplayArtifactLoader,
	provideReplayRuntimeConfigDecoder,
	runtimeopening.NewFactory,
)

var providerSessionServiceSet = wire.NewSet(
	provideProviderSessions,
)

var factorySessionsServicesSet = wire.NewSet(
	provideStandaloneSessionExecutionFactory,
	provideJavaScriptWorkflows,
)

var factoryDefinitionsServicesSet = wire.NewSet(
	provideFactoryDefinitionValidationService,
	provideFactoryDefinitionValidator,
	provideDefinitionValidationOperation,
	provideSubmittedDefinitionValidationOperation,
	provideAuthoredFactorySourceLoader,
	provideJavaScriptWorkflowDefinitions,
	provideWorkflowPreviewOperation,
	provideNamedFactoryCatalog,
	provideLoadedFactorySnapshotCapturer,
	provideFactoryScaffoldCommandInitializer,
	provideFactoryDefinitionPersistence,
	provideNamedFactoryPersistenceOperation,
)

var workerServiceSet = wire.NewSet(
	provideWorkerInvocationFactory,
	provideWorkerProcessEnvironment,
	provideWorkerCurrentWorkingDirectory,
)

var cliCommandOperationsSet = wire.NewSet(
	provideCLIObserver,
	provideNamedFactoryRootsResolver,
	provideNamedFactoryCandidatePathsResolver,
	provideBatchInputFileSystem,
	provideRunDirectoryCreator,
	provideBrowserOpener,
	provideCurrentFactoryDirectoryResolver,
	provideFactoryConfigRootResolver,
	provideFactoryConfigFileLoader,
	provideWorkRequestFileLoader,
	provideTerminalLoggerBuilder,
	provideLiveRecordingTargetPlanner,
	provideCLIRunDefaults,
	provideSubmitPayloadReader,
	provideOperatorDefaultsResolver,
	provideStandardCLIHTTPProtocol,
	provideExtendedCLIHTTPProtocol,
	provideSubmitWorkOperation,
	provideSubmitBatchOperation,
	provideListSessionsOperation,
	provideShowSessionOperation,
	providePauseSessionOperation,
	provideResumeSessionOperation,
	provideListSessionDispatchesOperation,
	provideCreateSessionOperation,
	provideDeleteSessionOperation,
	provideModelsCLIService,
	provideFlattenFactoryConfigOperation,
	provideExpandFactoryConfigOperation,
	provideInitSystemConfigOperation,
	provideQueryFactoryOperation,
	provideListFactoriesOperation,
	provideValidateFactoryOperation,
	provideCreateFactoryFromFileOperation,
	provideReplaceFactoryCurrentOperation,
	provideUpdateFactoryFromFileOperation,
	provideDeleteFactoryOperation,
	provideListWorkOperation,
	provideShowWorkOperation,
	provideMoveWorkOperation,
	provideWorkVisualizationOperation,
	provideVisualizeWorkOperation,
	wire.Struct(new(cli.CommandOperations), "*"),
)

// BundleSet is the one canonical provider set used by both public bundle
// injectors. It constructs only inert command and service initializers.
var BundleSet = wire.NewSet(
	platformSet,
	apiSet,
	servicesSet,
	providerSessionServiceSet,
	factorySessionsServicesSet,
	factoryDefinitionsServicesSet,
	workerServiceSet,
	cliCommandOperationsSet,
	provideSystemInitializationService,
	configinitcmd.NewInitializer,
	provideRuntimeOpener,
	provideApplicationRuntimeAdapter,
	provideLifecycleRunnerFactory,
	provideResponseEventValidator,
	provideWorkStopSummaryProjector,
	provideRuntimeOpeningRequestFactory,
	provideRunOpener,
	provideRuntimeInputResolver,
	applicationopening.New,
	initializerapplication.NewRuntimeRunnerBuilder,
	provideRunRuntimeRunnerBuilder,
	provideRunSelectionFactory,
	invocationopening.NewOperation,
	initializerapplication.NewStdioRunnerBuilder,
	initializerapplication.NewOpenedStdioRunnerBuilder,
	provideFixtureStdioApplicationBuilder,
	provideRuntimeStdioApplicationBuilder,
	provideSessionExecutionOpeningFactory,
	wire.Bind(new(factorysessions.StdioExecutionOpening), new(*sessionexecutionopening.Factory)),
	sessionexecutionopening.NewStdioOpeningService,
	wire.Bind(new(factorysessions.StdioOpeningOperation), new(*sessionexecutionopening.StdioOpeningService)),
	provideStdioApplicationOpener,
	provideDirectJavaScriptSyncRunner,
	sessionexecutionopening.NewDirectJavaScriptRunOperation,
	initializerapplication.NewInitializer,
	sessionexecutionopening.NewServiceBuilder,
	provideCLICommandFactory,
	initializerapplication.NewProcess,
	wire.Bind(new(processcontract.Initializer), new(*initializerapplication.Initializer)),
	wire.Bind(new(processcontract.CommandFactory), new(cli.CommandFactory)),
)

// InjectBundle is the single application-process injector. Callers provide
// production defaults or functional overrides through the same typed inputs.
func InjectBundle(
	ctx context.Context,
	edges edges.Edges,
) (*initializerapplication.Process, error) {
	wire.Build(
		BundleSet,
	)
	return nil, nil
}
