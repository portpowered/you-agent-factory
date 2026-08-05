package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	applicationopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/applicationopening"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors/persistence"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	runtimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	executionopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/executionopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
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
	DurableExecutionService              = durableexecution.Service
	StdioApplication                     = roles.StdioApplication
	FixtureStdioApplicationBuilder       = roles.FixtureStdioApplicationBuilder
	RuntimeStdioApplicationBuilder       = roles.RuntimeStdioApplicationBuilder
	StdioExecutionOpening                = roles.StdioExecutionOpening
	StdioOpeningOperation                = roles.StdioOpeningOperation
	DirectJavaScriptRunOperation         = roles.DirectJavaScriptRunOperation
	DirectJavaScriptSyncRunner           = roles.DirectJavaScriptSyncRunner
	DirectJavaScriptHostAdapter          = roles.DirectJavaScriptHostAdapter
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

	RuntimeOpeningExternalEffects                = runtimeopening.ExternalEffects
	RuntimeOpeningDependencies                   = runtimeopening.Dependencies
	InvocationRuntimeOpening                     = runtimeopening.InvocationRuntimeOpening
	ExecutionRuntimeOpening                      = runtimeopening.ExecutionRuntimeOpening
	ProviderSessionsRuntimeOpeningDependencies   = runtimeopening.ProviderSessionsDependencies
	FactoryRuntimeOpeningDependencies            = runtimeopening.FactoryRuntimeDependencies
	FactoryDefinitionsRuntimeOpeningDependencies = runtimeopening.FactoryDefinitionsDependencies
	FactorySessionsRuntimeOpeningDependencies    = runtimeopening.FactorySessionsDependencies
	WorkRuntimeOpeningDependencies               = runtimeopening.WorkDependencies
	AutomationsRuntimeOpeningDependencies        = runtimeopening.AutomationsDependencies
	ModelsRuntimeOpeningDependencies             = runtimeopening.ModelsDependencies
	RecordingsRuntimeOpeningDependencies         = runtimeopening.RecordingsDependencies
	WorkersRuntimeOpeningDependencies            = runtimeopening.WorkersDependencies
	OperatorSettingsRuntimeOpeningDependencies   = runtimeopening.OperatorSettingsDependencies
	WorkFactory                                  = runtimeopening.WorkFactory
	AutomationFactory                            = runtimeopening.AutomationFactory
	FactorySessionExecutionFactory               = runtimeopening.FactorySessionExecutionFactory
	ConductorInvocationWithProgressFactory       = runtimeopening.ConductorInvocationWithProgressFactory
	RecordingsProjectionFactory                  = runtimeopening.RecordingsProjectionFactory
	RecordingLifecycleFactory                    = runtimeopening.RecordingLifecycleFactory
	RuntimeLedgerFactory                         = runtimeopening.RuntimeLedgerFactory
	ReplayClockFactory                           = runtimeopening.ReplayClockFactory
	WorkersRuntimeFactory                        = runtimeopening.WorkersRuntimeFactory
	AutomationHostedSourcesFactory               = runtimeopening.AutomationHostedSourcesFactory
	WorkersLocalRuntimeHooksFactory              = runtimeopening.WorkersLocalRuntimeHooksFactory
	FactoryDefinitionsFactory                    = runtimeopening.FactoryDefinitionsFactory
	DurableExecutionFactory                      = runtimeopening.DurableExecutionFactory
	DurableExecution                             = runtimeopening.DurableExecution
	WorkerExecutionFactory                       = runtimeopening.WorkerExecutionFactory
	WorkerCommandRunnerAdapter                   = runtimeopening.WorkerCommandRunnerAdapter
	ProviderFromCommandRunnerFactory             = runtimeopening.ProviderFromCommandRunnerFactory
	FactoryRuntimeAssembler                      = runtimeopening.FactoryRuntimeAssembler
	RuntimeOpeningFactory                        = runtimeopening.Factory
	RuntimeRoot                                  = runtimeopening.RuntimeRoot
	ModelPullMetricsRecorder                     = factorysessioncontracts.ModelPullMetricsRecorder
	InvocationArtifactFileSystem                 = factorysessioncontracts.InvocationArtifactFileSystem
	InvocationArtifactExporter                   = factorysessioncontracts.InvocationArtifactExporter

	StandaloneSessionExecutionFactory   = executionopening.StandaloneSessionExecutionFactory
	WorkerInvocationFactory             = executionopening.WorkerInvocationFactory
	WorkerInvocationWithProgressFactory = executionopening.WorkerInvocationWithProgressFactory
	LiveChildInvocationFactory          = execution.LiveChildInvocationFactory
	ExecutionOpeningFactory             = executionopening.Factory
	StdioOpeningService                 = executionopening.StdioOpeningService
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

func NewRuntimeOpeningFactory(deps RuntimeOpeningDependencies) (*RuntimeOpeningFactory, error) {
	return runtimeopening.NewFactory(deps)
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
	openRuntime InvocationRuntimeOpening,
	modelsRoot models.Service,
	effects RuntimeOpeningExternalEffects,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (InvocationOperation, error) {
	return invocationwire.NewOperation(
		openRuntime,
		modelsRoot,
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
