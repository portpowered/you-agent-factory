package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors/persistence"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	runtimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	executionopening "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/executionopening"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	factorysessionroot "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/invocation/wire"
	factorysessionwirecontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
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
	RuntimeAssembly                      = roles.RuntimeAssembly
	RuntimeReader                        = roles.RuntimeReader
	OwnedExecutionService                = roles.OwnedExecutionService
	ExecutionServiceBuilder              = roles.ExecutionServiceBuilder
	DurableExecutionService              = durableexecution.Service
	StdioApplication                     = roles.StdioApplication
	FixtureStdioApplicationBuilder       = roles.FixtureStdioApplicationBuilder
	RuntimeStdioApplicationBuilder       = roles.RuntimeStdioApplicationBuilder
	StdioExecutionOpening                = roles.StdioExecutionOpening
	StdioOpeningOperation                = roles.StdioOpeningOperation
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
	LiveChangeCoordinator                = factorysessionwirecontracts.LiveChangeCoordinator
	OpeningPresentationOwner             = factorysessions.OpeningPresentationOwner
	RuntimeResources                     = roles.RuntimeResources
	RuntimeVisualizationServices         = roles.RuntimeVisualizationServices
	OpenedApplicationRuntime             = roles.OpenedApplicationRuntime
	OpenedInvocationRuntime              = roles.OpenedInvocationRuntime
	OpenedExecutionRuntime               = roles.OpenedExecutionRuntime
	RuntimeOpeningCapability             = roles.RuntimeOpening

	SyncWaitScheduler = execution.SyncWaitScheduler

	ContractFixtureReader = fileeffects.ContractFixtureReader
	InvocationInputReader = fileeffects.InvocationInputReader
	ReplayRecordingReader = fileeffects.ReplayRecordingReader
	InitialWorkReader     = fileeffects.InitialWorkReader

	ApplicationRuntimeOpening              = service.ApplicationRuntimeOpening
	InvocationRuntimeOpening               = service.InvocationRuntimeOpening
	ExecutionRuntimeOpening                = service.ExecutionRuntimeOpening
	ProviderSessionsRuntimeOpeningPorts    = service.ProviderSessionsPorts
	ProviderOverrideService                = service.ProviderOverrideService
	FactoryRuntimeOpeningPorts             = service.FactoryRuntimePorts
	FactoryDefinitionsRuntimeOpeningPorts  = service.FactoryDefinitionsPorts
	FactorySessionsRuntimeOpeningPorts     = service.FactorySessionsPorts
	WorkRuntimeOpeningPorts                = service.WorkPorts
	AutomationsRuntimeOpeningPorts         = service.AutomationsPorts
	ModelsRuntimeOpeningPorts              = service.ModelsPorts
	RecordingsRuntimeOpeningPorts          = service.RecordingsPorts
	WebhooksRuntimeOpeningPorts            = service.WebhooksPorts
	WorkersRuntimeOpeningPorts             = service.WorkersPorts
	OperatorSettingsRuntimeOpeningPorts    = service.OperatorSettingsPorts
	WorkFactory                            = service.WorkFactory
	FactorySessionExecutionFactory         = service.FactorySessionExecutionFactory
	ConductorInvocationWithProgressFactory = service.ConductorInvocationWithProgressFactory
	DurableExecutionFactory                = service.DurableExecutionFactory
	DurableExecution                       = service.DurableExecution
	WorkerCommandRunnerAdapter             = service.WorkerCommandRunnerAdapter
	ProviderCommandRunner                  = service.ProviderCommandRunner
	ScriptCommandRunner                    = service.ScriptCommandRunner
	ProviderFromCommandRunnerFactory       = service.ProviderFromCommandRunnerFactory
	FactoryRuntimeAssembler                = service.FactoryRuntimeAssembler
	FactoryRuntimeRoot                     = service.FactoryRuntimeRoot
	RuntimeOpening                         = roles.RuntimeOpening
	RuntimeRoot                            = service.RuntimeRoot
	ModelPullMetricsRecorder               = factorysessioncontracts.ModelPullMetricsRecorder
	InvocationArtifactFileSystem           = factorysessioncontracts.InvocationArtifactFileSystem
	InvocationArtifactExporter             = factorysessioncontracts.InvocationArtifactExporter

	Root = factorysessionroot.Root

	StandaloneSessionExecutionFactory   = executionopening.StandaloneSessionExecutionFactory
	WorkerExecution                     = executionopening.WorkerExecution
	WorkerInvocationWithProgressFactory = executionopening.WorkerInvocationWithProgressFactory
	ExecutionOpeningFactory             = executionopening.Factory
	StdioOpeningService                 = executionopening.StdioOpeningService
)

// NewDefinitionRuntimeRouter returns the zero-value, inert Definitions
// routing capability used by the canonical process graph.
func NewDefinitionRuntimeRouter() *factorysessions.DefinitionRuntimeRouter {
	return &factorysessions.DefinitionRuntimeRouter{}
}

var (
	NewCursorFileStore         = persistence.NewFileStore
	NewRuntimeProjectStore     = runtimepersist.NewLazyProjectStore
	NewProcessLifecycleFactory = processlifecycle.NewFactory
	NewRuntimeHostService      = runtimehosting.New
	NewDurableExecutionRuntime = service.NewDurableExecution
	ModelHostDiagnosticLogger  = service.ModelHostDiagnosticLogger
	ModelHostDiagnosticMetrics = service.ModelHostDiagnosticMetrics
	NewExecutionOpeningFactory = executionopening.NewFactory
)

func NewRoot(
	providerSessions *ProviderSessionsRuntimeOpeningPorts,
	factoryRuntime *FactoryRuntimeOpeningPorts,
	factoryDefinitions *FactoryDefinitionsRuntimeOpeningPorts,
	factorySessions *FactorySessionsRuntimeOpeningPorts,
	workPorts *WorkRuntimeOpeningPorts,
	automations *AutomationsRuntimeOpeningPorts,
	modelsPorts *ModelsRuntimeOpeningPorts,
	recordingsPorts *RecordingsRuntimeOpeningPorts,
	webhooksPorts *WebhooksRuntimeOpeningPorts,
	workersPorts *WorkersRuntimeOpeningPorts,
	operatorSettings *OperatorSettingsRuntimeOpeningPorts,
) (RuntimeOpening, error) {
	root, err := service.NewRoot(
		providerSessions,
		factoryRuntime,
		factoryDefinitions,
		factorySessions,
		workPorts,
		automations,
		modelsPorts,
		recordingsPorts,
		webhooksPorts,
		workersPorts,
		operatorSettings,
	)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func NewLifecyclePlanOperation() LifecyclePlanOperation {
	return processlifecycle.NewLifecyclePlanOperation()
}

func NewInvocationOperation(
	openRuntime InvocationRuntimeOpening,
	modelsRoot models.Service,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
	presentations OpeningPresentationOwner,
) (InvocationOperation, error) {
	return invocationwire.NewOperation(
		openRuntime,
		modelsRoot,
		workingDirectory,
		resolveCurrentDir,
		artifactExporter,
		modelTimeout,
		artifactRoots,
		generateSessionID,
		logger,
		presentations,
	)
}

func NewExecutionServiceBuilder(factory *ExecutionOpeningFactory) ExecutionServiceBuilder {
	return factory.Builder()
}

func NewStdioOpeningService(
	opening StdioExecutionOpening,
	buildFixture FixtureStdioApplicationBuilder,
	buildRuntime RuntimeStdioApplicationBuilder,
	presentations OpeningPresentationOwner,
) (*StdioOpeningService, error) {
	return executionopening.NewStdioOpeningService(opening, buildFixture, buildRuntime, presentations)
}
