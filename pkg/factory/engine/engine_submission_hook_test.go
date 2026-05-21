package engine

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/factory/subsystems"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

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
	assertStringSequence(t, order, []string{"request:" + requestRecords[0].RequestID, "input:work-hook"}, "record order")

	markingSnap := eng.GetMarking()
	if tokens := markingSnap.TokensInPlace("task:init"); len(tokens) != 1 {
		t.Fatalf("expected hook submission to inject 1 token, got %d", len(tokens))
	}
}

func TestSubmissionHooks_RunInPriorityThenNameOrderAndCarryContinuationState(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	var order []string

	makeHook := func(name string, priority int) *testSubmissionHook {
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

	assertStringSequence(t, order, []string{
		"early:", "alpha:", "beta:",
		"early:early", "alpha:alpha", "beta:beta",
	}, "hook order")
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
			return interfaces.SubmissionHookResult{KeepAlive: input.Snapshot.TickCount < 3}, nil
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
	assertIntSequence(t, seenTicks, []int{1, 2, 3}, "seen ticks")
}

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
			return &interfaces.TickResult{GeneratedBatches: []interfaces.GeneratedSubmissionBatch{batch, batch}}, nil
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
	if len(requests[0].Relations) != 1 ||
		requests[0].Relations[0].SourceWorkID != "work-review" ||
		requests[0].Relations[0].TargetWorkID != "work-draft" ||
		requests[0].Relations[0].RequiredState != "complete" {
		t.Fatalf("request relations = %#v, want canonical relation", requests[0].Relations)
	}
	if len(inputs) != 2 {
		t.Fatalf("work input records = %d, want 2", len(inputs))
	}
	if got := inputs[1].Tags["runtime"]; got != "true" {
		t.Fatalf("runtime tag = %q, want true", got)
	}
	assertStringSequence(t, order, []string{
		"request:generated-request-1",
		"input:work-draft",
		"input:work-review",
	}, "record order")

	markingSnap := eng.GetMarking()
	assertMarkingTokenIDs(t, markingSnap.TokensInPlace("task:init"), []string{"work-draft"}, "task:init")
	assertMarkingTokenIDs(t, markingSnap.TokensInPlace("task:complete"), []string{"work-review"}, "task:complete")
}

func assertStringSequence(t *testing.T, got, want []string, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %q, want %q (full %s %#v)", label, i, got[i], want[i], label, got)
		}
	}
}

func assertIntSequence(t *testing.T, got, want []int, label string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] = %d, want %d (full %s %v)", label, i, got[i], want[i], label, got)
		}
	}
}

func assertMarkingTokenIDs(t *testing.T, tokens []interfaces.Token, want []string, label string) {
	t.Helper()
	if len(tokens) != len(want) {
		t.Fatalf("%s tokens = %#v, want ids %v", label, tokens, want)
	}
	for i, id := range want {
		if tokens[i].Color.WorkID != id {
			t.Fatalf("%s tokens[%d] work ID = %q, want %q", label, i, tokens[i].Color.WorkID, id)
		}
	}
}
