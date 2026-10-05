package wire

import (
	"fmt"
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
	engineOpening *EngineOpening,
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
		engineOpening,
		submissionRecorder, dispatchRecorder, worldStateProjector, recordingsRuntime,
	)
}

// NewAssembly constructs the inert Factory Runtime assembly service selected by
// Wire. It does not start a runtime or sidecar.
func NewAssembly(
	bundleOpening BundleOpening,
	sidecars *SidecarOpening,
	instanceHost InstanceHost,
	preparation *RuntimePreparation,
	recordingsRuntime recordings.RuntimeScopeService,
	automationService automations.Service,
	progressFactory func(*zap.Logger) func(string) workers.ProgressPublisher,
	completionFactory func(string) func(string),
) (*Assembly, error) {
	return factoryruntimeinternal.NewAssembly(bundleOpening,
		sidecars,
		instanceHost,
		preparation,
		recordingsRuntime,
		automationService,
		progressFactory, completionFactory)
}

type SidecarOpening = factoryruntimeinternal.SidecarOpening

func NewSidecarOpening(automation automations.Service, metricsClock platformclock.TimerSource) *SidecarOpening {
	return factoryruntimeinternal.NewSidecarOpening(automation, metricsClock)
}

type BundleOpening = factoryruntimeinternal.BundleOpeningOperation

// NewBundleOpening constructs the fixed bundle-opening behavior once in Wire.
func NewBundleOpening(
	runtimeFactory *RuntimeFactory,
	workerAttemptScheduler platformclock.TimerSource,
	workerService workers.Service,
	workerSessions workersessions.Service,
	workerAttempts factoryruntime.WorkerAttemptOpener,
	requestResolver *WorkstationRequestExecutor,
	recordingsRuntime recordings.RuntimeScopeService,
	initialFactorySnapshot factorydefinitions.InitialFactorySnapshotFactory,
) (BundleOpening, error) {
	if runtimeFactory == nil {
		return nil, fmt.Errorf("factory runtime factory is required")
	}
	opening, err := factoryruntimeinternal.NewBundleOpening(runtimeFactory.Build, workerAttemptScheduler, workerService, workerSessions, workerAttempts, requestResolver, recordingsRuntime, initialFactorySnapshot)
	if err != nil {
		return nil, err
	}
	return opening.Open, nil
}

// NewOrchestratorDefinitionValidator returns the runtime-owned orchestrator
// validator injected into Factory Definition validation by Wire.
func NewOrchestratorDefinitionValidator(
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
) factorydefinitions.OrchestratorDefinitionValidator {
	return factoryruntimeinternal.NewOrchestratorDefinitionValidator(workflows)
}

// RuntimePreparation is the inert preparation owner passed through the T15 bridge.
type RuntimePreparation = runtimebuild.Service

// NewRuntimePreparation constructs fixed preparation once in canonical Wire.
func NewRuntimePreparation(workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factoryruntime.LoadedFactoryLoader, newID factoryruntime.IDGenerator,
	baseLogger *zap.Logger, providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner, scriptCommandRunner platformprocess.CommandRunner,
	mockCommandRunnerFactory factoryruntime.WorkersMockCommandRunnerFactory) *RuntimePreparation {
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

// EngineOpening owns the reusable engine collaborators; each Open owns fresh state.
type EngineOpening = runtime.EngineOpening

type OutboxOpening = dispatchplanning.OutboxOpening

func NewOutboxOpening() OutboxOpening {
	return dispatchplanningwire.NewOpening()
}

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
) *EngineOpening {
	return runtime.NewEngineOpening(interpolation, providerSessions, quorumPolicy, outputShaping, workPropagation, workService, workRequestIDs, newID, runtimeDirs, decisionEnvelopes, dispatchOpening)
}
