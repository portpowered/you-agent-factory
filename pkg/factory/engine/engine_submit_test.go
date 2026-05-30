package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestInjectTokensCreatesTokenInInitialPlace(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	engine := NewFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.injectTokens([]interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "t1",
		Tags:       map[string]string{"key": "val"},
	}})
	engine.mu.Unlock()

	snap := engine.GetMarking()
	tokens := snap.TokensInPlace("task:init")
	if len(tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(tokens))
	}
	if tokens[0].Color.Tags["key"] != "val" {
		t.Error("expected tag 'key'='val'")
	}
	if tokens[0].Color.DataType != interfaces.DataTypeWork {
		t.Errorf("expected DataType %q, got %q", interfaces.DataTypeWork, tokens[0].Color.DataType)
	}
}

func TestInjectTokensSkipsUnknownWorkType(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	engine := NewFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.injectTokens([]interfaces.SubmitRequest{{WorkTypeID: "nonexistent"}})
	engine.mu.Unlock()

	if got := len(engine.GetMarking().Tokens); got != 0 {
		t.Errorf("expected 0 tokens, got %d", got)
	}
}

func TestSubmit_RejectsWhenSubmissionIngressClosed(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	engine := NewFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.acceptingSubmits = false
	engine.mu.Unlock()

	_, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-after-stop"}})
	if err == nil {
		t.Fatal("expected submit to fail when submission ingress is closed")
	}
	if !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected terminated error, got %v", err)
	}

	engine.mu.Lock()
	defer engine.mu.Unlock()
	if len(engine.submissionHook.batches) != 0 {
		t.Fatalf("expected no queued submissions after rejection, got %d", len(engine.submissionHook.batches))
	}
}

func batchSubmitTestRequest() interfaces.WorkRequest {
	return interfaces.WorkRequest{
		RequestID: "request-batch-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "plan", WorkID: "work-plan", WorkTypeID: "task", TraceID: "trace-batch"},
			{Name: "test", WorkID: "work-test", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "test",
			TargetWorkName: "plan",
			RequiredState:  "complete",
		}},
	}
}

func assertAcceptedBatchSubmitResult(t *testing.T, result interfaces.WorkRequestSubmitResult) {
	t.Helper()
	if result.RequestID != "request-batch-1" || result.TraceID != "trace-batch" || !result.Accepted {
		t.Fatalf("submit result = %#v, want accepted original request metadata", result)
	}
}

func assertDuplicateBatchSubmitResult(t *testing.T, repeated, first interfaces.WorkRequestSubmitResult) {
	t.Helper()
	if repeated.RequestID != first.RequestID || repeated.TraceID != first.TraceID || repeated.Accepted {
		t.Fatalf("duplicate submit result = %#v, want original metadata with Accepted=false", repeated)
	}
	if repeated.WorkID != "work-plan" || repeated.Name != "plan" || repeated.WorkTypeName != "task" {
		t.Fatalf("duplicate submit identity = %#v, want preserved primary work metadata", repeated)
	}
}

func assertWorkInputsShareRequestID(t *testing.T, workInputs []interfaces.SubmitRequest, want string) {
	t.Helper()
	for _, req := range workInputs {
		if req.RequestID != want {
			t.Fatalf("work input request ID = %q, want %s", req.RequestID, want)
		}
	}
}

func TestSubmitWorkRequest_InjectsBatchAtomicallyAndIgnoresDuplicateRequestID(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	var workInputs []interfaces.SubmitRequest

	eng := NewFactoryEngine(n, marking, nil, WithWorkInputRecorder(func(_ int, req interfaces.SubmitRequest, _ interfaces.Token) {
		workInputs = append(workInputs, req)
	}))
	request := batchSubmitTestRequest()

	result, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	assertAcceptedBatchSubmitResult(t, result)
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	assertSubmittedTokensAndInputs(t, eng, workInputs, 2)

	repeated, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	assertDuplicateBatchSubmitResult(t, repeated, result)
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after duplicate: %v", err)
	}

	assertSubmittedTokensAndInputs(t, eng, workInputs, 2)
	assertWorkInputsShareRequestID(t, workInputs, "request-batch-1")
}

func TestSubmitWorkRequest_ValidationFailureQueuesNoPartialWork(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	eng := NewFactoryEngine(n, marking, nil)

	_, err := eng.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-invalid",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "valid", WorkTypeID: "task"},
			{Name: "invalid", WorkTypeID: "missing-type"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for unknown work type")
	}
	if !strings.Contains(err.Error(), "unknown work type") {
		t.Fatalf("validation error = %v, want unknown work type", err)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(eng.GetMarking().Tokens) != 0 {
		t.Fatalf("tokens after failed batch = %d, want 0", len(eng.GetMarking().Tokens))
	}
}

func TestSubmitWorkRequest_WrappedRequestsPreserveRuntimeFields(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	eng := NewFactoryEngine(n, marking, nil)
	request := interfaces.SubmitRequest{
		RequestID:   "request-unary-1",
		WorkID:      "work-unary-1",
		WorkTypeID:  "task",
		TraceID:     "trace-unary",
		TargetState: "complete",
		ExecutionID: "execution-1",
		Tags:        map[string]string{"_work_name": "Unary work"},
		Relations: []interfaces.Relation{{
			Type:          interfaces.RelationDependsOn,
			TargetWorkID:  "upstream-1",
			RequiredState: "complete",
		}},
	}
	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{request}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap := eng.GetMarking()
	tokens := snap.TokensInPlace("task:complete")
	if len(tokens) != 1 {
		t.Fatalf("tokens in target state = %d, want 1", len(tokens))
	}
	token := tokens[0]
	if token.Color.RequestID != "request-unary-1" || token.Color.WorkID != "work-unary-1" || token.Color.TraceID != "trace-unary" {
		t.Fatalf("token color = %#v, want submitted identity", token.Color)
	}
	if len(token.Color.Relations) != 1 || token.Color.Relations[0].TargetWorkID != "upstream-1" {
		t.Fatalf("token relations = %#v, want submitted relation", token.Color.Relations)
	}

	if _, err := submitWorkRequests(context.Background(), eng, []interfaces.SubmitRequest{request}); err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after duplicate: %v", err)
	}
	snap = eng.GetMarking()
	if tokens := snap.TokensInPlace("task:complete"); len(tokens) != 1 {
		t.Fatalf("tokens after duplicate unary submit = %d, want 1", len(tokens))
	}
}

func TestSubmitWorkRequest_RejectsUnknownExplicitStateBeforeEnqueue(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	eng := NewFactoryEngine(n, marking, nil)

	_, err := eng.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-invalid-state",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	})
	if err == nil {
		t.Fatal("expected validation error for unknown work state")
	}
	if !strings.Contains(err.Error(), `references unknown state "queued"`) {
		t.Fatalf("validation error = %v, want unknown state", err)
	}
	if len(eng.workRequests) != 0 {
		t.Fatalf("accepted request records = %d, want 0", len(eng.workRequests))
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(eng.GetMarking().Tokens) != 0 {
		t.Fatalf("tokens after failed state validation = %d, want 0", len(eng.GetMarking().Tokens))
	}
}

func TestSubmitWorkRequest_RejectsInvalidParentChildBatchBeforeEnqueue(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	eng := NewFactoryEngine(n, marking, nil)

	_, err := eng.SubmitWorkRequest(context.Background(), interfaces.WorkRequest{
		RequestID: "request-invalid-parent-child",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "parent", WorkTypeID: "task"},
			{Name: "child", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{
			{Type: interfaces.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
			{Type: interfaces.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for duplicate parent-child relation")
	}
	if !strings.Contains(err.Error(), "duplicates relations[0]") {
		t.Fatalf("validation error = %v, want duplicate relation", err)
	}
	if len(eng.workRequests) != 0 {
		t.Fatalf("accepted request records = %d, want 0", len(eng.workRequests))
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if len(eng.GetMarking().Tokens) != 0 {
		t.Fatalf("tokens after failed parent-child validation = %d, want 0", len(eng.GetMarking().Tokens))
	}
}

func assertSubmittedTokensAndInputs(t *testing.T, eng *FactoryEngine, workInputs []interfaces.SubmitRequest, want int) {
	t.Helper()
	snap := eng.GetMarking()
	if tokens := snap.TokensInPlace("task:init"); len(tokens) != want {
		t.Fatalf("tokens after submit = %d, want %d", len(tokens), want)
	}
	if len(workInputs) != want {
		t.Fatalf("work input records after submit = %d, want %d", len(workInputs), want)
	}
}
