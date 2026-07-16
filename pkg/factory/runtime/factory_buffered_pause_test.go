package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/work"
)

func TestServiceMode_MultipleSubmissionsWhilePaused_ResumeDrainsToQuiescence(t *testing.T) {
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

	traceIDs := []string{"trace-runtime-a", "trace-runtime-b", "trace-runtime-c"}
	for i, traceID := range traceIDs {
		result, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
			RequestID:  fmt.Sprintf("request-runtime-paused-%03d", i+1),
			WorkTypeID: "task",
			TraceID:    traceID,
		}})
		if err != nil {
			t.Fatalf("SubmitWorkRequest %s: %v", traceID, err)
		}
		if !result.Accepted {
			t.Fatalf("submit %s accepted = false, want true", traceID)
		}
	}

	assertPausedWithoutProcessedWork(t, f, 300*time.Millisecond)

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
		}
		if countTokensAtPlace(snap, "task:done") == len(traceIDs) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume drain: %v", err)
	}
	if got := countTokensAtPlace(snap, "task:done"); got != len(traceIDs) {
		t.Fatalf("task:done token count = %d, want %d", got, len(traceIDs))
	}

	cancel()
	waitForRunStop(t, errCh)
}

func TestServiceMode_WorkerPoolResultWhilePaused_ResumeDrainsWithoutExternalSignal(t *testing.T) {
	executor := &asyncRecordingExecutor{
		started: make(chan work.WorkDispatch, 1),
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

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		RequestID:  "request-runtime-pool-inflight-001",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-pool-inflight",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker-pool dispatch to start")
	}

	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	close(executor.release)

	assertPausedWithoutProcessedWork(t, f, 500*time.Millisecond)

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	waitForWorkAtPlace(t, f, "task:done", time.Second)

	cancel()
	waitForRunStop(t, errCh)
}

func assertPausedWithoutProcessedWork(t *testing.T, f factory.Factory, duration time.Duration) {
	t.Helper()
	pollPausedSnapshot(t, f, duration, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) {
		if snap.FactoryState != string(interfaces.FactoryStatePaused) {
			t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
		}
		if hasWorkAtPlace(snap, "task:done") {
			t.Fatalf("paused buffered work applied to marking = %#v", snap.Marking.Tokens)
		}
	})
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
