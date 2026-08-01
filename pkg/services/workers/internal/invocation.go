package internal

import (
	"context"
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
)

// NewInvocation constructs the narrow direct-invocation role used by
// standalone Factory Session execution.
func NewInvocation(
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return newInvocation(
		providersService, commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		nil, temporaryFileSystems...,
	)
}

// NewInvocationWithProgress constructs one direct-invocation role that publishes
// provider progress fragments through the supplied publisher.
func NewInvocationWithProgress(
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return newInvocation(
		providersService, commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		progressPublisher, temporaryFileSystems...,
	)
}

func newInvocation(
	providersService providers.Service,
	commandRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workers.ProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	if commandRunner == nil {
		return nil, fmt.Errorf("construct Worker invocation: command runner is required")
	}
	if commandClock == nil {
		return nil, fmt.Errorf("construct Worker invocation: command clock is required")
	}
	if allocator == nil {
		return nil, fmt.Errorf("construct Worker invocation: PTY allocator is required")
	}
	if providersService == nil {
		return nil, fmt.Errorf("construct Worker invocation: Providers service is required")
	}
	if progressPublisher == nil {
		progressPublisher = func(workers.ProgressFragment) {}
	}
	registry, err := runnerswire.NewAgentRegistry(runners.AgentDependencies{
		Providers: providersService,
		Publish:   progressPublisher,
	})
	if err != nil {
		return nil, fmt.Errorf("construct Worker invocation: %w", err)
	}
	return workerinvocation.NewExecutor(registryRunner{registry: registry}), nil
}

// registryRunner routes transitional InvocationExecutor traffic through
// the private runners.Service.Execute boundary rather than holding a Strategy.
type registryRunner struct{ registry runners.Service }

func (runner registryRunner) Execute(
	ctx context.Context,
	request workers.ProviderInferenceRequest,
) (workers.InferenceResponse, error) {
	if runner.registry == nil {
		return workers.InferenceResponse{}, fmt.Errorf("construct Worker invocation: agent runner registry is required")
	}
	return runner.registry.Execute(ctx, runners.ExecuteRequest{
		Identity: runners.AgentIdentity,
		Attempt:  request,
	})
}
