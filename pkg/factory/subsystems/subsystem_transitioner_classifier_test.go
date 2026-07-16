package subsystems

import (
	"context"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/ralph"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
)

func TestBuiltInRalphFactory_PlannerOutputFeedsRepeatingExecutor(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(ralph.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	net, err := (&factoryconfig.ConfigMapper{}).Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}
	planner := ralphWorkstation(t, cfg.Workstations, ralph.PackagedPlanWorkstationName)
	executor := ralphWorkstation(t, cfg.Workstations, ralph.PackagedExecuteWorkstationName)
	now := time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC)

	planned := executeRalphResult(t, net, planner, "ralph:init", interfaces.OutcomeAccepted, "1. Implement the requested change.", now)
	if got := planned.Mutations[0].NewToken.PlaceID; got != "ralph:execute" {
		t.Fatalf("planner destination = %q, want ralph:execute", got)
	}
	if got := string(planned.Mutations[0].NewToken.Color.Payload); got != "1. Implement the requested change." {
		t.Fatalf("planner payload = %q, want planned output", got)
	}

	continued := executeRalphResult(t, net, executor, "ralph:execute", interfaces.OutcomeContinue, "2. Continue execution.", now)
	if got := continued.Mutations[0].NewToken.PlaceID; got != "ralph:execute" {
		t.Fatalf("executor continuation destination = %q, want ralph:execute", got)
	}
	if got := string(continued.Mutations[0].NewToken.Color.Payload); got != "2. Continue execution." {
		t.Fatalf("executor continuation payload = %q, want latest execution output", got)
	}
}

func executeRalphResult(t *testing.T, net *state.Net, workstation interfaces.FactoryWorkstationConfig, place string, outcome interfaces.WorkOutcome, output string, now time.Time) *interfaces.TickResult {
	t.Helper()
	transition := ralphTransition(t, net, workstation.Name)
	transitioner := NewTransitioner(net, nil,
		WithTransitionerClock(func() time.Time { return now }),
		WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{Workstations: map[string]*interfaces.FactoryWorkstationConfig{workstation.Name: &workstation}}),
	)
	result, err := transitioner.Execute(context.Background(), &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch": {
				DispatchID:      "dispatch",
				TransitionID:    transition.ID,
				WorkstationName: workstation.Name,
				ConsumedTokens: []interfaces.Token{{
					ID:      "ralph-work",
					PlaceID: place,
					Color:   interfaces.TokenColor{WorkID: "ralph-work", WorkTypeID: ralph.PackagedWorkTypeName, Payload: []byte("customer request")},
					History: interfaces.TokenHistory{TotalVisits: map[string]int{}, ConsecutiveFailures: map[string]int{}, PlaceVisits: map[string]int{}},
				}},
			},
		},
		Results: []interfaces.WorkResult{{DispatchID: "dispatch", TransitionID: transition.ID, Outcome: outcome, Output: output}},
	})
	if err != nil {
		t.Fatalf("Execute(%s): %v", outcome, err)
	}
	if result == nil || len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want one mutation", result)
	}
	return result
}

func ralphTransition(t *testing.T, net *state.Net, name string) *petri.Transition {
	t.Helper()
	for _, transition := range net.Transitions {
		if transition.Name == name {
			return transition
		}
	}
	t.Fatalf("missing transition %q", name)
	return nil
}

func ralphWorkstation(t *testing.T, workstations []interfaces.FactoryWorkstationConfig, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	for _, workstation := range workstations {
		if workstation.Name == name {
			return workstation
		}
	}
	t.Fatalf("missing workstation %q", name)
	return interfaces.FactoryWorkstationConfig{}
}

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
	if completed.Outcome != interfaces.OutcomeAccepted {
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
	if completed.Outcome != interfaces.OutcomeFailed {
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
	if completed := result.CompletedDispatches[0]; completed.Outcome != interfaces.OutcomeFailed {
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
		WithTransitionerClock(func() time.Time { return now }),
		WithTransitionerRuntimeConfig(runtimefixtures.RuntimeWorkstationLookupFixture{
			Workstations: map[string]*interfaces.FactoryWorkstationConfig{
				"classifier": {Name: "classifier", Type: interfaces.WorkstationTypeClassify},
			},
		}),
	)
}

func newClassifierSnapshot(now time.Time, output string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"d-1": {
				DispatchID:      "d-1",
				TransitionID:    "t1",
				WorkstationName: "classifier",
				StartTime:       now.Add(-time.Second),
				ConsumedTokens: []interfaces.Token{{
					ID:        "tok-1",
					PlaceID:   "task:init",
					CreatedAt: now.Add(-time.Hour),
					EnteredAt: now.Add(-time.Hour),
					Color: interfaces.TokenColor{
						WorkID:     "work-1",
						WorkTypeID: "task",
					},
					History: interfaces.TokenHistory{
						TotalVisits:         map[string]int{},
						ConsecutiveFailures: map[string]int{},
						PlaceVisits:         map[string]int{},
					},
				}},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "d-1",
			TransitionID: "t1",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       output,
		}},
	}
}
func TestExecutorReviewReconcile_ProcessAcceptPreservesSiblingLaneReviewInit(t *testing.T) {
	const traceID = "trace-process-sibling"
	now := time.Date(2026, 6, 15, 15, 0, 0, 0, time.UTC)
	marking := buildExecutorReviewReconcileMarking(t, []*interfaces.Token{
		{
			ID: "review-lane-a-old", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-a-old", WorkTypeID: "review", Name: "lane-a",
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-lane-b-sibling", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-b", WorkTypeID: "review", Name: "lane-b",
				CurrentChainingTraceID: traceID,
			},
		},
	})

	consumed := []interfaces.Token{{
		ID: "task-input", PlaceID: "task:init",
		Color: interfaces.TokenColor{
			WorkID: "work-task-a", WorkTypeID: "task", Name: "lane-a",
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
	marking := buildExecutorReviewReconcileMarking(t, []*interfaces.Token{
		{
			ID: "review-lane-a-extra", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-a-extra", WorkTypeID: "review", Name: laneName,
				CurrentChainingTraceID: traceID,
			},
		},
		{
			ID: "review-lane-b-sibling", PlaceID: "review:init",
			Color: interfaces.TokenColor{
				WorkID: "work-review-b", WorkTypeID: "review", Name: "lane-b",
				CurrentChainingTraceID: traceID,
			},
		},
	})
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
