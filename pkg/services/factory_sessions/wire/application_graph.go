package wire

import (
	"context"
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
	OpeningPresentationOwner             = factorysessions.OpeningPresentationOwner
	RuntimeResources                     = roles.RuntimeResources
	RuntimeVisualizationServices         = roles.RuntimeVisualizationServices
	OpenedApplicationRuntime             = roles.OpenedApplicationRuntime
	OpenedProcessApplication             = roles.OpenedProcessApplication
	OpenedInvocationRuntime              = roles.OpenedInvocationRuntime
	OpenedExecutionRuntime               = roles.OpenedExecutionRuntime
	RuntimeOpeningCapability             = roles.RuntimeOpening

	ApplicationRuntimeInputs        = applicationopening.RuntimeInputs
	ApplicationRuntimeInputResolver = applicationopening.RuntimeInputResolver
	RuntimeAdapter                  = applicationopening.RuntimeAdapter
	ApplicationService              = applicationopening.Service

	SyncWaitScheduler = execution.SyncWaitScheduler

	ContractFixtureReader = fileeffects.ContractFixtureReader
	InvocationInputReader = fileeffects.InvocationInputReader
	ReplayRecordingReader = fileeffects.ReplayRecordingReader
	InitialWorkReader     = fileeffects.InitialWorkReader

	ApplicationRuntimeOpening              = runtimeopening.ApplicationRuntimeOpening
	InvocationRuntimeOpening               = runtimeopening.InvocationRuntimeOpening
	ExecutionRuntimeOpening                = runtimeopening.ExecutionRuntimeOpening
	ProviderSessionsRuntimeOpeningPorts    = runtimeopening.ProviderSessionsPorts
	ProviderOverrideService                = runtimeopening.ProviderOverrideService
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
	WorkFactory                            = runtimeopening.WorkFactory
	FactorySessionExecutionFactory         = runtimeopening.FactorySessionExecutionFactory
	ConductorInvocationWithProgressFactory = runtimeopening.ConductorInvocationWithProgressFactory
	DurableExecutionFactory                = runtimeopening.DurableExecutionFactory
	DurableExecution                       = runtimeopening.DurableExecution
	WorkerCommandRunnerAdapter             = runtimeopening.WorkerCommandRunnerAdapter
	ProviderCommandRunner                  = runtimeopening.ProviderCommandRunner
	ScriptCommandRunner                    = runtimeopening.ScriptCommandRunner
	ProviderFromCommandRunnerFactory       = runtimeopening.ProviderFromCommandRunnerFactory
	FactoryRuntimeAssembler                = runtimeopening.FactoryRuntimeAssembler
	FactoryRuntimeRoot                     = runtimeopening.FactoryRuntimeRoot
	RuntimeRootFactory                     = runtimeopening.RuntimeRootFactory
	RuntimeOpening                         = runtimeopening.Factory
	RuntimeRoot                            = runtimeopening.RuntimeRoot
	ModelPullMetricsRecorder               = factorysessioncontracts.ModelPullMetricsRecorder
	InvocationArtifactFileSystem           = factorysessioncontracts.InvocationArtifactFileSystem
	InvocationArtifactExporter             = factorysessioncontracts.InvocationArtifactExporter

	StandaloneSessionExecutionFactory   = executionopening.StandaloneSessionExecutionFactory
	WorkerExecution                     = executionopening.WorkerExecution
	WorkerInvocationWithProgressFactory = executionopening.WorkerInvocationWithProgressFactory
	ExecutionOpeningFactory             = executionopening.Factory
	StdioOpeningService                 = executionopening.StdioOpeningService
)

// RuntimeOpeningAdapter is the temporary compatibility view used by the old
// application, invocation, and execution opening seams. It retains only the
// already-composed Factory Sessions root and delegates every opening through
// that root; it owns no registry, runtime, or lifecycle.
type RuntimeOpeningAdapter struct {
	owner runtimeOpeningOwner
}

type runtimeOpeningOwner interface {
	OpenApplicationRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedApplicationRuntime, error)
	OpenInvocationRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedInvocationRuntime, error)
	OpenExecutionRuntime(
		context.Context,
		*factorysessions.RuntimeOpeningRequest,
	) (roles.OpenedExecutionRuntime, error)
}

// NewRuntimeOpeningAdapter binds the temporary opening view to the one
// process-scoped Factory Sessions root. Binding is construction-only and does
// not discover, construct, or activate another service.
func NewRuntimeOpeningAdapter(service factorysessions.Service) (*RuntimeOpeningAdapter, error) {
	if service == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime opening adapter: service root is required")
	}
	owner, ok := service.(runtimeOpeningOwner)
	if !ok || owner == nil {
		return nil, fmt.Errorf("construct Factory Sessions runtime opening adapter: service root does not expose opening operations")
	}
	return &RuntimeOpeningAdapter{owner: owner}, nil
}

func (adapter *RuntimeOpeningAdapter) ownerService() (runtimeOpeningOwner, error) {
	if adapter == nil || adapter.owner == nil {
		return nil, fmt.Errorf("Factory Sessions runtime opening adapter is unavailable")
	}
	return adapter.owner, nil
}

func (adapter *RuntimeOpeningAdapter) OpenApplicationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedApplicationRuntime, error) {
	owner, err := adapter.ownerService()
	if err != nil {
		return roles.OpenedApplicationRuntime{}, err
	}
	return owner.OpenApplicationRuntime(ctx, request)
}

func (adapter *RuntimeOpeningAdapter) OpenInvocationRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedInvocationRuntime, error) {
	owner, err := adapter.ownerService()
	if err != nil {
		return roles.OpenedInvocationRuntime{}, err
	}
	return owner.OpenInvocationRuntime(ctx, request)
}

func (adapter *RuntimeOpeningAdapter) OpenExecutionRuntime(
	ctx context.Context,
	request *factorysessions.RuntimeOpeningRequest,
) (roles.OpenedExecutionRuntime, error) {
	owner, err := adapter.ownerService()
	if err != nil {
		return roles.OpenedExecutionRuntime{}, err
	}
	return owner.OpenExecutionRuntime(ctx, request)
}

var _ ApplicationRuntimeOpening = (*RuntimeOpeningAdapter)(nil)
var _ InvocationRuntimeOpening = (*RuntimeOpeningAdapter)(nil)
var _ ExecutionRuntimeOpening = (*RuntimeOpeningAdapter)(nil)

// NewDefinitionRuntimeRouter returns the zero-value, inert Definitions
// routing capability used by the canonical process graph.
func NewDefinitionRuntimeRouter() *factorysessions.DefinitionRuntimeRouter {
	return &factorysessions.DefinitionRuntimeRouter{}
}

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

// RuntimeOpeningFromService retrieves the one owner-private opening
// capability retained by the canonical Factory Sessions root. The lookup is a
// construction-time assertion only; callers cannot replace the capability or
// construct another root through it.
func RuntimeOpeningFromService(service factorysessions.Service) (RuntimeOpeningCapability, error) {
	if service == nil {
		return nil, fmt.Errorf("Factory Sessions runtime opening requires the service root")
	}
	provider, ok := service.(interface {
		RuntimeOpening() RuntimeOpeningCapability
	})
	if !ok || provider == nil {
		return nil, fmt.Errorf("Factory Sessions service root does not retain its runtime opening")
	}
	opening := provider.RuntimeOpening()
	if opening == nil {
		return nil, fmt.Errorf("Factory Sessions service root has no runtime opening")
	}
	return opening, nil
}

var (
	NewCursorFileStore         = persistence.NewFileStore
	NewRuntimeProjectStore     = runtimepersist.NewProjectStore
	NewProcessLifecycleFactory = processlifecycle.NewFactory
	NewRuntimeHostService      = runtimehosting.New
	NewDurableExecutionRuntime = runtimeopening.NewDurableExecution
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
) (*ApplicationService, error) {
	return applicationopening.New(resolveInputs, openRuntime, adaptRuntime, planLifecycle)
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
