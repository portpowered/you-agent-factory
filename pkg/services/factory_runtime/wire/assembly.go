package wire

import (
	"io/fs"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimeinternal "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/definitionmapping"
	runtime "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtime"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// RuntimeFactory constructs hosted runtime bundles.
type RuntimeFactory = factoryruntimeinternal.RuntimeFactory

// DefinitionMapper holds reusable definition mapping behavior. Map allocates
// a new net for each opening while retaining only the selected ID generator.
type DefinitionMapper = definitionmapping.Mapper

func NewDefinitionMapper(newID factoryruntime.IDGenerator) (*DefinitionMapper, error) {
	return definitionmapping.New(newID)
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
	baseLogger *zap.Logger,
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
	definitionMapper *DefinitionMapper,
	engineOpening *EngineOpening,
) *RuntimeFactory {
	return factoryruntimeinternal.NewRuntimeFactory(
		baseLogger,
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
	)
}

// NewAssembly constructs the inert Factory Runtime assembly service selected by
// Wire. It does not start a runtime or sidecar.
func NewAssembly(
	runtimeFactory *RuntimeFactory,
	workerSessions workersessions.Service,
	workerAttempts factoryruntime.WorkerAttemptOpener,
	workerService workers.Service,
	metricsClock platformclock.TimerSource,
	instanceHost InstanceHost,
	preparation *RuntimePreparation,
	requestResolver *WorkstationRequestExecutor,
) (*Assembly, error) {
	return factoryruntimeinternal.NewAssembly(runtimeFactory, workerSessions, workerAttempts, workerService, metricsClock, instanceHost, preparation, requestResolver)
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
	baseLogger *zap.Logger) *RuntimePreparation {
	return runtimebuild.New(workstationLoader, loadFactory, newID, baseLogger)
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
) *EngineOpening {
	return runtime.NewEngineOpening(interpolation, providerSessions, quorumPolicy, outputShaping, workPropagation, workService, workRequestIDs, newID, runtimeDirs, decisionEnvelopes)
}
