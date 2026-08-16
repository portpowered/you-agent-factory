// Package construction owns inert construction of configured worker executors
// for the private Workers Runtime Assembly subservice.
package construction

import (
	"fmt"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// Builder constructs one configured worker without owning runtime lifecycle.
type Builder interface {
	Build(
		interfaces.RuntimeConfigLookup,
		string,
		string,
		*workerexecution.Context,
		logging.Logger,
		*bool,
		providers.Service,
		workers.ProgressPublisher,
		workers.ScriptEventRecorder,
		workers.InferenceEventRecorder,
		workeragentrun.AgentRunEventRecorder,
		func() time.Time,
		func() []string,
		func() (string, error),
		[]RunnerDecorator,
	) (Result, error)
	BuildLogical(
		interfaces.RuntimeConfigLookup,
		string,
		string,
		*workerexecution.Context,
		logging.Logger,
		func() time.Time,
		func() []string,
		func() (string, error),
	) Result
}

// RunnerDecorator adds session-owned behavior to a provider-backed runner.
// Decorators run in declaration order after provider recording is attached.
type RunnerDecorator func(workers.Runner, *interfaces.FactoryWorkerConfig) workers.Runner

// Result exposes both the dispatch executor and its direct-invocation boundary.
// Direct is nil for workers that do not support workstation request execution.
type Result struct {
	Dispatch workers.WorkerExecutor
	Direct   workers.WorkstationRequestExecutor
}

// Service is a stateless worker executor constructor.
type Service struct {
	providers          providers.Service
	interpolation      interfaces.InvocationInterpolationService
	executionPolicy    interfaces.WorkstationExecutionPolicyService
	decisionEnvelopes  interfaces.DecisionEnvelopeService
	factoryDocs        workers.FactoryDocsLoader
	worktreePreparer   workers.FactoryWorktreePreparer
	runWorktree        string
	runReasoningEffort string
	agentRunHarness    workeragentrun.HarnessAdapter
	retryRandom        platformrandom.Source
	workstationFiles   platformfilesystem.ReadFileInspector
	resolveRunner      workers.RunnerSelectionResolver
	resolveProvider    workers.ProviderIdentityResolver
}

// New constructs a worker executor service from process-owned factories.
func New(
	providerFactory providers.Service,
	interpolation interfaces.InvocationInterpolationService,
	executionPolicy interfaces.WorkstationExecutionPolicyService,
	factoryDocs workers.FactoryDocsLoader,
	worktreePreparer workers.FactoryWorktreePreparer,
	agentRunHarness workeragentrun.HarnessAdapter,
	retryRandom platformrandom.Source,
	workstationFiles platformfilesystem.ReadFileInspector,
	decisionEnvelopes ...interfaces.DecisionEnvelopeService,
) *Service {
	var selected interfaces.DecisionEnvelopeService
	if len(decisionEnvelopes) > 0 {
		selected = decisionEnvelopes[0]
	}
	return &Service{
		providers:         providerFactory,
		interpolation:     interpolation,
		executionPolicy:   executionPolicy,
		factoryDocs:       factoryDocs,
		worktreePreparer:  worktreePreparer,
		agentRunHarness:   agentRunHarness,
		retryRandom:       retryRandom,
		workstationFiles:  workstationFiles,
		decisionEnvelopes: selected,
	}
}

// WithRunWorktree returns a service copy that applies one run-scoped worktree
// selection to every non-logical workstation dispatch.
func (s *Service) WithRunWorktree(worktree string) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.runWorktree = worktree
	return &clone
}

// WithRunReasoningEffort returns a service copy that applies one run-scoped
// reasoning-effort selection to every provider-backed workstation dispatch.
func (s *Service) WithRunReasoningEffort(reasoningEffort string) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.runReasoningEffort = reasoningEffort
	return &clone
}

// WithRunnerSelection returns a service copy that uses the process-owned
// provider authority for runner identity and alias resolution.
func (s *Service) WithRunnerSelection(resolve workers.RunnerSelectionResolver) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.resolveRunner = resolve
	return &clone
}

// WithProviderIdentityResolution returns a service copy that validates
// invocation-resolved provider values through the process-owned authority.
func (s *Service) WithProviderIdentityResolution(resolve workers.ProviderIdentityResolver) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.resolveProvider = resolve
	return &clone
}

// WithExecutionFactories returns a service copy that uses a replacement
// provider service while preserving the narrow resolver callbacks used by
// legacy construction tests.
func (s *Service) WithExecutionFactories(
	providerFactory providers.Service,
) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	if providerFactory != nil {
		clone.providers = providerFactory
	}
	return &clone
}

// Build constructs one configured worker executor from direct collaborators.
// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func (s *Service) Build(
	runtimeConfig interfaces.RuntimeConfigLookup,
	workerName string,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride providers.Service,
	inferenceProgressPublisher workers.ProgressPublisher,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workers.InferenceEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	clock func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	runnerDecorators []RunnerDecorator,
) (Result, error) {
	if runtimeConfig == nil {
		return Result{}, fmt.Errorf("worker runtime config is required")
	}
	if clock == nil {
		return Result{}, fmt.Errorf("worker clock is required")
	}
	if s == nil || s.workstationFiles == nil {
		return Result{}, fmt.Errorf("Worker workstation filesystem is required")
	}
	if s == nil || s.factoryDocs == nil {
		return Result{}, fmt.Errorf("Worker Factory docs loader is required")
	}
	def, ok := runtimeConfig.Worker(workerName)
	if !ok || def == nil {
		return Result{}, nil
	}
	providersService := s.providers
	if providerOverride != nil {
		providersService = providerOverride
	}
	return s.buildConfiguredWorker(
		runtimeConfig, def, factoryRunnerID, workflowContext, logger,
		invocationSkipPermissionsOverride, providerOverride, providersService,
		inferenceProgressPublisher, inferenceRecorder, agentRunRecorder,
		clock, processEnvironment, currentWorkingDirectory, runnerDecorators,
	)
}

func (s *Service) buildConfiguredWorker(
	runtimeConfig interfaces.RuntimeConfigLookup,
	def *interfaces.FactoryWorkerConfig,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride providers.Service,
	providersService providers.Service,
	inferenceProgressPublisher workers.ProgressPublisher,
	inferenceRecorder workers.InferenceEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	clock func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	runnerDecorators []RunnerDecorator,
) (Result, error) {
	switch def.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		// Provider-backed attempts are admitted by the request-scoped Workers
		// service. Keeping a WorkstationExecutor binding here would recreate the
		// compatibility route that the runtime executor map intentionally omits.
		return Result{}, nil
	case interfaces.WorkstationTypeLogical:
		return s.buildLogicalWorker(
			runtimeConfig, factoryRunnerID, workflowContext, logger, providersService,
			clock, processEnvironment, currentWorkingDirectory,
		)
	default:
		return Result{}, nil
	}
}

func (s *Service) buildLogicalWorker(
	runtimeConfig interfaces.RuntimeConfigLookup,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	providersService providers.Service,
	clock func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
) (Result, error) {
	return workstationResult(runtimeConfig, clock), nil
}

// BuildLogical constructs the dispatch boundary for a workerless logical workstation.
func (s *Service) BuildLogical(
	runtimeConfig interfaces.RuntimeConfigLookup,
	_ string,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	clock func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
) Result {
	if s == nil {
		return Result{}
	}
	return workstationResult(runtimeConfig, clock)
}

func workstationResult(
	runtimeConfig interfaces.RuntimeConfigLookup,
	clock func() time.Time,
) Result {
	return Result{
		Dispatch: &workerexecutor.WorkstationExecutor{
			Now:           clock,
			RuntimeConfig: runtimeConfig,
		},
	}
}
