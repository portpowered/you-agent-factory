package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestConcurrentBatchIngressSameRequestIDReplayIsIdempotent_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	batchRequest := ingressBatchRequest(
		"request-idempotent-replay",
		"work-idempotent-replay",
		"trace-idempotent-replay",
	)

	first, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	assertIngressObservable(t, history, h.Factory, batchRequest.RequestID, "work-idempotent-replay")

	replayed, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("replay SubmitWorkRequest: %v", err)
	}
	if replayed.Accepted {
		t.Fatalf("replay accepted = true, want idempotent no-op")
	}
	if replayed.RequestID != first.RequestID || replayed.WorkID != first.WorkID {
		t.Fatalf("replay identity = %#v, want preserved %#v", replayed, first)
	}
	if countWorkRequestRecords(history, batchRequest.RequestID) != 1 {
		t.Fatalf("WORK_REQUEST count for %q = %d, want 1", batchRequest.RequestID, countWorkRequestRecords(history, batchRequest.RequestID))
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if countObservedWorkMaterializations(snap, "work-idempotent-replay") != 1 {
		t.Fatalf("observed work materializations for work-idempotent-replay = %d, want 1", countObservedWorkMaterializations(snap, "work-idempotent-replay"))
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func TestConcurrentBatchIngressDifferentRequestIDAfterVisibleAcceptanceDoesNotDuplicateWork_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	const workID = "work-visible-retry"
	firstRequest := ingressBatchRequest("request-visible-first", workID, "trace-visible-first")
	secondRequest := ingressBatchRequest("request-visible-retry", workID, "trace-visible-retry")

	first, err := h.Factory.SubmitWorkRequest(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	assertIngressObservable(t, history, h.Factory, firstRequest.RequestID, workID)

	second, err := h.Factory.SubmitWorkRequest(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("retry SubmitWorkRequest: %v", err)
	}
	if second.Accepted {
		t.Fatalf("different-request-ID retry accepted = true, want rejection after visible acceptance")
	}
	if countWorkRequestRecords(history, secondRequest.RequestID) != 0 {
		t.Fatalf("WORK_REQUEST count for retry request %q = %d, want 0", secondRequest.RequestID, countWorkRequestRecords(history, secondRequest.RequestID))
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if countObservedWorkMaterializations(snap, workID) != 1 {
		t.Fatalf("observed work materializations for %q = %d, want 1 durable materialization", workID, countObservedWorkMaterializations(snap, workID))
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func startBlockedDispatchIngressHarness(t *testing.T) (*ingressBlockingExecutor, *recordingfixtures.ScriptedRuntimeLedger, *serviceModeRunHarness) {
	t.Helper()

	executor := &ingressBlockingExecutor{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	history := &recordingfixtures.ScriptedRuntimeLedger{}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		WorkID:     "work-blocked-dispatch",
		WorkTypeID: "task",
		TraceID:    "trace-blocked-dispatch",
	}}); err != nil {
		t.Fatalf("submit initial work: %v", err)
	}

	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking dispatch to start")
	}

	return executor, history, h
}

func releaseBlockedDispatchHarness(t *testing.T, executor *ingressBlockingExecutor, h *serviceModeRunHarness) {
	t.Helper()
	close(executor.release)
	h.cancel()
	select {
	case <-h.errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func ingressBatchRequest(requestID, workID, traceID string) work.WorkRequest {
	return work.WorkRequest{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "ingress-batch",
			WorkID:     workID,
			WorkTypeID: "task",
			TraceID:    traceID,
		}},
	}
}

func assertIngressObservable(
	t *testing.T,
	history *recordingfixtures.ScriptedRuntimeLedger,
	factoryInstance factory.Factory,
	requestID string,
	workID string,
) {
	t.Helper()
	if !workRequestRecorded(history, requestID) {
		t.Fatalf("WORK_REQUEST not recorded for %q; work requests=%#v", requestID, history.WorkRequests)
	}
	snap, err := factoryInstance.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !snapshotObservesWork(snap, workID) {
		t.Fatalf("work %q not observable before retry; marking=%#v", workID, snap.Marking.Tokens)
	}
}

func countWorkRequestRecords(history *recordingfixtures.ScriptedRuntimeLedger, requestID string) int {
	count := 0
	for _, record := range history.WorkRequests {
		if record.RequestID == requestID {
			count++
		}
	}
	return count
}

func countMarkingTokensForWorkID(marking *petri.MarkingSnapshot, workID string) int {
	count := 0
	for _, token := range marking.Tokens {
		if token != nil && token.Color.WorkID == workID {
			count++
		}
	}
	return count
}

func countObservedWorkMaterializations(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) int {
	if snap == nil {
		return 0
	}
	count := countMarkingTokensForWorkID(&snap.Marking, workID)
	for _, entry := range snap.Dispatches {
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				count++
			}
		}
	}
	return count
}

type ingressBlockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *ingressBlockingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	select {
	case e.started <- struct{}{}:
	default:
	}
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}
