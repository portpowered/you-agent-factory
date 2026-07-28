package service

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
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/construction"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/execution/recording"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	runtimeassemblywire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/wire"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
	providerconductor "github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/services/testing"
	"go.uber.org/zap"
)

// NewRuntime constructs the public Workers runtime role.
func NewRuntime(
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
) (workers.RuntimeService, error) {
	runtimeService, err := New(
		sessions,
		modelService,
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
	assembly, err := newRuntimeAssembly(nil)
	if err != nil {
		return nil, err
	}
	runtimeService.runtimeAssembly = assembly
	runtimeService.workstations = workstationswire.NewService()
	return runtimeService, nil
}

// NewRuntimeWithSelection constructs the Workers runtime while preserving
// whether command runners came from an external edge or Wire's production
// adapter selection.
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
	runtimeService, err := NewRuntime(
		sessions, modelService, modelsScope, providerCommandRunner, scriptCommandRunner,
		allocator, logger, verbose, factoryRunnerID, invocationSkipPermissionsOverride,
		providerOverride, now, processEnvironment, currentWorkingDirectory, contentMaterializer, interpolation, executionPolicy,
		factoryDocs, resolveSymlinks, executableLocator, executableInspector, executableFiles, operatingSystem,
		worktreePreparer, agentRunHarness, retryRandom, workstationFiles, temporaryFiles, decisionEnvelopes,
	)
	if err != nil {
		return nil, err
	}
	service := runtimeService.(*Service)
	service.providerCommandInjected = providerCommandInjected
	service.scriptCommandInjected = scriptCommandInjected
	service.providerRegistry = providerRegistry
	service.providerRegistryRebinder = providerRegistryRebinder
	if providerRegistry != nil {
		assembly, assemblyErr := newRuntimeAssembly(providerRegistry)
		if assemblyErr != nil {
			return nil, assemblyErr
		}
		service.runtimeAssembly = assembly
		service.invocationConductor = providerconductor.New(providerRegistry)
		if builder, ok := service.executorBuilder.(*workerconstruction.Service); ok {
			service.executorBuilder = builder.
				WithRunnerSelection(providerRegistry.ResolveRunnerSelection).
				WithProviderIdentityResolution(providerRegistry.CanonicalIdentity).
				WithProviderRegistry(providerRegistry).
				WithAgentRunnerCutover(true)
		}
		service.agentDispatchUsesRegisteredRunner = true
	}
	return service, nil
}

func newRuntimeAssembly(
	registry *providerregistry.Registry,
) (runtimeassembly.Service, error) {
	registrations, err := runtimeAssemblyRegistrations(registry)
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
	registry *providerregistry.Registry,
) ([]runners.Registration, error) {
	implementation := runtimeAssemblyRunner{}
	if registry != nil {
		entries := registry.Entries()
		registrations := make([]runners.Registration, 0, len(entries))
		for _, entry := range entries {
			if !entry.Selectable() {
				continue
			}
			metadata, err := registry.RunnerMetadata(string(entry.Identity()))
			if err != nil {
				return nil, fmt.Errorf(
					"construct Workers Runtime Assembly runner metadata %q: %w",
					entry.Identity(),
					err,
				)
			}
			registrations = append(registrations, runners.Registration{
				Identity: metadata.ID,
				Metadata: metadata,
				Runner:   implementation,
			})
		}
		return registrations, nil
	}

	identities := []string{
		workers.RunnerIDAgy,
		workers.RunnerIDCodex,
		workers.RunnerIDCursorCLI,
		workers.RunnerIDGemini,
		workers.RunnerIDKiro,
		workers.RunnerIDOpenCode,
		workers.RunnerIDPi,
	}
	registrations := make([]runners.Registration, 0, len(identities))
	for _, identity := range identities {
		metadata, _ := workers.BuiltInRunnerMetadata(identity)
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
	providerOverride workers.Provider,
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
	return &mockworker.MockWorkerCommandRunner{
		Config:        config,
		RuntimeConfig: runtimeConfig,
		Next:          next,
	}
}

// LocalRuntimeHooks returns the Workers-owned recording hooks consumed by the
// Models runtime.
func LocalRuntimeHooks() models.LocalRuntimeHooks {
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
