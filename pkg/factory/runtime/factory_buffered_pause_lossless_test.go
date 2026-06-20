package runtime

import (
	"context"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestPause_AlreadyPausedIsNoOp(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
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
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	eventsAfterFirstPause := runtimeGeneratedEvents(t, f)
	pauseEventsAfterFirst := countFactoryStateEventsTo(t, eventsAfterFirstPause, factoryapi.FactoryStatePaused)

	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	eventsAfterRepeatedPause := runtimeGeneratedEvents(t, f)
	if got := countFactoryStateEventsTo(t, eventsAfterRepeatedPause, factoryapi.FactoryStatePaused); got != pauseEventsAfterFirst {
		t.Fatalf("pause state events = %d, want %d after repeated pause", got, pauseEventsAfterFirst)
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

func TestServiceMode_RepeatedPausePreservesBufferedSubmission(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
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
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		RequestID:  "request-repeated-pause-submit-001",
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-submit",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}

	time.Sleep(200 * time.Millisecond)
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatal("buffered submission applied while paused")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	waitForWorkAtPlace(t, f, "task:done", time.Second)

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

func TestServiceMode_RepeatedPausePreservesBufferedWorkerResult(t *testing.T) {
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
		RequestID:  "request-repeated-pause-result-001",
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-result",
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
		return snapshot.InFlightCount > 0
	})

	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("first Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	close(executor.release)
	time.Sleep(200 * time.Millisecond)

	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatal("buffered worker result applied while paused")
		}
		if snap.InFlightCount == 0 {
			t.Fatal("dispatch completed while paused")
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

func countFactoryStateEventsTo(t *testing.T, events []factoryapi.FactoryEvent, want factoryapi.FactoryState) int {
	t.Helper()
	count := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeFactoryStateResponse {
			continue
		}
		payload, err := event.Payload.AsFactoryStateResponseEventPayload()
		if err != nil {
			t.Fatalf("decode factory state event: %v", err)
		}
		if payload.State == want {
			count++
		}
	}
	return count
}
