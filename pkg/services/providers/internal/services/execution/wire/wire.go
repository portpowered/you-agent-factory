// Package wire constructs the parent-private Providers execution subservice.
package wire

import (
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	cursoradapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	geminiadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/gemini"
	kiroadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/kiro"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

// NewService constructs an inert execution service over the supplied canonical
// catalog and private adapter registrations.
func NewService(
	catalogService catalog.Service,
	registrations ...execution.Registration,
) (execution.Service, error) {
	return executionservice.New(catalogService, registrations...)
}

// NewBuiltInService constructs an inert execution service with the native
// adapters owned by Providers Execution.
func NewBuiltInService(
	catalogService catalog.Service,
	dependencies ...executionservice.BuiltInDependencies,
) (execution.Service, error) {
	return NewService(catalogService, executionservice.BuiltInRegistrations(dependencies...)...)
}

// BuiltInDependenciesFromRunner constructs built-in adapter effects from the
// shared platform process runner.
func BuiltInDependenciesFromRunner(
	runner platformprocess.CommandRunner,
) executionservice.BuiltInDependencies {
	return BuiltInDependenciesFromWorkersRunner(workerprocess.AdaptCommandRunner(runner))
}

// CursorPlatformDependencies are platform facts required for oversized Windows
// Cursor prompt materialization in the built-in Providers Execution adapter.
type CursorPlatformDependencies struct {
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

// BuiltInDependenciesFromWorkersRunner constructs built-in adapter effects
// from the shared Workers subprocess runner.
func BuiltInDependenciesFromWorkersRunner(
	runner workers.CommandRunner,
	cursorPlatform ...CursorPlatformDependencies,
) executionservice.BuiltInDependencies {
	var platform CursorPlatformDependencies
	if len(cursorPlatform) > 0 {
		platform = cursorPlatform[0]
	}
	return executionservice.BuiltInDependencies{
		Codex:  codexadapter.NewCommandEffect(runner),
		Claude: claudeadapter.NewCommandEffect(runner),
		Cursor: cursoradapter.NewCommandEffect(runner, cursoradapter.CommandEffectOptions{
			OperatingSystem: platform.OperatingSystem,
			TemporaryDir:    platform.TemporaryDir,
			TemporaryFiles:  platform.TemporaryFiles,
		}),
		Gemini:   geminiadapter.NewCommandEffect(runner),
		Kiro:     kiroadapter.NewCommandEffect(runner),
		OpenCode: opencodeadapter.NewCommandEffect(runner),
	}
}
