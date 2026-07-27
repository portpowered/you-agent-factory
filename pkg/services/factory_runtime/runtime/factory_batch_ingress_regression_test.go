package runtime

import (
	"context"
	"testing"
)

// TestBlockedDispatchConcurrentBatchIngressRegression_ServiceModeWorkerPool is the
// deterministic regression for issue #1352. It blocks one unrelated dispatch,
// submits a concurrent batch, and asserts observable acceptance (WORK_REQUEST plus
// marking visibility) and same-request-ID replay idempotency before the blocker
// completes. It fails on the pre-fix starvation path where submit returned while
// WORK_REQUEST and marking visibility were still missing.
func TestBlockedDispatchConcurrentBatchIngressRegression_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	const (
		requestID = "request-batch-ingress-regression"
		workID    = "work-batch-ingress-regression"
		traceID   = "trace-batch-ingress-regression"
	)

	batchRequest := ingressBatchRequest(requestID, workID, traceID)

	first, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true before blocked dispatch completes")
	}
	if first.RequestID != requestID {
		t.Fatalf("first submit request_id = %q, want %q", first.RequestID, requestID)
	}
	assertIngressObservable(t, history, h.Factory, requestID, workID)

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
	if countWorkRequestRecords(history, requestID) != 1 {
		t.Fatalf(
			"WORK_REQUEST count for %q = %d, want 1 before blocked dispatch completes",
			requestID,
			countWorkRequestRecords(history, requestID),
		)
	}

	select {
	case <-executor.release:
		t.Fatal("blocked dispatch completed before ingress regression assertions finished")
	default:
	}

	releaseBlockedDispatchHarness(t, executor, h)
}
