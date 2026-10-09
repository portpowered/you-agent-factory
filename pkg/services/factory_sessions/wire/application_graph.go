package wire

import (
	"context"
	"github.com/google/wire"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors/persistence"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	runtimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	factorysessionroot "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service/invocation"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	sessionservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessionwirecontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	webhooks "github.com/portpowered/infinite-you/pkg/services/webhooks"
	"go.uber.org/zap"
)

// The aliases in this file are the service-owned construction vocabulary used
// by the canonical application graph. They keep implementation package names
// private to Factory Sessions while the root Wire package composes exact roles.
type (
	HistoricalReplayBehavior             = recordingreplay.Behavior
	ApplicationRuntime                   = roles.ApplicationRuntime
	DirectoryInspection                  = roles.DirectoryInspection
	CursorPersistenceFileSystem          = roles.CursorPersistenceFileSystem
	CursorPersistenceTemporaryFile       = roles.CursorPersistenceTemporaryFile
	CursorPersistenceCreateTemporaryFile = roles.CursorPersistenceCreateTemporaryFile
	CursorStoreFactory                   = roles.CursorStoreFactory
	InvocationMetricsRecorder            = roles.InvocationMetricsRecorder
	RuntimeResolver                      = roles.RuntimeResolver
	CurrentRuntimeResolver               = roles.CurrentRuntimeResolver
	RuntimeAssembly                      = roles.RuntimeAssembly
	RuntimeReader                        = roles.RuntimeReader
	DurableExecutionService              = durableexecution.Service
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
	HistoricalApplicationInspection      = service.HistoricalApplicationInspection
	SessionPresentation                  = service.SessionPresentation

	SyncWaitScheduler = execution.SyncWaitScheduler

	ContractFixtureReader = fileeffects.ContractFixtureReader
	InvocationInputReader = fileeffects.InvocationInputReader
	ReplayRecordingReader = fileeffects.ReplayRecordingReader
	InitialWorkReader     = fileeffects.InitialWorkReader

	ProviderOverrideService      = service.ProviderOverrideService
	RuntimeResourceAcquisition   = service.RuntimeResourceAcquisition
	RuntimeOpening               = service.RuntimeOpening
	RuntimeOpeningCompletion     = service.RuntimeOpeningCompletion
	RuntimeOpeningBinding        = service.RuntimeOpeningBinding
	OpeningSessionIdentity       = service.OpeningSessionIdentity
	DurableOpening               = service.DurableOpening
	DurableExecution             = service.DurableExecution
	WorkerCommandRunnerAdapter   = service.WorkerCommandRunnerAdapter
	ProviderCommandRunner        = service.ProviderCommandRunner
	ScriptCommandRunner          = service.ScriptCommandRunner
	FactoryRuntimeRoot           = service.FactoryRuntimeRoot
	RuntimeRoot                  = service.RuntimeRoot
	RuntimeInputLoading          = service.RuntimeInputLoading
	ModelPullMetricsRecorder     = factorysessioncontracts.ModelPullMetricsRecorder
	InvocationArtifactFileSystem = factorysessioncontracts.InvocationArtifactFileSystem
	InvocationArtifactExporter   = factorysessioncontracts.InvocationArtifactExporter

	RuntimeModelInvocationOperation = modelinvocation.RuntimeModelInvocationOperation
	RuntimeModelFactoryConfigReader = modelinvocation.FactoryConfigReader
	RuntimeModelWorkerExecution     = modelinvocation.WorkerExecution
	Root                            = factorysessionroot.Root
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
	NewDurableOpening          = service.NewDurableOpening
	ModelHostDiagnosticLogger  = service.ModelHostDiagnosticLogger
	ModelHostDiagnosticMetrics = service.ModelHostDiagnosticMetrics
)

// OpeningAssembly projects the existing owner onto the opening operations.
type OpeningAssembly = interface {
	Resolve(string) *livesession.LiveSession
	PrepareNewFactoryScaffold(string) (string, error)
	CloseSession(context.Context, string) error
	ListLiveSessionIDs() []string
	BindHistoricalOpening(string, DurableExecutionService) (func(), error)
}

func RuntimeOpeningAssembly(assembly RuntimeAssembly) OpeningAssembly {
	opening, _ := assembly.(OpeningAssembly)
	return opening
}

// NewRuntimeOpening keeps the private implementation behind the Sessions Wire boundary.
func NewRuntimeOpening(assembly OpeningAssembly, durable DurableExecutionService,
	preparation *RuntimePreparation, snapshots *RuntimeSnapshotSelection,
	resources *RuntimeResourceAcquisition, durableOpening *DurableOpening,
	initialEngine *RuntimeInitialEngine, completion *RuntimeOpeningCompletion,
	binding *RuntimeOpeningBinding, runtimeRoot FactoryRuntimeRoot,
	replayBehavior *HistoricalReplayBehavior, recordingsService recordings.Service,
	recordingsRuntime recordings.RuntimeScopeService, executionBinding *ExecutionBinding,
	providerOverride ProviderOverrideService, runtimeLogs factoryruntime.RuntimeLogOwner,
	logger *zap.Logger, clock factoryruntime.Clock, resolveClock factoryruntime.ClockResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	definitions factorydefinitions.Service, inventory recordings.RecordedSessionInventory,
) *RuntimeOpening {
	return service.NewRuntimeOpening(assembly, durable, preparation, snapshots, resources, durableOpening,
		initialEngine, completion, binding, runtimeRoot, replayBehavior, recordingsService, recordingsRuntime,
		executionBinding, providerOverride, runtimeLogs, logger, clock, resolveClock, generateSessionID,
		generateRuntimeInstanceID, resolveHome, definitions, inventory)
}

// RuntimeOpeningStart selects the fixed opening operation without retaining Root.
func RuntimeOpeningStart(opening *RuntimeOpening) func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	return opening.Start
}

func RuntimeOpeningHistoricalInspection(opening *RuntimeOpening) func(context.Context, factorysessions.SessionStartRequest) (HistoricalApplicationInspection, bool, error) {
	return opening.InspectHistoricalApplication
}

func NewRoot(
	assembly RuntimeAssembly,
	durable DurableExecutionService,
	start func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error),
	liveChangeCoordinator factorysessionwirecontracts.LiveChangeCoordinator,
	inspectHistorical func(context.Context, factorysessions.SessionStartRequest) (HistoricalApplicationInspection, bool, error),
	definitions factorydefinitions.Service,
	recordingsService recordings.Service,
	recordingProjections recordings.ProjectionService,
	modelInvocation RuntimeModelInvocationOperation,
	generateSessionID factorysessions.SessionIDGenerator,
) (*Root, error) {
	return service.NewRoot(assembly, durable, start, liveChangeCoordinator, inspectHistorical,
		definitions, recordingsService, recordingProjections, modelInvocation, generateSessionID)
}

func NewLifecyclePlanOperation() LifecyclePlanOperation {
	return processlifecycle.NewLifecyclePlanOperation()
}

func NewInvocationOperation(
	sessions factorysessions.Service,
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
		sessions,
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

// NewRuntimeModelInvocation injects fixed peers into the session-scoped operation.
func NewRuntimeModelInvocation(modelService models.Service, gateway RuntimeModelFactoryConfigReader, workerService RuntimeModelWorkerExecution) RuntimeModelInvocationOperation {
	return modelinvocation.NewRuntimeModelInvocation(modelService, gateway, workerService)
}

// NewHistoricalReplayBehavior constructs fixed behavior inside its service owner.
func NewHistoricalReplayBehavior() *HistoricalReplayBehavior {
	return recordingreplay.NewBehavior()
}

func NewRuntimeInputLoading(
	loadFactory factorydefinitions.LoadedFactoryLoader,
	newLoadedFactory factorydefinitions.LoadedFactorySourceFactory,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.RuntimeScopeService,
	captureLoadedFactorySnapshot factorydefinitions.LoadedFactorySnapshotCapturer,
	newSessionLogger factoryruntime.SessionLoggerFactory,
	baseLogger *zap.Logger,
) *RuntimeInputLoading {
	return service.NewRuntimeInputLoading(loadFactory, newLoadedFactory, decodeReplayConfig,
		replayInputs, captureLoadedFactorySnapshot, newSessionLogger, baseLogger)
}

// RuntimeSnapshotSelection is the private selection owner exposed for composition.
type RuntimeSnapshotSelection = service.RuntimeSnapshotSelection

func NewRuntimeSnapshotSelection(
	definitions factorydefinitions.Service,
	decode factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.RuntimeScopeService,
	paths factorydefinitions.NamedPathResolver,
	resolveHome factorysessions.HomeDirectoryResolver,
) *RuntimeSnapshotSelection {
	return service.NewRuntimeSnapshotSelection(definitions.ResolveRuntimeSnapshot, decode, replayInputs,
		paths.ResolveCurrentDir, resolveHome)
}

// RuntimePreparation is the fixed preparation owner exposed for composition.
type RuntimePreparation = service.RuntimePreparation

func NewRuntimePreparation(
	loading *RuntimeInputLoading,
	paths factorydefinitions.NamedPathResolver,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	ensureBackendScope operatorsettings.BackendScopeEnsurer,
	providerIdentities factorysessions.ProviderIdentityResolver,
	validator factorydefinitions.Validator,
	replay recordings.RuntimeScopeService,
	resolveClock factoryruntime.ClockResolver,
) *RuntimePreparation {
	return service.NewRuntimePreparation(loading.Load, paths.ResolveCurrentDir,
		generateRuntimeInstanceID, resolveHome, ensureBackendScope,
		providerIdentities, validator, replay.ReplayClock, resolveClock)
}

func NewRuntimeResourceAcquisition(opening *DurableOpening, modelService models.Service, providerOverride ProviderOverrideService) *RuntimeResourceAcquisition {
	return service.NewRuntimeResourceAcquisition(opening.Open, modelService, providerOverride)
}

// RuntimeInitialEngine is the fixed initial activation behavior for live and checkpoint callers.
type RuntimeInitialEngine = service.RuntimeInitialEngine

func NewRuntimeInitialEngine(selection *RuntimeSnapshotSelection,
	activate factoryruntime.InitialRuntimeActivationOperation) *RuntimeInitialEngine {
	return service.NewRuntimeInitialEngine(selection.Resolve, activate)
}

func NewRuntimeOpeningCompletion(assembly RuntimeAssembly, routing *factorysessions.DefinitionRuntimeRouter,
	webhooksService webhooks.Service, host ProcessRuntimeFactory) *RuntimeOpeningCompletion {
	return service.NewRuntimeOpeningCompletion(assembly, routing, webhooksService, host)
}

// ExecutionBinding is the focused capability registration operation for live and replay opening.
type ExecutionBinding = service.ExecutionBinding

func NewExecutionBinding(providerOverride ProviderOverrideService, commandRunner ProviderCommandRunner) *ExecutionBinding {
	return service.NewExecutionBinding(providerOverride, commandRunner)
}

func NewRuntimeOpeningBinding(gateway OpeningSessionIdentity, recordingsService recordings.Service,
	providerOverride ProviderOverrideService, commandRunner ProviderCommandRunner) *RuntimeOpeningBinding {
	return service.NewRuntimeOpeningBinding(gateway, recordingsService, providerOverride, commandRunner)
}

// RuntimeOpeningBindingSet exposes exact roles of the one canonical gateway instance.
var RuntimeOpeningBindingSet = wire.NewSet(NewGateway, NewRuntimeOpeningBinding,
	wire.Bind(new(OpeningSessionIdentity), new(*sessionservice.Service)),
	wire.Bind(new(factorysessions.SessionInspectionService), new(*sessionservice.Service)),
	wire.Bind(new(roles.SessionGateway), new(*sessionservice.Service)))
