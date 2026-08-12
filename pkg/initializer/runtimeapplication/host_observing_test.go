package runtimeapplication

import (
	"context"
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
	runner := WithRuntimeHostObserver(managed, func(initializer.RuntimeHostBinding) {
		t.Fatal("readiness callback ran without a binding")
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
}

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
