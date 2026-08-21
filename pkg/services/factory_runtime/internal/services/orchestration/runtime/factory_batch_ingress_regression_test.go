package runtime

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

func assertLegacyWorkInput(t *testing.T, input workers.WorkInput) {
	t.Helper()
	if input.WorkID != "work-legacy" {
		t.Fatalf("legacy input WorkID = %q, want work-legacy", input.WorkID)
	}
	if len(input.Content) != 1 {
		t.Fatalf("legacy input content = %#v, want one payload part", input.Content)
	}
	if input.Content[0].Text != "legacy payload" {
		t.Fatalf("legacy input content = %#v, want payload fallback", input.Content)
	}
	if input.AttemptFacts.AttemptNumber != 3 || input.AttemptFacts.LastFailure != "last failure" {
		t.Fatalf("legacy input attempt facts = %#v, want attempt 3 and last failure", input.AttemptFacts)
	}
	if input.Tags["source"] != "legacy" {
		t.Fatalf("legacy input tags = %#v, want source tag", input.Tags)
	}
	if input.Kind != string(workers.DataTypeWork) || input.State != "init" {
		t.Fatalf("legacy input kind/state = %q/%q, want work/init", input.Kind, input.State)
	}
	if len(input.InputNames) != 1 {
		t.Fatalf("legacy input names = %#v, want primary", input.InputNames)
	}
	if input.InputNames[0] != "primary" {
		t.Fatalf("legacy input name = %q, want primary", input.InputNames[0])
	}
}

func assertContentWorkInput(t *testing.T, input workers.WorkInput) {
	t.Helper()
	if input.WorkID != "work-content" {
		t.Fatalf("content input WorkID = %q, want work-content", input.WorkID)
	}
	if input.Name != "content display name" {
		t.Fatalf("content input name = %q, want content display name", input.Name)
	}
	if len(input.Content) != 1 {
		t.Fatalf("content input content = %#v, want one body part", input.Content)
	}
	if input.Content[0].Text != "content body" {
		t.Fatalf("content input content = %#v, want content body", input.Content)
	}
	if input.AttemptFacts.AttemptNumber != 5 || input.State != "review" {
		t.Fatalf("content input facts/state = %#v/%q, want attempt 5/review", input.AttemptFacts, input.State)
	}
	if len(input.InputNames) != 1 {
		t.Fatalf("content input names = %#v, want secondary", input.InputNames)
	}
	if input.InputNames[0] != "secondary" {
		t.Fatalf("content input name = %q, want secondary", input.InputNames[0])
	}
}

func workInputsFromDispatchFixture() work.WorkDispatch {
	invocation := &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{
		"mode": {Values: []string{"fast"}},
	}}
	return work.WorkDispatch{
		TransitionID: "transition-inputs",
		InputBindings: map[string][]string{
			"primary":   {"legacy"},
			"secondary": {"content"},
		},
		InputTokens: workers.InputTokens(
			workers.Token{ID: "resource", State: "ready", Color: workers.Color{DataType: workers.DataTypeResource}},
			workers.Token{
				ID:    "legacy",
				State: "init",
				Color: workers.Color{
					DataType:            workers.DataTypeWork,
					WorkID:              "work-legacy",
					WorkTypeID:          "task",
					RequestID:           "request-legacy",
					Payload:             []byte("legacy payload"),
					Tags:                map[string]string{"source": "legacy"},
					InvocationArguments: invocation,
				},
				History: workers.History{
					TotalVisits: map[string]int{"transition-inputs": 2},
					FailureLog:  []workers.Failure{{Error: "last failure"}},
				},
			},
			workers.Token{
				ID:    "content",
				State: "review",
				Color: workers.Color{
					Name:       "content display name",
					DataType:   workers.DataTypeWork,
					WorkID:     "work-content",
					WorkTypeID: "review",
					Content: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "content body",
					}},
				},
				History: workers.History{TotalVisits: map[string]int{"transition-inputs": 4}},
			},
		),
	}
}
