package internal

import (
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
	workerprovidercontract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// NewProviderFromCommandRunner constructs one provider-backed worker from the
// supplied command runner using the same production edges as direct invocation.
func NewProviderFromCommandRunner(
	commandRunner workers.CommandRunner,
	commandClock workerprocess.Clock,
	allocator agypty.PTYAllocator,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFileSystems ...platformfilesystem.TemporaryFileSystem,
) (workerprovidercontract.Provider, error) {
	if commandRunner == nil {
		return nil, fmt.Errorf("construct provider-backed worker: command runner is required")
	}
	if commandClock == nil {
		return nil, fmt.Errorf("construct provider-backed worker: command clock is required")
	}
	if allocator == nil {
		return nil, fmt.Errorf("construct provider-backed worker: PTY allocator is required")
	}
	factory, err := workerprovider.NewFactory(
		commandRunner, commandClock, allocator, resolveSymlinks,
		executableLocator, executableInspector, executableFiles, operatingSystem,
		temporaryFileSystems...,
	)
	if err != nil {
		return nil, fmt.Errorf("construct provider-backed worker: %w", err)
	}
	provider, err := factory.New(false, nil, nil, nil, "")
	if err != nil {
		return nil, fmt.Errorf("construct provider-backed worker: %w", err)
	}
	return provider, nil
}
