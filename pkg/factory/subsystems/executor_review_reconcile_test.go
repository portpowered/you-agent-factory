package subsystems

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestExecutorReviewReconcile_ProcessAcceptConsumesExistingReviewInitForTrace(t *testing.T) {
	const traceID = "trace-process-review"
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	marking := buildExecutorReviewReconcileMarking(t, []*interfaces.Token{{
		ID:      "review-old",
		PlaceID: "review:init",
		Color: interfaces.TokenColor{
			WorkID:                 "work-review-old",
			WorkTypeID:             "review",
			Name:                   "lane-a",
			CurrentChainingTraceID: traceID,
		},
	}})

	consumed := []interfaces.Token{{
		ID:      "task-input",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID:                 "work-task-1",
			WorkTypeID:             "task",
			Name:                   "lane-a",
			CurrentChainingTraceID: traceID,
		},
	}}

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationProcess,
		interfaces.OutcomeAccepted,
		consumed,
		[]petri.Arc{
			{PlaceID: state.PlaceID("task", "in-review")},
			{PlaceID: state.PlaceID("review", "init")},
		},
		now,
	)
	if len(mutations) != 1 {
		t.Fatalf("mutations = %d, want 1 consume for existing review:init", len(mutations))
	}
	if mutations[0].Type != interfaces.MutationConsume || mutations[0].TokenID != "review-old" {
		t.Fatalf("mutation = %#v, want consume review-old", mutations[0])
	}
}

func TestExecutorReviewReconcile_ReviewAcceptConsumesDuplicateReviewsAndStaleTaskResidue(t *testing.T) {
	const (
		traceID  = "trace-review-complete"
		laneName = "lane-b"
	)
	now := time.Date(2026, 6, 15, 12, 5, 0, 0, time.UTC)
	marking := buildReviewReconcileResidueMarking(t, traceID, laneName)
	consumed := buildReviewReconcileActiveDispatchTokens(traceID, laneName)

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationReview,
		interfaces.OutcomeAccepted,
		consumed,
		[]petri.Arc{
			{PlaceID: state.PlaceID("task", "to-complete")},
			{PlaceID: state.PlaceID("review", "complete")},
		},
		now,
	)
	assertReviewReconcileResidueConsumes(t, mutations)
}

func buildReviewReconcileResidueMarking(t *testing.T, traceID, laneName string) *petri.MarkingSnapshot {
	t.Helper()

	return buildExecutorReviewReconcileMarking(t, []*interfaces.Token{
		{
			ID: "review-extra-1", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-extra-1", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-extra-2", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-extra-2", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "task-stale-init", PlaceID: "task:init",
			Color: interfaces.TokenColor{
				WorkID: "work-task-stale-init", WorkTypeID: "task", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "task-stale-failed", PlaceID: "task:failed",
			Color: interfaces.TokenColor{
				WorkID: "work-task-stale-failed", WorkTypeID: "task", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "task-other-lane", PlaceID: "task:failed",
			Color: interfaces.TokenColor{
				WorkID: "work-task-other", WorkTypeID: "task", Name: "other-lane",
				CurrentChainingTraceID: traceID,
			},
		},
	})
}

func buildReviewReconcileActiveDispatchTokens(traceID, laneName string) []interfaces.Token {
	return []interfaces.Token{
		{
			ID: "task-in-review", PlaceID: "task:in-review",
			Color: interfaces.TokenColor{
				WorkID: "work-task-1", WorkTypeID: "task", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-active", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-active", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
	}
}

func assertReviewReconcileResidueConsumes(t *testing.T, mutations []interfaces.MarkingMutation) {
	t.Helper()

	if len(mutations) != 4 {
		t.Fatalf("mutations = %d, want 4 consumes (2 review + init + failed)", len(mutations))
	}

	got := mutationTokenIDs(mutations)
	want := map[string]bool{
		"review-extra-1":    true,
		"review-extra-2":    true,
		"task-stale-init":   true,
		"task-stale-failed": true,
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("missing consume for %q in %#v", id, got)
		}
	}
	if got["task-other-lane"] {
		t.Fatalf("unexpected consume for unrelated lane token: %#v", got)
	}
	if got["review-active"] || got["task-in-review"] {
		t.Fatalf("consumed active dispatch tokens: %#v", got)
	}
}

func TestExecutorReviewReconcile_SkipsNonAcceptedOutcomes(t *testing.T) {
	marking := buildExecutorReviewReconcileMarking(t, []*interfaces.Token{{
		ID:      "review-old",
		PlaceID: "review:init",
		Color: interfaces.TokenColor{
			WorkTypeID:             "review",
			CurrentChainingTraceID: "trace-skip",
		},
	}})
	consumed := []interfaces.Token{{
		Color: interfaces.TokenColor{
			WorkTypeID:             "task",
			CurrentChainingTraceID: "trace-skip",
		},
	}}

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationProcess,
		interfaces.OutcomeRejected,
		consumed,
		[]petri.Arc{{PlaceID: state.PlaceID("review", "init")}},
		time.Now(),
	)
	if len(mutations) != 0 {
		t.Fatalf("mutations = %#v, want none for rejected outcome", mutations)
	}
}

func mutationTokenIDs(mutations []interfaces.MarkingMutation) map[string]bool {
	out := make(map[string]bool, len(mutations))
	for i := range mutations {
		if mutations[i].TokenID != "" {
			out[mutations[i].TokenID] = true
		}
	}
	return out
}

func buildExecutorReviewReconcileMarking(t *testing.T, tokens []*interfaces.Token) *petri.MarkingSnapshot {
	t.Helper()

	marking := petri.NewMarking("executor-review-reconcile")
	for _, token := range tokens {
		marking.AddToken(token)
	}
	snapshot := marking.Snapshot()
	return &snapshot
}

func TestHistoryTransitionerPipeline_ProcessAcceptReconcilesDuplicateReviewInit(t *testing.T) {
	const (
		traceID  = "trace-process-pipeline"
		laneName = "lane-process-reconcile"
	)
	now := time.Date(2026, 6, 15, 14, 0, 0, 0, time.UTC)

	marking := petri.NewMarking("process-reconcile-pipeline")
	marking.AddToken(&interfaces.Token{
		ID: "review-old-1", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-review-old-1", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&interfaces.Token{
		ID: "review-old-2", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-review-old-2", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})

	taskToken := interfaces.Token{
		ID: "task-init", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-task-1", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: marking.Snapshot(),
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-process": {
				DispatchID:     "d-process",
				TransitionID:   "process",
				ConsumedTokens: []interfaces.Token{taskToken},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "d-process",
			TransitionID: "process",
			Outcome:      interfaces.OutcomeAccepted,
		}},
	}

	tp := newTestPipeline(buildProcessReconcilePipelineNet())
	tp.transitioner.now = func() time.Time { return now }

	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected tick result")
	}

	consumed := mutationTokenIDs(result.Mutations)
	for _, id := range []string{"review-old-1", "review-old-2"} {
		if !consumed[id] {
			t.Fatalf("missing reconcile consume for %q in %#v", id, result.Mutations)
		}
	}
}

func TestHistoryTransitionerPipeline_ReviewAcceptReconcilesDuplicateReviewAndStaleTask(t *testing.T) {
	const (
		traceID  = "trace-review-pipeline"
		laneName = "lane-reconcile"
	)
	now := time.Date(2026, 6, 15, 13, 0, 0, 0, time.UTC)

	marking := petri.NewMarking("review-reconcile-pipeline")
	marking.AddToken(&interfaces.Token{
		ID: "review-extra", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-review-extra", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&interfaces.Token{
		ID: "task-stale-init", PlaceID: "task:init", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-task-stale-init", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})
	marking.AddToken(&interfaces.Token{
		ID: "task-stale-failed", PlaceID: "task:failed", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-task-stale-failed", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	})

	taskToken := interfaces.Token{
		ID: "task-in-review", PlaceID: "task:in-review", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-task-1", WorkTypeID: "task", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	}
	reviewToken := interfaces.Token{
		ID: "review-active", PlaceID: "review:init", CreatedAt: now, EnteredAt: now,
		Color: interfaces.TokenColor{
			WorkID: "work-review-active", WorkTypeID: "review", Name: laneName,
			CurrentChainingTraceID: traceID,
		},
	}

	snapshot := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Marking: marking.Snapshot(),
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-review": {
				DispatchID:     "d-review",
				TransitionID:   "review",
				ConsumedTokens: []interfaces.Token{taskToken, reviewToken},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "d-review",
			TransitionID: "review",
			Outcome:      interfaces.OutcomeAccepted,
		}},
	}

	tp := newTestPipeline(buildReviewReconcilePipelineNet())
	tp.transitioner.now = func() time.Time { return now }

	result, err := tp.Execute(context.Background(), &snapshot)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("expected tick result")
	}

	consumed := mutationTokenIDs(result.Mutations)
	for _, id := range []string{"review-extra", "task-stale-init", "task-stale-failed"} {
		if !consumed[id] {
			t.Fatalf("missing reconcile consume for %q in %#v", id, result.Mutations)
		}
	}
	if consumed["review-active"] || consumed["task-in-review"] {
		t.Fatalf("consumed active dispatch tokens: %#v", consumed)
	}
}

func buildProcessReconcilePipelineNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":       {ID: "task:init", TypeID: "task", State: "init"},
			"task:in-review":  {ID: "task:in-review", TypeID: "task", State: "in-review"},
			"task:failed":     {ID: "task:failed", TypeID: "task", State: "failed"},
			"review:init":     {ID: "review:init", TypeID: "review", State: "init"},
			"review:complete": {ID: "review:complete", TypeID: "review", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"process": {
				ID:         "process",
				Name:       "process",
				WorkerType: "processor",
				InputArcs: []petri.Arc{
					{ID: "task-in", Name: "task", PlaceID: "task:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "task-out", Name: "task", PlaceID: "task:in-review", Direction: petri.ArcOutput},
					{ID: "review-out", Name: "review", PlaceID: "review:init", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "in-review", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
			"review": {
				ID: "review",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}

func buildReviewReconcilePipelineNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":        {ID: "task:init", TypeID: "task", State: "init"},
			"task:in-review":   {ID: "task:in-review", TypeID: "task", State: "in-review"},
			"task:to-complete": {ID: "task:to-complete", TypeID: "task", State: "to-complete"},
			"task:failed":      {ID: "task:failed", TypeID: "task", State: "failed"},
			"review:init":      {ID: "review:init", TypeID: "review", State: "init"},
			"review:complete":  {ID: "review:complete", TypeID: "review", State: "complete"},
		},
		Transitions: map[string]*petri.Transition{
			"review": {
				ID:         "review",
				Name:       "review",
				WorkerType: "processor",
				InputArcs: []petri.Arc{
					{ID: "task-in", Name: "task", PlaceID: "task:in-review", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
					{ID: "review-in", Name: "review", PlaceID: "review:init", Direction: petri.ArcInput, Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne}},
				},
				OutputArcs: []petri.Arc{
					{ID: "task-out", Name: "task", PlaceID: "task:to-complete", Direction: petri.ArcOutput},
					{ID: "review-out", Name: "review", PlaceID: "review:complete", Direction: petri.ArcOutput},
				},
			},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "in-review", Category: state.StateCategoryProcessing},
					{Value: "to-complete", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
			"review": {
				ID: "review",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
	}
}
