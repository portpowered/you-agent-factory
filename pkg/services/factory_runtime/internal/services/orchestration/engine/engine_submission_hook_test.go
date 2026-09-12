package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/subsystems"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	token_transformer "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token_transformer"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func assertZeroWorkRequestSubmitResult(t *testing.T, result work.WorkRequestSubmitResult) {
	t.Helper()
	if result.RequestID != "" || result.TraceID != "" || result.WorkID != "" ||
		result.Name != "" || result.WorkTypeName != "" || result.Accepted || len(result.Works) != 0 {
		t.Fatalf("WorkRequestSubmitResult = %#v, want zero result", result)
	}
}

type submissionSnapshot = interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]

func TestSubmissionHook_GeneratedBatchRecordsCanonicalHistoryBeforeInjection(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")

	var order []string
	var requestRecords []work.WorkRequestRecord
	var workInputs []work.SubmitRequest
	hook := &testSubmissionHook{
		name:     "file-preseed",
		priority: 10,
		onTick: func(_ context.Context, input interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
			if input.Snapshot.TickCount != 1 {
				t.Fatalf("hook snapshot tick = %d, want 1", input.Snapshot.TickCount)
			}
			if len(input.Snapshot.Marking.Tokens) != 0 {
				t.Fatalf("hook should run before injection, saw %d tokens", len(input.Snapshot.Marking.Tokens))
			}
			return interfaces.SubmissionHookResult{
				GeneratedBatches: []work.GeneratedSubmissionBatch{{
					Request: work.WorkRequest{
						Type: work.WorkRequestTypeFactoryRequestBatch,
						Works: []work.Work{{
							Name:       "hook-work",
							WorkID:     "work-hook",
							WorkTypeID: "task",
							TraceID:    "trace-hook",
						}},
					},
					Metadata: work.GeneratedSubmissionBatchMetadata{Source: "inputs/task/default"},
				}},
			}, nil
		},
	}

	eng := newTestFactoryEngine(n, marking, nil,
		WithSubmissionHook(hook),
		WithWorkRequestRecorder(func(_ int, record work.WorkRequestRecord) {
			order = append(order, "request:"+record.RequestID)
			requestRecords = append(requestRecords, record)
		}),
		WithWorkInputRecorder(func(_ int, req work.SubmitRequest, _ factorytoken.Token) {
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

func TestSubmissionHook_GeneratedParentChildBatchCompletesRegistrationProjection(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	hook := &testSubmissionHook{
		name:     "generated-parent-child",
		priority: 1,
		onTick: func(_ context.Context, _ interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
			return interfaces.SubmissionHookResult{GeneratedBatches: []work.GeneratedSubmissionBatch{{
				Request: work.WorkRequest{
					Type: work.WorkRequestTypeFactoryRequestBatch,
					Works: []work.Work{
						{Name: "parent", WorkID: "parent-work", WorkTypeID: "task"},
						{Name: "child", WorkID: "child-work", WorkTypeID: "task"},
					},
					Relations: []work.WorkRelation{{
						Type:           work.WorkRelationParentChild,
						SourceWorkName: "child",
						TargetWorkName: "parent",
					}},
				},
			}}}, nil
		},
	}

	eng := newTestFactoryEngine(n, marking, nil, WithSubmissionHook(hook))
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	registration := eng.GetMarking().ParentChildRegistrations["parent-work"]
	if !registration.Complete || len(registration.Children) != 1 {
		t.Fatalf("generated parent-child registration = %#v, want one complete child", registration)
	}
	if got := registration.Children[0].Color.WorkID; got != "child-work" {
		t.Fatalf("generated registered child WorkID = %q, want child-work", got)
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

	eng := newTestFactoryEngine(n, marking, nil,
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
				Results: []workerexecution.WorkResult{{
					DispatchID:   "dispatch-1",
					TransitionID: "transition-1",
					Outcome:      workerexecution.OutcomeAccepted,
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

	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{observer}, WithSubmissionHook(hook))
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
		onTick: func(_ context.Context, input interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
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

	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{terminator}, WithSubmissionHook(hook))
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
	batch := work.GeneratedSubmissionBatch{
		Request: work.WorkRequest{
			RequestID: "generated-request-1",
			Type:      work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.Work{
				{Name: "draft", WorkID: "work-draft", WorkTypeID: "task", TraceID: "trace-generated"},
				{Name: "review", WorkID: "work-review", WorkTypeID: "task"},
			},
			Relations: []work.WorkRelation{{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "draft",
				RequiredState:  "complete",
			}},
		},
		Metadata: work.GeneratedSubmissionBatchMetadata{Source: "generator:test"},
		Submissions: []work.SubmitRequest{{
			Name:        "review",
			WorkID:      "work-review",
			TargetState: "complete",
			Tags:        map[string]string{"runtime": "true"},
		}},
	}
	sub := &mockSubsystem{
		group: subsystems.Transitioner,
		execFn: func(_ context.Context, _ *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) (*interfaces.TickResult, error) {
			return &interfaces.TickResult{GeneratedBatches: []work.GeneratedSubmissionBatch{batch, batch}}, nil
		},
	}

	var order []string
	var requests []work.WorkRequestRecord
	var inputs []work.SubmitRequest
	eng := newTestFactoryEngine(n, marking, []subsystems.Subsystem{sub},
		WithWorkRequestRecorder(func(_ int, record work.WorkRequestRecord) {
			order = append(order, "request:"+record.RequestID)
			requests = append(requests, record)
		}),
		WithWorkInputRecorder(func(_ int, req work.SubmitRequest, _ factorytoken.Token) {
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

func TestSubmissionHook_AppliesMarkingMutationsAndRecordsSubmissionID(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	token := newMoveTestToken("tok-hook-move", "work-hook-move", "task:failed")
	token.History = factorytoken.History{
		TotalVisits:         map[string]int{"transition-build": 1},
		ConsecutiveFailures: map[string]int{"transition-build": 2},
		PlaceVisits:         map[string]int{"task:failed": 1},
		LastError:           "provider timeout",
		FailureLog:          []factorytoken.Failure{{Error: "provider timeout"}},
	}
	marking.AddToken(token)

	var submissions []work.FactorySubmissionRecord
	hook := &testSubmissionHook{
		name:     "marking-mutator",
		priority: 1,
		onTick: func(_ context.Context, input interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
			if _, ok := input.Snapshot.Marking.Tokens["tok-hook-move"]; !ok {
				t.Fatalf("expected seeded token visible to submission hook")
			}
			return interfaces.SubmissionHookResult{
				MarkingMutations: []interfaces.MarkingMutation{{
					Type:      interfaces.MutationMove,
					TokenID:   "tok-hook-move",
					FromPlace: "task:failed",
					ToPlace:   "task:init",
					Reason:    "submission hook relocate",
				}},
				GeneratedBatches: []work.GeneratedSubmissionBatch{{
					Request: work.WorkRequest{
						Type: work.WorkRequestTypeFactoryRequestBatch,
						Works: []work.Work{{
							Name:       "hook-recorded",
							WorkID:     "work-hook-recorded",
							WorkTypeID: "task",
							TraceID:    "trace-hook-recorded",
						}},
					},
					// Empty Metadata.Source exercises generatedSubmissionSource defaulting to hook name.
				}},
			}, nil
		},
	}

	eng := newTestFactoryEngine(n, marking, nil,
		WithSubmissionHook(hook),
		WithSubmissionRecorder(func(record work.FactorySubmissionRecord) {
			submissions = append(submissions, record)
		}),
	)
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}

	if marking.Tokens["tok-hook-move"].PlaceID != "task:init" {
		t.Fatalf("token place = %q, want task:init after hook marking mutation", marking.Tokens["tok-hook-move"].PlaceID)
	}
	if marking.Tokens["tok-hook-move"].History.LastError != "provider timeout" {
		t.Fatalf("failure history LastError = %q, want preserved provider timeout", marking.Tokens["tok-hook-move"].History.LastError)
	}
	if len(marking.Tokens["tok-hook-move"].History.ConsecutiveFailures) != 0 {
		t.Fatalf("guard consecutive failures = %#v, want cleared when leaving failed place", marking.Tokens["tok-hook-move"].History.ConsecutiveFailures)
	}
	if len(submissions) != 1 {
		t.Fatalf("submission records = %d, want 1", len(submissions))
	}
	wantSubmissionID := submissionRecordID(1, "marking-mutator", 0)
	if submissions[0].SubmissionID != wantSubmissionID {
		t.Fatalf("submission ID = %q, want %q", submissions[0].SubmissionID, wantSubmissionID)
	}
	if submissions[0].Source != "marking-mutator" || submissions[0].Request.WorkID != "work-hook-recorded" {
		t.Fatalf("submission record = %#v, want marking-mutator / work-hook-recorded", submissions[0])
	}
	markingSnap := eng.GetMarking()
	if tokens := markingSnap.TokensInPlace("task:init"); len(tokens) != 2 {
		t.Fatalf("task:init tokens = %d, want relocated seed + generated submission", len(tokens))
	}
}

func replaySubmissionBatch(requestID string, workIDs ...string) work.GeneratedSubmissionBatch {
	works := make([]work.Work, 0, len(workIDs))
	for _, workID := range workIDs {
		works = append(works, work.Work{Name: workID, WorkID: workID, WorkTypeID: "task"})
	}
	return work.GeneratedSubmissionBatch{
		Request:  work.WorkRequest{RequestID: requestID, Type: work.WorkRequestTypeFactoryRequestBatch, Works: works},
		Metadata: work.GeneratedSubmissionBatchMetadata{Source: "external-submit"},
	}
}

func TestSubmissionHook_DeduplicatesOnlySeededReplayWork(t *testing.T) {

	t.Run("seeded work is acknowledged while new replay work is materialized", func(t *testing.T) {
		n := buildTestNet()
		marking := petri.NewMarking("test-wf")
		marking.AddToken(newMoveTestToken("token-seeded", "work-seeded", "task:init"))
		hook := &testSubmissionHook{
			name:     factoryruntime.ReplaySubmissionHookName,
			priority: -100,
			onTick: func(_ context.Context, _ interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
				return interfaces.SubmissionHookResult{
					GeneratedBatches: []work.GeneratedSubmissionBatch{
						replaySubmissionBatch("replay-request", "work-seeded", "work-replayed"),
					},
				}, nil
			},
		}
		var inputWorkIDs []string
		eng := newTestFactoryEngine(n, marking, nil,
			WithSeededRestoredWorkIDs("work-seeded"),
			WithSubmissionHook(hook),
			WithWorkInputRecorder(func(_ int, request work.SubmitRequest, _ factorytoken.Token) {
				inputWorkIDs = append(inputWorkIDs, request.WorkID)
			}),
		)

		if err := eng.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error: %v", err)
		}
		markingSnapshot := eng.GetMarking()
		assertMarkingTokenIDs(t, markingSnapshot.TokensInPlace("task:init"), []string{"work-seeded", "work-replayed"}, "seeded replay marking")
		assertStringSequence(t, inputWorkIDs, []string{"work-replayed"}, "seeded replay work inputs")
	})

	t.Run("replay without seeded work keeps materializing", func(t *testing.T) {
		n := buildTestNet()
		eng := newTestFactoryEngine(n, petri.NewMarking("test-wf"), nil,
			WithSubmissionHook(&testSubmissionHook{
				name:     factoryruntime.ReplaySubmissionHookName,
				priority: -100,
				onTick: func(_ context.Context, _ interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
					return interfaces.SubmissionHookResult{GeneratedBatches: []work.GeneratedSubmissionBatch{
						replaySubmissionBatch("replay-request", "work-replayed"),
					}}, nil
				},
			}),
		)
		if err := eng.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error: %v", err)
		}
		markingSnapshot := eng.GetMarking()
		assertMarkingTokenIDs(t, markingSnapshot.TokensInPlace("task:init"), []string{"work-replayed"}, "unseeded replay marking")
	})

	t.Run("other generated hooks are not deduplicated by Work ID", func(t *testing.T) {
		n := buildTestNet()
		marking := petri.NewMarking("test-wf")
		marking.AddToken(newMoveTestToken("token-seeded", "work-seeded", "task:init"))
		eng := newTestFactoryEngine(n, marking, nil,
			WithSeededRestoredWorkIDs("work-seeded"),
			WithSubmissionHook(&testSubmissionHook{
				name:     "ordinary-generator",
				priority: 1,
				onTick: func(_ context.Context, _ interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
					return interfaces.SubmissionHookResult{GeneratedBatches: []work.GeneratedSubmissionBatch{
						replaySubmissionBatch("ordinary-request", "work-seeded"),
					}}, nil
				},
			}),
		)
		if err := eng.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error: %v", err)
		}
		markingSnapshot := eng.GetMarking()
		assertMarkingTokenIDs(t, markingSnapshot.TokensInPlace("task:init"), []string{"work-seeded", "work-seeded"}, "ordinary generated marking")
	})
}

func TestSubmissionHook_ReplacesSeededReplayWorkWithRecordedDispatch(t *testing.T) {
	n := buildTestNet()
	marking := petri.NewMarking("test-wf")
	marking.AddToken(newMoveTestToken("token-seeded", "work-seeded", "task:init"))
	eng := newTestFactoryEngine(n, marking, nil,
		WithSeededRestoredWorkIDs("work-seeded"),
		WithSeededReplayWorkIDsWithRecordedDispatch("work-seeded"),
		WithSubmissionHook(&testSubmissionHook{
			name:     factoryruntime.ReplaySubmissionHookName,
			priority: -100,
			onTick: func(_ context.Context, _ interfaces.SubmissionHookContext[submissionSnapshot]) (interfaces.SubmissionHookResult, error) {
				return interfaces.SubmissionHookResult{GeneratedBatches: []work.GeneratedSubmissionBatch{
					replaySubmissionBatch("replay-request", "work-seeded"),
				}}, nil
			},
		}),
	)

	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error: %v", err)
	}
	markingSnapshot := eng.GetMarking()
	tokens := markingSnapshot.TokensInPlace("task:init")
	if len(tokens) != 1 {
		t.Fatalf("task:init token count = %d, want one replacement token", len(tokens))
	}
	if tokens[0].Color.WorkID != "work-seeded" || tokens[0].ID == "token-seeded" {
		t.Fatalf("replayed replacement token = %#v, want one fresh token for work-seeded", tokens[0])
	}
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

func assertMarkingTokenIDs(t *testing.T, tokens []factorytoken.Token, want []string, label string) {
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

func TestSubmitWorkRequest_RejectsExplicitWorkIDReservedByHistoricalReplay(t *testing.T) {
	t.Parallel()

	n := buildTestNet()
	eng := newTestFactoryEngine(
		n,
		petri.NewMarking("test-wf"),
		nil,
		WithHistoricalWorkIDs("work-task-3"),
	)

	result, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-reuses-consumed-history",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "new-task-with-old-id",
			WorkID:     "work-task-3",
			WorkTypeID: "task",
			TraceID:    "trace-reuses-consumed-history",
		}},
	})
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("SubmitWorkRequest error = %v, want Work Request conflict", err)
	}
	assertZeroWorkRequestSubmitResult(t, result)
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after rejected historical ID: %v", err)
	}
	marking := eng.GetMarking()
	if got := len(marking.TokensInPlace("task:init")); got != 0 {
		t.Fatalf("tokens in task:init = %d, want zero after atomic rejection", got)
	}
}

func TestSubmitWorkRequest_RejectsDuplicateExplicitWorkIDsWithinBatchAtomically(t *testing.T) {
	t.Parallel()

	eng := newTestFactoryEngine(buildTestNet(), petri.NewMarking("test-wf"), nil)
	result, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-duplicate-explicit-ids",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "first", WorkID: "explicit-duplicate", WorkTypeID: "task"},
			{Name: "second", WorkID: "explicit-duplicate", WorkTypeID: "task"},
		},
	})
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("SubmitWorkRequest error = %v, want Work Request conflict", err)
	}
	assertZeroWorkRequestSubmitResult(t, result)
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("Tick after rejected duplicate IDs: %v", err)
	}
	marking := eng.GetMarking()
	if got := len(marking.TokensInPlace("task:init")); got != 0 {
		t.Fatalf("tokens in task:init = %d, want zero after atomic duplicate-ID rejection", got)
	}
}

func TestSubmitWorkRequest_RejectsPreviouslyAdmittedWorkIDAfterItLeavesBoard(t *testing.T) {
	t.Parallel()

	eng := newTestFactoryEngine(buildTestNet(), petri.NewMarking("test-wf"), nil)
	first := work.WorkRequest{
		RequestID: "request-consumed-first",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: "consumed-first", WorkID: "explicit-consumed", WorkTypeID: "task",
		}},
	}
	result, err := eng.SubmitWorkRequest(context.Background(), first)
	if err != nil || !result.Accepted {
		t.Fatalf("first SubmitWorkRequest = (%#v, %v), want accepted", result, err)
	}
	if err := eng.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick: %v", err)
	}
	marking := eng.GetMarking()
	tokens := marking.TokensInPlace("task:init")
	if len(tokens) != 1 {
		t.Fatalf("first task:init tokens = %d, want one", len(tokens))
	}
	eng.runtimeState.Marking.RemoveToken(tokens[0].ID)

	result, err = eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-after-consumption",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: "new-after-consumption", WorkID: "explicit-consumed", WorkTypeID: "task",
		}},
	})
	if !errors.Is(err, work.ErrWorkRequestConflict) {
		t.Fatalf("second SubmitWorkRequest error = %v, want Work Request conflict", err)
	}
	assertZeroWorkRequestSubmitResult(t, result)
}

func TestSubmitWorkRequest_RejectsExplicitWorkIDReservedByCurrentMarkingOrActiveDispatch(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		marking    *petri.Marking
		configure  func(*FactoryEngine)
		wantTokens int
	}{
		{
			name: "current marking",
			marking: func() *petri.Marking {
				marking := petri.NewMarking("test-wf")
				marking.AddToken(&factorytoken.Token{
					ID: "existing-token", PlaceID: "task:init",
					Color: factorytoken.Color{
						Name: "existing-work", WorkID: "work-current",
						WorkTypeID: "task", DataType: factorytoken.DataTypeWork,
					},
				})
				return marking
			}(),
			wantTokens: 1,
		},
		{
			name:    "active dispatch",
			marking: petri.NewMarking("test-wf"),
			configure: func(eng *FactoryEngine) {
				eng.runtimeState.Dispatches["dispatch-existing"] = &interfaces.DispatchEntry{
					DispatchID: "dispatch-existing",
					ConsumedTokens: factorytoken.ToWorkerSlice([]factorytoken.Token{{
						Color: factorytoken.Color{
							Name: "active-work", WorkID: "work-active",
							WorkTypeID: "task", DataType: factorytoken.DataTypeWork,
						},
					}}),
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			eng := newTestFactoryEngine(buildTestNet(), test.marking, nil)
			if test.configure != nil {
				test.configure(eng)
			}

			result, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
				RequestID: "request-reuses-" + test.name,
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.Work{{
					Name: "new-work", WorkID: map[string]string{
						"current marking": "work-current",
						"active dispatch": "work-active",
					}[test.name], WorkTypeID: "task",
				}},
			})
			if !errors.Is(err, work.ErrWorkRequestConflict) {
				t.Fatalf("SubmitWorkRequest error = %v, want Work Request conflict", err)
			}
			assertZeroWorkRequestSubmitResult(t, result)
			if len(eng.workRequests) != 0 || len(eng.submissionHook.batches) != 0 {
				t.Fatalf("conflict mutated admission state: workRequests=%d queuedBatches=%d", len(eng.workRequests), len(eng.submissionHook.batches))
			}
			if got := len(eng.GetMarking().Tokens); got != test.wantTokens {
				t.Fatalf("marking token count = %d, want %d", got, test.wantTokens)
			}
		})
	}
}

func TestSubmitWorkRequest_ConcurrentExplicitWorkIDHasOneWinnerAndTypedLosers(t *testing.T) {
	t.Parallel()

	const claimantCount = 8
	eng := newTestFactoryEngine(buildTestNet(), petri.NewMarking("test-wf"), nil)
	start := make(chan struct{})
	results := make(chan struct {
		result work.WorkRequestSubmitResult
		err    error
	}, claimantCount)
	var wait sync.WaitGroup
	wait.Add(claimantCount)
	for index := 0; index < claimantCount; index++ {
		index := index
		go func() {
			defer wait.Done()
			<-start
			result, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
				RequestID: fmt.Sprintf("concurrent-request-%d", index),
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works: []work.Work{{
					Name: "concurrent-work", WorkID: "work-concurrent", WorkTypeID: "task",
				}},
			})
			results <- struct {
				result work.WorkRequestSubmitResult
				err    error
			}{result: result, err: err}
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	winners := 0
	for outcome := range results {
		if outcome.err == nil {
			if !outcome.result.Accepted {
				t.Fatalf("successful concurrent admission returned unaccepted result: %#v", outcome.result)
			}
			winners++
			continue
		}
		if !errors.Is(outcome.err, work.ErrWorkRequestConflict) {
			t.Fatalf("concurrent losing admission error = %v, want Work Request conflict", outcome.err)
		}
		assertZeroWorkRequestSubmitResult(t, outcome.result)
	}
	if winners != 1 {
		t.Fatalf("concurrent explicit-ID winners = %d, want exactly one", winners)
	}
	if len(eng.workRequests) != 1 || len(eng.submissionHook.batches) != 1 {
		t.Fatalf("concurrent admission state = workRequests:%d queuedBatches:%d, want one each", len(eng.workRequests), len(eng.submissionHook.batches))
	}
}

func TestSubmitWorkRequest_ReservesExplicitGeneratedIDBeforeCrossTypeOutput(t *testing.T) {
	t.Parallel()

	net := buildTestNet()
	review := &state.WorkType{
		ID: "review", Name: "Review",
		States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}},
	}
	net.WorkTypes[review.ID] = review
	for _, place := range review.GeneratePlaces() {
		net.Places[place.ID] = place
	}
	eng := newTestFactoryEngine(net, petri.NewMarking("test-wf"), nil)
	result, err := eng.SubmitWorkRequest(context.Background(), work.WorkRequest{
		RequestID: "request-high-explicit",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: "high explicit", WorkID: "work-task-1000", WorkTypeID: "task",
		}},
	})
	if err != nil || !result.Accepted {
		t.Fatalf("SubmitWorkRequest() = (%#v, %v), want accepted", result, err)
	}

	generated, err := eng.transformer.OutputToken(token_transformer.OutputTokenInput{
		ArcIndex: 0,
		Arcs:     []petri.Arc{{PlaceID: "review:init", Direction: petri.ArcOutput}},
		InputColors: []factorytoken.Color{{
			WorkID: "work-task-1000", WorkTypeID: "task", DataType: factorytoken.DataTypeWork,
		}},
		Outcome: workerexecution.OutcomeAccepted,
		Now:     time.Now(),
	})
	if err != nil {
		t.Fatalf("OutputToken() after explicit admission: %v", err)
	}
	if got := generated.Color.WorkID; got != "work-review-1001" {
		t.Fatalf("generated output Work ID = %q, want work-review-1001", got)
	}
}
