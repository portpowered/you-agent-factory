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
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
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
	return NewServiceWithContentMaterializer(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		agentToolFiles,
		nil,
		providerOverrides...,
	)
}

// NewServiceWithContentMaterializer constructs the production Workers root
// with the Work-owned materialization capability used before runner execution.
func NewServiceWithContentMaterializer(
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
	contentMaterializer work.ContentMaterializer,
	providerOverrides ...providers.Service,
) (workers.Service, error) {
	if err := validateConstructionPorts(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
	); err != nil {
		return nil, err
	}
	privateScriptDependencies := privateScriptDependencies(scriptDependencies, logger, clock)
	runnerRegistry, err := runnerswire.NewProductionRegistry(
		agentDependencies,
		scriptConfig,
		privateScriptDependencies,
		inferenceConfig,
		inferenceDependencies,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	var providerOverride providers.Service
	if len(providerOverrides) > 0 {
		providerOverride = providerOverrides[0]
	}
	executeService, err := executeservice.NewWithProviderOverrideAndContentMaterializer(
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
		contentMaterializer,
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
	return NewMockServiceWithContentMaterializer(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		mockWorkers,
		mockDependencies,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		agentToolFiles,
		nil,
		providerOverrides...,
	)
}

// NewMockServiceWithContentMaterializer constructs the explicit mock-feature
// Workers root with the Work-owned materialization capability.
func NewMockServiceWithContentMaterializer(
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
	contentMaterializer work.ContentMaterializer,
	providerOverrides ...providers.Service,
) (workers.Service, error) {
	if mockWorkers == nil {
		return nil, fmt.Errorf("construct mock Workers: mock workers config is required")
	}
	if err := validateConstructionPorts(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
	); err != nil {
		return nil, err
	}
	privateScriptDependencies := privateScriptDependencies(scriptDependencies, logger, clock)
	runnerRegistry, err := runnerswire.NewMockProductionRegistry(
		agentDependencies,
		scriptConfig,
		privateScriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		MockConfig{WorkersConfig: mockWorkers},
		runners.MockDependencies{Next: workerprocess.AdaptPlatformCommandRunner(mockDependencies.Next)},
	)
	if err != nil {
		return nil, fmt.Errorf("construct mock Workers: %w", err)
	}
	var providerOverride providers.Service
	if len(providerOverrides) > 0 {
		providerOverride = providerOverrides[0]
	}
	executeService, err := executeservice.NewWithProviderOverrideAndContentMaterializer(
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
		contentMaterializer,
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
) runners.ScriptDependencies {
	commandRunner := newContextualMockWorkerCommandRunner(
		workerprocess.AdaptPlatformCommandRunner(dependencies.CommandRunner),
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

func validateConstructionPorts(
	agentDependencies AgentDependencies,
	scriptConfig ScriptConfig,
	scriptDependencies ScriptDependencies,
	inferenceConfig InferenceConfig,
	inferenceDependencies InferenceDependencies,
) error {
	if agentDependencies.Providers == nil {
		return fmt.Errorf("construct Workers: agent Providers service is required")
	}
	if agentDependencies.Publish == nil {
		return fmt.Errorf("construct Workers: agent progress publisher is required")
	}
	if strings.TrimSpace(scriptConfig.Command) == "" && !scriptConfig.RequestSelected {
		return fmt.Errorf("construct Workers: script command is required")
	}
	if scriptDependencies.CommandRunner == nil {
		return fmt.Errorf("construct Workers: script command runner is required")
	}
	if scriptDependencies.FactoryDocs == nil {
		return fmt.Errorf("construct Workers: script Factory docs loader is required")
	}
	if scriptDependencies.Now == nil {
		return fmt.Errorf("construct Workers: script clock is required")
	}
	if scriptDependencies.Publish == nil {
		return fmt.Errorf("construct Workers: script progress publisher is required")
	}
	if scriptDependencies.Record == nil {
		return fmt.Errorf("construct Workers: script event recorder is required")
	}
	if strings.TrimSpace(inferenceConfig.Worker.Name) == "" {
		return fmt.Errorf("construct Workers: inference worker name is required")
	}
	if inferenceDependencies.Models == nil {
		return fmt.Errorf("construct Workers: inference Models service is required")
	}
	return nil
}

var NewFactoryDocsLoader = workerprompting.NewFactoryDocsLoader

var NewExecutor = invocation.NewExecutor
var NewLibraryHarnessAdapter = agentrun.NewLibraryHarnessAdapter
