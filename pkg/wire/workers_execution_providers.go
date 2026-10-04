package wire

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
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
	"go.uber.org/zap"
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

// provideStatelessWorkersService composes the process-scoped Execute owner.
// It is deliberately independent of Factory Runtime and Factory Session
// opening: a caller can execute one detached target before either lifecycle is
// opened, while the legacy runtime root receives this same owner below.
func provideStatelessWorkersService(
	providersService providers.Service,
	modelsService models.Service,
	contentMaterializer work.ContentMaterializer,
	mediaFiles platformfilesystem.ReadOpener,
	scriptCommandRunner factorysessionwire.ScriptCommandRunner,
	factoryDocsFileSystem platformfilesystem.ReadFileTree,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	worktreePreparer workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	providerOverride providerOverrideService,
	agentToolFileSystem workers.AgentToolFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
) (workers.Service, error) {
	return provideStatelessWorkersServiceWithMock(
		providersService,
		modelsService,
		contentMaterializer,
		mediaFiles,
		scriptCommandRunner,
		factoryDocsFileSystem,
		clock,
		logger,
		worktreePreparer,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentToolFileSystem,
		decisionEnvelopes,
		nil,
	)
}

func provideStatelessWorkersServiceWithMock(
	providersService providers.Service,
	modelsService models.Service,
	contentMaterializer work.ContentMaterializer,
	mediaFiles platformfilesystem.ReadOpener,
	scriptCommandRunner factorysessionwire.ScriptCommandRunner,
	factoryDocsFileSystem platformfilesystem.ReadFileTree,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	worktreePreparer workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
	providerOverride providerOverrideService,
	agentToolFileSystem workers.AgentToolFileSystem,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	mockWorkers *workers.MockWorkersConfig,
) (workers.Service, error) {
	if clock == nil {
		return nil, fmt.Errorf("construct stateless Workers: clock is required")
	}
	factoryDocs, err := workerswire.NewFactoryDocsLoader(factoryDocsFileSystem)
	if err != nil {
		return nil, fmt.Errorf("construct stateless Workers: %w", err)
	}
	scriptRunner := scriptCommandRunner
	agentDependencies := workerswire.AgentDependencies{
		Providers: providersService,
		Publish:   func(workers.ProgressFragment) {},
		// Decision-envelope interpretation belongs to Factory Definitions.
		// The detached Execute path routes envelope output through this
		// injected owner instead of re-implementing the contract.
		DecisionEnvelopes: decisionEnvelopes,
	}
	scriptConfig := workerswire.ScriptConfig{RequestSelected: true}
	scriptDependencies := workerswire.ScriptDependencies{
		CommandRunner: scriptRunner,
		FactoryDocs:   factoryDocs,
		Now:           clock.Now,
		Publish:       func(workers.ProgressFragment) {},
		Record:        func(workers.ScriptEvent) {},
	}
	inferenceConfig := workerswire.InferenceConfig{
		Worker: models.LocalWorker{
			Name: "request-selected-inference",
			Type: factorydefinitions.WorkerTypeInference,
		},
	}
	inferenceDependencies := workerswire.InferenceDependencies{
		Models: modelsService, ContentMaterializer: contentMaterializer, MediaFiles: mediaFiles,
	}
	loggerValue := logging.NewZapLogger(logger, false)
	if mockWorkers != nil {
		return workerswire.NewMockService(
			agentDependencies,
			scriptConfig,
			scriptDependencies,
			inferenceConfig,
			inferenceDependencies,
			mockWorkers,
			workerswire.MockDependencies{},
			nil,
			loggerValue,
			clock.Now,
			worktreePreparer,
			worktreeRelease,
			temporaryFiles,
			agentToolFileSystem,
			providerOverride,
		)
	}
	return workerswire.NewService(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
		nil,
		loggerValue,
		clock.Now,
		worktreePreparer,
		worktreeRelease,
		temporaryFiles,
		agentToolFileSystem,
		providerOverride,
	)
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

// provideProvidersAgyPTYPlatform projects the Providers-owned PTY effect into
// the Workers-private invocation seam at the canonical composition boundary.
func provideProvidersAgyPTYPlatform(edges serviceedges.Edges) (providerswire.AgyPTYPlatformDependencies, error) {
	allocator, err := provideProvidersAgyPTYAllocator(edges)
	if err != nil {
		return providerswire.AgyPTYPlatformDependencies{}, err
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	return providerswire.AgyPTYPlatformDependencies{
		Allocator: allocator,
		Locator:   executableLocator,
		Inspector: executableInspector,
	}, nil
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

func provideConductorInvocationWithProgressFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
	allocator providerswire.PTYAllocator,
) factorysessionwire.ConductorInvocationWithProgressFactory {
	commandClock := edges.Clock
	if commandClock == nil {
		commandClock = platformclock.Real{}
	}
	resolveSymlinks := edges.WorkersResolveSymlinks
	if resolveSymlinks == nil {
		resolveSymlinks = filepath.EvalSymlinks
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	executableFiles := edges.WorkersExecutableFileReader
	if executableFiles == nil {
		executableFiles = platformfilesystem.Local{}
	}
	operatingSystem := resolveWorkersOperatingSystem(edges)
	temporaryFiles := provideWorkersProviderTemporaryFileSystem(edges)
	return func(
		selectedProviders providers.Service,
		runner platformprocess.CommandRunner,
		publisher workers.ProgressPublisher,
	) (workers.InvocationExecutor, error) {
		if selectedProviders == nil {
			selectedProviders = providersService
		}
		return workerswire.NewConductorInvocationWithProgress(
			selectedProviders,
			runner,
			commandClock,
			allocator,
			resolveSymlinks,
			executableLocator,
			executableInspector,
			executableFiles,
			operatingSystem,
			publisher,
			temporaryFiles,
		)
	}
}

func provideProviderFromCommandRunnerFactory(
	providersService providers.Service,
	edges serviceedges.Edges,
) factorysessionwire.ProviderFromCommandRunnerFactory {
	commandClock := edges.Clock
	if commandClock == nil {
		commandClock = platformclock.Real{}
	}
	resolveSymlinks := edges.WorkersResolveSymlinks
	if resolveSymlinks == nil {
		resolveSymlinks = filepath.EvalSymlinks
	}
	executableLocator := edges.WorkersExecutableLocator
	if executableLocator == nil {
		executableLocator = platformprocess.HostExecutableLocator{}
	}
	executableInspector := edges.WorkersExecutablePathInspector
	if executableInspector == nil {
		executableInspector = platformfilesystem.Local{}
	}
	executableFiles := edges.WorkersExecutableFileReader
	if executableFiles == nil {
		executableFiles = platformfilesystem.Local{}
	}
	operatingSystem := resolveWorkersOperatingSystem(edges)
	temporaryFiles := provideWorkersProviderTemporaryFileSystem(edges)
	return func(runner platformprocess.CommandRunner) (providers.Service, error) {
		return workerswire.NewProviderFromCommandRunner(
			providersService, runner, commandClock, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem, temporaryFiles,
		)
	}
}

// provideInvocationWorkPolicy constructs only pure Work return policy, avoiding
// the Work runtime resolver's dependency on the Sessions assembly.
func provideInvocationWorkPolicy() factorysessionwire.InvocationWorkPolicy {
	return work.NewInvocationPolicyService()
}
