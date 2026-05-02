package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/submission"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// mockSubsystem records calls and returns configured results.
type mockSubsystem struct {
	group     subsystems.TickGroup
	execFn    func(ctx context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error)
	callCount int
	lastSnap  *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]
}

func TestRuntimeStateSnapshot_IncludesActiveThrottlePausesFromSubsystem(t *testing.T) {
	pausedAt := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	pausedUntil := pausedAt.Add(5 * time.Minute)
	observer := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				ActiveThrottlePauses: []interfaces.ActiveThrottlePause{
					{
						LaneID:      "claude/claude-sonnet",
						Provider:    "claude",
						Model:       "claude-sonnet",
						PausedAt:    pausedAt,
						PausedUntil: pausedUntil,
					},
				},
				ThrottlePausesObserved: true,
			}, nil
		},
	}
	eng := NewFactoryEngine(
		&state.Net{ID: "net", Places: map[string]*petri.Place{}},
		petri.NewMarking("net"),
		[]subsystems.Subsystem{observer},
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap := eng.GetRuntimeStateSnapshot()
	if len(snap.ActiveThrottlePauses) != 1 {
		t.Fatalf("ActiveThrottlePauses = %d, want 1", len(snap.ActiveThrottlePauses))
	}
	pause := snap.ActiveThrottlePauses[0]
	if pause.Provider != "claude" || pause.Model != "claude-sonnet" || pause.LaneID != "claude/claude-sonnet" {
		t.Fatalf("unexpected active throttle pause: %#v", pause)
	}
	if !pause.PausedAt.Equal(pausedAt) || !pause.PausedUntil.Equal(pausedUntil) {
		t.Fatalf("unexpected pause window: %#v", pause)
	}

	snap.ActiveThrottlePauses[0].Provider = "mutated"
	next := eng.GetRuntimeStateSnapshot()
	if next.ActiveThrottlePauses[0].Provider != "claude" {
		t.Fatalf("runtime snapshot did not deep-copy active throttle pauses: %#v", next.ActiveThrottlePauses[0])
	}
}

func TestRuntimeStateSnapshot_ClearsActiveThrottlePausesWhenObservedEmpty(t *testing.T) {
	pausedAt := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	observer := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if len(snap.ActiveThrottlePauses) == 0 {
				return &interfaces.TickResult{
					ActiveThrottlePauses: []interfaces.ActiveThrottlePause{
						{
							LaneID:      "claude/claude-sonnet",
							Provider:    "claude",
							Model:       "claude-sonnet",
							PausedAt:    pausedAt,
							PausedUntil: pausedAt.Add(5 * time.Minute),
						},
					},
					ThrottlePausesObserved: true,
				}, nil
			}
			return &interfaces.TickResult{
				ActiveThrottlePauses:   []interfaces.ActiveThrottlePause{},
				ThrottlePausesObserved: true,
			}, nil
		},
	}
	eng := NewFactoryEngine(
		&state.Net{ID: "net", Places: map[string]*petri.Place{}},
		petri.NewMarking("net"),
		[]subsystems.Subsystem{observer},
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	first := eng.GetRuntimeStateSnapshot()
	if len(first.ActiveThrottlePauses) != 1 {
		t.Fatalf("first ActiveThrottlePauses = %d, want 1", len(first.ActiveThrottlePauses))
	}

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick: %v", err)
	}
	second := eng.GetRuntimeStateSnapshot()
	if len(second.ActiveThrottlePauses) != 0 {
		t.Fatalf("second ActiveThrottlePauses = %d, want 0", len(second.ActiveThrottlePauses))
	}
}

func (m *mockSubsystem) TickGroup() subsystems.TickGroup { return m.group }

func (m *mockSubsystem) Execute(ctx context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
	m.callCount++
	m.lastSnap = snap
	if m.execFn != nil {
		return m.execFn(ctx, snap)
	}
	return &interfaces.TickResult{}, nil
}

func submitWorkRequests(ctx context.Context, engine *FactoryEngine, reqs []interfaces.SubmitRequest) (interfaces.WorkRequestSubmitResult, error) {
	return engine.SubmitWorkRequest(ctx, submission.WorkRequestFromSubmitRequests(reqs))
}

type testSubmissionHook struct {
	name     string
	priority int
	onTick   func(ctx context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error)
}

func (h *testSubmissionHook) Name() string {
	return h.name
}

func (h *testSubmissionHook) Priority() int {
	return h.priority
}

func (h *testSubmissionHook) OnTick(ctx context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	if h.onTick != nil {
		return h.onTick(ctx, input)
	}
	return interfaces.SubmissionHookResult{}, nil
}

type testDispatchResultHook struct {
	waitCh   chan struct{}
	submit   func(context.Context, interfaces.WorkDispatch) error
	onTick   func(context.Context, interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.DispatchResultHookResult, error)
	submits  []interfaces.WorkDispatch
	results  []interfaces.WorkResult
	waitOnce bool
}

func newTestDispatchResultHook() *testDispatchResultHook {
	return &testDispatchResultHook{waitCh: make(chan struct{}, 1)}
}

func (h *testDispatchResultHook) SubmitDispatch(ctx context.Context, dispatch interfaces.WorkDispatch) error {
	h.submits = append(h.submits, dispatch)
	if h.submit != nil {
		return h.submit(ctx, dispatch)
	}
	return nil
}

func (h *testDispatchResultHook) OnTick(ctx context.Context, input interfaces.DispatchResultHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.DispatchResultHookResult, error) {
	if h.onTick != nil {
		return h.onTick(ctx, input)
	}
	if len(h.results) == 0 {
		return interfaces.DispatchResultHookResult{}, nil
	}
	results := make([]interfaces.WorkResult, len(h.results))
	copy(results, h.results)
	h.results = nil
	return interfaces.DispatchResultHookResult{Results: results}, nil
}

func (h *testDispatchResultHook) WaitCh() <-chan struct{} {
	return h.waitCh
}

// buildTestNet creates a minimal net with one work type (init → complete → failed).
func buildTestNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, p := range wt.GeneratePlaces() {
		places[p.ID] = p
	}
	return &state.Net{
		ID:          "test-net",
		Places:      places,
		Transitions: make(map[string]*petri.Transition),
		WorkTypes:   map[string]*state.WorkType{"task": wt},
		Resources:   make(map[string]*state.ResourceDef),
	}
}

func TestTickCallsSubsystem(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	// Submit a token via the canonical batch ingress and tick once.
	if _, err := submitWorkRequests(context.Background(), engine, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if sub.callCount != 1 {
		t.Errorf("expected subsystem called once, got %d", sub.callCount)
	}
	if sub.lastSnap == nil {
		t.Fatal("subsystem did not receive a marking snapshot")
	}

	// The marking should contain the injected token in task:init.
	tokensInInit := sub.lastSnap.Marking.TokensInPlace("task:init")
	if len(tokensInInit) != 1 {
		t.Fatalf("expected 1 token in task:init, got %d", len(tokensInInit))
	}
	if tokensInInit[0].Color.WorkTypeID != "task" {
		t.Errorf("expected WorkTypeID 'task', got %q", tokensInInit[0].Color.WorkTypeID)
	}
	if tokensInInit[0].Color.TraceID != "trace-1" {
		t.Errorf("expected TraceID 'trace-1', got %q", tokensInInit[0].Color.TraceID)
	}
}

func TestTickNRunsMultipleTicks(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	if err := engine.TickN(context.Background(), 3); err != nil {
		t.Fatalf("TickN() error: %v", err)
	}
	if sub.callCount != 3 {
		t.Errorf("expected 3 calls, got %d", sub.callCount)
	}
}

func TestTickUntilStopsOnPredicate(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	err := engine.TickUntil(context.Background(), func(snap *petri.MarkingSnapshot) bool {
		return snap.TickCount >= 2
	}, 10)
	if err != nil {
		t.Fatalf("TickUntil() error: %v", err)
	}
	if sub.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", sub.callCount)
	}
}

func TestTickUntilReturnsErrorOnMaxTicks(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	sub := &mockSubsystem{group: subsystems.Scheduler}
	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub})

	err := engine.TickUntil(context.Background(), func(snap *petri.MarkingSnapshot) bool {
		return false // never satisfied
	}, 3)
	if err == nil {
		t.Fatal("expected error when predicate never satisfied")
	}
}

func TestSubsystemsSortedByTickGroup(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	var order []subsystems.TickGroup
	makeSub := func(g subsystems.TickGroup) *mockSubsystem {
		return &mockSubsystem{
			group: g,
			execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
				order = append(order, g)
				return &interfaces.TickResult{}, nil
			},
		}
	}

	// Pass in reverse order — engine should sort them.
	subs := []subsystems.Subsystem{
		makeSub(subsystems.TerminationCheck),
		makeSub(subsystems.CircuitBreaker),
		makeSub(subsystems.Scheduler),
	}

	engine := NewFactoryEngine(n, marking, subs)
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	expected := []subsystems.TickGroup{subsystems.CircuitBreaker, subsystems.Scheduler, subsystems.TerminationCheck}
	if len(order) != len(expected) {
		t.Fatalf("expected %d subsystems called, got %d", len(expected), len(order))
	}
	for i, g := range expected {
		if order[i] != g {
			t.Errorf("position %d: expected TickGroup %d, got %d", i, g, order[i])
		}
	}
}

func TestInjectTokensCreatesTokenInInitialPlace(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	engine := NewFactoryEngine(n, marking, nil)

	engine.mu.Lock()
	engine.injectTokens([]interfaces.SubmitRequest{
		{WorkTypeID: "task", TraceID: "t1", Tags: map[string]string{"key": "val"}},
	})
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
	engine.injectTokens([]interfaces.SubmitRequest{
		{WorkTypeID: "nonexistent"},
	})
	engine.mu.Unlock()

	snap := engine.GetMarking()
	if len(snap.Tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(snap.Tokens))
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

func TestSubmitWorkRequest_InjectsBatchAtomicallyAndIgnoresDuplicateRequestID(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	var workInputs []interfaces.SubmitRequest

	eng := NewFactoryEngine(n, marking, nil, WithWorkInputRecorder(func(_ int, req interfaces.SubmitRequest, _ interfaces.Token) {
		workInputs = append(workInputs, req)
	}))
	request := interfaces.WorkRequest{
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

	result, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if result.RequestID != "request-batch-1" || result.TraceID != "trace-batch" || !result.Accepted {
		t.Fatalf("submit result = %#v, want accepted original request metadata", result)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap := eng.GetMarking()
	tokens := snap.TokensInPlace("task:init")
	if len(tokens) != 2 {
		t.Fatalf("tokens after first submit = %d, want 2", len(tokens))
	}
	if len(workInputs) != 2 {
		t.Fatalf("work input records after first submit = %d, want 2", len(workInputs))
	}

	repeated, err := eng.SubmitWorkRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate SubmitWorkRequest: %v", err)
	}
	if repeated.RequestID != result.RequestID || repeated.TraceID != result.TraceID || repeated.Accepted {
		t.Fatalf("duplicate submit result = %#v, want original metadata with Accepted=false", repeated)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after duplicate: %v", err)
	}

	snap = eng.GetMarking()
	if tokens := snap.TokensInPlace("task:init"); len(tokens) != 2 {
		t.Fatalf("tokens after duplicate submit = %d, want 2", len(tokens))
	}
	if len(workInputs) != 2 {
		t.Fatalf("work input records after duplicate submit = %d, want 2", len(workInputs))
	}
	for _, req := range workInputs {
		if req.RequestID != "request-batch-1" {
			t.Fatalf("work input request ID = %q, want request-batch-1", req.RequestID)
		}
	}
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
	snap := eng.GetMarking()
	if len(snap.Tokens) != 0 {
		t.Fatalf("tokens after failed batch = %d, want 0", len(snap.Tokens))
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

func TestSubmissionHook_GeneratedBatchRecordsCanonicalHistoryBeforeInjection(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	var order []string
	var requestRecords []interfaces.WorkRequestRecord
	var workInputs []interfaces.SubmitRequest
	hook := &testSubmissionHook{
		name:     "file-preseed",
		priority: 10,
		onTick: func(_ context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
			if input.Snapshot.TickCount != 1 {
				t.Fatalf("hook snapshot tick = %d, want 1", input.Snapshot.TickCount)
			}
			if len(input.Snapshot.Marking.Tokens) != 0 {
				t.Fatalf("hook should run before injection, saw %d tokens", len(input.Snapshot.Marking.Tokens))
			}
			return interfaces.SubmissionHookResult{
				GeneratedBatches: []interfaces.GeneratedSubmissionBatch{{
					Request: interfaces.WorkRequest{
						Type: interfaces.WorkRequestTypeFactoryRequestBatch,
						Works: []interfaces.Work{{
							Name:       "hook-work",
							WorkID:     "work-hook",
							WorkTypeID: "task",
							TraceID:    "trace-hook",
						}},
					},
					Metadata: interfaces.GeneratedSubmissionBatchMetadata{Source: "inputs/task/default"},
				}},
			}, nil
		},
	}

	eng := NewFactoryEngine(n, marking, nil,
		WithSubmissionHook(hook),
		WithWorkRequestRecorder(func(_ int, record interfaces.WorkRequestRecord) {
			order = append(order, "request:"+record.RequestID)
			requestRecords = append(requestRecords, record)
		}),
		WithWorkInputRecorder(func(_ int, req interfaces.SubmitRequest, _ interfaces.Token) {
			order = append(order, "input:"+req.WorkID)
			workInputs = append(workInputs, req)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if len(requestRecords) != 1 {
		t.Fatalf("work request records = %d, want 1", len(requestRecords))
	}
	if requestRecords[0].RequestID == "" || requestRecords[0].Source != "inputs/task/default" {
		t.Fatalf("work request record = %#v, want generated request ID from inputs/task/default", requestRecords[0])
	}
	if len(workInputs) != 1 {
		t.Fatalf("work input records = %d, want 1", len(workInputs))
	}
	if workInputs[0].WorkID != "work-hook" || workInputs[0].TraceID != "trace-hook" {
		t.Fatalf("work input = %#v, want hook work with trace-hook", workInputs[0])
	}
	if workInputs[0].RequestID != requestRecords[0].RequestID {
		t.Fatalf("work input request ID = %q, want generated request record ID %q", workInputs[0].RequestID, requestRecords[0].RequestID)
	}
	expectedOrder := []string{"request:" + requestRecords[0].RequestID, "input:work-hook"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("record order = %#v, want %#v", order, expectedOrder)
	}
	for i := range expectedOrder {
		if order[i] != expectedOrder[i] {
			t.Fatalf("record order[%d] = %q, want %q (full order %#v)", i, order[i], expectedOrder[i], order)
		}
	}

	snap := eng.GetMarking()
	if tokens := snap.TokensInPlace("task:init"); len(tokens) != 1 {
		t.Fatalf("expected hook submission to inject 1 token, got %d", len(tokens))
	}
}

func TestSubmissionHooks_RunInPriorityThenNameOrderAndCarryContinuationState(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	var order []string

	makeHook := func(name string, priority int) factory.SubmissionHook {
		return &testSubmissionHook{
			name:     name,
			priority: priority,
			onTick: func(_ context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
				order = append(order, name+":"+input.ContinuationState["seen"])
				return interfaces.SubmissionHookResult{ContinuationState: map[string]string{"seen": name}}, nil
			},
		}
	}

	eng := NewFactoryEngine(n, marking, nil,
		WithSubmissionHook(makeHook("beta", 5)),
		WithSubmissionHook(makeHook("alpha", 5)),
		WithSubmissionHook(makeHook("early", 1)),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error: %v", err)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error: %v", err)
	}

	expected := []string{
		"early:", "alpha:", "beta:",
		"early:early", "alpha:alpha", "beta:beta",
	}
	if len(order) != len(expected) {
		t.Fatalf("expected order %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("order[%d] = %q, want %q (full order %v)", i, order[i], expected[i], order)
		}
	}
}

func TestSubmissionHook_ResultsAreVisibleToTick(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	hook := &testSubmissionHook{
		name:     "replay-due-results",
		priority: 1,
		onTick: func(_ context.Context, _ interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
			return interfaces.SubmissionHookResult{
				Results: []interfaces.WorkResult{{
					DispatchID:   "dispatch-1",
					TransitionID: "transition-1",
					Outcome:      interfaces.OutcomeAccepted,
				}},
			}, nil
		},
	}
	observer := &mockSubsystem{
		group: subsystems.History,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if len(snap.Results) != 1 {
				t.Fatalf("expected hook result visible to subsystem, got %d results", len(snap.Results))
			}
			return &interfaces.TickResult{}, nil
		},
	}

	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{observer}, WithSubmissionHook(hook))
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
}

func TestRun_KeepsTickingWhileSubmissionHookRequestsKeepAlive(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	var seenTicks []int

	hook := &testSubmissionHook{
		name:     "replay-keepalive",
		priority: 1,
		onTick: func(_ context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
			seenTicks = append(seenTicks, input.Snapshot.TickCount)
			return interfaces.SubmissionHookResult{
				KeepAlive: input.Snapshot.TickCount < 3,
			}, nil
		},
	}

	terminator := &mockSubsystem{
		group: subsystems.History,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{ShouldTerminate: true}, nil
		},
	}

	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{terminator}, WithSubmissionHook(hook))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := eng.Run(ctx); err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	expected := []int{1, 2, 3}
	if len(seenTicks) != len(expected) {
		t.Fatalf("seen ticks = %v, want %v", seenTicks, expected)
	}
	for i := range expected {
		if seenTicks[i] != expected[i] {
			t.Fatalf("seenTicks[%d] = %d, want %d (full sequence %v)", i, seenTicks[i], expected[i], seenTicks)
		}
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-generated-batch-fixture review=2026-07-18 removal=split-recording-order-and-idempotency-assertions-before-next-generated-batch-change
func TestTickResultGeneratedBatchesRecordedBeforeInputsAndIdempotent(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	batch := interfaces.GeneratedSubmissionBatch{
		Request: interfaces.WorkRequest{
			RequestID: "generated-request-1",
			Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
			Works: []interfaces.Work{
				{Name: "draft", WorkID: "work-draft", WorkTypeID: "task", TraceID: "trace-generated"},
				{Name: "review", WorkID: "work-review", WorkTypeID: "task"},
			},
			Relations: []interfaces.WorkRelation{{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "draft",
				RequiredState:  "complete",
			}},
		},
		Metadata: interfaces.GeneratedSubmissionBatchMetadata{Source: "generator:test"},
		Submissions: []interfaces.SubmitRequest{{
			Name:        "review",
			WorkID:      "work-review",
			TargetState: "complete",
			Tags:        map[string]string{"runtime": "true"},
		}},
	}

	sub := &mockSubsystem{
		group: subsystems.Transitioner,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				GeneratedBatches: []interfaces.GeneratedSubmissionBatch{batch, batch},
			}, nil
		},
	}

	var order []string
	var requests []interfaces.WorkRequestRecord
	var inputs []interfaces.SubmitRequest
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{sub},
		WithWorkRequestRecorder(func(_ int, record interfaces.WorkRequestRecord) {
			order = append(order, "request:"+record.RequestID)
			requests = append(requests, record)
		}),
		WithWorkInputRecorder(func(_ int, req interfaces.SubmitRequest, _ interfaces.Token) {
			order = append(order, "input:"+req.WorkID)
			inputs = append(inputs, req)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("work request records = %d, want idempotent single record", len(requests))
	}
	if requests[0].Source != "generator:test" {
		t.Fatalf("work request source = %q, want generator:test", requests[0].Source)
	}
	if len(requests[0].Relations) != 1 || requests[0].Relations[0].SourceWorkID != "work-review" {
		t.Fatalf("request relations = %#v, want canonical relation", requests[0].Relations)
	}
	if len(inputs) != 2 {
		t.Fatalf("work input records = %d, want 2", len(inputs))
	}
	if got := inputs[1].Tags["runtime"]; got != "true" {
		t.Fatalf("runtime tag = %q, want true", got)
	}
	expectedOrder := []string{
		"request:generated-request-1",
		"input:work-draft",
		"input:work-review",
	}
	if len(order) != len(expectedOrder) {
		t.Fatalf("record order = %#v, want %#v", order, expectedOrder)
	}
	for i := range expectedOrder {
		if order[i] != expectedOrder[i] {
			t.Fatalf("record order[%d] = %q, want %q (full order %#v)", i, order[i], expectedOrder[i], order)
		}
	}

	snap := eng.GetMarking()
	if tokens := snap.TokensInPlace("task:init"); len(tokens) != 1 || tokens[0].Color.WorkID != "work-draft" {
		t.Fatalf("task:init tokens = %#v, want only draft", tokens)
	}
	if tokens := snap.TokensInPlace("task:complete"); len(tokens) != 1 || tokens[0].Color.WorkID != "work-review" {
		t.Fatalf("task:complete tokens = %#v, want only review", tokens)
	}
}

func TestDispatchRecordsTrackedInRunningDispatches(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	// Subsystem that produces DispatchRecords on the first call only.
	// This tests the engine's dispatch tracking independently of mutation
	// application (which is already covered by other tests and stress tests).
	alreadyDispatched := false
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if alreadyDispatched {
				return nil, nil
			}
			alreadyDispatched = true
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{
					{
						Dispatch: interfaces.WorkDispatch{DispatchID: "d1", TransitionID: "t1", WorkerType: "test-worker"},
						Mutations: []interfaces.MarkingMutation{
							{Type: interfaces.MutationConsume, TokenID: "tok-1", FromPlace: "task:init", Reason: "consumed by transition t1"},
						},
					},
				},
			}, nil
		},
	}

	var dispatched []string
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(d interfaces.WorkDispatch) {
			dispatched = append(dispatched, d.TransitionID)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	// Verify the dispatch was forwarded.
	if len(dispatched) != 1 || dispatched[0] != "t1" {
		t.Errorf("expected 1 dispatch for t1, got %v", dispatched)
	}

	// Verify the running dispatches map is populated.
	running := eng.RunningDispatches()
	if len(running) != 1 {
		t.Fatalf("expected 1 running dispatch, got %d", len(running))
	}
	mutations, ok := running["d1"]
	if !ok {
		t.Fatal("expected running dispatch for d1")
	}
	if len(mutations) != 1 || mutations[0].TokenID != "tok-1" {
		t.Errorf("expected 1 mutation consuming tok-1, got %v", mutations)
	}

	// Simulate result arrival — enqueue a result and notify the engine.
	// Dispatch entries are retired at end-of-tick based on processed results.
	eng.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "d1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	eng.NotifyResult()
	// Drain channels and process the result (including end-of-tick retirement).
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	running = eng.RunningDispatches()
	if len(running) != 0 {
		t.Errorf("expected 0 running dispatches after result, got %d", len(running))
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-dispatch-hook-fixture review=2026-07-18 removal=split-dispatch-recording-and-payload-assertions-before-next-dispatch-hook-change
func TestDispatchResultHook_RecordsDispatchBeforeSubmittingToHook(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{
						DispatchID:   "dispatch-1",
						TransitionID: "transition-1",
						WorkerType:   "worker-a",
						Execution: interfaces.ExecutionMetadata{
							TraceID:   "trace-1",
							WorkIDs:   []string{"work-1"},
							ReplayKey: "transition-1/trace-1/work-1",
						},
						InputTokens: workers.InputTokens(interfaces.Token{
							ID:      "token-1",
							PlaceID: "task:init",
						}),
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "token-1",
						FromPlace: "task:init",
					}},
				}},
			}, nil
		},
	}

	var records []interfaces.FactoryDispatchRecord
	hook := newTestDispatchResultHook()
	var eng *FactoryEngine
	hook.submit = func(_ context.Context, dispatch interfaces.WorkDispatch) error {
		if len(records) != 1 {
			t.Fatalf("dispatch submitted before recorder observed it; record count = %d", len(records))
		}
		if _, ok := eng.runtimeState.Dispatches[dispatch.DispatchID]; !ok {
			t.Fatalf("dispatch %q submitted before engine running-dispatch tracking", dispatch.DispatchID)
		}
		return nil
	}

	eng = NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			records = append(records, record)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 recorded dispatch, got %d", len(records))
	}
	record := records[0]
	if record.Dispatch.Execution.DispatchCreatedTick != 1 {
		t.Fatalf("dispatch execution created tick = %d, want 1", record.Dispatch.Execution.DispatchCreatedTick)
	}
	if record.Dispatch.Execution.CurrentTick != 1 {
		t.Fatalf("dispatch execution current tick = %d, want 1", record.Dispatch.Execution.CurrentTick)
	}
	if record.Dispatch.Execution.ReplayKey != "transition-1/trace-1/work-1" {
		t.Fatalf("dispatch execution replay key = %q, want transition-1/trace-1/work-1", record.Dispatch.Execution.ReplayKey)
	}
	if record.Dispatch.DispatchID != "dispatch-1" {
		t.Fatalf("unexpected dispatch record: %#v", record)
	}
	if len(record.ConsumedTokens) != 1 || record.ConsumedTokens[0] != "token-1" {
		t.Fatalf("consumed tokens = %#v, want [token-1]", record.ConsumedTokens)
	}
	if len(hook.submits) != 1 {
		t.Fatalf("expected hook to receive 1 dispatch, got %d", len(hook.submits))
	}
	if hook.submits[0].Execution.DispatchCreatedTick != 1 || hook.submits[0].Execution.CurrentTick != 1 {
		t.Fatalf("hook dispatch execution metadata = %#v, want created/current tick 1", hook.submits[0].Execution)
	}
}

func TestDispatchEntry_SubmitsRawInterfacesWorkDispatch(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	inputToken := interfaces.Token{
		ID:      "token-raw",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-raw",
			WorkTypeID: "task",
			TraceID:    "trace-raw",
		},
	}
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{{
					Dispatch: interfaces.WorkDispatch{
						DispatchID:      "dispatch-raw",
						TransitionID:    "transition-raw",
						WorkerType:      "worker-raw",
						WorkstationName: "station-raw",
						InputTokens:     workers.InputTokens(inputToken),
						InputBindings:   map[string][]string{"work": []string{"token-raw"}},
					},
					Mutations: []interfaces.MarkingMutation{{
						Type:      interfaces.MutationConsume,
						TokenID:   "token-raw",
						FromPlace: "task:init",
					}},
				}},
			}, nil
		},
	}

	hook := newTestDispatchResultHook()
	var handled []interfaces.WorkDispatch
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchResultHook(hook),
		WithDispatchHandler(func(dispatch interfaces.WorkDispatch) {
			handled = append(handled, dispatch)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(hook.submits) != 1 {
		t.Fatalf("hook submits = %d, want 1", len(hook.submits))
	}
	if len(handled) != 1 {
		t.Fatalf("handled dispatches = %d, want 1", len(handled))
	}

	for label, dispatch := range map[string]interfaces.WorkDispatch{
		"hook":    hook.submits[0],
		"handler": handled[0],
	} {
		if dispatch.DispatchID != "dispatch-raw" || dispatch.WorkerType != "worker-raw" {
			t.Fatalf("%s dispatch identity = %#v, want raw dispatch identity", label, dispatch)
		}
		if len(dispatch.InputBindings) == 0 {
			t.Fatalf("%s dispatch payload = %#v, want canonical dispatch-owned bindings preserved", label, dispatch)
		}
		if got := dispatch.InputBindings["work"]; len(got) != 1 || got[0] != "token-raw" {
			t.Fatalf("%s input bindings = %#v, want token-raw binding", label, dispatch.InputBindings)
		}
		tokens := workers.WorkDispatchInputTokens(dispatch)
		if len(tokens) != 1 || tokens[0].ID != "token-raw" {
			t.Fatalf("%s input tokens = %#v, want token-raw", label, tokens)
		}
		if dispatch.Execution.DispatchCreatedTick != 1 || dispatch.Execution.CurrentTick != 1 {
			t.Fatalf("%s dispatch execution = %#v, want tick metadata from raw entry", label, dispatch.Execution)
		}
	}
}

func TestDispatchResultHook_CompletionRecordedAtObservedTick(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	hook := newTestDispatchResultHook()
	hook.results = []interfaces.WorkResult{{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		Outcome:      interfaces.OutcomeAccepted,
	}}

	var records []interfaces.FactoryCompletionRecord
	observer := &mockSubsystem{
		group: subsystems.History,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if len(snap.Results) != 1 {
				t.Fatalf("expected completion result visible to subsystem, got %d results", len(snap.Results))
			}
			return &interfaces.TickResult{}, nil
		},
	}
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{observer},
		WithDispatchResultHook(hook),
		WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			records = append(records, record)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 completion record, got %d", len(records))
	}
	if records[0].ObservedTick != 1 {
		t.Fatalf("observed tick = %d, want 1", records[0].ObservedTick)
	}
	if records[0].DispatchID != "dispatch-1" {
		t.Fatalf("dispatch ID = %q, want dispatch-1", records[0].DispatchID)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-dispatch-completion-fixture review=2026-07-18 removal=split-dispatch-entry-and-completion-identity-assertions-before-next-completion-change
func TestTokenNamePopulatedOnDispatchAndCompletion(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	// Subsystem that produces a DispatchRecord with InputTokens carrying a Name.
	alreadyDispatched := false
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if alreadyDispatched {
				return nil, nil
			}
			alreadyDispatched = true
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{
					{
						Dispatch: interfaces.WorkDispatch{
							DispatchID:   "d1",
							TransitionID: "t1",
							WorkerType:   "test-worker",
							InputTokens: workers.InputTokens(interfaces.Token{
								ID:      "tok-1",
								PlaceID: "task:init",
								Color: interfaces.TokenColor{
									Name:       "my-task-name",
									WorkID:     "work-1",
									WorkTypeID: "task",
								},
							}),
						},
						Mutations: []interfaces.MarkingMutation{
							{Type: interfaces.MutationConsume, TokenID: "tok-1", FromPlace: "task:init", Reason: "consumed"},
						},
					},
				},
			}, nil
		},
	}

	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(_ interfaces.WorkDispatch) {}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	// Verify dispatch identity remains available in raw consumed tokens.
	snap := eng.GetRuntimeStateSnapshot()
	entry, ok := snap.Dispatches["d1"]
	if !ok {
		t.Fatal("expected dispatch entry for d1")
	}
	if len(entry.ConsumedTokens) != 1 {
		t.Fatalf("expected 1 consumed token on DispatchEntry, got %d", len(entry.ConsumedTokens))
	}
	if got := entry.ConsumedTokens[0].Color.Name; got != "my-task-name" {
		t.Errorf("expected token name on DispatchEntry = my-task-name, got %q", got)
	}

	// Simulate result arrival to trigger CompletedDispatch creation.
	eng.GetResultBuffer().Write(context.Background(), interfaces.WorkResult{
		DispatchID:   "d1",
		TransitionID: "t1",
		Outcome:      interfaces.OutcomeAccepted,
	})
	eng.NotifyResult()
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	// Verify completed dispatch identity remains available in raw records.
	snap = eng.GetRuntimeStateSnapshot()
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	completed := snap.DispatchHistory[0]
	if len(completed.ConsumedTokens) != 1 {
		t.Fatalf("expected 1 consumed token on CompletedDispatch, got %d", len(completed.ConsumedTokens))
	}
	if got := completed.ConsumedTokens[0].Color.Name; got != "my-task-name" {
		t.Errorf("expected token name on CompletedDispatch = my-task-name, got %q", got)
	}
	if got := completed.ConsumedTokens[0].Color.WorkID; got != "work-1" {
		t.Errorf("expected work ID on CompletedDispatch = work-1, got %q", got)
	}
}

func TestDispatchRecordsAlwaysTracked(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	// Subsystem with DispatchRecords — the only dispatch mechanism.
	dispatchSub := &mockSubsystem{
		group: subsystems.Dispatcher,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Dispatches: []interfaces.DispatchRecord{
					{
						Dispatch: interfaces.WorkDispatch{DispatchID: "d1", TransitionID: "t1", WorkerType: "test-worker"},
						Mutations: []interfaces.MarkingMutation{
							{Type: interfaces.MutationConsume, TokenID: "tok-1", FromPlace: "task:init", Reason: "consumed by transition t1"},
						},
					},
				},
			}, nil
		},
	}

	var dispatched []string
	eng := NewFactoryEngine(n, marking, []subsystems.Subsystem{dispatchSub},
		WithDispatchHandler(func(d interfaces.WorkDispatch) {
			dispatched = append(dispatched, d.TransitionID)
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	// Dispatch forwarded via the unified dispatch path.
	if len(dispatched) != 1 || dispatched[0] != "t1" {
		t.Errorf("expected 1 dispatch for t1, got %v", dispatched)
	}

	// Dispatches are always tracked in running dispatches.
	running := eng.RunningDispatches()
	if len(running) != 1 {
		t.Errorf("expected 1 running dispatch, got %d", len(running))
	}
	if _, ok := running["d1"]; !ok {
		t.Fatal("expected running dispatch for d1")
	}
}

func TestMutationsAppliedBetweenSubsystems(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	// Pre-add a token.
	marking.AddToken(&interfaces.Token{
		ID:      "tok-1",
		PlaceID: "task:init",
		Color:   interfaces.TokenColor{WorkTypeID: "task"},
		History: interfaces.TokenHistory{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	})

	// First subsystem moves token from init to complete.
	mover := &mockSubsystem{
		group: subsystems.Scheduler,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{
				Mutations: []interfaces.MarkingMutation{
					{Type: interfaces.MutationMove, TokenID: "tok-1", FromPlace: "task:init", ToPlace: "task:complete"},
				},
			}, nil
		},
	}

	// Second subsystem should see the token in task:complete.
	var observedPlace string
	observer := &mockSubsystem{
		group: subsystems.Tracer,
		execFn: func(_ context.Context, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			if tok, ok := snap.Marking.Tokens["tok-1"]; ok {
				observedPlace = tok.PlaceID
			}
			return &interfaces.TickResult{}, nil
		},
	}

	engine := NewFactoryEngine(n, marking, []subsystems.Subsystem{mover, observer})
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if observedPlace != "task:complete" {
		t.Errorf("expected observer to see token in 'task:complete', got %q", observedPlace)
	}
}
