package runtimeapplication

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// Both completion orders must retain the startup cause when lifecycle cleanup
// also fails. Controlled result channels avoid racing the observer goroutines.
func TestHostObserverPreservesReadinessAndCleanupFailures(t *testing.T) {
	t.Parallel()
	for _, order := range []string{"readiness first", "run first", "reader delivers during cancellation"} {
		t.Run(order, func(t *testing.T) {
			t.Parallel()
			primary := &fs.PathError{Op: "read recording", Path: "retained.json", Err: fs.ErrPermission}
			cleanup := &fs.PathError{Op: "close recording", Path: "retained.json", Err: fs.ErrClosed}
			runner := hostObservingRunner{
				runner:  &hostReadinessRunner{readinessConfigured: true},
				onReady: func(initializer.RuntimeHostBinding) { t.Error("failed startup reported readiness") },
			}
			var err error
			if order == "readiness first" {
				runResult := make(chan error, 1)
				runResult <- cleanup
				err = runner.finishAfterReadinessResult(t.Context(), runtimeHostResult{err: primary}, runResult)
			} else {
				readyResult := make(chan runtimeHostResult, 1)
				cancel := func() {}
				if order == "run first" {
					readyResult <- runtimeHostResult{err: primary}
				} else {
					var delivered sync.Once
					cancel = func() { delivered.Do(func() { readyResult <- runtimeHostResult{err: primary} }) }
				}
				err = runner.finishAfterRunResult(t.Context(), cleanup, readyResult, cancel)
			}
			var startupErr *initializer.RuntimeHostStartupError
			if !errors.As(err, &startupErr) || !errors.Is(err, primary) || !errors.Is(err, cleanup) {
				t.Fatalf("startup error = %v, want classified primary and cleanup causes", err)
			}
			diagnostic := logging.SafeErrorCause(err)
			for _, want := range []string{`read recording "retained.json": permission denied`, `close recording "retained.json": file already closed`} {
				if !strings.Contains(diagnostic, want) {
					t.Fatalf("safe diagnostic %q omits %q", diagnostic, want)
				}
			}
		})
	}
}

func TestHostObservingRunnerReportsReadinessAndJoinsCancellation(t *testing.T) {
	transport := &hostObservingTestComponent{started: make(chan struct{})}
	managed, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	ready := make(chan initializer.RuntimeHostBinding, 1)
	managed.SetRuntimeHostReady(ready)

	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := WithRuntimeHostObserver(managed, func(binding initializer.RuntimeHostBinding) {
		observed <- binding
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	completed := make(chan error, 1)
	go func() {
		completed <- runner.Run(ctx)
	}()

	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatal("transport did not start")
	}
	ready <- initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437}
	select {
	case binding := <-observed:
		if binding.Port != 7437 {
			t.Fatalf("observed binding port = %d, want 7437", binding.Port)
		}
	case <-time.After(time.Second):
		t.Fatal("readiness was not observed")
	}

	cancel()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("Run after ordinary cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not join after cancellation")
	}
}

func TestHostObserverClassifiesPrimaryFailureDespiteCleanupCancellation(t *testing.T) {
	t.Parallel()
	primary := &fs.PathError{Op: "read recording", Path: "retained.json", Err: fs.ErrPermission}
	runner := hostObservingRunner{runner: &hostReadinessRunner{readinessConfigured: true}}
	runResult := make(chan error, 1)
	runResult <- context.Canceled
	err := runner.finishAfterReadinessResult(t.Context(), runtimeHostResult{err: errors.Join(primary, context.Canceled)}, runResult)
	var startupErr *initializer.RuntimeHostStartupError
	if !errors.As(err, &startupErr) || !errors.Is(err, primary) || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want classified primary with cleanup cancellation identity", err)
	}
}

func TestHostObservingRunnerWaitsForManagedCancellationResult(t *testing.T) {
	transport := &hostObservingTestComponent{started: make(chan struct{})}
	managed, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	managed.SetRuntimeHostReady(make(chan initializer.RuntimeHostBinding))
	observed := make(chan struct{}, 1)
	runner := WithRuntimeHostObserver(managed, func(initializer.RuntimeHostBinding) {
		observed <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	completed := make(chan error, 1)
	go func() {
		completed <- runner.Run(ctx)
	}()
	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatal("transport did not start")
	}
	cancel()

	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("Run after ordinary cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not return after cancellation")
	}
	select {
	case <-observed:
		t.Fatal("readiness callback ran without a binding")
	default:
	}
}

func TestHostObservingRunnerForwardsReadinessAndDiagnostics(t *testing.T) {
	diagnostics := runtimeartifact.Diagnostics{Path: "runtime.log"}
	transport := &readinessComponent{started: make(chan struct{})}
	managed, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, diagnostics)
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	ready := make(chan initializer.RuntimeHostBinding, 1)
	managed.SetRuntimeHostReady(ready)
	expected := initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 8743}
	ready <- expected

	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := WithRuntimeHostObserver(managed, func(binding initializer.RuntimeHostBinding) {
		observed <- binding
	})
	reader, ok := runner.(runtimeHostReader)
	if !ok {
		t.Fatal("wrapped runner does not expose host readiness")
	}
	if got, err := reader.RuntimeHostBinding(t.Context()); err != nil || got != expected {
		t.Fatalf("RuntimeHostBinding() = %#v, %v; want %#v, nil", got, err, expected)
	}
	diagnosticProvider, ok := runner.(interface {
		RuntimeLogDiagnostics() runtimeartifact.Diagnostics
	})
	if !ok {
		t.Fatal("wrapped runner does not expose diagnostics")
	}
	if diagnosticProvider.RuntimeLogDiagnostics() != diagnostics {
		t.Fatalf("wrapped RuntimeLogDiagnostics() = %#v, want %#v", diagnosticProvider.RuntimeLogDiagnostics(), diagnostics)
	}
}

func TestHostObservingRunnerRunsCompletionAfterReadiness(t *testing.T) {
	transport := &readinessComponent{started: make(chan struct{})}
	managed, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	ready := make(chan initializer.RuntimeHostBinding, 1)
	managed.SetRuntimeHostReady(ready)
	expected := initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 8743}
	ready <- expected
	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := WithRuntimeHostObserver(managed, func(binding initializer.RuntimeHostBinding) {
		observed <- binding
	})
	completionRunner, ok := runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		t.Fatal("wrapped runner does not expose completion operation")
	}
	completed := make(chan error, 1)
	completionCalled := make(chan struct{})
	go func() {
		completed <- completionRunner.RunWithCompletion(t.Context(), func(context.Context) error {
			close(completionCalled)
			return nil
		})
	}()
	select {
	case <-completionCalled:
	case <-time.After(time.Second):
		t.Fatal("completion was not called")
	}
	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("RunWithCompletion: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunWithCompletion did not finish")
	}
	select {
	case got := <-observed:
		if got != expected {
			t.Fatalf("observed binding = %#v, want %#v", got, expected)
		}
	case <-time.After(time.Second):
		t.Fatal("wrapped runner did not report readiness")
	}
}

func TestHostObservingRunnerRunsCompletionWithoutReadiness(t *testing.T) {
	noReadyTransport := &readinessComponent{started: make(chan struct{})}
	noReady, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: noReadyTransport, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner without readiness: %v", err)
	}
	noReadyRunner := WithRuntimeHostObserver(noReady, func(initializer.RuntimeHostBinding) {
		t.Fatal("readiness callback ran without a configured host")
	})
	noReadyCompletion, ok := noReadyRunner.(initializer.CompletionRuntimeRunner)
	if !ok {
		t.Fatal("no-readiness runner does not expose completion operation")
	}
	if err := noReadyCompletion.RunWithCompletion(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("RunWithCompletion without readiness: %v", err)
	}
}

func TestHostObservingRunnerReportsBindingAfterUnderlyingRunCompletes(t *testing.T) {
	runStarted := make(chan struct{})
	runRelease := make(chan struct{})
	bindingRequested := make(chan struct{})
	bindingRelease := make(chan struct{})
	base := &hostReadinessRunner{
		runStarted:                runStarted,
		runRelease:                runRelease,
		bindingRequested:          bindingRequested,
		bindingRelease:            bindingRelease,
		readinessConfigured:       true,
		binding:                   initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 9461},
		ignoreBindingCancellation: true,
	}
	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := WithRuntimeHostObserver(base, func(binding initializer.RuntimeHostBinding) {
		observed <- binding
	})
	completed := make(chan error, 1)
	go func() { completed <- runner.Run(t.Context()) }()
	select {
	case <-runStarted:
	case <-time.After(time.Second):
		t.Fatal("underlying runner did not start")
	}
	close(runRelease)
	select {
	case <-bindingRequested:
	case <-time.After(time.Second):
		t.Fatal("readiness was not requested after the run completed")
	}
	close(bindingRelease)
	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("wrapped runner did not finish")
	}
	select {
	case binding := <-observed:
		if binding.Port != 9461 {
			t.Fatalf("observed binding port = %d, want 9461", binding.Port)
		}
	case <-time.After(time.Second):
		t.Fatal("readiness callback was not called")
	}
}

func TestHostObservingRunnerClassifiesPreReadinessFailure(t *testing.T) {
	cause := errors.New("runtime startup failed")
	base := &hostReadinessRunner{
		readinessConfigured: true,
		bindingRelease:      make(chan struct{}),
		runErr:              cause,
	}
	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := WithRuntimeHostObserver(base, func(binding initializer.RuntimeHostBinding) {
		observed <- binding
	})

	err := runner.Run(context.Background())
	var startupErr *initializer.RuntimeHostStartupError
	if !errors.As(err, &startupErr) {
		t.Fatalf("Run() error = %v, want RuntimeHostStartupError", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Run() error = %v, want cause %v", err, cause)
	}
	select {
	case binding := <-observed:
		t.Fatalf("pre-readiness failure reported binding %#v", binding)
	default:
	}
}

func TestHostObservingRunnerPreservesFailureAfterReadiness(t *testing.T) {
	cause := errors.New("runtime stopped after binding")
	runRelease := make(chan struct{})
	base := &hostReadinessRunner{
		runStarted:          make(chan struct{}),
		runRelease:          runRelease,
		readinessConfigured: true,
		binding:             initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437},
		runErr:              cause,
	}
	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := WithRuntimeHostObserver(base, func(binding initializer.RuntimeHostBinding) {
		observed <- binding
	})
	completed := make(chan error, 1)
	go func() { completed <- runner.Run(context.Background()) }()
	select {
	case binding := <-observed:
		if binding.Port != 7437 {
			t.Fatalf("observed binding port = %d, want 7437", binding.Port)
		}
	case <-time.After(time.Second):
		t.Fatal("readiness was not observed")
	}
	close(runRelease)
	select {
	case err := <-completed:
		if !errors.Is(err, cause) {
			t.Fatalf("Run() error = %v, want cause %v", err, cause)
		}
		var startupErr *initializer.RuntimeHostStartupError
		if errors.As(err, &startupErr) {
			t.Fatalf("post-readiness error = %v, must not be RuntimeHostStartupError", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runner did not return after readiness failure")
	}
}

func TestHostObservingRunnerHandlesBufferedReadinessWhenRunCompletesFirst(t *testing.T) {
	tests := []struct {
		name   string
		runErr error
	}{
		{name: "successful run", runErr: nil},
		{name: "failed run", runErr: errors.New("runtime stopped after binding")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			readyResult := make(chan runtimeHostResult, 1)
			expected := initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437}
			readyResult <- runtimeHostResult{binding: expected}
			observed := make(chan initializer.RuntimeHostBinding, 1)
			runner := hostObservingRunner{
				runner: &hostReadinessRunner{readinessConfigured: true},
				onReady: func(binding initializer.RuntimeHostBinding) {
					observed <- binding
				},
			}
			completed := make(chan error, 1)
			go func() {
				completed <- runner.finishAfterRunResult(
					context.Background(),
					tt.runErr,
					readyResult,
					func() {},
				)
			}()

			select {
			case err := <-completed:
				if tt.runErr == nil {
					if err != nil {
						t.Fatalf("finishAfterRunResult() = %v, want nil", err)
					}
					break
				}
				if !errors.Is(err, tt.runErr) {
					t.Fatalf("finishAfterRunResult() = %v, want %v", err, tt.runErr)
				}
				var startupErr *initializer.RuntimeHostStartupError
				if errors.As(err, &startupErr) {
					t.Fatalf("post-readiness error = %v, must not be RuntimeHostStartupError", err)
				}
			case <-time.After(time.Second):
				t.Fatal("finishAfterRunResult() blocked after consuming buffered readiness")
			}
			select {
			case binding := <-observed:
				if binding != expected {
					t.Fatalf("observed binding = %#v, want %#v", binding, expected)
				}
			default:
				t.Fatal("buffered readiness was not observed")
			}
		})
	}
}

func TestHostObservingRunnerClassifiesPreReadinessCompletionFailure(t *testing.T) {
	cause := errors.New("completion transport failed before binding")
	runner := WithRuntimeHostObserver(
		completionHostFailureRunner{err: cause},
		func(initializer.RuntimeHostBinding) { t.Fatal("readiness callback ran before failure") },
	)
	completionRunner, ok := runner.(initializer.CompletionRuntimeRunner)
	if !ok {
		t.Fatal("wrapped runner does not expose completion operation")
	}
	err := completionRunner.RunWithCompletion(context.Background(), func(context.Context) error {
		t.Fatal("completion ran before readiness failure")
		return nil
	})
	var startupErr *initializer.RuntimeHostStartupError
	if !errors.As(err, &startupErr) || !errors.Is(err, cause) {
		t.Fatalf("RunWithCompletion() error = %v, want pre-readiness startup failure %v", err, cause)
	}
}

func TestHostObservingRunnerDelegatesUnsupportedCapabilities(t *testing.T) {
	plain := &plainLocalRuntimeRunner{}
	if got := WithRuntimeHostObserver(plain, func(initializer.RuntimeHostBinding) {}); got != plain {
		t.Fatal("plain runner was wrapped without host-readiness support")
	}
	if got := WithRuntimeHostObserver(nil, func(initializer.RuntimeHostBinding) {}); got != nil {
		t.Fatal("nil runner was wrapped")
	}

	sentinel := errors.New("plain runner result")
	base := &hostReadinessRunner{runErr: sentinel, binding: initializer.RuntimeHostBinding{Port: 1}}
	runner := WithRuntimeHostObserver(base, func(initializer.RuntimeHostBinding) {})
	completionRunner, ok := runner.(interface {
		RunWithCompletion(context.Context, initializer.CompletionOperation) error
	})
	if !ok {
		t.Fatal("wrapped runner does not expose completion operation")
	}
	if err := completionRunner.RunWithCompletion(t.Context(), nil); !errors.Is(err, sentinel) {
		t.Fatalf("unsupported completion capability error = %v, want %v", err, sentinel)
	}
	reader, ok := runner.(runtimeHostReader)
	if !ok {
		t.Fatal("wrapped runner does not expose host readiness")
	}
	if got, err := reader.RuntimeHostBinding(t.Context()); err != nil || got.Port != 1 {
		t.Fatalf("RuntimeHostBinding() = %#v, %v; want port 1, nil", got, err)
	}
	diagnostics, ok := runner.(interface {
		RuntimeLogDiagnostics() runtimeartifact.Diagnostics
	})
	if !ok || diagnostics.RuntimeLogDiagnostics() != (runtimeartifact.Diagnostics{}) {
		t.Fatal("wrapped runner leaked non-existent diagnostics")
	}
}

type hostReadinessRunner struct {
	runStarted                chan struct{}
	runRelease                <-chan struct{}
	runErr                    error
	bindingRequested          chan struct{}
	bindingRelease            <-chan struct{}
	binding                   initializer.RuntimeHostBinding
	readinessConfigured       bool
	ignoreBindingCancellation bool
}

func (runner *hostReadinessRunner) Run(ctx context.Context) error {
	if runner.runStarted != nil {
		close(runner.runStarted)
	}
	if runner.runRelease != nil {
		select {
		case <-runner.runRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return runner.runErr
}

func (runner *hostReadinessRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	if runner.bindingRequested != nil {
		close(runner.bindingRequested)
	}
	if runner.bindingRelease != nil {
		if runner.ignoreBindingCancellation {
			<-runner.bindingRelease
			return runner.binding, nil
		}
		select {
		case <-runner.bindingRelease:
		case <-ctx.Done():
			return initializer.RuntimeHostBinding{}, ctx.Err()
		}
	}
	return runner.binding, nil
}

func (runner *hostReadinessRunner) RuntimeHostReadinessConfigured() bool {
	return runner.readinessConfigured
}

func (runner *hostReadinessRunner) Stop(context.Context) error { return nil }

type plainLocalRuntimeRunner struct{}

func (*plainLocalRuntimeRunner) Run(context.Context) error { return nil }

type completionHostFailureRunner struct {
	err error
}

func (runner completionHostFailureRunner) Run(context.Context) error { return runner.err }

func (runner completionHostFailureRunner) RunWithCompletion(context.Context, initializer.CompletionOperation) error {
	return runner.err
}

func (completionHostFailureRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	<-ctx.Done()
	return initializer.RuntimeHostBinding{}, ctx.Err()
}

func (completionHostFailureRunner) RuntimeHostReadinessConfigured() bool { return true }

type hostObservingTestComponent struct {
	started chan struct{}
}

func (component *hostObservingTestComponent) Start(context.Context) error {
	close(component.started)
	return nil
}

func (*hostObservingTestComponent) Stop(context.Context) error { return nil }

func (*hostObservingTestComponent) Wait(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// A successful finite run can finish before its readiness reader delivers the
// already-published binding. Presentation must join that read before canceling.
func TestHostObservingRunnerJoinsPendingReadinessAfterSuccessfulRun(t *testing.T) {
	expected := initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437}
	readyResult := make(chan runtimeHostResult, 1)
	readyCtx, cancelReady := context.WithCancel(t.Context())
	defer cancelReady()
	observed := make(chan initializer.RuntimeHostBinding, 1)
	runner := hostObservingRunner{
		runner:  &pendingHostReadinessRunner{publish: func() { readyResult <- runtimeHostResult{binding: expected, err: readyCtx.Err()} }},
		onReady: func(binding initializer.RuntimeHostBinding) { observed <- binding },
	}
	err := runner.finishAfterRunResult(t.Context(), nil, readyResult, cancelReady)
	if err != nil {
		t.Fatalf("finishAfterRunResult = %v, want nil", err)
	}
	select {
	case binding := <-observed:
		if binding != expected {
			t.Fatalf("observed binding = %#v, want %#v", binding, expected)
		}
	default:
		t.Fatal("successful finite run canceled pending readiness before presentation")
	}
}

type pendingHostReadinessRunner struct {
	hostReadinessRunner
	publish func()
}

func (runner *pendingHostReadinessRunner) RuntimeHostReadinessConfigured() bool {
	if runner.publish != nil {
		publish := runner.publish
		runner.publish = nil
		publish()
	}
	return true
}
