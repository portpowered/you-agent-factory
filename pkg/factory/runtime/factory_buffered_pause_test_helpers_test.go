package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

type serviceModeRunHarness struct {
	t      *testing.T
	Factory factory.Factory
	cancel context.CancelFunc
	errCh  chan error
}

func startServiceModeRunHarness(t *testing.T, opts ...factory.FactoryOption) *serviceModeRunHarness {
	t.Helper()

	f, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	return &serviceModeRunHarness{t: t, Factory: f, cancel: cancel, errCh: errCh}
}

func (h *serviceModeRunHarness) pauseAndWait() {
	h.t.Helper()
	if err := h.Factory.Pause(context.Background()); err != nil {
		h.t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(h.t, h.Factory, interfaces.FactoryStatePaused, time.Second)
}

func (h *serviceModeRunHarness) resumeAndWait() {
	h.t.Helper()
	if err := h.Factory.Resume(context.Background()); err != nil {
		h.t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(h.t, h.Factory, interfaces.FactoryStateRunning, time.Second)
}

func (h *serviceModeRunHarness) stop() {
	h.t.Helper()
	h.cancel()
	select {
	case err := <-h.errCh:
		if err != nil {
			h.t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		h.t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}

func submitPausedBufferTask(t *testing.T, f factory.Factory, requestID, traceID string) {
	t.Helper()
	result, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		RequestID:  requestID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}})
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit accepted = false, want true")
	}
}

func waitForBlockingWorkerStart(t *testing.T, executor *blockingExecutor, errCh <-chan error) {
	t.Helper()
	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker to start")
	}
}

func pollPausedSnapshot(
	t *testing.T,
	f factory.Factory,
	duration time.Duration,
	assertFn func(*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]),
) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
		}
		assertFn(snap)
		time.Sleep(20 * time.Millisecond)
	}
}

func assertPausedSubmissionNotApplied(t *testing.T, f factory.Factory) {
	t.Helper()
	pollPausedSnapshot(t, f, 300*time.Millisecond, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:init") || hasWorkAtPlace(snap, "task:done") {
			t.Fatalf("paused submission applied to marking = %#v", snap.Marking.Tokens)
		}
		if snap.InFlightCount > 0 || len(snap.Dispatches) > 0 {
			t.Fatalf("running dispatches while paused inFlight=%d dispatches=%d", snap.InFlightCount, len(snap.Dispatches))
		}
	})
}

func assertPausedWorkerResultBuffered(t *testing.T, f factory.Factory, duration time.Duration) {
	t.Helper()
	pollPausedSnapshot(t, f, duration, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatal("worker result applied while paused")
		}
		if snap.InFlightCount == 0 {
			t.Fatalf("dispatch completed while paused inFlight=%d", snap.InFlightCount)
		}
	})
}

func assertPausedSubmissionNotDone(t *testing.T, f factory.Factory) {
	t.Helper()
	pollPausedSnapshot(t, f, 300*time.Millisecond, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatal("buffered submission applied while paused")
		}
	})
}

func assertPausedWorkerResultNotDone(t *testing.T, f factory.Factory) {
	t.Helper()
	assertPausedWorkerResultBuffered(t, f, 500*time.Millisecond)
}

func assertTaskDoneOnce(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if count := countTokensAtPlace(snap, "task:done"); count != 1 {
		t.Fatalf("task:done token count = %d, want 1", count)
	}
}

func assertNoInFlightDispatches(t *testing.T, f factory.Factory) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snap.InFlightCount != 0 {
		t.Fatalf("inFlightCount = %d, want 0 after resume", snap.InFlightCount)
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
