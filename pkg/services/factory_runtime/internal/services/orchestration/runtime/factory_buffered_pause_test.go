package runtime

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestServiceMode_MultipleSubmissionsWhilePaused_ResumeDrainsToQuiescence(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
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

	assertPausedWithoutProcessedWork(t, f)

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)

	snap := waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return countTokensAtPlace(snapshot, "task:done") == len(traceIDs)
	})
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

	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	buffered := observeNextBufferedResult(t, f)

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
	waitForBufferedResult(t, buffered)
	assertPausedWithoutProcessedWork(t, f)

	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStateRunning, time.Second)
	waitForWorkAtPlace(t, f, "task:done", time.Second)

	cancel()
	waitForRunStop(t, errCh)
}

func assertPausedWithoutProcessedWork(t *testing.T, f factoryhost.Engine) {
	t.Helper()
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

func TestServiceMode_WorkerResultWhilePaused_BuffersUntilResume(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()
	buffered := observeNextBufferedResult(t, h.Factory)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		RequestID: "request-runtime-paused-result-001", WorkTypeID: "task", TraceID: "trace-runtime-paused-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	waitForBlockingWorkerStart(t, executor, h.errCh)
	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0 && len(snapshot.Dispatches) > 0
	})

	h.pauseAndWait()
	close(executor.release)
	waitForBufferedResult(t, buffered)
	assertPausedWorkerResultBuffered(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertNoInFlightDispatches(t, h.Factory)
	assertTaskDoneOnce(t, h.Factory)
}

func TestServiceMode_SubmissionWhilePaused_BuffersUntilResume(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitPausedBufferTask(t, h.Factory, "request-runtime-paused-submit-001", "trace-runtime-paused-submit")
	assertPausedSubmissionNotApplied(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertTaskDoneOnce(t, h.Factory)
}
