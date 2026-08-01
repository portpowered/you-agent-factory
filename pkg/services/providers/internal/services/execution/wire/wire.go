// Package wire constructs the parent-private Providers execution subservice.
package wire

import (
	"context"
	"errors"
	"sync"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source) (effects.PTYAllocator, error) {
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
	runner effects.CommandRunner,
	platform ...BuiltInRunnerPlatformDependencies,
) executionservice.BuiltInDependencies {
	return builtInDependenciesFromRunner(runner, platform...)
}

// AdaptPlatformCommandRunner projects the policy-free process edge into the
// Providers-owned private effect contract.
func AdaptPlatformCommandRunner(runner platformprocess.CommandRunner) effects.CommandRunner {
	if runner == nil {
		return nil
	}
	return platformCommandRunner{runner: runner}
}

// AgyPTYPlatformDependencies are platform facts required for the built-in Agy
// PTY execution adapter.
type AgyPTYPlatformDependencies struct {
	Allocator effects.PTYAllocator
	Clock     platformclock.Source
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// BuiltInRunnerPlatformDependencies carries optional platform facts for
// built-in adapter effects constructed from the Providers subprocess effect.
type BuiltInRunnerPlatformDependencies struct {
	Clock  platformclock.Source
	AgyPTY AgyPTYPlatformDependencies
}

func builtInDependenciesFromRunner(
	runner effects.CommandRunner,
	platform ...BuiltInRunnerPlatformDependencies,
) executionservice.BuiltInDependencies {
	var deps BuiltInRunnerPlatformDependencies
	if len(platform) > 0 {
		deps = platform[0]
	}
	return executionservice.BuiltInDependencies{
		Antigravity: agyadapter.NewPTYEffect(agyadapter.PTYEffectOptions{
			Allocator: deps.AgyPTY.Allocator,
			Clock:     deps.AgyPTY.Clock,
			ExecutableDependencies: agyadapter.ExecutableDependencies{
				Locator:   deps.AgyPTY.Locator,
				Inspector: deps.AgyPTY.Inspector,
			},
		}),
		Codex:  codexadapter.NewCommandEffect(runner, deps.Clock),
		Claude: claudeadapter.NewCommandEffect(runner, deps.Clock),
	}
}

type platformCommandRunner struct {
	runner platformprocess.CommandRunner
}

func (runner platformCommandRunner) Run(
	ctx context.Context,
	request effects.CommandRequest,
) (effects.CommandResult, error) {
	result, err := runner.runner.Run(ctx, platformprocess.CommandRequest{
		Command: request.Command,
		Args:    request.Args,
		Stdin:   request.Stdin,
		Env:     request.Env,
		WorkDir: request.WorkDir,
	})
	return effects.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func (runner platformCommandRunner) RunStreaming(
	ctx context.Context,
	request effects.CommandRequest,
	observer effects.OutputChunkObserver,
) (effects.CommandResult, error) {
	streaming, ok := runner.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	platformRequest := platformprocess.CommandRequest{
		Command: request.Command,
		Args:    request.Args,
		Stdin:   request.Stdin,
		Env:     request.Env,
		WorkDir: request.WorkDir,
	}
	if !ok {
		result, err := runner.Run(ctx, request)
		err = errors.Join(err, publishCompleteOutput(observer, result.Stdout, result.Stderr))
		return result, err
	}
	var observerMu sync.Mutex
	var observerErr error
	result, err := streaming.RunStreaming(ctx, platformRequest, func(stream string, chunk []byte) {
		if observer == nil {
			return
		}
		observerMu.Lock()
		defer observerMu.Unlock()
		if observerErr == nil {
			observerErr = observer(stream, chunk)
		}
	})
	observerMu.Lock()
	err = errors.Join(err, observerErr)
	observerMu.Unlock()
	return effects.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func publishCompleteOutput(observer effects.OutputChunkObserver, stdout, stderr []byte) error {
	if observer == nil {
		return nil
	}
	if len(stdout) > 0 {
		if err := observer(effects.OutputStreamStdout, append([]byte(nil), stdout...)); err != nil {
			return err
		}
	}
	if len(stderr) > 0 {
		return observer(effects.OutputStreamStderr, append([]byte(nil), stderr...))
	}
	return nil
}
