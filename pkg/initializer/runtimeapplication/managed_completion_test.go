package runtimeapplication

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func TestManagedRunnerWaitsForCompletionAfterSuccessfulTransport(t *testing.T) {
	transport := &gracefulCompletionTransport{
		started: make(chan struct{}),
		finish:  make(chan struct{}),
	}
	runner, err := NewManagedRunner(lifecycle.Plan{Components: []lifecycle.NamedComponent{{
		Name: "transport", Component: transport, Primary: true,
	}}}, runtimeartifact.Diagnostics{})
	if err != nil {
		t.Fatalf("NewManagedRunner: %v", err)
	}

	completionStarted := make(chan struct{})
	releaseCompletion := make(chan struct{})
	completionResult := make(chan error, 1)
	runResult := make(chan error, 1)
	go func() {
		runResult <- runner.RunWithCompletion(t.Context(), func(ctx context.Context) error {
			close(completionStarted)
			select {
			case <-releaseCompletion:
				completionResult <- nil
				return nil
			case <-ctx.Done():
				completionResult <- ctx.Err()
				return ctx.Err()
			}
		})
	}()

	waitForManagedSignal(t, transport.started, "transport did not start")
	waitForManagedSignal(t, completionStarted, "completion did not start")

	close(transport.finish)
	select {
	case err := <-runResult:
		t.Fatalf("RunWithCompletion returned before completion was released: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseCompletion)
	select {
	case err := <-completionResult:
		if err != nil {
			t.Fatalf("completion received cancellation after successful transport: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("completion did not finish")
	}
	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("RunWithCompletion: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunWithCompletion did not finish")
	}
}

func waitForManagedSignal(t *testing.T, signal <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(failure)
	}
}

type gracefulCompletionTransport struct {
	started chan struct{}
	finish  chan struct{}
}

func (transport *gracefulCompletionTransport) Start(context.Context) error {
	close(transport.started)
	return nil
}

func (*gracefulCompletionTransport) Stop(context.Context) error { return nil }

func (transport *gracefulCompletionTransport) Wait(ctx context.Context) error {
	select {
	case <-transport.finish:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

var _ initializer.CompletionRuntimeRunner = (*ManagedRunner)(nil)
