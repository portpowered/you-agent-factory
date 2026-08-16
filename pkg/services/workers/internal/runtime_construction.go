package internal

import (
	"fmt"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func buildExecutionFactories(
	providerRunner workers.CommandRunner,
	scriptRunner workers.CommandRunner,
	commandClock workers.Clock,
	allocator workers.PTYAllocator,
	factoryDocs workers.FactoryDocsLoader,
	resolveSymlinks workers.ResolveExecutableSymlinks,
	executableLocator platformprocess.ExecutableLocator,
	executableInspector platformfilesystem.PathInspector,
	executableFiles platformfilesystem.ReadOpener,
	operatingSystem workers.OperatingSystem,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
) (workers.CommandRunner, workers.CommandRunner, error) {
	if providerRunner == nil {
		return nil, nil, fmt.Errorf("construct Worker execution service: provider command runner is required")
	}
	if scriptRunner == nil {
		return nil, nil, fmt.Errorf("construct Worker execution service: script command runner is required")
	}
	if commandClock == nil {
		return nil, nil, fmt.Errorf("construct Worker execution service: command clock is required")
	}
	if allocator == nil {
		return nil, nil, fmt.Errorf("construct Worker execution service: Agy PTY allocator is required")
	}
	if factoryDocs == nil {
		return nil, nil, fmt.Errorf("construct Worker execution service: Factory docs loader is required")
	}
	return providerRunner, scriptRunner, nil
}

func serviceCommandClock(s *Service) workers.Clock {
	return workers.ClockFunc(func() time.Time {
		if s != nil && s.clock != nil {
			return s.clock()
		}
		return time.Now()
	})
}

func (s *Service) ProviderCommandRunner() workers.CommandRunner {
	if s == nil {
		return nil
	}
	return s.providerCommandRunner
}

func (s *Service) ScriptCommandRunner() workers.CommandRunner {
	if s == nil {
		return nil
	}
	return s.scriptCommandRunner
}
