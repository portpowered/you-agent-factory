package runtimeapplication

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

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
