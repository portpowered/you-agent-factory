package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	applicationopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/applicationopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/cursors/persistence"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution"
	runtimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/execution/runtimepersist"
	executionopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/executionopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/runtimeopening"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/services/invocation/wire"
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
	WorkerHostedPollersFactory      = runtimeopening.WorkerHostedPollersFactory
	WorkersLocalRuntimeHooksFactory = runtimeopening.WorkersLocalRuntimeHooksFactory
	FactoryDefinitionsFactory       = runtimeopening.FactoryDefinitionsFactory
	DurableExecutionFactory         = runtimeopening.DurableExecutionFactory
	WorkerExecutionFactory          = runtimeopening.WorkerExecutionFactory
	WorkerCommandRunnerAdapter      = runtimeopening.WorkerCommandRunnerAdapter
	FactoryRuntimeAssembler         = runtimeopening.FactoryRuntimeAssembler
	RuntimeOpeningFactory           = runtimeopening.Factory
	RuntimeRoot                     = runtimeopening.RuntimeRoot

	StandaloneSessionExecutionFactory = executionopening.StandaloneSessionExecutionFactory
	WorkerInvocationFactory           = executionopening.WorkerInvocationFactory
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
	WorkerHostedPollersFactory      WorkerHostedPollersFactory
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
	AdaptWorkerCommandRunner        WorkerCommandRunnerAdapter
	ProcessRuntimeFactory           factorysessions.ProcessRuntimeFactory
	EnsureOperatorBackendScope      operatorsettings.BackendScopeEnsurer
	GenerateRuntimeInstanceID       factorysessions.RuntimeInstanceIDGenerator
	ResolveHome                     factorysessions.HomeDirectoryResolver
	ReplayFiles                     ReplayRecordingReader
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
		deps.WorkersMockCommandRunnerFactory, deps.WorkerHostedPollersFactory,
		deps.WorkersLocalRuntimeHooksFactory, deps.FactoryDefinitionsFactory,
		deps.FactoryScaffoldInitializer, deps.EditableFactoryValidator,
		deps.InitialFactorySnapshotFactory, deps.FactoryRuntimeAssembler, deps.ContentMaterializer,
		deps.LoadFactory, deps.NewLoadedFactory, deps.DecodeReplayConfig, deps.LoadReplay,
		deps.CaptureLoadedFactorySnapshot, deps.ResolveClock, deps.NewSessionLogger,
		deps.AdaptWorkerCommandRunner, deps.ProcessRuntimeFactory, deps.EnsureOperatorBackendScope,
		deps.GenerateRuntimeInstanceID, deps.ResolveHome, deps.ReplayFiles,
	)
}

func NewLifecyclePlanOperation() factorysessions.LifecyclePlanOperation {
	return processlifecycle.NewLifecyclePlanOperation()
}

func NewApplicationService(
	resolveInputs ApplicationRuntimeInputResolver,
	openRuntime RuntimeOpener,
	adaptRuntime RuntimeAdapter,
	planLifecycle factorysessions.LifecyclePlanOperation,
) (*ApplicationService, error) {
	return applicationopening.New(resolveInputs, openRuntime, adaptRuntime, planLifecycle)
}

func NewInvocationOperation(
	openRuntime *RuntimeOpeningFactory,
	edges serviceedges.Edges,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessions.InvocationOperation, error) {
	return invocationwire.NewOperation(
		openRuntime,
		edges,
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
	)
}

func NewExecutionServiceBuilder(factory *ExecutionOpeningFactory) factorysessions.ExecutionServiceBuilder {
	return executionopening.NewServiceBuilder(factory)
}

func NewDirectJavaScriptRunOperation(
	build factorysessions.ExecutionServiceBuilder,
	runSync factorysessions.DirectJavaScriptSyncRunner,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessions.DirectJavaScriptRunOperation, error) {
	return executionopening.NewDirectJavaScriptRunOperation(build, runSync, generateSessionID)
}

func NewStdioOpeningService(
	opening factorysessions.StdioExecutionOpening,
	buildFixture factorysessions.FixtureStdioApplicationBuilder,
	buildRuntime factorysessions.RuntimeStdioApplicationBuilder,
) (*StdioOpeningService, error) {
	return executionopening.NewStdioOpeningService(opening, buildFixture, buildRuntime)
}
