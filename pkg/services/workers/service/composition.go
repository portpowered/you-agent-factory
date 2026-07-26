package service

import (
	"context"
	"fmt"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/construction"
	modelrecording "github.com/portpowered/infinite-you/pkg/services/workers/execution/recording"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	runtimeassembly "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly"
	runtimeassemblywire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/wire"
	providerconductor "github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	hostedworkers "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/workers/services/hosted_logic/linear"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/services/testing"
	"go.uber.org/zap"
)

// NewRuntime constructs the public Workers runtime role.
func NewRuntime(
	sessions CurrentRuntimeResolver,
	modelService models.Service,
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
	assembly, err := newRuntimeAssembly(nil)
	if err != nil {
		return nil, err
	}
	runtimeService.runtimeAssembly = assembly
	return runtimeService, nil
}

// NewRuntimeWithSelection constructs the Workers runtime while preserving
// whether command runners came from an external edge or Wire's production
// adapter selection.
func NewRuntimeWithSelection(
	sessions CurrentRuntimeResolver,
	modelService models.Service,
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
) (workers.RuntimeService, error) {
	runtimeService, err := NewRuntime(
		sessions, modelService, providerCommandRunner, scriptCommandRunner,
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
				WithProviderIdentityResolution(providerRegistry.CanonicalIdentity)
		}
	}
	return service, nil
}

func newRuntimeAssembly(
	registry *providerregistry.Registry,
) (runtimeassembly.Service, error) {
	resolveRunner := func(
		_ context.Context,
		identity string,
	) (workers.ResolvedRunnerSelection, bool, error) {
		canonical := workers.NormalizeRunnerID(identity)
		if registry != nil {
			resolved, err := registry.CanonicalIdentity(canonical)
			if err != nil {
				return workers.ResolvedRunnerSelection{}, false, nil
			}
			canonical = resolved
		} else if !workers.IsBuiltInRunnerID(canonical) {
			return workers.ResolvedRunnerSelection{}, false, nil
		}
		return workers.ResolvedRunnerSelection{
			RunnerID: canonical,
			Source:   workers.RunnerSelectionSourceFactory,
		}, true, nil
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
	return runtimeassemblywire.NewService(resolveRunner, assembleBinding)
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
	providerOverride workerprovider.Provider,
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

// NewHostedPollers constructs Workers-owned hosted integration pollers.
func NewHostedPollers(
	logger *zap.Logger,
	clock hostedworkers.Clock,
	httpClient hostedlinear.HTTPDoer,
	secretResolver hostedlinear.SecretResolver,
	linearEndpoint string,
) automations.HostedPollers {
	return hostedworkers.New(logger, clock, httpClient, secretResolver, linearEndpoint)
}

// LocalRuntimeHooks returns the Workers-owned recording hooks consumed by the
// Models runtime.
func LocalRuntimeHooks() models.LocalRuntimeHooks {
	return modelrecording.Hooks()
}
