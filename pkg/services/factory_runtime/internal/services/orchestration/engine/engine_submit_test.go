package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestInjectTokensCreatesTokenInInitialPlace(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	engine := newTestFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.injectTokens([]work.SubmitRequest{{
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
	if tokens[0].Color.DataType != factorytoken.DataTypeWork {
		t.Errorf("expected DataType %q, got %q", factorytoken.DataTypeWork, tokens[0].Color.DataType)
	}
}

func TestInjectTokensSkipsUnknownWorkType(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	engine := newTestFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.injectTokens([]work.SubmitRequest{{WorkTypeID: "nonexistent"}})
	engine.mu.Unlock()

	if got := len(engine.GetMarking().Tokens); got != 0 {
		t.Errorf("expected 0 tokens, got %d", got)
	}
}

func TestSubmit_RejectsWhenSubmissionIngressClosed(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	engine := newTestFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.acceptingSubmits = false
	engine.mu.Unlock()

	_, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-after-stop"}})
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

func batchSubmitTestRequest() work.WorkRequest {
	return work.WorkRequest{
		RequestID: "request-batch-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "plan", WorkID: "work-plan", WorkTypeID: "task", TraceID: "trace-batch"},
			{Name: "test", WorkID: "work-test", WorkTypeID: "task"},
		},
		Relations: []work.WorkRelation{{
			Type:           work.WorkRelationDependsOn,
			SourceWorkName: "test",
			TargetWorkName: "plan",
			RequiredState:  "complete",
		}},
	}
}

func assertAcceptedBatchSubmitResult(t *testing.T, result work.WorkRequestSubmitResult) {
	t.Helper()
	if result.RequestID != "request-batch-1" || result.TraceID != "trace-batch" || !result.Accepted {
		t.Fatalf("submit result = %#v, want accepted original request metadata", result)
	}
}

func assertDuplicateBatchSubmitResult(t *testing.T, repeated, first work.WorkRequestSubmitResult) {
	t.Helper()
	if repeated.RequestID != first.RequestID || repeated.TraceID != first.TraceID || repeated.Accepted {
		t.Fatalf("duplicate submit result = %#v, want original metadata with Accepted=false", repeated)
	}
	if repeated.WorkID != "work-plan" || repeated.Name != "plan" || repeated.WorkTypeName != "task" {
		t.Fatalf("duplicate submit identity = %#v, want preserved primary work metadata", repeated)
	}
}

func assertWorkInputsShareRequestID(t *testing.T, workInputs []work.SubmitRequest, want string) {
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
	var workInputs []work.SubmitRequest

	eng := newTestFactoryEngine(n, marking, nil, WithWorkInputRecorder(func(_ int, req work.SubmitRequest, _ factorytoken.Token) {
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
	eng := newTestFactoryEngine(n, marking, nil)

	_, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-invalid",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
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
	eng := newTestFactoryEngine(n, marking, nil)
	request := work.SubmitRequest{
		RequestID:   "request-unary-1",
		WorkID:      "work-unary-1",
		WorkTypeID:  "task",
		TraceID:     "trace-unary",
		TargetState: "complete",
		ExecutionID: "execution-1",
		Tags:        map[string]string{"_work_name": "Unary work"},
		Relations: []work.Relation{{
			Type:          work.RelationDependsOn,
			TargetWorkID:  "upstream-1",
			RequiredState: "complete",
		}},
	}
	if _, err := submitWorkRequests(context.Background(), eng, []work.SubmitRequest{request}); err != nil {
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

	if _, err := submitWorkRequests(context.Background(), eng, []work.SubmitRequest{request}); err != nil {
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
	eng := newTestFactoryEngine(n, marking, nil)

	_, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-invalid-state",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
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
	eng := newTestFactoryEngine(n, marking, nil)

	_, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-invalid-parent-child",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "parent", WorkTypeID: "task"},
			{Name: "child", WorkTypeID: "task"},
		},
		Relations: []work.WorkRelation{
			{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
			{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
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

func assertSubmittedTokensAndInputs(t *testing.T, eng *FactoryEngine, workInputs []work.SubmitRequest, want int) {
	t.Helper()
	snap := eng.GetMarking()
	if tokens := snap.TokensInPlace("task:init"); len(tokens) != want {
		t.Fatalf("tokens after submit = %d, want %d", len(tokens), want)
	}
	if len(workInputs) != want {
		t.Fatalf("work input records after submit = %d, want %d", len(workInputs), want)
	}
}
func TestSubmitWhileAutomaticTicksPaused_AcceptsAndBuffersUntilResume(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	request := work.WorkRequest{
		RequestID: "request-paused-submit-001",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "paused-submit",
			WorkTypeID: "task",
			TraceID:    "trace-paused-submit",
		}},
	}
	result, err := engine.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}
	if !result.Accepted {
		t.Fatalf("submit result accepted = false, want true")
	}
	if result.RequestID != request.RequestID {
		t.Fatalf("submit result requestID = %q, want %q", result.RequestID, request.RequestID)
	}

	assertNoTokensInPlace(t, engine, "task:init")
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	assertNoTokensInPlace(t, engine, "task:init")
	if sub.callCount != 0 {
		t.Fatalf("subsystem callCount = %d, want 0 while paused", sub.callCount)
	}

	paused = false
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume: %v", err)
	}
	snap := engine.GetMarking()
	tokens := (&snap).TokensInPlace("task:init")
	if len(tokens) != 1 {
		t.Fatalf("tokens in task:init = %d, want 1 after resume", len(tokens))
	}
	if tokens[0].Color.TraceID != "trace-paused-submit" {
		t.Fatalf("token traceID = %q, want trace-paused-submit", tokens[0].Color.TraceID)
	}
	if sub.callCount != 1 {
		t.Fatalf("subsystem callCount = %d, want 1 after resume", sub.callCount)
	}

	repeated, err := engine.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if repeated.Accepted {
		t.Fatal("duplicate submit should be idempotent no-op")
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after duplicate submit: %v", err)
	}
	snap = engine.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("token count after duplicate submit = %d, want 1", len((&snap).TokensInPlace("task:init")))
	}
}

func TestWakeForPendingProcessing_SignalsBufferedSubmissionAfterPausedWake(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}

	paused := true
	engine := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub}, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	if _, err := submitWorkRequests(context.Background(), engine, []work.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-paused-wake",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	// Simulate a paused wake attempt consuming the submit signal without processing.
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick while paused: %v", err)
	}
	assertNoTokensInPlace(t, engine, "task:init")

	paused = false
	engine.WakeForPendingProcessing()
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after resume wake: %v", err)
	}
	snap := engine.GetMarking()
	if len((&snap).TokensInPlace("task:init")) != 1 {
		t.Fatalf("buffered submission was not reachable after paused wake and resume")
	}
}

func assertNoTokensInPlace(t *testing.T, engine *FactoryEngine, placeID string) {
	t.Helper()
	snap := engine.GetMarking()
	if got := len((&snap).TokensInPlace(placeID)); got != 0 {
		t.Fatalf("tokens in %s = %d, want 0 while paused", placeID, got)
	}
}

func TestSubmitWorkRequest_WithoutRunLoopReturnsBeforeProjection(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	eng := newTestFactoryEngine(n, marking, nil)
	request := work.WorkRequest{
		RequestID: "request-no-run-loop",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "no-run-loop",
			WorkTypeID: "task",
			TraceID:    "trace-no-run-loop",
		}},
	}

	result, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if !result.Accepted || result.RequestID != request.RequestID {
		t.Fatalf("submit result = %#v, want accepted request metadata", result)
	}
	assertNoTokensInPlace(t, eng, "task:init")
}

func TestSubmitWorkRequest_WithRunLoopActiveReturnsAfterObservableProjection(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	sub := &mockSubsystem{group: subsystems.Scheduler}
	var recordedRequestIDs []string
	eng := newTestFactoryEngine(
		n,
		marking,
		[]subsystems.Subsystem{sub},
		WithWorkRequestRecorder(func(_ int, record work.WorkRequestRecord) {
			recordedRequestIDs = append(recordedRequestIDs, record.RequestID)
		}),
	)
	request := work.WorkRequest{
		RequestID: "request-run-loop-await",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "run-loop-await",
			WorkTypeID: "task",
			TraceID:    "trace-run-loop-await",
		}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Run(ctx)
	}()
	waitForEngineRunLoopActive(t, eng, time.Second)

	result, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if !result.Accepted || result.RequestID != request.RequestID {
		t.Fatalf("submit result = %#v, want accepted request metadata", result)
	}
	if len(recordedRequestIDs) != 1 || recordedRequestIDs[0] != request.RequestID {
		t.Fatalf("recorded WORK_REQUEST ids = %#v, want [%q] before submit returned", recordedRequestIDs, request.RequestID)
	}
	snap := eng.GetMarking()
	tokens := (&snap).TokensInPlace("task:init")
	if len(tokens) != 1 {
		t.Fatalf("tokens in task:init = %d, want 1 when submit returned", len(tokens))
	}
	if tokens[0].Color.TraceID != "trace-run-loop-await" {
		t.Fatalf("token traceID = %q, want trace-run-loop-await", tokens[0].Color.TraceID)
	}

	cancel()
	if err := <-errCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunLoopCancelsWhilePausedWithBufferedSubmission(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	paused := true
	eng := newTestFactoryEngine(n, marking, nil, WithAutomaticTicksPaused(func() bool {
		return paused
	}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- eng.Run(ctx)
	}()
	waitForEngineRunLoopActive(t, eng, time.Second)

	if _, err := submitWorkRequests(context.Background(), eng, []work.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-paused-buffered-shutdown",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest while paused: %v", err)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Run to stop while paused with buffered submission")
	}
}

func TestSubmitWorkRequest_RejectsDifferentRequestIDWhenWorkIDAlreadyMaterialized(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	eng := newTestFactoryEngine(n, marking, nil)

	first := work.WorkRequest{
		RequestID: "request-materialized-first",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "materialized",
			WorkID:     "work-materialized",
			WorkTypeID: "task",
			TraceID:    "trace-materialized",
		}},
	}
	firstResult, err := eng.SubmitWorkRequest(context.Background(), first)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !firstResult.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	second := work.WorkRequest{
		RequestID: "request-materialized-retry",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "materialized-retry",
			WorkID:     "work-materialized",
			WorkTypeID: "task",
			TraceID:    "trace-materialized-retry",
		}},
	}
	secondResult, err := eng.SubmitWorkRequest(context.Background(), second)
	if err != nil {
		t.Fatalf("retry SubmitWorkRequest: %v", err)
	}
	if secondResult.Accepted {
		t.Fatalf("retry accepted = true, want rejection when work ID is already materialized")
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after retry: %v", err)
	}
	snap := eng.GetMarking()
	if tokens := snap.TokensInPlace("task:init"); len(tokens) != 1 {
		t.Fatalf("tokens in task:init = %d, want 1 after rejected retry", len(tokens))
	}
}
