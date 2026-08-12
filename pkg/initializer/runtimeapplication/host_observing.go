package runtimeapplication

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

type runtimeHostReader interface {
	RuntimeHostBinding(context.Context) (initializer.RuntimeHostBinding, error)
}

type runtimeHostReadinessProbe interface {
	RuntimeHostReadinessConfigured() bool
}

type hostObservingRunner struct {
	runner  initializer.LocalRuntimeRunner
	onReady func(initializer.RuntimeHostBinding)
}

// WithRuntimeHostObserver keeps host-readiness ordering inside the initializer
// lifecycle boundary. The transport supplies only a value callback for the
// already-bound endpoint; it does not own lifecycle goroutines or channels.
func WithRuntimeHostObserver(
	runner initializer.LocalRuntimeRunner,
	onReady func(initializer.RuntimeHostBinding),
) initializer.LocalRuntimeRunner {
	if runner == nil || onReady == nil {
		return runner
	}
	if _, ok := runner.(runtimeHostReader); !ok {
		return runner
	}
	return hostObservingRunner{runner: runner, onReady: onReady}
}

func (runner hostObservingRunner) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	reader, ok := runner.runner.(runtimeHostReader)
	if !ok {
		return runner.runner.Run(ctx)
	}

	readyCtx, cancelReady := context.WithCancel(ctx)
	defer cancelReady()

	runResult := make(chan error, 1)
	go func() {
		runResult <- runner.runner.Run(ctx)
	}()
	readyResult := make(chan runtimeHostResult, 1)
	go func() {
		binding, err := reader.RuntimeHostBinding(readyCtx)
		readyResult <- runtimeHostResult{binding: binding, err: err}
	}()

	select {
	case result := <-readyResult:
		if result.err == nil {
			runner.onReady(result.binding)
		}
		return <-runResult
	case err := <-runResult:
		cancelReady()
		if err == nil {
			if probe, ok := runner.runner.(runtimeHostReadinessProbe); ok && probe.RuntimeHostReadinessConfigured() {
				result := <-readyResult
				if result.err == nil {
					runner.onReady(result.binding)
				}
			} else {
				select {
				case result := <-readyResult:
					if result.err == nil {
						runner.onReady(result.binding)
					}
				default:
				}
			}
		}
		return err
	case <-ctx.Done():
		cancelReady()
		// Let the managed lifecycle normalize ordinary cancellation after it
		// has stopped and joined its components.
		return <-runResult
	}
}

func (runner hostObservingRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, func(completionCtx context.Context) error {
		binding, err := runner.runner.(runtimeHostReader).RuntimeHostBinding(completionCtx)
		if err != nil && !isRuntimeHostUnavailable(err) {
			return err
		}
		if err == nil {
			runner.onReady(binding)
		}
		return completion(completionCtx)
	})
}

func (runner hostObservingRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	return runner.runner.(runtimeHostReader).RuntimeHostBinding(ctx)
}

func (runner hostObservingRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	if provider, ok := runner.runner.(interface {
		RuntimeLogDiagnostics() runtimeartifact.Diagnostics
	}); ok {
		return provider.RuntimeLogDiagnostics()
	}
	return runtimeartifact.Diagnostics{}
}

type runtimeHostResult struct {
	binding initializer.RuntimeHostBinding
	err     error
}

func isRuntimeHostUnavailable(err error) bool {
	return errors.Is(err, initializer.ErrRuntimeHostReadinessUnavailable)
}
