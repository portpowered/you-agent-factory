package subsystems

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
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

func newClassifierTransitionerFixture(now time.Time) *TransitionerSubsystem {
	net := &state.Net{
		Places: map[string]*petri.Place{
			"task:init":     {ID: "task:init", TypeID: "task", State: "init"},
			"task:approved": {ID: "task:approved", TypeID: "task", State: "approved"},
			"task:review":   {ID: "task:review", TypeID: "task", State: "review"},
			"task:failed":   {ID: "task:failed", TypeID: "task", State: "failed"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {ID: "task"},
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
