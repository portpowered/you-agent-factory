package wire

import (
	"fmt"

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
	factorysessionwirecontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
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
	LiveChangeCoordinator                = factorysessionwirecontracts.LiveChangeCoordinator
	ApplicationOpeningRequest            = roles.ApplicationOpeningRequest
	OpeningPresentationOwner             = factorysessions.OpeningPresentationOwner
	RuntimeResources                     = roles.RuntimeResources
	RuntimeHTTPServices                  = roles.RuntimeHTTPServices
	RuntimeVisualizationServices         = roles.RuntimeVisualizationServices
	OpenedApplicationRuntime             = roles.OpenedApplicationRuntime
	OpenedProcessApplication             = roles.OpenedProcessApplication
	OpenedInvocationRuntime              = roles.OpenedInvocationRuntime
	OpenedExecutionRuntime               = roles.OpenedExecutionRuntime

	ApplicationRuntimeInputs        = applicationopening.RuntimeInputs
	ApplicationRuntimeInputResolver = applicationopening.RuntimeInputResolver
	RuntimeAdapter                  = applicationopening.RuntimeAdapter
	ApplicationService              = applicationopening.Service

	SyncWaitScheduler = execution.SyncWaitScheduler

	ContractFixtureReader = fileeffects.ContractFixtureReader
	InvocationInputReader = fileeffects.InvocationInputReader
	ReplayRecordingReader = fileeffects.ReplayRecordingReader
	InitialWorkReader     = fileeffects.InitialWorkReader

	ProcessLifecycleFactory = processlifecycle.Factory
	RuntimeHostService      = runtimehosting.Service

	ApplicationRuntimeOpening              = runtimeopening.ApplicationRuntimeOpening
	InvocationRuntimeOpening               = runtimeopening.InvocationRuntimeOpening
	ExecutionRuntimeOpening                = runtimeopening.ExecutionRuntimeOpening
	ProviderSessionsRuntimeOpeningPorts    = runtimeopening.ProviderSessionsPorts
	FactoryRuntimeOpeningPorts             = runtimeopening.FactoryRuntimePorts
	FactoryDefinitionsRuntimeOpeningPorts  = runtimeopening.FactoryDefinitionsPorts
	FactorySessionsRuntimeOpeningPorts     = runtimeopening.FactorySessionsPorts
	WorkRuntimeOpeningPorts                = runtimeopening.WorkPorts
	AutomationsRuntimeOpeningPorts         = runtimeopening.AutomationsPorts
	ModelsRuntimeOpeningPorts              = runtimeopening.ModelsPorts
	RecordingsRuntimeOpeningPorts          = runtimeopening.RecordingsPorts
	WebhooksRuntimeOpeningPorts            = runtimeopening.WebhooksPorts
	WorkersRuntimeOpeningPorts             = runtimeopening.WorkersPorts
	OperatorSettingsRuntimeOpeningPorts    = runtimeopening.OperatorSettingsPorts
	AutomationFactory                      = runtimeopening.AutomationFactory
	FactorySessionExecutionFactory         = runtimeopening.FactorySessionExecutionFactory
	ConductorInvocationWithProgressFactory = runtimeopening.ConductorInvocationWithProgressFactory
	WorkersRuntimeFactory                  = runtimeopening.WorkersRuntimeFactory
	AutomationHostedSourcesFactory         = runtimeopening.AutomationHostedSourcesFactory
	WorkersLocalRuntimeHooksFactory        = runtimeopening.WorkersLocalRuntimeHooksFactory
	FactoryDefinitionsFactory              = runtimeopening.FactoryDefinitionsFactory
	DurableExecutionFactory                = runtimeopening.DurableExecutionFactory
	DurableExecution                       = runtimeopening.DurableExecution
	WorkerExecutionFactory                 = runtimeopening.WorkerExecutionFactory
	WorkerCommandRunnerAdapter             = runtimeopening.WorkerCommandRunnerAdapter
	ProviderCommandRunner                  = runtimeopening.ProviderCommandRunner
	ScriptCommandRunner                    = runtimeopening.ScriptCommandRunner
	ProviderFromCommandRunnerFactory       = runtimeopening.ProviderFromCommandRunnerFactory
	FactoryRuntimeAssembler                = runtimeopening.FactoryRuntimeAssembler
	RuntimeRootFactory                     = runtimeopening.RuntimeRootFactory
	RuntimeOpening                         = runtimeopening.Factory
	RuntimeRoot                            = runtimeopening.RuntimeRoot
	ModelPullMetricsRecorder               = factorysessioncontracts.ModelPullMetricsRecorder
	InvocationArtifactFileSystem           = factorysessioncontracts.InvocationArtifactFileSystem
	InvocationArtifactExporter             = factorysessioncontracts.InvocationArtifactExporter

	StandaloneSessionExecutionFactory   = executionopening.StandaloneSessionExecutionFactory
	WorkerInvocationFactory             = executionopening.WorkerInvocationFactory
	WorkerInvocationWithProgressFactory = executionopening.WorkerInvocationWithProgressFactory
	ExecutionOpeningFactory             = executionopening.Factory
	StdioOpeningService                 = executionopening.StdioOpeningService
	RuntimeVisualizationSinkOwner       = factoryvisualization.RuntimeSinkOwner
)

// RuntimeAssemblyFromService narrows the one Wire-constructed Factory
// Sessions root to its owner-private runtime capability. The assertion is
// performed once during process composition; runtime operations never ask the
// public root to construct or discover another service.
func RuntimeAssemblyFromService(service factorysessions.Service) (RuntimeAssembly, error) {
	if service == nil {
		return nil, fmt.Errorf("Factory Sessions runtime assembly requires the service root")
	}
	assembly, ok := service.(RuntimeAssembly)
	if !ok || assembly == nil {
		return nil, fmt.Errorf("Factory Sessions service root does not expose its runtime capability")
	}
	return assembly, nil
}

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

func NewRuntimeOpening(
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
) (*RuntimeOpening, error) {
	return runtimeopening.NewFactory(
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
}

func NewLifecyclePlanOperation() LifecyclePlanOperation {
	return processlifecycle.NewLifecyclePlanOperation()
}

func NewApplicationService(
	resolveInputs ApplicationRuntimeInputResolver,
	openRuntime ApplicationRuntimeOpening,
	adaptRuntime RuntimeAdapter,
	planLifecycle LifecyclePlanOperation,
	visualization RuntimeVisualizationSinkOwner,
) (*ApplicationService, error) {
	return applicationopening.New(resolveInputs, openRuntime, adaptRuntime, planLifecycle, visualization)
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
	return executionopening.NewServiceBuilder(factory)
}

func NewDirectJavaScriptRunOperation(
	build ExecutionServiceBuilder,
	runSync DirectJavaScriptSyncRunner,
	generateSessionID factorysessions.SessionIDGenerator,
	host roles.DirectJavaScriptHostAdapter,
	presentations OpeningPresentationOwner,
) (DirectJavaScriptRunOperation, error) {
	return executionopening.NewDirectJavaScriptRunOperation(build, runSync, generateSessionID, host, presentations)
}

func NewStdioOpeningService(
	opening StdioExecutionOpening,
	buildFixture FixtureStdioApplicationBuilder,
	buildRuntime RuntimeStdioApplicationBuilder,
	presentations OpeningPresentationOwner,
) (*StdioOpeningService, error) {
	return executionopening.NewStdioOpeningService(opening, buildFixture, buildRuntime, presentations)
}
