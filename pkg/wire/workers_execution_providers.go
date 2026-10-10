package wire

import (
	"context"
	"os"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformrandom "github.com/portpowered/infinite-you/pkg/platform/random"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
)

func provideWorkersWorktree(
	edges serviceedges.Edges,
) (workers.FactoryWorktreePreparer, error) {
	worktreeFileSystem := edges.WorkersWorktreeFileSystem
	if worktreeFileSystem == nil {
		worktreeFileSystem = platformfilesystem.Local{}
	}
	worktreeGit := edges.WorkersWorktreeGit
	if worktreeGit == nil {
		processRunner, err := providePlatformProcessCommandRunner(edges)
		if err != nil {
			return nil, err
		}
		adapter, err := workerswire.NewPlatformGitCommander(processRunner)
		if err != nil {
			return nil, err
		}
		worktreeGit = adapter
	}
	worktreePreparer, err := workerswire.NewWorktree(worktreeFileSystem, worktreeGit)
	if err != nil {
		return nil, err
	}
	return worktreePreparer, nil
}

type factoryWorktreeReleaser interface {
	Release(context.Context, workers.FactoryWorktreePreparation) error
}

func provideWorkersWorktreeRelease(
	worktreePreparer workers.FactoryWorktreePreparer,
) func(context.Context, workers.FactoryWorktreePreparation) error {
	releaser, ok := worktreePreparer.(factoryWorktreeReleaser)
	if !ok {
		return nil
	}
	return releaser.Release
}

// Distinct interface types give Wire each completed strategy edge without
// wrapper instances or selection through a dependency container.
type workersAgentRunner workers.Runner
type workersScriptRunner workers.Runner
type workersInferenceRunner workers.Runner
type workersContextualScriptCommandRunner platformprocess.CommandRunner
type workersLoggedScriptCommandRunner platformprocess.CommandRunner

func provideWorkersContextualScriptCommandRunner(command factorysessionwire.ScriptCommandRunner) workersContextualScriptCommandRunner {
	return workerswire.NewContextualMockWorkerCommandRunner(command, nil)
}

func provideWorkersLoggedScriptCommandRunner(command workersContextualScriptCommandRunner,
	logger logging.Logger, clock factoryruntime.Clock,
) workersLoggedScriptCommandRunner {
	return workerswire.NewLoggingCommandRunner(command, logger, clock.Now)
}

func provideWorkersHarness(files workers.AgentToolFileSystem) workerswire.Harness {
	return workerswire.NewLibraryHarnessAdapter(files)
}

func provideWorkersFactoryDocs(files platformfilesystem.ReadFileTree) (workers.FactoryDocsLoader, error) {
	return workerswire.NewFactoryDocsLoader(files)
}

func provideWorkersAgentRunner(service providers.Service,
	envelopes factorydefinitions.DecisionEnvelopeService,
) (workersAgentRunner, error) {
	return workerswire.NewAgentRunner(service, func(workers.ProgressFragment) {}, envelopes)
}

func provideWorkersScriptRunner(command workersLoggedScriptCommandRunner,
	docs workers.FactoryDocsLoader, clock factoryruntime.Clock,
) (workersScriptRunner, error) {
	return workerswire.NewScriptRunner(workerswire.ScriptConfig{RequestSelected: true},
		command, docs, clock.Now, func(workers.ProgressFragment) {}, func(workers.ScriptEvent) {})
}

func provideWorkersInferenceRunner(service models.Service, agent workersAgentRunner,
	content work.ContentMaterializer, media platformfilesystem.ReadOpener,
) (workersInferenceRunner, error) {
	return workerswire.NewInferenceRunner(workerswire.InferenceConfig{
		Worker: models.LocalWorker{Name: "request-selected-inference", Type: factorydefinitions.WorkerTypeInference},
	}, service, agent, content, media)
}

func provideWorkersRegistry(agent workersAgentRunner, script workersScriptRunner,
	inference workersInferenceRunner,
) (workerswire.Registry, error) {
	return workerswire.NewProductionRegistry(agent, script, inference)
}

// provideWorkersExecute captures completed process behavior. Worktree leases,
// command attempts, observations and cleanup remain owned by each Execute call.
func provideWorkersExecute(registry workerswire.Registry, service providers.Service,
	logger logging.Logger, clock factoryruntime.Clock,
	worktree workers.FactoryWorktreePreparer,
	release func(context.Context, workers.FactoryWorktreePreparation) error,
	temporary platformfilesystem.TemporaryFileSystem, override providerOverrideService,
	harness workerswire.Harness, envelopes factorydefinitions.DecisionEnvelopeService,
	docs workers.FactoryDocsLoader,
) (workerswire.ExecuteCapability, error) {
	return workerswire.NewExecute(registry, service, nil, logger, clock.Now,
		worktree, release, temporary, override, harness, envelopes, docs)
}

func provideWorkersRetryRandomSource(edges serviceedges.Edges) platformrandom.Source {
	if edges.WorkersRetryRandomSource != nil {
		return edges.WorkersRetryRandomSource
	}
	return platformrandom.CryptoSource{}
}

func provideWorkersWorkstationFileSystem(edges serviceedges.Edges) platformfilesystem.ReadFileInspector {
	if edges.WorkersWorkstationFileSystem != nil {
		return edges.WorkersWorkstationFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkersInferenceMediaFileReader(edges serviceedges.Edges) platformfilesystem.ReadOpener {
	if edges.WorkersInferenceMediaFileReader != nil {
		return edges.WorkersInferenceMediaFileReader
	}
	return platformfilesystem.Local{}
}

func provideWorkersProviderTemporaryFileSystem(edges serviceedges.Edges) platformfilesystem.TemporaryFileSystem {
	if edges.WorkersProviderTemporaryFileSystem != nil {
		return edges.WorkersProviderTemporaryFileSystem
	}
	return platformfilesystem.Local{}
}

// provideProvidersAgyPTYEffect completes the native adapter before root assembly.
func provideProvidersAgyPTYEffect(edges serviceedges.Edges) (providerswire.AgyEffect, error) {
	allocator, err := provideProvidersAgyPTYAllocator(edges)
	if err != nil {
		return nil, err
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	return providerswire.NewAgyPTYEffect(
		allocator, executableLocator, executableInspector,
		effectiveProviderCommandClock(edges), providerswire.AgyPTYPolicy{},
	), nil
}

func provideWorkersFactoryDocsFileSystem(edges serviceedges.Edges) platformfilesystem.ReadFileTree {
	if edges.WorkersFactoryDocsFileSystem != nil {
		return edges.WorkersFactoryDocsFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkerProcessEnvironment() func() []string {
	return os.Environ
}

func provideWorkersAgentToolFileSystem(edges serviceedges.Edges) workers.AgentToolFileSystem {
	if edges.WorkersAgentToolFileSystem != nil {
		return edges.WorkersAgentToolFileSystem
	}
	return platformfilesystem.Local{}
}

func provideWorkerCurrentWorkingDirectory() func() (string, error) {
	return os.Getwd
}

func provideWorkersMockCommandRunnerFactory() factoryruntime.WorkersMockCommandRunnerFactory {
	return func(
		config *workers.MockWorkersConfig,
		runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
		next platformprocess.CommandRunner,
	) platformprocess.CommandRunner {
		return workerswire.NewMockCommandRunner(
			config,
			runtimeConfig,
			next,
			platformfilesystem.Local{},
		)
	}
}

// provideInvocationWorkPolicy constructs only pure Work return policy, avoiding
// the Work runtime resolver's dependency on the Sessions assembly.
func provideInvocationWorkPolicy() factorysessionwire.InvocationWorkPolicy {
	return work.NewInvocationPolicyService()
}

func provideProvidersCodexPromptFiles(edges serviceedges.Edges) providerswire.CodexPromptFileSystem {
	if edges.ProvidersCodexPromptFiles != nil {
		return edges.ProvidersCodexPromptFiles
	}
	return platformfilesystem.Local{}
}

func provideProvidersCodexHomeResolver(edges serviceedges.Edges) func() (string, error) {
	if edges.ProvidersCodexResolveHomeDirectory != nil {
		return edges.ProvidersCodexResolveHomeDirectory
	}
	return os.UserHomeDir
}
