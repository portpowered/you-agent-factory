package wire

import (
	"fmt"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerswire "github.com/portpowered/infinite-you/pkg/services/workers/wire"
	"go.uber.org/zap"
)

// provideStatelessWorkersService composes the process-scoped Execute owner.
// It is deliberately independent of Factory Runtime and Factory Session
// opening: a caller can execute one detached target before either lifecycle is
// opened, while the legacy runtime root receives this same owner below.
func provideStatelessWorkersService(
	providersService providers.Service,
	modelsService models.Service,
	scriptCommandRunner factorysessionwire.ScriptCommandRunner,
	factoryDocsFileSystem platformfilesystem.ReadFileTree,
	clock factoryruntime.Clock,
	logger *zap.Logger,
	worktreeLifecycle workers.FactoryWorktreeLifecycle,
	temporaryFiles platformfilesystem.TemporaryFileSystem,
) (workers.Service, error) {
	if clock == nil {
		return nil, fmt.Errorf("construct stateless Workers: clock is required")
	}
	factoryDocs, err := workerswire.NewFactoryDocsLoader(factoryDocsFileSystem)
	if err != nil {
		return nil, fmt.Errorf("construct stateless Workers: %w", err)
	}
	scriptRunner := workers.LoggingCommandRunner{
		Runner: scriptCommandRunner,
		Logger: logging.NoopLogger{},
		Clock:  workers.ClockFunc(clock.Now),
	}
	return workerswire.NewService(
		workerswire.AgentDependencies{
			Providers: providersService,
			Publish:   func(workers.ProgressFragment) {},
		},
		workerswire.ScriptConfig{RequestSelected: true},
		workerswire.ScriptDependencies{
			CommandRunner: scriptRunner,
			FactoryDocs:   factoryDocs,
			Now:           clock.Now,
			Publish:       func(workers.ProgressFragment) {},
			Record:        func(workers.ScriptEvent) {},
		},
		workerswire.InferenceConfig{
			Worker: models.LocalWorker{
				Name: "request-selected-inference",
				Type: factorydefinitions.WorkerTypeInference,
			},
		},
		workerswire.InferenceDependencies{Models: modelsService},
		nil,
		logging.NewZapLogger(logger, false),
		clock.Now,
		worktreeLifecycle,
		worktreeLifecycle,
		temporaryFiles,
	)
}
