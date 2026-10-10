// Package wire is the Workers service composition boundary.
//
// Wire performs construction only, returns the singular workers.Service root
// interface, and starts no lifecycle components. Runner ownership stays inside
// the owner service assembly path; peers depend on Service rather than owner
// internals or construction ports. Hosted runner ownership is not constructed.
package wire

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"

	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/prompting"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"

	worktree "github.com/portpowered/infinite-you/pkg/services/workers/internal/worktree"
)

var (
	NewWorktree             = worktree.New
	NewPlatformGitCommander = worktree.NewPlatformGitCommander
)

// The runner construction records stay private to the Workers wire package;
// these aliases expose only their detached inputs to the canonical process
// graph without exposing runner implementations or registries.
type AgentDependencies = runners.AgentDependencies
type ScriptConfig = runners.ScriptConfig
type ScriptDependencies struct {
	CommandRunner platformprocess.CommandRunner
	FactoryDocs   workers.FactoryDocsLoader
	Now           func() time.Time
	Publish       workers.ProgressPublisher
	Record        workers.ScriptEventRecorder
}
type InferenceConfig = runners.InferenceConfig
type InferenceDependencies = runners.InferenceDependencies
type MockConfig = runners.MockConfig
type MockDependencies struct {
	Next platformprocess.CommandRunner
}

// NewService constructs an inert Workers root from construction ports. It
// composes the private runner registry once and installs a request-scoped
// Execute capability without publishing runner or executor objects on the
// returned service root.
func NewService(
	agentDependencies AgentDependencies,
	scriptConfig ScriptConfig,
	scriptDependencies ScriptDependencies,
	inferenceConfig InferenceConfig,
	inferenceDependencies InferenceDependencies,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	agentToolFiles workers.AgentToolFileSystem,
	providerOverrides ...providers.Service,
) (workers.Service, error) {
	privateScriptDependencies := privateScriptDependencies(scriptDependencies, logger, clock, nil)
	agentRunner, scriptRunner, inferenceRunner, err := productionStrategies(
		agentDependencies, scriptConfig, privateScriptDependencies, inferenceConfig, inferenceDependencies,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	runnerRegistry, err := runnerswire.NewProductionRegistry(agentRunner, scriptRunner, inferenceRunner)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	var providerOverride providers.Service
	if len(providerOverrides) > 0 {
		providerOverride = providerOverrides[0]
	}
	executeService, err := executeservice.NewWithProviderOverride(
		runnerRegistry,
		agentDependencies.Providers,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentrun.NewLibraryHarnessAdapter(agentToolFiles),
		agentDependencies.DecisionEnvelopes,
		scriptDependencies.FactoryDocs,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	return workersinternal.NewRoot(executeService)
}

// NewMockService constructs the explicit Workers mock-feature root. The mock
// strategy is registered only in this opt-in composition path; ordinary
// NewService construction remains a production registry with no mock entry.
// The returned root uses the same Execute normalization, observations,
// cleanup, and Worktree behavior as production Workers.
func NewMockService(
	agentDependencies AgentDependencies,
	scriptConfig ScriptConfig,
	scriptDependencies ScriptDependencies,
	inferenceConfig InferenceConfig,
	inferenceDependencies InferenceDependencies,
	mockWorkers *workers.MockWorkersConfig,
	mockDependencies MockDependencies,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	agentToolFiles workers.AgentToolFileSystem,
	providerOverrides ...providers.Service,
) (workers.Service, error) {
	if mockWorkers == nil {
		return nil, fmt.Errorf("construct mock Workers: mock workers config is required")
	}
	privateScriptDependencies := privateScriptDependencies(scriptDependencies, logger, clock, agentToolFiles)
	agentRunner, scriptRunner, inferenceRunner, err := productionStrategies(
		agentDependencies, scriptConfig, privateScriptDependencies, inferenceConfig, inferenceDependencies,
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	mockRunner, err := runnerswire.NewMockRunner(
		runnerswire.MockRunnerConfig{WorkersConfig: mockWorkers},
		workerprocess.AdaptPlatformCommandRunner(mockDependencies.Next), agentToolFiles,
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", invalidRunnerConstruction(runners.MockIdentity, err))
	}
	runnerRegistry, err := runnerswire.NewMockProductionRegistry(agentRunner, scriptRunner, inferenceRunner, mockRunner)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	var providerOverride providers.Service
	if len(providerOverrides) > 0 {
		providerOverride = providerOverrides[0]
	}
	executeService, err := executeservice.NewWithProviderOverride(
		runnerRegistry,
		agentDependencies.Providers,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentrun.NewLibraryHarnessAdapter(agentToolFiles),
		agentDependencies.DecisionEnvelopes,
		scriptDependencies.FactoryDocs,
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	return workersinternal.NewRoot(executeService)
}

func privateScriptDependencies(
	dependencies ScriptDependencies,
	logger logging.Logger,
	clock func() time.Time,
	gateFiles workers.AgentToolFileSystem,
) runners.ScriptDependencies {
	commandRunner := newContextualMockWorkerCommandRunner(
		workerprocess.AdaptPlatformCommandRunner(dependencies.CommandRunner),
		gateFiles,
	)
	if commandRunner != nil && clock != nil {
		commandRunner = workerprocess.CommandRunnerWithLogging(
			commandRunner,
			logger,
			workerprocess.ClockFunc(clock),
		)
	}
	return runners.ScriptDependencies{
		CommandRunner: commandRunner,
		FactoryDocs:   dependencies.FactoryDocs,
		Now:           dependencies.Now,
		Publish:       dependencies.Publish,
		Record:        dependencies.Record,
	}
}

var NewFactoryDocsLoader = workerprompting.NewFactoryDocsLoader

var NewExecutor = invocation.NewExecutor
var NewLibraryHarnessAdapter = agentrun.NewLibraryHarnessAdapter

// productionStrategies is the retained outer construction boundary pending the
// canonical Wire cutover. The registry receives only completed strategies.
func productionStrategies(
	agentDependencies runners.AgentDependencies,
	scriptConfig runners.ScriptConfig,
	scriptDependencies runners.ScriptDependencies,
	inferenceConfig runners.InferenceConfig,
	inferenceDependencies runners.InferenceDependencies,
) (workers.Runner, workers.Runner, workers.Runner, error) {
	agent, err := agentImplementation(agentDependencies)
	if err != nil {
		return nil, nil, nil, invalidRunnerConstruction(runners.AgentIdentity, err)
	}
	script, err := scriptImplementation(scriptConfig, scriptDependencies)
	if err != nil {
		return nil, nil, nil, invalidRunnerConstruction(runners.ScriptIdentity, err)
	}
	delegate := inferenceDependencies.Delegate
	if delegate == nil {
		delegate = agent
	}
	inferenceDependencies.Delegate = delegate
	inference, err := inferenceImplementation(inferenceConfig, inferenceDependencies)
	if err != nil {
		return nil, nil, nil, invalidRunnerConstruction(runners.InferenceIdentity, err)
	}
	return agent, script, inference, nil
}
func invalidRunnerConstruction(identity string, err error) error {
	return fmt.Errorf(
		"%w: %s runner construction failed: %w",
		workers.ErrInvalidRunnerRegistration,
		identity,
		err,
	)
}

func scriptImplementation(
	config runners.ScriptConfig,
	dependencies runners.ScriptDependencies,
) (workers.Runner, error) {
	if strings.TrimSpace(config.Command) == "" && !config.RequestSelected {
		return nil, workers.NewProviderError(workers.WorkFailureTypeMisconfigured, "script command is required", nil)
	}
	commandRunner, ok := dependencies.CommandRunner.(workerprocess.StreamingCommandRunner)
	if !ok {
		return nil, workers.NewProviderError(workers.WorkFailureTypeMisconfigured, "script command runner must support streaming", nil)
	}
	return runnerswire.NewScriptRunner(
		runnerswire.ScriptRunnerConfig{
			Command:          config.Command,
			Args:             append([]string(nil), config.Args...),
			Stdin:            config.Stdin,
			FactoryDirectory: config.FactoryDirectory,
			RequestSelected:  config.RequestSelected,
		},
		commandRunner,
		dependencies.FactoryDocs,
		dependencies.Now,
		dependencies.Publish,
		dependencies.Record,
	), nil
}
func inferenceImplementation(
	config runners.InferenceConfig,
	dependencies runners.InferenceDependencies,
) (workers.Runner, error) {
	return runnerswire.NewInferenceRunner(
		runnerswire.InferenceRunnerConfig{
			Worker: snapshotInferenceWorker(config.Worker),
			Resources: append(
				[]models.LocalResource(nil),
				config.Resources...,
			),
			Scope: config.Scope,
		},
		dependencies.Models, dependencies.Delegate, dependencies.ContentMaterializer, dependencies.MediaFiles,
	)
}

func agentImplementation(
	dependencies runners.AgentDependencies,
) (workers.Runner, error) {
	return runnerswire.NewAgentRunner(
		dependencies.Providers,
		dependencies.Publish,
		dependencies.DecisionEnvelopes,
	)
}

func snapshotInferenceWorker(worker models.LocalWorker) models.LocalWorker {
	worker.Resources = append([]models.LocalResource(nil), worker.Resources...)
	return worker
}
