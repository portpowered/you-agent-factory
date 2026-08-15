package internal

import (
	"context"
	"fmt"
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
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	runtimeassemblywire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/wire"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution/recording"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
	"go.uber.org/zap"
)

// NewRuntime constructs the public Workers runtime role.
func NewRuntime(
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
) (workers.RuntimeService, error) {
	runtimeService, err := New(
		modelService,
		providersService,
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
		nil,
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
	)
	if err != nil {
		return nil, err
	}
	runtimeService.modelsScope = modelsScope
	assembly, err := newRuntimeAssembly(providersService)
	if err != nil {
		return nil, err
	}
	runtimeService.Root = RootFrom(
		assembly,
		workstationswire.NewService(logging.NewZapLogger(logger, verbose)),
	)
	return runtimeService, nil
}

// NewConfiguredRuntime constructs the legacy runtime compatibility role while preserving
// whether command runners came from an external edge or Wire's production
// adapter selection.
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
	providersRebinder ProvidersRebinder,
	statelessExecute workers.Service,
) (workers.RuntimeService, error) {
	runtimeService, err := NewRuntime(
		modelService, providersService, modelsScope, providerCommandRunner, scriptCommandRunner,
		progressPublisher, allocator, logger, verbose, factoryRunnerID, runWorktree, workerReasoningEffort, invocationSkipPermissionsOverride,
		providerOverride, now, processEnvironment, currentWorkingDirectory, contentMaterializer, interpolation, executionPolicy,
		factoryDocs, resolveSymlinks, executableLocator, executableInspector, executableFiles, operatingSystem,
		worktreePreparer, agentRunHarness, retryRandom, workstationFiles, temporaryFiles, decisionEnvelopes,
	)
	if err != nil {
		return nil, err
	}
	service := runtimeService.(*Service)
	if statelessExecute == nil {
		return nil, fmt.Errorf("construct Workers Runtime: stateless Execute service is required")
	}
	service.Root = service.Root.ReplaceExecute(statelessExecute)
	service.providerCommandInjected = providerCommandInjected
	service.scriptCommandInjected = scriptCommandInjected
	service.providerLifecycles = &ownedProviderLifecycles{}
	if providersLifecycleOwned {
		service.providerLifecycles.Add(providersService)
	}
	service.providersRebinder = providersRebinder
	return service, nil
}

func newRuntimeAssembly(
	providersService providers.Service,
) (runtimeassembly.Service, error) {
	registrations, err := runtimeAssemblyRegistrations(providersService)
	if err != nil {
		return nil, err
	}
	return newRuntimeAssemblyFromRegistrations(registrations)
}

func newRuntimeAssemblyFromRegistrations(
	registrations []runners.Registration,
) (runtimeassembly.Service, error) {
	runnerRegistry, err := runnerswire.NewService(registrations)
	if err != nil {
		return nil, fmt.Errorf("construct Workers Runtime Assembly runner registry: %w", err)
	}
	assembleBinding := func(
		_ context.Context,
		role workers.RuntimeBuildRoleRequest,
		_ workers.RuntimeBuildOpeningOptions,
		selection workers.ResolvedRunnerSelection,
	) (workers.AssembledRuntimeBinding, error) {
		return workers.AssembledRuntimeBinding{
			RoleName:        role.Name,
			RoleKind:        role.Kind,
			RunnerSelection: selection,
		}, nil
	}
	return runtimeassemblywire.NewService(runnerRegistry, assembleBinding)
}

type runtimeAssemblyRunner struct{}

func (runtimeAssemblyRunner) Execute(
	context.Context,
	workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	return workers.RunnerExecutionResult{}, fmt.Errorf(
		"%w: runtime assembly binding cannot execute",
		workers.ErrIncompleteRuntimeAssembly,
	)
}

func runtimeAssemblyRegistrations(
	providersService providers.Service,
) ([]runners.Registration, error) {
	implementation := runtimeAssemblyRunner{}
	if providersService != nil {
		listed, err := providersService.ListProviders(
			context.Background(),
			providers.ListProvidersRequest{},
		)
		if err != nil {
			return nil, fmt.Errorf("list Providers for runtime assembly: %w", err)
		}
		registrations := make([]runners.Registration, 0, len(listed.Providers))
		for _, descriptor := range listed.Providers {
			if descriptor.Availability != providers.AvailabilitySelectable {
				continue
			}
			metadata := runnerMetadataFromProvider(descriptor)
			registrations = append(registrations, runners.Registration{
				Identity: metadata.ID,
				Metadata: metadata,
				Runner:   implementation,
			})
		}
		return registrations, nil
	}

	identities := []string{
		workers.RunnerIDAntigravity,
		workers.RunnerIDClaude,
		workers.RunnerIDCodex,
	}
	registrations := make([]runners.Registration, 0, len(identities))
	for _, identity := range identities {
		metadata, _ := workerrunner.BuiltInRunnerMetadata(identity)
		registrations = append(registrations, runners.Registration{
			Identity: identity,
			Metadata: metadata,
			Runner:   implementation,
		})
	}
	return registrations, nil
}

// BuildRuntimeExecutors invokes the concrete Workers implementation selected
// by composition without exposing that implementation to Initializer.
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
	service, ok := runtimeService.(*Service)
	if !ok || service == nil {
		return nil, fmt.Errorf("Workers runtime service has an unsupported implementation")
	}
	return service.BuildRuntimeExecutors(
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
	return &runnermockworker.MockWorkerCommandRunner{
		Config:        config,
		RuntimeConfig: runtimeConfig,
		Next:          next,
	}
}

// LocalRuntimeHooks returns the Workers-owned recording hooks consumed by the
// Models runtime.
func LocalRuntimeHooks() workers.LocalRuntimeHooks {
	return modelrecording.Hooks()
}

func resolveInferenceRunner(
	inner workers.Runner,
	modelsService models.Service,
	modelsScope models.RuntimeScopeRef,
	factoryCfg *factorydefinitions.FactoryConfig,
	workerCfg *factorydefinitions.FactoryWorkerConfig,
) workers.Runner {
	if factoryCfg == nil {
		return runnerswire.NewInferenceCompositionRunner(
			inner, modelsService, modelsScope, workerCfg, nil,
		)
	}
	return runnerswire.NewInferenceCompositionRunner(
		inner,
		modelsService,
		modelsScope,
		workerCfg,
		factoryCfg.Resources,
	)
}
