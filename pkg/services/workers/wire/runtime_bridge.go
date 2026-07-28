package wire

import (
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	"go.uber.org/zap"
)

// ProviderRegistryRebinder reconstructs the provider registry for runtime command edges.
type ProviderRegistryRebinder = workersinternal.ProviderRegistryRebinder

// CurrentRuntimeResolver resolves the active Factory Session runtime.
type CurrentRuntimeResolver = workersinternal.CurrentRuntimeResolver

// NewRuntimeWithSelection constructs the Workers runtime through owner-internal implementation.
func NewRuntimeWithSelection(
	sessions CurrentRuntimeResolver,
	modelService models.Service,
	modelsScope models.RuntimeScopeRef,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	allocator agypty.PTYAllocator,
	logger *zap.Logger,
	verbose bool,
	factoryRunnerID string,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	now func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	contentMaterializer work.ContentMaterializer,
	interpolation factorydefinitions.InvocationInterpolationService,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	factoryDocs workers.FactoryDocsLoader,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	worktreePreparer workers.FactoryWorktreePreparer,
	agentRunHarness workeragentrun.HarnessAdapter,
	retryRandom platformrandom.Source,
	workstationFiles platformfilesystem.ReadFileInspector,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	providerCommandInjected bool,
	scriptCommandInjected bool,
	providerRegistry *providerregistry.Registry,
	providerRegistryRebinder ProviderRegistryRebinder,
) (workers.RuntimeService, error) {
	return workersinternal.NewRuntimeWithSelection(
		sessions,
		modelService,
		modelsScope,
		providerCommandRunner,
		scriptCommandRunner,
		allocator,
		logger,
		verbose,
		factoryRunnerID,
		invocationSkipPermissionsOverride,
		providerOverride,
		now,
		processEnvironment,
		currentWorkingDirectory,
		contentMaterializer,
		interpolation,
		executionPolicy,
		factoryDocs,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		worktreePreparer,
		agentRunHarness,
		retryRandom,
		workstationFiles,
		temporaryFiles,
		decisionEnvelopes,
		providerCommandInjected,
		scriptCommandInjected,
		providerRegistry,
		providerRegistryRebinder,
	)
}

// BuildRuntimeExecutors invokes the concrete Workers runtime implementation.
func BuildRuntimeExecutors(
	runtimeService workers.RuntimeService,
	runtimeConfig factorydefinitions.RuntimeConfigLookup,
	factoryConfig *factorydefinitions.FactoryConfig,
	factoryRunnerID string,
	workflowContext *workers.Context,
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	providerOverride workers.Provider,
	progressPublisher workers.ProgressPublisher,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
	modelRecorder workers.ModelEventRecorder,
	agentRunRecorder workers.AgentRunEventRecorder,
	now func() time.Time,
) (map[string]workers.WorkerExecutor, error) {
	return workersinternal.BuildRuntimeExecutors(
		runtimeService,
		runtimeConfig,
		factoryConfig,
		factoryRunnerID,
		workflowContext,
		logger,
		skipBuiltInRunnerPrerequisiteValidation,
		invocationSkipPermissionsOverride,
		providerOverride,
		progressPublisher,
		scriptRecorder,
		inferenceRecorder,
		modelRecorder,
		agentRunRecorder,
		now,
	)
}

// NewMockCommandRunner decorates a command edge with configured mock behavior.
func NewMockCommandRunner(
	config *workers.MockWorkersConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	next workers.CommandRunner,
) workers.CommandRunner {
	return workersinternal.NewMockCommandRunner(config, runtimeConfig, next)
}

// LocalRuntimeHooks returns Workers-owned recording hooks for the Models runtime.
func LocalRuntimeHooks() models.LocalRuntimeHooks {
	return workersinternal.LocalRuntimeHooks()
}

// NewInvocation constructs the narrow direct-invocation role.
func NewInvocation(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewInvocation(
		commandRunner,
		commandClock,
		allocator,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		temporaryFileSystems...,
	)
}

// NewInvocationWithProgress constructs direct invocation with provider progress publishing.
func NewInvocationWithProgress(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewInvocationWithProgress(
		commandRunner,
		commandClock,
		allocator,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		progressPublisher,
		temporaryFileSystems...,
	)
}

// NewConductorInvocationWithProgress routes external integrations through the conductor.
func NewConductorInvocationWithProgress(
	registry *providerregistry.Registry,
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewConductorInvocationWithProgress(
		registry,
		commandRunner,
		commandClock,
		allocator,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		progressPublisher,
		temporaryFileSystems...,
	)
}

// NewProviderFromCommandRunner constructs one provider-backed worker from a command runner.
func NewProviderFromCommandRunner(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workerprovider.Provider, error) {
	return workersinternal.NewProviderFromCommandRunner(
		commandRunner,
		commandClock,
		allocator,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		temporaryFileSystems...,
	)
}

// ResolveTemplateFields exposes the Workers-owned template resolver for composition.
func ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	workflowContext *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	return workersinternal.ResolveTemplateFields(
		workingDirectory,
		environment,
		tokens,
		workflowContext,
		worktree,
	)
}
