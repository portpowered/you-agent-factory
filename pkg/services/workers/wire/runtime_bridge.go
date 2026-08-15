package wire

import (
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"go.uber.org/zap"
)

// ProviderRegistryRebinder reconstructs the provider registry for runtime command edges.
type ProviderRegistryRebinder = workersinternal.ProviderRegistryRebinder

// NewConfiguredRuntime constructs the legacy runtime compatibility role through
// the owner-internal implementation. Process composition uses NewService and
// never invokes this compatibility path.
func NewConfiguredRuntime(
	modelService models.Service,
	providersService providers.Service,
	modelsScope models.RuntimeScopeRef,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	allocator workers.PTYAllocator,
	logger *zap.Logger,
	verbose bool,
	factoryRunnerID string,
	runWorktree string,
	workerReasoningEffort string,
	invocationSkipPermissionsOverride *bool,
	providerOverride providers.Service,
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
	providersLifecycleOwned bool,
	providerRegistry workers.ProviderRegistry,
	providerRegistryRebinder ProviderRegistryRebinder,
	statelessExecute workers.Service,
) (workers.RuntimeService, error) {
	return workersinternal.NewConfiguredRuntime(
		modelService,
		providersService,
		modelsScope,
		providerCommandRunner,
		scriptCommandRunner,
		progressPublisher,
		allocator,
		logger,
		verbose,
		factoryRunnerID,
		runWorktree,
		workerReasoningEffort,
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
		providersLifecycleOwned,
		providerRegistry,
		providerRegistryRebinder,
		statelessExecute,
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
	providerOverride providers.Service,
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
func LocalRuntimeHooks() workers.LocalRuntimeHooks {
	return workersinternal.LocalRuntimeHooks()
}

// NewInvocation constructs the narrow direct-invocation role.
func NewInvocation(
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewInvocation(
		providersService,
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
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewInvocationWithProgress(
		providersService,
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
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return workersinternal.NewConductorInvocationWithProgress(
		providersService,
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
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (providers.Service, error) {
	return workersinternal.NewProviderFromService(providersService)
}

// NewProviderInvocationExecutor constructs the executor behind
// workers.ProviderInvocationRoute: the Worker whose every selection -- prompt,
// model, provider, reasoning effort, output schema -- arrives on the execution
// request rather than being rendered from an authored workstation.
//
// This is the one construction that lets an orchestrator whose Workers have no
// Factory definition behind them -- a JavaScript workflow's agent.run children
// -- still reach the provider through Workers' own pool, admission, and
// cancellation, and therefore through Worker Sessions supervision. A nil
// invocation boundary yields a nil executor so composition treats "no provider
// invocation available" as an absent route rather than a route that fails at
// dispatch time.
func NewProviderInvocationExecutor(
	invocation workers.InvocationExecutor,
) workers.WorkstationRequestExecutor {
	executor := workerexecutor.NewProviderInvocationExecutor(invocation)
	if executor == nil {
		return nil
	}
	return executor
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
