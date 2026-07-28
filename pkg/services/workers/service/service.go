// Package service owns Worker execution construction and direct invocation.
package service

import (
	"context"
	"fmt"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/construction"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	providerconductor "github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	"github.com/portpowered/infinite-you/pkg/services/workers/skippermissions"
	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service is the canonical Worker execution application service.
type Service struct {
	workersinternal.Root
	sessions                          CurrentRuntimeResolver
	models                            models.Service
	modelsScope                       models.RuntimeScopeRef
	providerFactory                   *workerprovider.Factory
	scriptFactory                     *workerexecutor.ScriptFactory
	executorBuilder                   workerconstruction.Builder
	providerCommandRunner             workers.CommandRunner
	scriptCommandRunner               workers.CommandRunner
	providerCommandInjected           bool
	scriptCommandInjected             bool
	logger                            *zap.Logger
	verbose                           bool
	factoryRunnerID                   string
	invocationSkipPermissionsOverride *bool
	providerOverride                  providercontract.Provider
	clock                             func() time.Time
	processEnvironment                func() []string
	currentWorkingDirectory           func() (string, error)
	modelInvocationExecutorOverride   ModelInvocationExecutor
	interpolation                     interfaces.InvocationInterpolationService
	executionPolicy                   interfaces.WorkstationExecutionPolicyService
	decisionEnvelopes                 interfaces.DecisionEnvelopeService
	factoryDocs                       workers.FactoryDocsLoader
	worktreePreparer                  workers.FactoryWorktreePreparer
	agentRunHarness                   workeragentrun.HarnessAdapter
	retryRandom                       platformrandom.Source
	workstationFiles                  platformfilesystem.ReadFileInspector
	temporaryFiles                    platformfilesystem.TemporaryFileSystem
	executableLocator                 platformprocess.ExecutableLocator
	providerRegistry                  *providerregistry.Registry
	providerRegistryRebinder          ProviderRegistryRebinder
	invocationConductor               *providerconductor.Conductor
	agentDispatchUsesRegisteredRunner bool
}

var _ workers.RuntimeService = (*Service)(nil)

type CurrentRuntimeResolver interface {
	CurrentRuntime() *factorysessions.LiveRuntime
}

// ModelInvocationExecutor builds a direct executor for one model-bound Worker.
type ModelInvocationExecutor func(
	interfaces.RuntimeConfigLookup,
	*interfaces.FactoryConfig,
	string,
) (workers.WorkstationRequestExecutor, error)

type workflowContextProvider interface {
	WorkflowContext() *workerexecution.Context
}

// New constructs a Worker execution service from injected dependencies.
// backendsizecheck:ignore-function service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
// pkgmaintcheck:ignore-function-lines service-ownership migration preserves this orchestration flow; extract focused helpers and remove this exemption.
func New(
	sessions CurrentRuntimeResolver,
	modelService models.Service,
	providerCommandRunner workers.CommandRunner,
	scriptCommandRunner workers.CommandRunner,
	agyPTYAllocator agypty.PTYAllocator,
	logger *zap.Logger,
	verbose bool,
	factoryRunnerID string,
	invocationSkipPermissionsOverride *bool,
	providerOverride providercontract.Provider,
	clock func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	modelInvocationExecutor ModelInvocationExecutor,
	contentMaterializer work.ContentMaterializer,
	interpolation interfaces.InvocationInterpolationService,
	executionPolicy interfaces.WorkstationExecutionPolicyService,
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
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) (*Service, error) {
	if sessions == nil {
		return nil, fmt.Errorf("construct Worker execution service: Factory Session runtime is required")
	}
	if modelService == nil {
		return nil, fmt.Errorf("construct Worker execution service: Models service is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("construct Worker execution service: logger is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("construct Worker execution service: clock is required")
	}
	if processEnvironment == nil {
		return nil, fmt.Errorf("construct Worker execution service: process environment is required")
	}
	if currentWorkingDirectory == nil {
		return nil, fmt.Errorf("construct Worker execution service: current working directory resolver is required")
	}
	if worktreePreparer == nil {
		return nil, fmt.Errorf("construct Worker execution service: worktree preparer is required")
	}
	if agentRunHarness == nil {
		return nil, fmt.Errorf("construct Worker execution service: agent-run harness is required")
	}
	if retryRandom == nil {
		return nil, fmt.Errorf("construct Worker execution service: provider retry random source is required")
	}
	if workstationFiles == nil {
		return nil, fmt.Errorf("construct Worker execution service: workstation filesystem is required")
	}
	if temporaryFiles == nil {
		return nil, fmt.Errorf("construct Worker execution service: provider temporary filesystem is required")
	}
	providerFactory, scriptFactory, providerRunner, scriptRunner, err := buildExecutionFactories(
		providerCommandRunner, scriptCommandRunner, workerprocess.ClockFunc(clock), agyPTYAllocator,
		factoryDocs, resolveSymlinks, executableLocator, executableInspector, executableFiles, operatingSystem,
		temporaryFiles,
	)
	if err != nil {
		return nil, err
	}
	providerFactory = providerFactory.WithContentMaterializer(contentMaterializer)
	decisionEnvelopeService := firstDecisionEnvelopeService(decisionEnvelopes)
	executorBuilder := workerconstruction.New(
		providerFactory,
		scriptFactory,
		interpolation,
		executionPolicy,
		factoryDocs,
		worktreePreparer,
		agentRunHarness,
		retryRandom,
		workstationFiles,
		decisionEnvelopeService,
	)
	return &Service{
		sessions:                          sessions,
		models:                            modelService,
		providerFactory:                   providerFactory,
		scriptFactory:                     scriptFactory,
		executorBuilder:                   executorBuilder,
		providerCommandRunner:             providerRunner,
		scriptCommandRunner:               scriptRunner,
		providerCommandInjected:           providerCommandRunner != nil,
		scriptCommandInjected:             scriptCommandRunner != nil,
		logger:                            logger,
		verbose:                           verbose,
		factoryRunnerID:                   factoryRunnerID,
		invocationSkipPermissionsOverride: invocationSkipPermissionsOverride,
		providerOverride:                  providerOverride,
		clock:                             clock,
		processEnvironment:                processEnvironment,
		currentWorkingDirectory:           currentWorkingDirectory,
		modelInvocationExecutorOverride:   modelInvocationExecutor,
		interpolation:                     interpolation,
		executionPolicy:                   executionPolicy,
		decisionEnvelopes:                 decisionEnvelopeService,
		factoryDocs:                       factoryDocs,
		worktreePreparer:                  worktreePreparer,
		agentRunHarness:                   agentRunHarness,
		retryRandom:                       retryRandom,
		workstationFiles:                  workstationFiles,
		temporaryFiles:                    temporaryFiles,
		executableLocator:                 executableLocator,
	}, nil
}

func firstDecisionEnvelopeService(
	services []interfaces.DecisionEnvelopeService,
) interfaces.DecisionEnvelopeService {
	if len(services) == 0 {
		return nil
	}
	return services[0]
}

// BuildRuntime delegates the singular Workers root operation to its
// parent-private Runtime Assembly capability.
func (s *Service) BuildRuntime(
	ctx context.Context,
	request workers.RuntimeBuildRequest,
) (workers.RuntimeBuildResult, error) {
	if s == nil {
		return workers.RuntimeBuildResult{}, fmt.Errorf(
			"%w: Workers Runtime Assembly is required",
			workers.ErrIncompleteRuntimeAssembly,
		)
	}
	return s.Root.BuildRuntime(ctx, request)
}

// StartWorkstationPool delegates lifecycle activation to the parent-private
// workstation capability.
func (s *Service) StartWorkstationPool(
	ctx context.Context,
	request workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	if s == nil {
		return workers.WorkstationPoolStartResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return s.Root.StartWorkstationPool(ctx, request)
}

// StopWorkstationPool delegates terminal shutdown to the parent-private
// workstation capability.
func (s *Service) StopWorkstationPool(
	ctx context.Context,
) (workers.WorkstationPoolStopResult, error) {
	if s == nil {
		return workers.WorkstationPoolStopResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return s.Root.StopWorkstationPool(ctx)
}

// WorkstationRoute reports availability through the private lifecycle owner.
func (s *Service) WorkstationRoute(
	ctx context.Context,
	request workers.WorkstationRouteRequest,
) (workers.WorkstationRouteResult, error) {
	if s == nil {
		return workers.WorkstationRouteResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return s.Root.WorkstationRoute(ctx, request)
}

// DispatchWorkstation delegates execution to the private workstation owner.
func (s *Service) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	if s == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return s.Root.DispatchWorkstation(ctx, request)
}

// CancelWorkstationDispatch delegates explicit cancellation to the private
// workstation owner.
func (s *Service) CancelWorkstationDispatch(
	ctx context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	if s == nil {
		return workers.WorkstationDispatchCancelResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return s.Root.CancelWorkstationDispatch(ctx, request)
}

func (s *Service) modelInvocationExecutor(
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerName string,
) (workers.WorkstationRequestExecutor, error) {
	if s != nil && s.modelInvocationExecutorOverride != nil {
		return s.modelInvocationExecutorOverride(runtimeCfg, factoryCfg, workerName)
	}
	return s.BuildModelInvocationExecutor(runtimeCfg, factoryCfg, workerName)
}

// CurrentModelRuntimeConfig returns the selected session's runtime configuration.
func (s *Service) CurrentModelRuntimeConfig() interfaces.RuntimeConfigLookup {
	if s == nil || s.sessions == nil {
		return nil
	}
	runtime := s.sessions.CurrentRuntime()
	if runtime == nil {
		return nil
	}
	return runtime.RuntimeConfig
}

// BuildModelInvocationExecutor constructs a direct executor through the same
// canonical Worker constructor used by Factory runtimes.
func (s *Service) BuildModelInvocationExecutor(runtimeCfg interfaces.RuntimeConfigLookup, factoryCfg *interfaces.FactoryConfig, workerName string) (workers.WorkstationRequestExecutor, error) {
	if s == nil || runtimeCfg == nil || factoryCfg == nil {
		return nil, fmt.Errorf("runtime config is required")
	}
	if s.executorBuilder == nil {
		return nil, fmt.Errorf("Worker application is required")
	}
	workerDef, ok := runtimeCfg.Worker(workerName)
	if !ok || workerDef == nil {
		return nil, fmt.Errorf("worker %q is not configured", workerName)
	}
	if err := skippermissions.ValidateInvocationSkipPermissionsForWorker(workerDef, s.invocationSkipPermissionsOverride); err != nil {
		return nil, fmt.Errorf("worker %q: %w", workerName, err)
	}
	var workflowContext *workerexecution.Context
	if selected := s.sessions.CurrentRuntime(); selected != nil && selected.Factory != nil {
		if provider, ok := selected.Factory.(workflowContextProvider); ok {
			workflowContext = provider.WorkflowContext()
		}
	}
	result, err := s.executorBuilder.Build(
		runtimeCfg, workerName, s.factoryRunnerID, workflowContext,
		logging.NewZapLogger(s.logger, s.verbose),
		s.invocationSkipPermissionsOverride, s.providerOverride,
		nil, nil, nil, nil, s.clock, s.processEnvironment, s.currentWorkingDirectory,
		s.runtimeRunnerDecorators(runtimeCfg, factoryCfg, nil, s.clock, s.providerOverride == nil, nil),
	)
	if err != nil {
		return nil, fmt.Errorf("construct model worker %q: %w", workerName, err)
	}
	if result.Direct == nil {
		return nil, fmt.Errorf("model worker %q does not support direct invocation", workerName)
	}
	return result.Direct, nil
}
