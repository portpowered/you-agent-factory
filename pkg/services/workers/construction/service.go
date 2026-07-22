// Package construction owns construction of configured worker executors.
package construction

import (
	"fmt"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/executor"
	workeragentrun "github.com/portpowered/infinite-you/pkg/services/workers/executor/agentrun"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	providercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	providerstructured "github.com/portpowered/infinite-you/pkg/services/workers/provider/structured"
	"github.com/portpowered/infinite-you/pkg/services/workers/skippermissions"
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
	workstationFiles  platformfilesystem.ReadFileInspector
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
		runner, err := s.providerRunner(
			runtimeConfig, def, logger, invocationSkipPermissionsOverride,
			providerOverride, inferenceProgressPublisher, inferenceRecorder, clock,
		)
		if err != nil {
			return Result{}, err
		}
		for _, decorate := range runnerDecorators {
			if decorate != nil {
				runner = decorate(runner, def)
			}
		}
		inference := workerexecutor.NewAgentExecutorWithRunner(
			runtimeConfig,
			runner,
			logger,
			clock,
			s.retryRandom,
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
		), nil
	case interfaces.WorkstationTypeLogical:
		return workstationResult(
			runtimeConfig, factoryRunnerID, workflowContext, logger, nil,
			s.interpolation, s.executionPolicy, clock, processEnvironment, currentWorkingDirectory, s.factoryDocs, s.worktreePreparer, s.workstationFiles,
		), nil
	case interfaces.WorkerTypeScript:
		if s == nil || s.scriptFactory == nil {
			return Result{}, fmt.Errorf("script worker factory is required")
		}
		direct, err := s.scriptFactory.New(
			def, logger, runtimeConfig.FactoryDir(), scriptRecorder, clock,
		)
		if err != nil {
			return Result{}, err
		}
		return workstationResult(
			runtimeConfig, factoryRunnerID, workflowContext, logger, direct, s.interpolation,
			s.executionPolicy, clock, processEnvironment, currentWorkingDirectory, s.factoryDocs, s.worktreePreparer, s.workstationFiles,
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
	)
}

func (s *Service) providerRunner(
	runtimeConfig interfaces.RuntimeConfigLookup,
	def *interfaces.FactoryWorkerConfig,
	logger logging.Logger,
	invocationSkipPermissionsOverride *bool,
	providerOverride providercontract.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	clock func() time.Time,
) (workers.Runner, error) {
	var runner workers.Runner
	if providerOverride != nil {
		runner = workerexecutor.RunnerFromProvider(providerOverride)
	} else {
		if s == nil || s.providerFactory == nil {
			return nil, fmt.Errorf("provider worker factory is required")
		}
		var responseExecutor workerprovider.ResponseStreamExecutor
		if inferenceProgressPublisher != nil {
			responseExecutor = providerstructured.NewExecutor()
		}
		built, err := s.providerFactory.New(
			skippermissions.EffectiveSkipPermissions(
				def.SkipPermissions,
				def.Type,
				invocationSkipPermissionsOverride,
			),
			logger,
			nil,
			inferenceProgressPublisher,
			responseExecutor,
			strings.TrimSpace(runtimeConfig.FactoryDir()),
		)
		if err != nil {
			return nil, err
		}
		runner = built
	}
	if inferenceRecorder == nil {
		return runner, nil
	}
	if providerOverride != nil {
		return workerexecutor.RunnerFromProvider(workerprovider.NewRecordingProvider(
			providerOverride, inferenceRecorder, clock,
		)), nil
	}
	providerRunner, ok := runner.(*workerprovider.ScriptWrapProvider)
	if !ok {
		return runner, nil
	}
	return workerexecutor.RunnerFromProvider(workerprovider.NewRecordingProvider(
		providerRunner, inferenceRecorder, clock,
	)), nil
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
) Result {
	renderer := &workerprompting.DefaultPromptRenderer{FactoryDocs: factoryDocs}
	return Result{
		Direct: direct,
		Dispatch: &workerexecutor.WorkstationExecutor{
			Now:                     clock,
			ProcessEnvironment:      processEnvironment,
			CurrentWorkingDirectory: currentWorkingDirectory,
			RuntimeConfig:           runtimeConfig, DefaultRunnerID: factoryRunnerID,
			WorkflowContext: workflowContext, Executor: direct,
			Interpolation:   interpolation,
			ExecutionPolicy: executionPolicy,
			Renderer:        renderer, Logger: logger,
			WorktreePreparer: worktreePreparer,
			FileSystem:       workstationFiles,
		},
	}
}
