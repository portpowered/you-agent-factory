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

func TestServiceMode_SubmissionWhilePaused_BuffersUntilResume(t *testing.T) {
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
		t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	result, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		RequestID:  "request-runtime-paused-submit-001",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-paused-submit",
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit accepted = false, want true")
	}

	assertPausedWithoutProcessedWork(t, f, 300*time.Millisecond)

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)

	waitForWorkAtPlace(t, f, "task:done", time.Second)

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if countTokensAtPlace(snap, "task:done") != 1 {
		t.Fatalf("task:done token count = %d, want 1", countTokensAtPlace(snap, "task:done"))
	}

	cancel()
	waitForRunStop(t, errCh)
}

func assertPausedWithoutProcessedWork(t *testing.T, f factory.Factory, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatalf("paused buffered work applied to marking = %#v", snap.Marking.Tokens)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForFactoryState(t *testing.T, f factory.Factory, want interfaces.FactoryState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if snap.FactoryState == string(want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	t.Fatalf("factory state = %q, want %q before timeout", snap.FactoryState, want)
}

func waitForWorkAtPlace(t *testing.T, f factory.Factory, placeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		if hasWorkAtPlace(snap, placeID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for work at %s", placeID)
}

func waitForRunStop(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}

func hasWorkAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) bool {
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}

func countTokensAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) int {
	count := 0
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			count++
		}
	}
	return count
}
