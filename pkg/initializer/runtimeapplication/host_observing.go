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
			return <-runResult
		}
		err := <-runResult
		if err == nil && !errors.Is(result.err, context.Canceled) {
			err = result.err
		}
		return runtimeHostStartupResult(ctx, runner.runtimeHostReadinessConfigured(), err)
	case err := <-runResult:
		var readinessErr error
		select {
		case result := <-readyResult:
			if result.err == nil {
				runner.onReady(result.binding)
				cancelReady()
				return err
			}
			readinessErr = result.err
		default:
		}
		cancelReady()
		if readinessErr == nil && runner.runtimeHostReadinessConfigured() {
			result := <-readyResult
			if result.err == nil {
				runner.onReady(result.binding)
			} else {
				readinessErr = result.err
			}
		}
		if err == nil && readinessErr != nil && !errors.Is(readinessErr, context.Canceled) {
			err = readinessErr
		}
		return runtimeHostStartupResult(ctx, runner.runtimeHostReadinessConfigured(), err)
	case <-ctx.Done():
		cancelReady()
		// Let the managed lifecycle normalize ordinary cancellation after it
		// has stopped and joined its components.
		return <-runResult
	}
}

func (runner hostObservingRunner) runtimeHostReadinessConfigured() bool {
	probe, ok := runner.runner.(runtimeHostReadinessProbe)
	if !ok {
		return true
	}
	return probe.RuntimeHostReadinessConfigured()
}

func runtimeHostStartupResult(ctx context.Context, configured bool, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || !configured || (ctx != nil && ctx.Err() != nil) {
		return err
	}
	return &initializer.RuntimeHostStartupError{Cause: err}
}

func (runner hostObservingRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	managed, ok := runner.runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		return runner.runner.Run(ctx)
	}
	if !runner.runtimeHostReadinessConfigured() {
		return managed.RunWithCompletion(ctx, completion)
	}
	readinessObserved := false
	err := managed.RunWithCompletion(ctx, func(completionCtx context.Context) error {
		binding, bindingErr := runner.runner.(runtimeHostReader).RuntimeHostBinding(completionCtx)
		if bindingErr != nil && !isRuntimeHostUnavailable(bindingErr) {
			return bindingErr
		}
		if bindingErr == nil {
			readinessObserved = true
			runner.onReady(binding)
		}
		return completion(completionCtx)
	})
	if readinessObserved {
		return err
	}
	return runtimeHostStartupResult(ctx, true, err)
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
