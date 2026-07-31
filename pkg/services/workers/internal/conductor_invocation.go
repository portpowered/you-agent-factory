package internal

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewConductorInvocationWithProgress is retained as a compatibility name for
// direct Factory Session invocation. Provider routing now stays behind the
// singular Providers root instead of sharing registry and conductor objects.
func NewConductorInvocationWithProgress(
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
	return NewInvocationWithProgress(
		providersService,
		commandRunner,
		commandClock,
		allocator,
		resolveSymlinks,
		executableLocator,
		executableInspector,
		executableFiles,
		operatingSystem,
		progressPublisher,
		temporaryFileSystems...,
	)
}
