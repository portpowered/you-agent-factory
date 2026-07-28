package internal

import (
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerinvocation "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	workerproviderstructured "github.com/portpowered/infinite-you/pkg/services/workers/provider/structured"
)

// NewInvocation constructs the narrow direct-invocation role used by
// standalone Factory Session execution.
func NewInvocation(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return newInvocation(
		commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		nil, temporaryFileSystems...,
	)
}

// NewInvocationWithProgress constructs one direct-invocation role that publishes
// provider progress fragments through the supplied publisher.
func NewInvocationWithProgress(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workerprovider.InferenceProgressPublisher,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workers.InvocationExecutor, error) {
	return newInvocation(
		commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		progressPublisher, temporaryFileSystems...,
	)
}

func newInvocation(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	progressPublisher workerprovider.InferenceProgressPublisher,
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
	if progressPublisher == nil {
		provider, err := NewProviderFromCommandRunner(
			commandRunner, commandClock, allocator, resolveSymlinks,
			executableLocator, executableInspector, executableFiles, operatingSystem,
			temporaryFileSystems...,
		)
		if err != nil {
			return nil, err
		}
		return workerinvocation.NewExecutor(provider), nil
	}

	factory, err := workerprovider.NewFactory(
		commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		temporaryFileSystems...,
	)
	if err != nil {
		return nil, fmt.Errorf("construct Worker provider: %w", err)
	}
	provider, err := factory.New(
		false, nil, progressPublisher, workerproviderstructured.NewExecutor(), "",
	)
	if err != nil {
		return nil, fmt.Errorf("construct Worker provider: %w", err)
	}
	return workerinvocation.NewExecutor(provider), nil
}
