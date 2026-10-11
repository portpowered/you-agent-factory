package runtime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

func TestServiceMode_WorkerPoolResultWhilePaused_CompletesBeforeResume(t *testing.T) {
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
	case <-time.After(time.Minute):
		t.Fatal("timed out waiting for worker-pool dispatch to start")
	}

	if err := f.Pause(context.Background()); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	waitForFactoryState(t, f, interfaces.FactoryStatePaused, time.Second)

	close(executor.release)
	waitForBufferedResult(t, buffered)
	assertPausedWorkerResultApplied(t, f)

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

func TestServiceMode_WorkerResultWhilePaused_CompletesBeforeResume(t *testing.T) {
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
	assertPausedWorkerResultApplied(t, h.Factory)

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

func TestPause_AlreadyPausedIsNoOp(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()

	eventsAfterFirstPause := runtimeGeneratedEvents(t, h.Factory)
	pauseEventsAfterFirst := countFactoryStateEventsTo(t, eventsAfterFirstPause, factoryapi.FactoryStatePaused)

	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, h.Factory, interfaces.FactoryStatePaused, time.Second)

	eventsAfterRepeatedPause := runtimeGeneratedEvents(t, h.Factory)
	if got := countFactoryStateEventsTo(t, eventsAfterRepeatedPause, factoryapi.FactoryStatePaused); got != pauseEventsAfterFirst {
		t.Fatalf("pause state events = %d, want %d after repeated pause", got, pauseEventsAfterFirst)
	}
}

func TestServiceMode_RepeatedPausePreservesBufferedSubmission(t *testing.T) {
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	defer h.stop()

	h.pauseAndWait()
	submitPausedBufferTask(t, h.Factory, "request-repeated-pause-submit-001", "trace-repeated-pause-submit")

	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, h.Factory, interfaces.FactoryStatePaused, time.Second)
	assertPausedSubmissionNotDone(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
}

func TestServiceMode_RepeatedPausePreservesCompletedWorkerResult(t *testing.T) {
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
		RequestID:  "request-repeated-pause-result-001",
		WorkTypeID: "task",
		TraceID:    "trace-repeated-pause-result",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	waitForBlockingWorkerStart(t, executor, h.errCh)
	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	h.pauseAndWait()
	close(executor.release)
	waitForBufferedResult(t, buffered)

	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("second Pause: %v", err)
	}
	if err := h.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("third Pause: %v", err)
	}
	waitForFactoryState(t, h.Factory, interfaces.FactoryStatePaused, time.Second)
	assertPausedWorkerResultApplied(t, h.Factory)

	h.resumeAndWait()
	waitForWorkAtPlace(t, h.Factory, "task:done", time.Second)
	assertNoInFlightDispatches(t, h.Factory)
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

type serviceModeRunHarness struct {
	t       *testing.T
	Factory factoryhost.Engine
	cancel  context.CancelFunc
	errCh   chan error
}

func startServiceModeRunHarness(t *testing.T, opts ...testFactoryOption) *serviceModeRunHarness {
	t.Helper()

	f, err := newTestFactory(opts...)
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

func submitPausedBufferTask(t *testing.T, f factoryhost.Engine, requestID, traceID string) {
	t.Helper()
	result, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
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

func assertPausedSubmissionNotApplied(t *testing.T, f factoryhost.Engine) {
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

func observeNextBufferedResult(t *testing.T, f factoryhost.Engine) <-chan struct{} {
	t.Helper()
	impl, ok := f.(*factoryImpl)
	if !ok || impl.dispatchFlow == nil {
		t.Fatal("test factory does not expose a canonical dispatch result hook")
	}
	written := make(chan struct{})
	var once sync.Once
	notify := func() { once.Do(func() { close(written) }) }
	impl.dispatchFlow.SetOnBufferedResult(notify)
	return written
}

func assertPausedWorkerResultApplied(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	waitForWorkAtPlace(t, f, "task:done", time.Minute)
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if snap.FactoryState != string(interfaces.FactoryStatePaused) {
		t.Fatalf("factory state = %q, want PAUSED", snap.FactoryState)
	}
	if countTokensAtPlace(snap, "task:done") != 1 || snap.InFlightCount != 0 || len(snap.DispatchHistory) != 1 {
		t.Fatalf("paused result was not applied exactly once: %#v", snap)
	}
}

func assertPausedSubmissionNotDone(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot while paused: %v", err)
	}
	if hasWorkAtPlace(snap, "task:done") {
		t.Fatal("buffered submission applied while paused")
	}
}

func assertTaskDoneOnce(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if count := countTokensAtPlace(snap, "task:done"); count != 1 {
		t.Fatalf("task:done token count = %d, want 1", count)
	}
}

func assertNoInFlightDispatches(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after resume: %v", err)
	}
	if snap.InFlightCount != 0 {
		t.Fatalf("inFlightCount = %d, want 0 after resume", snap.InFlightCount)
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

func submitTaskWithWorkID(t *testing.T, f factoryhost.Engine, workID, traceID string) {
	t.Helper()
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest for %q: %v", workID, err)
	}
}

func assertWorkNotAtDonePlace(t *testing.T, f factoryhost.Engine, workID string) {
	t.Helper()
	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:done") {
		t.Fatalf("marking = %#v, want work %q to remain unprocessed before resume", snap.Marking.Tokens, workID)
	}
}

func assertWorksNotAtDonePlace(t *testing.T, f factoryhost.Engine, workIDs []string) {
	t.Helper()
	for _, workID := range workIDs {
		assertWorkNotAtDonePlace(t, f, workID)
	}
}

func waitForWorkDoneAfterResume(t *testing.T, f factoryhost.Engine, workID string) {
	t.Helper()
	waitForAggregateSnapshotWithTimeout(t, f, 2*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return markingContainsWorkAtPlace(&snap.Marking, workID, "task:done")
	})
}

func waitForQuiescentWorksAtDone(t *testing.T, f factoryhost.Engine, workIDs []string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	return waitForAggregateSnapshotWithTimeout(t, f, 5*time.Second, func(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return allWorksAtDonePlace(&snap.Marking, workIDs) && snap.InFlightCount == 0
	})
}

func assertDispatchOrder(t *testing.T, history []interfaces.CompletedDispatch, wantWorkIDs []string) {
	t.Helper()
	gotOrder := workIDsFromDispatchHistory(history)
	for i, wantWorkID := range wantWorkIDs {
		if gotOrder[i] != wantWorkID {
			t.Fatalf("dispatch history order = %v, want %v", gotOrder, wantWorkIDs)
		}
	}
}

func resumeFactory(t *testing.T, f factoryhost.Engine) {
	t.Helper()
	if err := f.Resume(context.Background()); err != nil {
		t.Fatalf("Resume: %v", err)
	}
}
