package wire

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"io/fs"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/scheduler"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// RuntimeFactory constructs hosted runtime bundles.
type RuntimeFactory = factoryruntimeinternal.RuntimeFactory

// DefinitionMapper holds reusable definition mapping behavior. Bind allocates
// opaque, detached state while retaining only the selected ID generator.
type DefinitionMapper = definitionmapping.Mapping

func NewDefinitionMapper(newID factoryruntime.IDGenerator) (DefinitionMapper, error) {
	mapper, err := definitionmapping.New(newID)
	if err != nil {
		return nil, err
	}
	return mapper, nil
}

// Assembly owns the product-policy dependencies used to assemble each
// session-owned Factory Runtime.
type Assembly = factoryruntimeinternal.Assembly

// NewInitialActivation binds initial behavior once; Open allocates scoped state.
func NewInitialActivation(assembly factoryruntimeinternal.InitialAssembly, clock factoryruntime.Clock, logger *zap.Logger,
	materialize func(*factorydefinitions.RuntimeSnapshot, string) (factorydefinitions.MutableLoadedFactorySource, error),
) factoryruntime.InitialRuntimeActivationOperation {
	return factoryruntimeinternal.NewInitialActivation(assembly, clock, logger, materialize).Open
}

// InputFileSystem is the Factory Runtime construction seam for its selected
// input tree. The service root does not publish this host-effect port.
type InputFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// NewRuntimeFactory constructs a hosted runtime bundle factory.
func NewRuntimeFactory(
	loggerFactory factoryruntime.RuntimeLoggerFactory,
	runtimeLogs factoryruntime.RuntimeLogOwner,
	runtimeMetrics factoryruntime.RuntimeMetricsOwner,
	newID factoryruntime.IDGenerator,
	workRequestIDs work.RequestIDGenerator,
	runtimeDirs factoryruntime.RuntimeDirectoryFileSystem,
	inputFiles InputFileSystem,
	inputDirectoryWalker factoryruntime.InputDirectoryWalker,
	orchestrationCompilation factoryruntime.OrchestrationCompilation,
	workerAttemptScheduler platformclock.TimerSource,
	definitionMapper DefinitionMapper,
	engineOpening EngineOpening,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	worldStateProjector factoryruntime.WorldStateProjector,
	recordingsRuntime recordings.RuntimeScopeService,
) *RuntimeFactory {
	return factoryruntimeinternal.NewRuntimeFactory(
		loggerFactory,
		runtimeLogs,
		runtimeMetrics,
		newID,
		workRequestIDs,
		runtimeDirs,
		inputFiles,
		inputDirectoryWalker,
		orchestrationCompilation,
		workerAttemptScheduler,
		definitionMapper,
		runtime.SelectedEngineOpening(engineOpening),
		submissionRecorder, dispatchRecorder, worldStateProjector, recordingsRuntime,
	)
}

// NewAssembly constructs the inert Factory Runtime assembly service selected by
// Wire. It does not start a runtime or sidecar.
func NewAssembly(
	opening BundleOpening,
	sidecars *SidecarOpening,
	instanceHost InstanceHost,
	preparation RuntimePreparation,
	recordingsRuntime recordings.RuntimeScopeService,
	automationService automations.Service,
	progressFactory func(*zap.Logger) func(string) workers.ProgressPublisher,
	completionFactory func(string) func(string),
	recoverOwners recordings.WorkerCapturePreparationOperation,
) (*Assembly, error) {
	return factoryruntimeinternal.NewAssembly(factoryruntimeinternal.SelectedBundleOpening(opening).Open,
		sidecars,
		instanceHost,
		preparation,
		recordingsRuntime,
		automationService,
		progressFactory, completionFactory, recoverOwners)
}

// These sealed construction values carry one focused owner each. Operational
// method sets are resolved only at this owner boundary and stay internal.
type Enablement = scheduler.EnablementSelection
type EngineOpening = runtime.EngineOpeningSelection
type BundleOpening = factoryruntimeinternal.BundleOpeningSelection

// RuntimeResourceOpening selects the existing resource owner for BundleOpening.
type RuntimeResourceOpening = factoryruntimeinternal.RuntimeFactorySelection

func NewEnablementEvaluator() Enablement { return scheduler.NewEnablementEvaluator() }

func NewEngineOpening(
	interpolation factorydefinitions.InvocationInterpolationService,
	providerSessions providersessions.Service,
	quorumPolicy factorydefinitions.QuorumPolicyService,
	outputShaping factorydefinitions.InvocationOutputShapingService,
	workPropagation factorydefinitions.WorkPropagationPolicyService,
	workService work.Service,
	workRequestIDs work.RequestIDGenerator,
	newID factoryruntime.IDGenerator,
	runtimeDirs factoryruntime.RuntimeDirectoryFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	dispatchOpening OutboxOpening,
	enablement Enablement,
) EngineOpening {
	return runtime.NewEngineOpening(interpolation, providerSessions, quorumPolicy, outputShaping,
		workPropagation, workService, workRequestIDs, newID, runtimeDirs, decisionEnvelopes, dispatchOpening, scheduler.SelectedEnablement(enablement))
}

func NewBundleOpening(
	runtimeFactory RuntimeResourceOpening,
	workerAttemptScheduler platformclock.TimerSource,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factoryruntime.WorkerAttemptOpener,
	requestResolver *WorkstationRequestExecutor,
	recordingsRuntime recordings.RuntimeScopeService,
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory,
) (BundleOpening, error) {
	opening, err := factoryruntimeinternal.NewBundleOpening(factoryruntimeinternal.SelectedRuntimeFactory(runtimeFactory).Build, workerAttemptScheduler, workerService,
		workerSessions, workerAttempts, requestResolver, recordingsRuntime, initialFactorySnapshot)
	if err != nil {
		return nil, err
	}
	return opening, nil
}

type SidecarOpening = factoryruntimeinternal.SidecarOpening

func NewSidecarOpening(automation automations.Service, metricsClock platformclock.TimerSource) *SidecarOpening {
	return factoryruntimeinternal.NewSidecarOpening(automation, metricsClock)
}

// NewOrchestratorDefinitionValidator returns the runtime-owned orchestrator
// validator injected into Factory Definition validation by Wire.
func NewOrchestratorDefinitionValidator(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
) factorydefinitions.OrchestratorDefinitionValidator {
	return factoryruntimeinternal.NewOrchestratorDefinitionValidator(workflows)
}

// RuntimePreparation is the inert preparation owner passed through the T15 bridge.
type RuntimePreparation = runtimebuild.ExecutionPreparation

// NewRuntimePreparation constructs fixed preparation once in canonical Wire.
func NewRuntimePreparation(workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factoryruntime.LoadedFactoryLoader, newID factoryruntime.IDGenerator,
	baseLogger *zap.Logger, providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner, scriptCommandRunner platformprocess.CommandRunner,
	mockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory) RuntimePreparation {
	return runtimebuild.New(workstationLoader,
		loadFactory,
		newID,
		baseLogger,
		providerOverride,
		providerCommandRunner,
		scriptCommandRunner,
		runtimebuild.MockCommandRunnerFactory(mockCommandRunnerFactory))
}

// Fixed typed roles consumed by canonical composition.
type WorkstationRequestExecutor = runtime.WorkstationRequestExecutor
type RequestPromptRenderer = runtime.PromptRenderer
type RequestTemplateFieldResolver = runtime.TemplateFieldResolver
type ExpectedArtifactFileSystem interface {
	Glob(string) ([]string, error)
	Stat(string) (fs.FileInfo, error)
	EvalSymlinks(string) (string, error)
}

func NewWorkstationRequestExecutor(service workers.Service,
	interpolation factorydefinitions.InvocationInterpolationService, invocationFiles factorydefinitions.FileReader,
	newID factoryruntime.IDGenerator, prompts RequestPromptRenderer, templateFields RequestTemplateFieldResolver,
	progress workers.ProgressPublisher,
	expectedArtifacts ExpectedArtifactFileSystem, logger factoryruntime.Logger,
) *WorkstationRequestExecutor {
	return runtime.NewWorkstationRequestExecutor(service, interpolation, invocationFiles, newID,
		prompts, templateFields, invocationFiles, progress, expectedArtifacts, logger)
}

type OutboxOpening = dispatchplanning.OutboxOpening

func NewOutboxOpening() OutboxOpening {
	return dispatchplanningwire.NewOpening()
}
