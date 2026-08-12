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

type readinessComponent struct {
	started chan struct{}
}

func (component *readinessComponent) Start(context.Context) error {
	close(component.started)
	return nil
}

func (component *readinessComponent) Stop(context.Context) error { return nil }

func (component *readinessComponent) Wait(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestManagedRunnerReadinessCachesBindingAndReportsScopeState(t *testing.T) {
	diagnostics := runtimeartifact.Diagnostics{Path: "runtime.log", MetricsPath: "runtime.metrics"}
	runner, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: &lifecyclePlanComponentStub{}, Primary: true,
	}}}, diagnostics)
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	if runner.RuntimeHostReadinessConfigured() {
		t.Fatal("readiness reported configured before a stream was supplied")
	}
	if got := runner.RuntimeLogDiagnostics(); got != diagnostics {
		t.Fatalf("RuntimeLogDiagnostics() = %#v, want %#v", got, diagnostics)
	}
	if _, err := runner.RuntimeHostBinding(t.Context()); !errors.Is(err, initializer.ErrRuntimeHostReadinessUnavailable) {
		t.Fatalf("RuntimeHostBinding without a stream = %v, want readiness-unavailable", err)
	}

	ready := make(chan initializer.RuntimeHostBinding, 1)
	runner.SetRuntimeHostReady(ready)
	if !runner.RuntimeHostReadinessConfigured() {
		t.Fatal("readiness not reported configured after a stream was supplied")
	}
	expected := initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437}
	ready <- expected
	got, err := runner.RuntimeHostBinding(t.Context())
	if err != nil {
		t.Fatalf("RuntimeHostBinding first read: %v", err)
	}
	if got != expected {
		t.Fatalf("RuntimeHostBinding first read = %#v, want %#v", got, expected)
	}
	cached, err := runner.RuntimeHostBinding(t.Context())
	if err != nil {
		t.Fatalf("RuntimeHostBinding cached read: %v", err)
	}
	if cached != expected {
		t.Fatalf("RuntimeHostBinding cached read = %#v, want %#v", cached, expected)
	}
	if err := runner.Run(t.Context()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestManagedRunnerReadinessReportsClosedAndCanceledStreams(t *testing.T) {
	closed := make(chan initializer.RuntimeHostBinding)
	close(closed)
	closedRunner, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: &lifecyclePlanComponentStub{}, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner closed: %v", err)
	}
	closedRunner.SetRuntimeHostReady(closed)
	if _, err := closedRunner.RuntimeHostBinding(t.Context()); err == nil {
		t.Fatal("RuntimeHostBinding on a closed stream succeeded")
	}

	canceled := make(chan initializer.RuntimeHostBinding)
	canceledRunner, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: &lifecyclePlanComponentStub{}, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner canceled: %v", err)
	}
	canceledRunner.SetRuntimeHostReady(canceled)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := canceledRunner.RuntimeHostBinding(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("RuntimeHostBinding canceled = %v, want context.Canceled", err)
	}
}

func TestManagedRunnerNilMethodsFailClosed(t *testing.T) {
	var runner *ManagedRunner
	if err := runner.Run(t.Context()); err == nil {
		t.Fatal("nil ManagedRunner.Run succeeded")
	}
	runner.SetRuntimeHostReady(nil)
	if runner.RuntimeHostReadinessConfigured() {
		t.Fatal("nil ManagedRunner reported readiness configured")
	}
	if _, err := runner.RuntimeHostBinding(t.Context()); err == nil {
		t.Fatal("nil ManagedRunner.RuntimeHostBinding succeeded")
	}
	if got := runner.RuntimeLogDiagnostics(); got != (runtimeartifact.Diagnostics{}) {
		t.Fatalf("nil ManagedRunner diagnostics = %#v", got)
	}
	if err := runner.RunWithCompletion(t.Context(), func(context.Context) error { return nil }); err == nil {
		t.Fatal("nil ManagedRunner.RunWithCompletion succeeded")
	}
}

func TestManagedRunnerRunsCompletionAfterRuntimeHostReadiness(t *testing.T) {
	transport := &readinessComponent{started: make(chan struct{})}
	runner, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}
	ready := make(chan initializer.RuntimeHostBinding, 1)
	runner.SetRuntimeHostReady(ready)
	completionStarted := make(chan struct{})
	completed := make(chan error, 1)
	go func() {
		completed <- runner.RunWithCompletion(t.Context(), func(ctx context.Context) error {
			close(completionStarted)
			binding, err := runner.RuntimeHostBinding(ctx)
			if err != nil {
				return err
			}
			if binding.Port != 7437 {
				return &unexpectedBindingError{port: binding.Port}
			}
			return nil
		})
	}()
	select {
	case <-transport.started:
	case <-time.After(time.Second):
		t.Fatal("transport did not start")
	}
	select {
	case <-completionStarted:
		t.Fatal("completion ran before host readiness")
	default:
	}
	ready <- initializer.RuntimeHostBinding{Host: "127.0.0.1", Port: 7437}
	select {
	case err := <-completed:
		if err != nil {
			t.Fatalf("RunWithCompletion: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("completion did not finish after host readiness")
	}
}

type unexpectedBindingError struct{ port int }

func (err *unexpectedBindingError) Error() string { return "unexpected host port" }
