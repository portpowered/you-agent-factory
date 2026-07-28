package subsystems

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestTransitioner_ClassifierAcceptedRoutesOnlyMatchedLabel(t *testing.T) {
	now := time.Date(2026, time.April, 18, 3, 0, 0, 0, time.UTC)
	transitioner := newClassifierTransitionerFixture(now)
	snapshot := newClassifierSnapshot(now, "approved")

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one matched classifier output mutation", result)
	}
	if result.Mutations[0].ToPlace != "task:approved" {
		t.Fatalf("matched classifier output place = %q, want task:approved", result.Mutations[0].ToPlace)
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1", result)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("completed outcome = %s, want ACCEPTED", completed.Outcome)
	}
	if completed.SelectedClassificationLabel != "approved" {
		t.Fatalf("selected classification label = %q, want approved", completed.SelectedClassificationLabel)
	}
	if len(completed.OutputMutations) != 1 || completed.OutputMutations[0].ToPlace != "task:approved" {
		t.Fatalf("completed output mutations = %#v, want only approved route evidence", completed.OutputMutations)
	}
}

func TestTransitioner_ClassifierUnknownLabelFallsThroughFailureWithoutSelectedLabel(t *testing.T) {
	now := time.Date(2026, time.April, 18, 3, 10, 0, 0, time.UTC)
	transitioner := newClassifierTransitionerFixture(now)
	snapshot := newClassifierSnapshot(now, "unknown")

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one failure mutation", result)
	}
	if result.Mutations[0].ToPlace != "task:failed" {
		t.Fatalf("failure output place = %q, want task:failed", result.Mutations[0].ToPlace)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("completed outcome = %s, want FAILED", completed.Outcome)
	}
	if completed.SelectedClassificationLabel != "" {
		t.Fatalf("selected classification label = %q, want empty on failed classifier", completed.SelectedClassificationLabel)
	}
	if !strings.Contains(completed.Reason, `classifier label "unknown" did not match any authored classification route`) {
		t.Fatalf("completed reason = %q, want unknown classifier label explanation", completed.Reason)
	}
}

func TestTransitioner_ClassifierUnknownLabelUsesImplicitNormalizedFailureArc(t *testing.T) {
	now := time.Date(2026, time.April, 18, 3, 20, 0, 0, time.UTC)
	transitioner := newClassifierTransitionerFixtureWithoutExplicitFailure(now)
	snapshot := newClassifierSnapshot(now, "unknown")

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one failure mutation", result)
	}
	if result.Mutations[0].ToPlace != "task:failed" {
		t.Fatalf("failure output place = %q, want task:failed", result.Mutations[0].ToPlace)
	}
	if completed := result.CompletedDispatches[0]; completed.Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("completed outcome = %s, want FAILED", completed.Outcome)
	}
}

func newClassifierTransitionerFixture(now time.Time) *TransitionerSubsystem {
	net := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:approved": {ID: "task:approved", TypeID: "task", State: "approved"},
			"task:review":   {ID: "task:review", TypeID: "task", State: "review"},
			"task:failed":   {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "approved", Category: state.StateCategoryTerminal},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "classifier",
				OutputArcs: []petri.Arc{
					{ID: "approved", PlaceID: "task:approved", ClassificationLabel: "approved"},
					{ID: "needs-review", PlaceID: "task:review", ClassificationLabel: "needs_review"},
				},
				FailureArcs: []petri.Arc{
					{ID: "failed", PlaceID: "task:failed"},
				},
			},
		},
	}

	return newClassifierTransitionerFromNet(now, net)
}

func newClassifierTransitionerFixtureWithoutExplicitFailure(now time.Time) *TransitionerSubsystem {
	net := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:approved": {ID: "task:approved", TypeID: "task", State: "approved"},
			"task:review":   {ID: "task:review", TypeID: "task", State: "review"},
			"task:failed":   {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "approved", Category: state.StateCategoryTerminal},
					{Value: "review", Category: state.StateCategoryProcessing},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID:   "t1",
				Name: "classifier",
				InputArcs: []petri.Arc{
					{ID: "in", PlaceID: "task:init", Direction: petri.ArcInput},
				},
				OutputArcs: []petri.Arc{
					{ID: "approved", PlaceID: "task:approved", ClassificationLabel: "approved"},
					{ID: "needs-review", PlaceID: "task:review", ClassificationLabel: "needs_review"},
				},
			},
		},
	}
	state.NormalizeTransitionTopology(net, nil)

	return newClassifierTransitionerFromNet(now, net)
}

func newClassifierTransitionerFromNet(now time.Time, net *state.Net) *TransitionerSubsystem {
	return NewTransitioner(
		net,
		nil,
		func() time.Time { return now }, testTokenTransformer(
			net),

		runtimefixtures.RuntimeWorkstationLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Name: "classifier", Type: interfaces.WorkstationTypeClassify},
			},
		},
		nil,
		nil,
		testWorkPropagationPolicy())

}

func newClassifierSnapshot(now time.Time, output string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-1": {
				DispatchID:      "d-1",
				TransitionID:    "t1",
				WorkstationName: "classifier",
				StartTime:       now.Add(-time.Second),
				ConsumedTokens: []factorytoken.Token{{
					ID:        "tok-1",
					PlaceID:   "task:init",
					CreatedAt: now.Add(-time.Hour),
					EnteredAt: now.Add(-time.Hour),
					Color: factorytoken.Color{
						WorkID:     "work-1",
						WorkTypeID: "task",
					},
					History: factorytoken.History{
						TotalVisits:         map[string]int{},
						ConsecutiveFailures: map[string]int{},
						PlaceVisits:         map[string]int{},
					},
				}},
			},
		},
		Results: []workerexecution.WorkResult{{
			DispatchID:   "d-1",
			TransitionID: "t1",
			Outcome:      workerexecution.OutcomeAccepted,
			Output:       output,
		}},
	}
}
func TestExecutorReviewReconcile_ProcessAcceptPreservesSiblingLaneReviewInit(t *testing.T) {
	const traceID = "trace-process-sibling"
	now := time.Date(2026, 6, 15, 15, 0, 0, 0, time.UTC)
	marking := buildExecutorReviewReconcileMarking(t, []*factorytoken.Token{
		{
			ID: "review-lane-a-old", PlaceID: "review:init",
			Color: factorytoken.Color{
				WorkID: "work-review-a-old", WorkTypeID: "review", Name: "lane-a",
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-lane-b-sibling", PlaceID: "review:init",
			Color: factorytoken.Color{
				WorkID: "work-review-b", WorkTypeID: "review", Name: "lane-b",
				CurrentChainingTraceID: traceID,
			},
		},
	})

	consumed := []factorytoken.Token{{
		ID: "task-input", PlaceID: "task:init",
		Color: factorytoken.Color{
			WorkID: "work-task-a", WorkTypeID: "task", Name: "lane-a",
			CurrentChainingTraceID: traceID,
		},
	}}

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationProcess,
		workerexecution.OutcomeAccepted,
		consumed,
		[]petri.Arc{
			{PlaceID: state.PlaceID("task", "in-review")},
			{PlaceID: state.PlaceID("review", "init")},
		},
		now,
	)
	got := mutationTokenIDs(mutations)
	if !got["review-lane-a-old"] {
		t.Fatalf("expected consume for same-lane duplicate review:init, got %#v", got)
	}
	if got["review-lane-b-sibling"] {
		t.Fatalf("consumed sibling lane review:init on same trace: %#v", got)
	}
}

func TestExecutorReviewReconcile_ReviewAcceptPreservesSiblingLaneReviewInit(t *testing.T) {
	const (
		traceID  = "trace-review-sibling"
		laneName = "lane-a"
	)
	now := time.Date(2026, 6, 15, 15, 5, 0, 0, time.UTC)
	marking := buildExecutorReviewReconcileMarking(t, []*factorytoken.Token{
		{
			ID: "review-lane-a-extra", PlaceID: "review:init",
			Color: factorytoken.Color{
				WorkID: "work-review-a-extra", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-lane-b-sibling", PlaceID: "review:init",
			Color: factorytoken.Color{
				WorkID: "work-review-b", WorkTypeID: "review", Name: "lane-b",
				CurrentChainingTraceID: traceID,
			},
		},
	})
	consumed := buildReviewReconcileActiveDispatchTokens(traceID, laneName)

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationReview,
		workerexecution.OutcomeAccepted,
		consumed,
		[]petri.Arc{
			{PlaceID: state.PlaceID("task", "to-complete")},
			{PlaceID: state.PlaceID("review", "complete")},
		},
		now,
	)
	got := mutationTokenIDs(mutations)
	if !got["review-lane-a-extra"] {
		t.Fatalf("expected consume for same-lane duplicate review:init, got %#v", got)
	}
	if got["review-lane-b-sibling"] {
		t.Fatalf("consumed sibling lane review:init on same trace: %#v", got)
	}
	if got["review-active"] || got["task-in-review"] {
		t.Fatalf("consumed active dispatch tokens: %#v", got)
	}
}

func TestExecutorReviewReconcile_ProcessAcceptConsumesExistingReviewInitForTrace(t *testing.T) {
	const traceID = "trace-process-review"
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	marking := buildExecutorReviewReconcileMarking(t, []*factorytoken.Token{{
		ID:      "review-old",
		PlaceID: "review:init",
		Color: factorytoken.Color{
			WorkID:                 "work-review-old",
			WorkTypeID:             "review",
			Name:                   "lane-a",
			CurrentChainingTraceID: traceID,
		},
	}})

	consumed := []factorytoken.Token{{
		ID:      "task-input",
		PlaceID: "task:init",
		Color: factorytoken.Color{
			WorkID:                 "work-task-1",
			WorkTypeID:             "task",
			Name:                   "lane-a",
			CurrentChainingTraceID: traceID,
		},
	}}

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationProcess,
		workerexecution.OutcomeAccepted,
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
		workerexecution.OutcomeAccepted,
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

	return buildExecutorReviewReconcileMarking(t, []*factorytoken.Token{
		{
			ID: "review-extra-1", PlaceID: "review:init",
			Color: factorytoken.Color{
				WorkID: "work-review-extra-1", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-extra-2", PlaceID: "review:init",
			Color: factorytoken.Color{
				WorkID: "work-review-extra-2", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "task-stale-init", PlaceID: "task:init",
			Color: factorytoken.Color{
				WorkID: "work-task-stale-init", WorkTypeID: "task", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "task-stale-failed", PlaceID: "task:failed",
			Color: factorytoken.Color{
				WorkID: "work-task-stale-failed", WorkTypeID: "task", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "task-other-lane", PlaceID: "task:failed",
			Color: factorytoken.Color{
				WorkID: "work-task-other", WorkTypeID: "task", Name: "other-lane",
				CurrentChainingTraceID: traceID,
			},
		},
	})
}

func buildReviewReconcileActiveDispatchTokens(traceID, laneName string) []factorytoken.Token {
	return []factorytoken.Token{
		{
			ID: "task-in-review", PlaceID: "task:in-review",
			Color: factorytoken.Color{
				WorkID: "work-task-1", WorkTypeID: "task", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-active", PlaceID: "review:init",
			Color: factorytoken.Color{
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
	marking := buildExecutorReviewReconcileMarking(t, []*factorytoken.Token{{
		ID:      "review-old",
		PlaceID: "review:init",
		Color: factorytoken.Color{
			WorkTypeID:             "review",
			CurrentChainingTraceID: "trace-skip",
		},
	}})
	consumed := []factorytoken.Token{{
		Color: factorytoken.Color{
			WorkTypeID:             "task",
			CurrentChainingTraceID: "trace-skip",
		},
	}}

	mutations := executorReviewReconcileMutations(
		marking,
		executorReviewWorkstationProcess,
		workerexecution.OutcomeRejected,
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

func buildExecutorReviewReconcileMarking(t *testing.T, tokens []*factorytoken.Token) *petri.MarkingSnapshot {
	t.Helper()

	marking := petri.NewMarking("executor-review-reconcile")
	for _, token := range tokens {
		marking.AddToken(token)
	}
	snapshot := marking.Snapshot()
	return &snapshot
}
