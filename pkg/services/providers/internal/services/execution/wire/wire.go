// Package wire constructs the parent-private Providers execution subservice.
package wire

import (
	"context"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	cursoradapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/cursor"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation behind
// the Workers root allocation port.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source) (workers.PTYAllocator, error) {
	return agypty.NewAllocator(host, clock)
}

// NewService constructs an inert execution service over the supplied canonical
// catalog and private adapter registrations.
func NewService(
	catalogService catalog.Service,
	registrations ...execution.Registration,
) (execution.Service, error) {
	return executionservice.New(catalogService, registrations...)
}

// NewACPRegistration delegates one configured ACP identity to the already
// constructed parent-private ACP service.
func NewACPRegistration(id providers.ID, service acp.Service) execution.Registration {
	return execution.Registration{
		Provider: id,
		Attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return service.Execute(ctx, id, request)
		},
	}
}

// NewBuiltInService constructs an inert execution service with the native
// adapters owned by Providers Execution.
func NewBuiltInService(
	catalogService catalog.Service,
	dependencies ...executionservice.BuiltInDependencies,
) (execution.Service, error) {
	return NewService(catalogService, executionservice.BuiltInRegistrations(dependencies...)...)
}

// BuiltInRegistrations exposes the complete native registration set to the
// Providers root so configured ACP registrations can be appended.
func BuiltInRegistrations(dependencies ...executionservice.BuiltInDependencies) []execution.Registration {
	return executionservice.BuiltInRegistrations(dependencies...)
}

// BuiltInDependenciesFromRunner constructs built-in adapter effects from the
// shared platform process runner.
func BuiltInDependenciesFromRunner(
	runner platformprocess.CommandRunner,
) executionservice.BuiltInDependencies {
	return BuiltInDependenciesFromWorkersRunner(workers.AdaptCommandRunner(runner))
}

// CursorPlatformDependencies are platform facts required for oversized Windows
// Cursor prompt materialization in the built-in Providers Execution adapter.
type CursorPlatformDependencies struct {
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

// AgyPTYPlatformDependencies are platform facts required for the built-in Agy
// PTY execution adapter.
type AgyPTYPlatformDependencies struct {
	Allocator agypty.PTYAllocator
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// BuiltInRunnerPlatformDependencies carries optional platform facts for
// built-in adapter effects constructed from the Workers subprocess runner.
type BuiltInRunnerPlatformDependencies struct {
	Cursor CursorPlatformDependencies
	AgyPTY AgyPTYPlatformDependencies
}

// BuiltInDependenciesFromWorkersRunner constructs built-in adapter effects
// from the shared Workers subprocess runner.
func BuiltInDependenciesFromWorkersRunner(
	runner workers.CommandRunner,
	platform ...BuiltInRunnerPlatformDependencies,
) executionservice.BuiltInDependencies {
	var deps BuiltInRunnerPlatformDependencies
	if len(platform) > 0 {
		deps = platform[0]
	}
	return executionservice.BuiltInDependencies{
		Antigravity: agyadapter.NewPTYEffect(agyadapter.PTYEffectOptions{
			Allocator: deps.AgyPTY.Allocator,
			ExecutableDependencies: agyadapter.ExecutableDependencies{
				Locator:   deps.AgyPTY.Locator,
				Inspector: deps.AgyPTY.Inspector,
			},
		}),
		Codex:  codexadapter.NewCommandEffect(runner),
		Claude: claudeadapter.NewCommandEffect(runner),
		Cursor: cursoradapter.NewCommandEffect(runner, cursoradapter.CommandEffectOptions{
			OperatingSystem: deps.Cursor.OperatingSystem,
			TemporaryDir:    deps.Cursor.TemporaryDir,
			TemporaryFiles:  deps.Cursor.TemporaryFiles,
		}),
	}
}
