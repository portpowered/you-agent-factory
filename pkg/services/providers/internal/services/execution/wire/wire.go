// Package wire constructs the parent-private Providers execution subservice.
package wire

import (
	"context"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	platformpty "github.com/portpowered/infinite-you/pkg/platform/pty"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	acp "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/acp"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty"
	claudeadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/claude"
	codexadapter "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/service"
)

// NewAgyPTYAllocator constructs the Providers-owned PTY implementation behind
// the Workers root allocation port.
func NewAgyPTYAllocator(host platformpty.Host, clock platformclock.Source) (providerservice.PTYAllocator, error) {
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
func NewACPRegistration(id providers.ID, service acp.ContinuationService) execution.Registration {
	return execution.Registration{
		Provider: id,
		Attempt: func(ctx context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
			return service.Execute(ctx, id, request)
		},
		Continue: func(ctx context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
			if request.ResumeSession == nil {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind:    providers.ExecuteFailureKindInvalidRequest,
					Message: "provider continuation is missing its session reference",
				}
			}
			return service.Continue(ctx, id, request.ExecuteRequest, *request.ResumeSession)
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
	return BuiltInDependenciesFromCommandRunner(AdaptPlatformCommandRunner(runner))
}

// BuiltInDependenciesFromWorkersRunner is a compatibility entry point for
// older composition tests. The provider package accepts the edge opaquely and
// projects its named request/result fields at this boundary.
func BuiltInDependenciesFromWorkersRunner(
	runner any,
	platform ...BuiltInRunnerPlatformDependencies,
) executionservice.BuiltInDependencies {
	return BuiltInDependenciesFromCommandRunner(providerservice.AdaptCommandRunner(runner), platform...)
}

// AgyPTYPlatformDependencies are platform facts required for the built-in Agy
// PTY execution adapter.
type AgyPTYPlatformDependencies struct {
	Allocator agypty.PTYAllocator
	Locator   platformprocess.ExecutableLocator
	Inspector platformfilesystem.PathInspector
}

// BuiltInRunnerPlatformDependencies carries optional platform facts for
// built-in adapter effects constructed from the Providers subprocess effect.
type BuiltInRunnerPlatformDependencies struct {
	AgyCommandRunner providerservice.CommandRunner
	AgyCommandClock  platformclock.Source
	AgyPTY           AgyPTYPlatformDependencies
}

// BuiltInDependenciesFromCommandRunner constructs built-in adapter effects
// from the shared Providers subprocess effect.
func BuiltInDependenciesFromCommandRunner(
	runner providerservice.CommandRunner,
	platform ...BuiltInRunnerPlatformDependencies,
) executionservice.BuiltInDependencies {
	var deps BuiltInRunnerPlatformDependencies
	if len(platform) > 0 {
		deps = platform[0]
	}
	antigravity := agyadapter.NewPTYEffect(agyadapter.PTYEffectOptions{
		Allocator: deps.AgyPTY.Allocator,
		ExecutableDependencies: agyadapter.ExecutableDependencies{
			Locator:   deps.AgyPTY.Locator,
			Inspector: deps.AgyPTY.Inspector,
		},
	})
	if deps.AgyCommandRunner != nil {
		clock := deps.AgyCommandClock
		if clock == nil {
			clock = platformclock.Real{}
		}
		antigravity = agyadapter.NewCommandEffect(deps.AgyCommandRunner, clock)
	}
	return executionservice.BuiltInDependencies{
		Antigravity: antigravity,
		Codex:       codexadapter.NewCommandEffect(runner, platformclock.Real{}),
		Claude:      claudeadapter.NewCommandEffect(runner, platformclock.Real{}),
	}
}

// AdaptPlatformCommandRunner projects the policy-free platform process edge
// into the Providers-owned execution effect contract.
func AdaptPlatformCommandRunner(runner platformprocess.CommandRunner) providerservice.CommandRunner {
	if runner == nil {
		return nil
	}
	return platformCommandRunner{runner: runner}
}

type platformCommandRunner struct {
	runner platformprocess.CommandRunner
}

func (runner platformCommandRunner) Run(
	ctx context.Context,
	request providerservice.CommandRequest,
) (providerservice.CommandResult, error) {
	result, err := runner.runner.Run(ctx, platformprocess.CommandRequest{
		Command:                  request.Command,
		Args:                     request.Args,
		Stdin:                    request.Stdin,
		Env:                      request.Env,
		WorkDir:                  request.WorkDir,
		ExecutionScopeID:         request.FactorySessionID,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	})
	return providerservice.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func (runner platformCommandRunner) RunStreaming(
	ctx context.Context,
	request providerservice.CommandRequest,
	observer providerservice.OutputChunkObserver,
) (providerservice.CommandResult, error) {
	platformRequest := platformprocess.CommandRequest{
		Command:                  request.Command,
		Args:                     request.Args,
		Stdin:                    request.Stdin,
		Env:                      request.Env,
		WorkDir:                  request.WorkDir,
		ExecutionScopeID:         request.FactorySessionID,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
	streaming, ok := runner.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if !ok {
		result, err := runner.Run(ctx, request)
		if observerErr := publishCompleteOutput(observer, result.Stdout, result.Stderr); err == nil {
			err = observerErr
		}
		return result, err
	}
	var observerErr error
	result, err := streaming.RunStreaming(ctx, platformRequest, func(stream string, chunk []byte) {
		if observerErr != nil || observer == nil {
			return
		}
		observerErr = observer(stream, chunk)
	})
	if err == nil {
		err = observerErr
	}
	return providerservice.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func publishCompleteOutput(observer providerservice.OutputChunkObserver, stdout, stderr []byte) error {
	if observer == nil {
		return nil
	}
	if len(stdout) > 0 {
		if err := observer(providerservice.OutputStreamStdout, append([]byte(nil), stdout...)); err != nil {
			return err
		}
	}
	if len(stderr) > 0 {
		return observer(providerservice.OutputStreamStderr, append([]byte(nil), stderr...))
	}
	return nil
}
