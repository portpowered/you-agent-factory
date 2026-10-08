package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors/persistence"
	execution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
	runtimepersist "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/processlifecycle"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimehosting"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	factorysessionroot "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service"
	invocationwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service/invocation"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	factorysessionwirecontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	webhooks "github.com/portpowered/infinite-you/pkg/services/webhooks"
	work "github.com/portpowered/infinite-you/pkg/services/work"
	workers "github.com/portpowered/infinite-you/pkg/services/workers"
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

func NewRoot(
	providerSessions providersessions.Service,
	logger *zap.Logger,
	runtimeLogs factoryruntime.RuntimeLogOwner,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	runtimeRoot FactoryRuntimeRoot,
	resolveClock factoryruntime.ClockResolver,

	clock factoryruntime.Clock,
	providerOverride ProviderOverrideService,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	definitions factorydefinitions.Service,
	runtimeRouter *factorysessions.DefinitionRuntimeRouter,

	snapshotSelection *RuntimeSnapshotSelection,
	preparation *RuntimePreparation,

	assembly RuntimeAssembly,
	durableOpening *DurableOpening,
	processRuntimeFactory ProcessRuntimeFactory,
	generateSessionID factorysessions.SessionIDGenerator,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	workService work.Service,
	automationService automations.Service,
	webhooksService webhooks.Service,
	modelService models.Service,
	recordingsService recordings.Service,
	recordingsRuntime recordings.RuntimeScopeService,
	workerService workers.Service,
	providerCommandRunner ProviderCommandRunner,
	scriptCommandRunner ScriptCommandRunner,
	initialActivation factoryruntime.InitialRuntimeActivationOperation,
	modelInvocation RuntimeModelInvocationOperation,
	replayBehavior *HistoricalReplayBehavior,
	liveChangeCoordinator factorysessionwirecontracts.LiveChangeCoordinator,
	recordingProjections recordings.ProjectionService,
) (*Root, error) {
	return service.NewRoot(
		providerSessions,
		logger,
		runtimeLogs,
		factoryWorkflows,
		workflowPreview,
		runtimeRoot,
		resolveClock,

		clock,
		providerOverride,
		submissionRecorder,
		dispatchRecorder,
		definitions,
		runtimeRouter,

		snapshotSelection,
		preparation,

		assembly,
		durableOpening,
		processRuntimeFactory,
		generateSessionID,
		generateRuntimeInstanceID,
		resolveHome,
		workService,
		automationService,
		webhooksService,
		modelService,
		recordingsService,
		recordingsRuntime,
		workerService,
		providerCommandRunner,
		scriptCommandRunner,
		initialActivation,
		modelInvocation,
		replayBehavior,
		liveChangeCoordinator,
		recordingProjections,
	)
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
