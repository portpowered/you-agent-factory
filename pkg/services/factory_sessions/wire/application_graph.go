package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	applicationopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/applicationopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors/persistence"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	runtimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	executionopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/executionopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// The aliases in this file are the service-owned construction vocabulary used
// by the canonical application graph. They keep implementation package names
// private to Factory Sessions while the root Wire package composes exact roles.
type (
	ApplicationRuntime                   = roles.ApplicationRuntime
	DirectoryInspection                  = roles.DirectoryInspection
	CursorPersistenceFileSystem          = roles.CursorPersistenceFileSystem
	CursorPersistenceTemporaryFile       = roles.CursorPersistenceTemporaryFile
	CursorPersistenceCreateTemporaryFile = roles.CursorPersistenceCreateTemporaryFile
	CursorStoreFactory                   = roles.CursorStoreFactory
	ExecutionOpeningFileSystem           = roles.ExecutionOpeningFileSystem
	InvocationMetricsRecorder            = roles.InvocationMetricsRecorder
	RuntimeResolver                      = roles.RuntimeResolver
	CurrentRuntimeResolver               = roles.CurrentRuntimeResolver
	RuntimeReader                        = roles.RuntimeReader
	OwnedExecutionService                = roles.OwnedExecutionService
	ExecutionServiceBuilder              = roles.ExecutionServiceBuilder
	StdioApplication                     = roles.StdioApplication
	FixtureStdioApplicationBuilder       = roles.FixtureStdioApplicationBuilder
	RuntimeStdioApplicationBuilder       = roles.RuntimeStdioApplicationBuilder
	StdioExecutionOpening                = roles.StdioExecutionOpening
	StdioOpeningOperation                = roles.StdioOpeningOperation
	DirectJavaScriptRunOperation         = roles.DirectJavaScriptRunOperation
	DirectJavaScriptSyncRunner           = roles.DirectJavaScriptSyncRunner
	DirectJavaScriptHostAdapter          = roles.DirectJavaScriptHostAdapter
	DirectJavaScriptLifecycle            = roles.DirectJavaScriptLifecycle
	RequestPreparation                   = roles.RequestPreparation
	Registry                             = roles.Registry
	RuntimePersistenceStore              = roles.RuntimePersistenceStore
	RuntimePersistenceFileSystem         = roles.RuntimePersistenceFileSystem
	RuntimePersistenceStoreFactory       = roles.RuntimePersistenceStoreFactory
	LifecycleRuntime                     = roles.LifecycleRuntime
	ProcessRuntime                       = roles.ProcessRuntime
	ProcessRuntimeFactory                = roles.ProcessRuntimeFactory
	RuntimeHostOperation                 = roles.RuntimeHostOperation
	LifecyclePlanRequest                 = roles.LifecyclePlanRequest
	LifecyclePlanOperation               = roles.LifecyclePlanOperation
	SessionInvoker                       = roles.SessionInvoker
	InvocationInputResolver              = roles.InvocationInputResolver
	ModelInvocationOperation             = roles.ModelInvocationOperation
	InvocationOperation                  = roles.InvocationOperation
	InvocationTarget                     = roles.InvocationTarget
	FactoryInvocationOutcome             = roles.FactoryInvocationOutcome
	ApplicationOpeningPorts              = roles.ApplicationOpeningPorts
	ApplicationOpeningRequest            = roles.ApplicationOpeningRequest
	RuntimeResources                     = roles.RuntimeResources
	RuntimeHTTPServices                  = roles.RuntimeHTTPServices
	RuntimeVisualizationServices         = roles.RuntimeVisualizationServices
	OpenedApplicationRuntime             = roles.OpenedApplicationRuntime
	OpenedProcessApplication             = roles.OpenedProcessApplication
	OpenedInvocationRuntime              = roles.OpenedInvocationRuntime
	OpenedExecutionRuntime               = roles.OpenedExecutionRuntime

	ApplicationRuntimeInputs        = applicationopening.RuntimeInputs
	ApplicationRuntimeInputResolver = applicationopening.RuntimeInputResolver
	RuntimeOpener                   = applicationopening.RuntimeOpener
	RuntimeAdapter                  = applicationopening.RuntimeAdapter
	ApplicationService              = applicationopening.Service

	SyncWaitScheduler = execution.SyncWaitScheduler

	ContractFixtureReader = fileeffects.ContractFixtureReader
	InvocationInputReader = fileeffects.InvocationInputReader
	ReplayRecordingReader = fileeffects.ReplayRecordingReader
	InitialWorkReader     = fileeffects.InitialWorkReader

	ProcessLifecycleFactory = processlifecycle.Factory
	RuntimeHostService      = runtimehosting.Service

	RuntimeOpeningExternalEffects   = runtimeopening.ExternalEffects
	WorkFactory                     = runtimeopening.WorkFactory
	AutomationFactory               = runtimeopening.AutomationFactory
	FactorySessionExecutionFactory  = runtimeopening.FactorySessionExecutionFactory
	RecordingsProjectionFactory     = runtimeopening.RecordingsProjectionFactory
	RecordingsFactory               = runtimeopening.RecordingsFactory
	RuntimeLedgerFactory            = runtimeopening.RuntimeLedgerFactory
	ReplayClockFactory              = runtimeopening.ReplayClockFactory
	WorkersRuntimeFactory           = runtimeopening.WorkersRuntimeFactory
	AutomationHostedSourcesFactory      = runtimeopening.AutomationHostedSourcesFactory
	WorkersLocalRuntimeHooksFactory = runtimeopening.WorkersLocalRuntimeHooksFactory
	FactoryDefinitionsFactory       = runtimeopening.FactoryDefinitionsFactory
	DurableExecutionFactory         = runtimeopening.DurableExecutionFactory
	WorkerExecutionFactory          = runtimeopening.WorkerExecutionFactory
	WorkerCommandRunnerAdapter           = runtimeopening.WorkerCommandRunnerAdapter
	ProviderFromCommandRunnerFactory     = runtimeopening.ProviderFromCommandRunnerFactory
	FactoryRuntimeAssembler              = runtimeopening.FactoryRuntimeAssembler
	RuntimeOpeningFactory           = runtimeopening.Factory
	RuntimeRoot                     = runtimeopening.RuntimeRoot

	StandaloneSessionExecutionFactory = executionopening.StandaloneSessionExecutionFactory
	WorkerInvocationFactory             = executionopening.WorkerInvocationFactory
	WorkerInvocationWithProgressFactory = executionopening.WorkerInvocationWithProgressFactory
	LiveChildInvocationFactory          = execution.LiveChildInvocationFactory
	ExecutionOpeningFactory           = executionopening.Factory
	StdioOpeningService               = executionopening.StdioOpeningService
)

var (
	NewCursorFileStore         = persistence.NewFileStore
	NewRuntimeProjectStore     = runtimepersist.NewProjectStore
	NewProcessLifecycleFactory = processlifecycle.NewFactory
	NewRuntimeHostService      = runtimehosting.New
	NewDurableExecutionRuntime = runtimeopening.NewDurableExecution
	NewWorkerExecutionRuntime  = runtimeopening.NewWorkerExecution
	ModelHostDiagnosticLogger  = runtimeopening.ModelHostDiagnosticLogger
	ModelHostDiagnosticMetrics = runtimeopening.ModelHostDiagnosticMetrics
	NewExecutionOpeningFactory = executionopening.NewFactory
)

type RuntimeOpeningDependencies struct {
	ProviderSessions                providersessions.Service
	FactoryWorkflows                factoryruntime.JavaScriptWorkflowDefinitions
	WorkflowPreview                 factoryruntime.WorkflowPreviewOperation
	FactoryDefinitionValidator      factorydefinitions.Validator
	NamedPaths                      factorydefinitions.NamedPathResolver
	DurableExecutionFactory         DurableExecutionFactory
	WorkerExecutionFactory          WorkerExecutionFactory
	ModelService                    models.Service
	WorkFactory                     WorkFactory
	AutomationFactory               AutomationFactory
	FactorySessionsService          factorysessions.Service
	FactorySessionExecutionFactory  FactorySessionExecutionFactory
	RecordingsProjectionFactory     RecordingsProjectionFactory
	RecordingsFactory               RecordingsFactory
	RuntimeLedgerFactory            RuntimeLedgerFactory
	RuntimeRecorderFactory          recordings.RuntimeRecorderFactory
	ReplayClockFactory              ReplayClockFactory
	ReplayExecutionFactory          recordings.ReplayExecutionFactory
	WorkersRuntimeFactory           WorkersRuntimeFactory
	WorkersRuntimeExecutorsFactory  factoryruntime.WorkersRuntimeExecutorsFactory
	WorkersMockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory
	AutomationHostedSourcesFactory AutomationHostedSourcesFactory
	WorkersLocalRuntimeHooksFactory WorkersLocalRuntimeHooksFactory
	FactoryDefinitionsFactory       FactoryDefinitionsFactory
	FactoryScaffoldInitializer      factorysessions.FactoryScaffoldInitializer
	EditableFactoryValidator        factorysessions.EditableFactoryValidator
	InitialFactorySnapshotFactory   factorydefinitions.InitialFactorySnapshotFactory
	FactoryRuntimeAssembler         FactoryRuntimeAssembler
	ContentMaterializer             work.ContentMaterializer
	LoadFactory                     factorydefinitions.LoadedFactoryLoader
	NewLoadedFactory                factorydefinitions.LoadedFactorySourceFactory
	DecodeReplayConfig              factorydefinitions.ReplayRuntimeConfigDecoder
	LoadReplay                      recordings.ReplayArtifactLoader
	CaptureLoadedFactorySnapshot    factorydefinitions.LoadedFactorySnapshotCapturer
	ResolveClock                    factoryruntime.ClockResolver
	NewSessionLogger                factoryruntime.SessionLoggerFactory
	AdaptWorkerCommandRunner            WorkerCommandRunnerAdapter
	ProviderFromCommandRunnerFactory    ProviderFromCommandRunnerFactory
	ProcessRuntimeFactory               ProcessRuntimeFactory
	EnsureOperatorBackendScope      operatorsettings.BackendScopeEnsurer
	GenerateRuntimeInstanceID       factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                     factorysessions.HomeDirectoryResolver
	ReplayFiles                     ReplayRecordingReader
	ProviderIdentities              factorysessions.ProviderIdentityResolver
}

func NewRuntimeOpeningFactory(deps RuntimeOpeningDependencies) (*RuntimeOpeningFactory, error) {
	return runtimeopening.NewFactory(
		deps.ProviderSessions, deps.FactoryWorkflows, deps.WorkflowPreview,
		deps.FactoryDefinitionValidator, deps.NamedPaths, deps.DurableExecutionFactory,
		deps.WorkerExecutionFactory, deps.ModelService, deps.WorkFactory, deps.AutomationFactory,
		deps.FactorySessionsService, deps.FactorySessionExecutionFactory,
		deps.RecordingsProjectionFactory, deps.RecordingsFactory, deps.RuntimeLedgerFactory,
		deps.RuntimeRecorderFactory, deps.ReplayClockFactory, deps.ReplayExecutionFactory,
		deps.WorkersRuntimeFactory, deps.WorkersRuntimeExecutorsFactory,
		deps.WorkersMockCommandRunnerFactory, deps.AutomationHostedSourcesFactory,
		deps.WorkersLocalRuntimeHooksFactory, deps.FactoryDefinitionsFactory,
		deps.FactoryScaffoldInitializer, deps.EditableFactoryValidator,
		deps.InitialFactorySnapshotFactory, deps.FactoryRuntimeAssembler, deps.ContentMaterializer,
		deps.LoadFactory, deps.NewLoadedFactory, deps.DecodeReplayConfig, deps.LoadReplay,
		deps.CaptureLoadedFactorySnapshot, deps.ResolveClock, deps.NewSessionLogger,
		deps.AdaptWorkerCommandRunner, deps.ProviderFromCommandRunnerFactory, deps.ProcessRuntimeFactory,
		deps.EnsureOperatorBackendScope,
		deps.GenerateRuntimeInstanceID, deps.ResolveHome, deps.ReplayFiles,
		deps.ProviderIdentities,
	)
}

func NewLifecyclePlanOperation() LifecyclePlanOperation {
	return processlifecycle.NewLifecyclePlanOperation()
}

func NewApplicationService(
	resolveInputs ApplicationRuntimeInputResolver,
	openRuntime RuntimeOpener,
	adaptRuntime RuntimeAdapter,
	planLifecycle LifecyclePlanOperation,
) (*ApplicationService, error) {
	return applicationopening.New(resolveInputs, openRuntime, adaptRuntime, planLifecycle)
}

func NewInvocationOperation(
	openRuntime *RuntimeOpeningFactory,
	effects RuntimeOpeningExternalEffects,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (InvocationOperation, error) {
	return invocationwire.NewOperation(
		openRuntime,
		effects,
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
	)
}

func NewExecutionServiceBuilder(factory *ExecutionOpeningFactory) ExecutionServiceBuilder {
	return executionopening.NewServiceBuilder(factory)
}

func NewDirectJavaScriptRunOperation(
	build ExecutionServiceBuilder,
	runSync DirectJavaScriptSyncRunner,
	generateSessionID factorysessions.SessionIDGenerator,
	host roles.DirectJavaScriptHostAdapter,
) (DirectJavaScriptRunOperation, error) {
	return executionopening.NewDirectJavaScriptRunOperation(build, runSync, generateSessionID, host)
}

func NewStdioOpeningService(
	opening StdioExecutionOpening,
	buildFixture FixtureStdioApplicationBuilder,
	buildRuntime RuntimeStdioApplicationBuilder,
) (*StdioOpeningService, error) {
	return executionopening.NewStdioOpeningService(opening, buildFixture, buildRuntime)
}
