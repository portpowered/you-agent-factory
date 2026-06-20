package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestServiceMode_WorkerResultWhilePaused_BuffersUntilResume(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("mock", executor),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		RequestID:  "request-runtime-paused-result-001",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-paused-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}
	waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0 && len(snapshot.Dispatches) > 0
	})

	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	close(executor.release)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatalf("worker result applied while paused")
		}
		if snap.InFlightCount == 0 {
			t.Fatalf("dispatch completed while paused inFlight=%d", snap.InFlightCount)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	waitForWorkAtPlace(t, f, "task:done", time.Second)

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snap.InFlightCount != 0 {
		t.Fatalf("inFlightCount = %d, want 0 after resume", snap.InFlightCount)
	}
	if countTokensAtPlace(snap, "task:done") != 1 {
		t.Fatalf("task:done token count = %d, want 1", countTokensAtPlace(snap, "task:done"))
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}