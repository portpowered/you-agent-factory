package subsystems

import (
	"context"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestTransitioner_CanceledDispatchRestoresConsumedWorkWithoutFailureRoute(t *testing.T) {
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(
		net,
		nil,
		func() time.Time { return now },
		testTokenTransformer(net),
		nil,
		nil,
		nil,
		testWorkPropagationPolicy(),
	)
	snapshot := workerBatchSnapshot("")
	snapshot.Dispatches["dispatch-1"].HeldMutations = []interfaces.MarkingMutation{
		{
			Type:      interfaces.MutationConsume,
			TokenID:   "tok-source",
			FromPlace: "task:init",
		},
	}
	snapshot.Results[0] = workerexecution.WorkResult{
		DispatchID:   "dispatch-1",
		TransitionID: "t1",
		Outcome:      workerexecution.OutcomeCanceled,
		Cancellation: workerexecution.NewDispatchCancellation(workerexecution.DispatchCancellationReasonSuperseded),
		Error:        "losing dispatch was superseded",
	}

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("Execute() result = nil, want canceled dispatch retirement")
	}
	if len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want exactly one", result.CompletedDispatches)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != workerexecution.OutcomeCanceled || completed.Cancellation == nil ||
		completed.Cancellation.Reason != workerexecution.DispatchCancellationReasonSuperseded {
		t.Fatalf("completed dispatch = %#v, want SUPERSEDED cancellation", completed)
	}
	if completed.FailureDetail != nil || completed.FailureMetadata != nil {
		t.Fatalf("completed cancellation retained failure facts: %#v", completed)
	}
	if len(result.Mutations) != 1 {
		t.Fatalf("mutations = %#v, want exactly one restored Work token", result.Mutations)
	}
	restored := result.Mutations[0]
	if restored.Type != interfaces.MutationCreate || restored.TokenID != "tok-source" || restored.ToPlace != "task:init" || restored.NewToken == nil {
		t.Fatalf("restored mutation = %#v, want CREATE tok-source at task:init", restored)
	}
	if restored.NewToken.ID != "tok-source" || restored.NewToken.State != "init" || restored.NewToken.EnteredAt != now {
		t.Fatalf("restored token = %#v, want original identity at cancellation time", restored.NewToken)
	}
	if restored.ToPlace == "task:failed" {
		t.Fatal("canceled dispatch emitted a terminal failure mutation")
	}
	if len(completed.OutputMutations) != 1 || completed.OutputMutations[0].Type != interfaces.MutationCreate {
		t.Fatalf("completed output mutations = %#v, want the non-failure restoration", completed.OutputMutations)
	}
}

func TestCalculateArcs_CanceledDispatchHasNoBusinessRoute(t *testing.T) {
	transition := &petri.Transition{
		ID:          "t1",
		OutputArcs:  []petri.Arc{{ID: "accepted", PlaceID: "task:complete"}},
		FailureArcs: []petri.Arc{{ID: "failed", PlaceID: "task:failed"}},
	}
	arcs, err := calculateArcs(transition, workerexecution.OutcomeCanceled)
	if err != nil {
		t.Fatalf("calculateArcs(CANCELED) error = %v, want nil", err)
	}
	if len(arcs) != 0 {
		t.Fatalf("calculateArcs(CANCELED) = %#v, want no business route", arcs)
	}
}
