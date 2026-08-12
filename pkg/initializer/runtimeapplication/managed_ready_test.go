package runtimeapplication

import (
	"context"
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
