// Package construction owns inert construction of configured worker executors
// for the private Workers Runtime Assembly subservice.
package construction

import (
	"context"
	"fmt"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	providerconductor "github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/providersroot"
	providerregistry "github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
	providerstructured "github.com/portpowered/infinite-you/pkg/services/workers/provider/structured"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/skippermissions"
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
		providercontract.Provider,
		workerprovider.InferenceProgressPublisher,
		workerexecutor.ScriptEventRecorder,
		workerprovider.InferenceEventRecorder,
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
	providerFactory   *workerprovider.Factory
	scriptFactory     *workerexecutor.ScriptFactory
	interpolation     interfaces.InvocationInterpolationService
	executionPolicy   interfaces.WorkstationExecutionPolicyService
	decisionEnvelopes interfaces.DecisionEnvelopeService
	factoryDocs       workers.FactoryDocsLoader
	worktreePreparer  workers.FactoryWorktreePreparer
	agentRunHarness   workeragentrun.HarnessAdapter
	retryRandom       platformrandom.Source
	workstationFiles                  platformfilesystem.ReadFileInspector
	resolveRunner                     workers.RunnerSelectionResolver
	resolveProvider                   workers.ProviderIdentityResolver
	providerRegistry                  *providerregistry.Registry
	agentDispatchUsesRegisteredRunner bool
}

// New constructs a worker executor service from process-owned factories.
func New(
	providerFactory *workerprovider.Factory,
	scriptFactory *workerexecutor.ScriptFactory,
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
		providerFactory:   providerFactory,
		scriptFactory:     scriptFactory,
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

// WithProviderRegistry returns a service copy that can route agent dispatch
// through conductor-backed provider integrations on the Providers root.
func (s *Service) WithProviderRegistry(registry *providerregistry.Registry) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.providerRegistry = registry
	return &clone
}

// WithAgentRunnerCutover returns a service copy that resolves agent dispatch
// through the registered parent-private Agent Runner and injected Providers
// root instead of the superseded provider-factory runner path.
func (s *Service) WithAgentRunnerCutover(enabled bool) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	clone.agentDispatchUsesRegisteredRunner = enabled
	return &clone
}

// WithExecutionFactories returns a service copy that uses replacement provider
// and script factories while preserving registry-backed runner selection and
// provider-identity resolution wiring.
func (s *Service) WithExecutionFactories(
	providerFactory *workerprovider.Factory,
	scriptFactory *workerexecutor.ScriptFactory,
) *Service {
	if s == nil {
		return nil
	}
	clone := *s
	if providerFactory != nil {
		clone.providerFactory = providerFactory
	}
	if scriptFactory != nil {
		clone.scriptFactory = scriptFactory
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
	providerOverride providercontract.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	scriptRecorder workerexecutor.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
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
	switch def.Type {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeAgent, interfaces.WorkerTypeInference:
		effectiveSkipPermissions := effectiveWorkerSkipPermissions(def, invocationSkipPermissionsOverride)
		runner, err := s.agentRunner(
			runtimeConfig, def, logger, effectiveSkipPermissions,
			providerOverride, inferenceProgressPublisher,
		)
		if err != nil {
			return Result{}, err
		}
		runner = decorateProviderRunner(runner, def, runnerDecorators, effectiveSkipPermissions)
		runner = recordProviderRunner(runner, inferenceRecorder, clock)
		inference := workerexecutor.NewAgentExecutorWithRunner(
			runtimeConfig,
			runner,
			logger,
			clock,
			s.decisionEnvelopes,
		)
		agentRun := workeragentrun.NewAgentRunExecutorWithDependencies(
			runtimeConfig,
			runner,
			logger,
			s.agentRunHarness,
			agentRunRecorder,
			clock,
			s.decisionEnvelopes,
		)
		direct := &workerexecutor.WorkstationBehaviorRouter{
			RuntimeConfig: runtimeConfig, InferenceExecutor: inference, AgentRunExecutor: agentRun,
		}
		return workstationResult(
			runtimeConfig, factoryRunnerID, workflowContext, logger, direct, s.interpolation,
			s.executionPolicy, clock, processEnvironment, currentWorkingDirectory, s.factoryDocs, s.worktreePreparer, s.workstationFiles,
			s.resolveRunner,
			s.resolveProvider,
		), nil
	case interfaces.WorkstationTypeLogical:
		return workstationResult(
			runtimeConfig, factoryRunnerID, workflowContext, logger, nil,
			s.interpolation, s.executionPolicy, clock, processEnvironment, currentWorkingDirectory, s.factoryDocs, s.worktreePreparer, s.workstationFiles,
			s.resolveRunner,
			s.resolveProvider,
		), nil
	case interfaces.WorkerTypeScript:
		if s == nil || s.scriptFactory == nil {
			return Result{}, fmt.Errorf("script worker factory is required")
		}
		direct, err := s.scriptFactory.New(
			def, logger, runtimeConfig.FactoryDir(),
			inferenceProgressPublisher, scriptRecorder, clock,
		)
		if err != nil {
			return Result{}, err
		}
		return workstationResult(
			runtimeConfig, factoryRunnerID, workflowContext, logger, direct, s.interpolation,
			s.executionPolicy, clock, processEnvironment, currentWorkingDirectory, s.factoryDocs, s.worktreePreparer, s.workstationFiles,
			s.resolveRunner,
			s.resolveProvider,
		), nil
	default:
		return Result{}, nil
	}
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
	if s == nil || s.factoryDocs == nil {
		return Result{}
	}
	return workstationResult(
		runtimeConfig,
		factoryRunnerID,
		workflowContext,
		logger,
		nil,
		s.interpolation,
		s.executionPolicy,
		clock,
		processEnvironment,
		currentWorkingDirectory,
		s.factoryDocs,
		s.worktreePreparer,
		s.workstationFiles,
		s.resolveRunner,
		s.resolveProvider,
	)
}

func (s *Service) agentRunner(
	runtimeConfig interfaces.RuntimeConfigLookup,
	def *interfaces.FactoryWorkerConfig,
	logger logging.Logger,
	effectiveSkipPermissions bool,
	providerOverride providercontract.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
) (workers.Runner, error) {
	if providerOverride != nil {
		return workerexecutor.RunnerFromProvider(providerOverride), nil
	}
	if s != nil && s.agentDispatchUsesRegisteredRunner {
		return s.resolveRegisteredAgentRunner(
			runtimeConfig,
			logger,
			effectiveSkipPermissions,
			inferenceProgressPublisher,
		)
	}
	return s.legacyProviderRunner(
		runtimeConfig,
		logger,
		effectiveSkipPermissions,
		inferenceProgressPublisher,
	)
}

func (s *Service) resolveRegisteredAgentRunner(
	runtimeConfig interfaces.RuntimeConfigLookup,
	logger logging.Logger,
	effectiveSkipPermissions bool,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
) (workers.Runner, error) {
	if s == nil || s.providerFactory == nil {
		return nil, fmt.Errorf("provider worker factory is required")
	}
	publish := agentProgressPublisherOrNoop(inferenceProgressPublisher)
	providersConfig := providersroot.Config{
		Factory:          s.providerFactory,
		SkipPermissions:  effectiveSkipPermissions,
		Logger:           logger,
		Publish:          publish,
		FactoryDirectory: strings.TrimSpace(runtimeConfig.FactoryDir()),
	}
	if s.providerRegistry != nil {
		providersConfig.ProviderRegistry = s.providerRegistry
		providersConfig.Conductor = providerconductor.New(s.providerRegistry)
	}
	providersRoot, err := providersroot.NewService(providersConfig)
	if err != nil {
		return nil, fmt.Errorf("construct Providers root for agent dispatch: %w", err)
	}
	registry, err := runnerswire.NewAgentRegistry(runners.AgentDependencies{
		Providers: providersRoot,
		Publish:   publish,
	})
	if err != nil {
		return nil, fmt.Errorf("construct agent runner registry: %w", err)
	}
	binding, err := registry.Resolve(runners.ResolutionRequest{
		Identity: runners.AgentIdentity,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve agent runner: %w", err)
	}
	if binding.Runner == nil {
		return nil, fmt.Errorf("resolve agent runner: runner is nil")
	}
	return binding.Runner, nil
}

func agentProgressPublisherOrNoop(
	publisher workerprovider.InferenceProgressPublisher,
) workerprovider.InferenceProgressPublisher {
	if publisher != nil {
		return publisher
	}
	return func(_ workers.ProgressFragment) {}
}

func (s *Service) legacyProviderRunner(
	runtimeConfig interfaces.RuntimeConfigLookup,
	logger logging.Logger,
	effectiveSkipPermissions bool,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
) (workers.Runner, error) {
	if s == nil || s.providerFactory == nil {
		return nil, fmt.Errorf("provider worker factory is required")
	}
	var responseExecutor workerprovider.ResponseStreamExecutor
	if inferenceProgressPublisher != nil {
		responseExecutor = providerstructured.NewExecutor()
	}
	built, err := s.providerFactory.New(
		effectiveSkipPermissions,
		logger,
		inferenceProgressPublisher,
		responseExecutor,
	)
	if err != nil {
		return nil, err
	}
	return built, nil
}

// runnerProviderAdapter lets the provider-boundary recorder observe the final
// decorated runner. Registry-selected conductor routes and retained native
// routes therefore emit the same canonical inference events exactly once.
type runnerProviderAdapter struct {
	runner workers.Runner
}

func recordProviderRunner(
	runner workers.Runner,
	recorder workerprovider.InferenceEventRecorder,
	clock func() time.Time,
) workers.Runner {
	if recorder == nil {
		return runner
	}
	return workerexecutor.RunnerFromProvider(workerprovider.NewRecordingProvider(
		runnerProviderAdapter{runner: runner},
		recorder,
		clock,
	))
}

func (a runnerProviderAdapter) Infer(
	ctx context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	if a.runner == nil {
		return workerexecution.InferenceResponse{}, workerprovider.NewProviderError(
			workerexecution.WorkFailureTypeMisconfigured,
			"recording runner requires an implementation",
			nil,
		)
	}
	return a.runner.Execute(ctx, request)
}

// effectiveSkipPermissionsRunner installs invocation-local policy outside all
// execution decorators. This is intentionally the outermost runner so a
// conductor route that does not call the retained native runner still receives
// the same effective worker policy.
type effectiveSkipPermissionsRunner struct {
	next    workers.Runner
	enabled bool
}

func effectiveWorkerSkipPermissions(
	def *interfaces.FactoryWorkerConfig,
	invocationOverride *bool,
) bool {
	return skippermissions.EffectiveSkipPermissions(
		def.SkipPermissions,
		def.Type,
		invocationOverride,
	)
}

func decorateProviderRunner(
	runner workers.Runner,
	def *interfaces.FactoryWorkerConfig,
	decorators []RunnerDecorator,
	effectiveSkipPermissions bool,
) workers.Runner {
	for _, decorate := range decorators {
		if decorate != nil {
			runner = decorate(runner, def)
		}
	}
	return effectiveSkipPermissionsRunner{
		next:    runner,
		enabled: effectiveSkipPermissions,
	}
}

func (r effectiveSkipPermissionsRunner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	request.SkipPermissions = r.enabled
	return r.next.Execute(ctx, request)
}

func workstationResult(
	runtimeConfig interfaces.RuntimeConfigLookup,
	factoryRunnerID string,
	workflowContext *workerexecution.Context,
	logger logging.Logger,
	direct workers.WorkstationRequestExecutor,
	interpolation interfaces.InvocationInterpolationService,
	executionPolicy interfaces.WorkstationExecutionPolicyService,
	clock func() time.Time,
	processEnvironment func() []string,
	currentWorkingDirectory func() (string, error),
	factoryDocs workers.FactoryDocsLoader,
	worktreePreparer workers.FactoryWorktreePreparer,
	workstationFiles platformfilesystem.ReadFileInspector,
	resolveRunner workers.RunnerSelectionResolver,
	resolveProvider workers.ProviderIdentityResolver,
) Result {
	renderer := &workerprompting.DefaultPromptRenderer{FactoryDocs: factoryDocs}
	return Result{
		Direct: direct,
		Dispatch: &workerexecutor.WorkstationExecutor{
			Now:                     clock,
			ProcessEnvironment:      processEnvironment,
			CurrentWorkingDirectory: currentWorkingDirectory,
			RuntimeConfig:           runtimeConfig, DefaultRunnerID: factoryRunnerID,
			ResolveRunnerSelection:  resolveRunner,
			ResolveProviderIdentity: resolveProvider,
			WorkflowContext:         workflowContext, Executor: direct,
			Interpolation:   interpolation,
			ExecutionPolicy: executionPolicy,
			Renderer:        renderer, Logger: logger,
			WorktreePreparer: worktreePreparer,
			FileSystem:       workstationFiles,
		},
	}
}
