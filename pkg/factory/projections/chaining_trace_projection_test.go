package projections

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func chainingTraceProjectionEvents() []factoryapi.FactoryEvent {
	t0 := time.Date(2026, 4, 22, 12, 0, 0, 0, time.UTC)
	inputZ := interfaces.FactoryWorkItem{
		ID:                     "work-z",
		WorkTypeID:             "task",
		DisplayName:            "Input Z",
		CurrentChainingTraceID: "chain-z",
		TraceID:                "chain-z",
		PlaceID:                "task:init",
	}
	inputA := interfaces.FactoryWorkItem{
		ID:                     "work-a",
		WorkTypeID:             "task",
		DisplayName:            "Input A",
		CurrentChainingTraceID: "chain-a",
		TraceID:                "chain-a",
		PlaceID:                "task:init",
	}
	output := interfaces.FactoryWorkItem{
		ID:                       "work-out",
		WorkTypeID:               "task",
		DisplayName:              "Merged output",
		CurrentChainingTraceID:   "chain-next",
		PreviousChainingTraceIDs: []string{"chain-a", "chain-z"},
		TraceID:                  "chain-next",
	}
	requestEvent := generatedProjectionEvent(
		factoryapi.FactoryEventTypeDispatchRequest,
		"request/dispatch-1",
		2,
		t0.Add(3*time.Second),
		factoryapi.FactoryEventContext{
			DispatchId:               stringPtrForProjectionTest("dispatch-1"),
			CurrentChainingTraceId:   stringPtrForProjectionTest("chain-z"),
			PreviousChainingTraceIds: stringSlicePtrForProjectionTest([]string{"chain-a", "chain-z"}),
			TraceIds:                 stringSlicePtrForProjectionTest([]string{"chain-z", "chain-a"}),
			WorkIds:                  stringSlicePtrForProjectionTest([]string{"work-z", "work-a"}),
		},
		factoryapi.DispatchRequestEventPayload{
			TransitionId:             "t-review",
			CurrentChainingTraceId:   stringPtrForProjectionTest("payload-current"),
			PreviousChainingTraceIds: stringSlicePtrForProjectionTest([]string{"payload-a", "payload-z"}),
			Inputs: []factoryapi.DispatchConsumedWorkRef{
				{WorkId: inputZ.ID},
				{WorkId: inputA.ID},
			},
		},
	)
	responsePayload := factoryapi.DispatchResponseEventPayload{
		TransitionId:             "t-review",
		CurrentChainingTraceId:   stringPtrForProjectionTest("payload-current"),
		PreviousChainingTraceIds: stringSlicePtrForProjectionTest([]string{"payload-a", "payload-z"}),
		Outcome:                  factoryapi.WorkOutcomeAccepted,
		OutputWork:               &[]factoryapi.Work{generatedWorkForProjectionTest(output, "")},
	}

	return []factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), inputZ),
		workInputEvent(1, t0.Add(2*time.Second), inputA),
		requestEvent,
		generatedProjectionEvent(
			factoryapi.FactoryEventTypeDispatchResponse,
			"response/dispatch-1",
			3,
			t0.Add(4*time.Second),
			factoryapi.FactoryEventContext{
				DispatchId:               stringPtrForProjectionTest("dispatch-1"),
				CurrentChainingTraceId:   stringPtrForProjectionTest("chain-z"),
				PreviousChainingTraceIds: stringSlicePtrForProjectionTest([]string{"chain-a", "chain-z"}),
				TraceIds:                 stringSlicePtrForProjectionTest([]string{"chain-next"}),
				WorkIds:                  stringSlicePtrForProjectionTest([]string{"work-out"}),
			},
			responsePayload,
		),
	}
}

func assertChainingTraceProjectionActiveState(t *testing.T, state interfaces.FactoryWorldState) {
	t.Helper()

	activeDispatch := state.ActiveDispatches["dispatch-1"]
	if activeDispatch.CurrentChainingTraceID != "chain-z" {
		t.Fatalf("active dispatch current chaining trace ID = %q, want chain-z", activeDispatch.CurrentChainingTraceID)
	}
	if got := activeDispatch.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("active dispatch previous chaining trace IDs = %#v, want [chain-a chain-z]", got)
	}
}

func assertChainingTraceProjectionCompletedState(t *testing.T, state interfaces.FactoryWorldState) {
	t.Helper()

	if len(state.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want one completion", state.CompletedDispatches)
	}
	completion := state.CompletedDispatches[0]
	if completion.CurrentChainingTraceID != "chain-z" {
		t.Fatalf("completion current chaining trace ID = %q, want chain-z", completion.CurrentChainingTraceID)
	}
	if got := completion.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("completion previous chaining trace IDs = %#v, want [chain-a chain-z]", got)
	}
	if got := completion.OutputWorkItems; len(got) != 1 || len(got[0].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("completion output work items = %#v, want retained output chaining lineage", got)
	}
	if got := state.WorkItemsByID["work-out"].PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("projected output previous chaining trace IDs = %#v, want [chain-a chain-z]", got)
	}
}

func assertChainingTraceProjectionActiveView(t *testing.T, view interfaces.FactoryWorldView) {
	t.Helper()

	activeExecution := view.Runtime.ActiveExecutionsByDispatchID["dispatch-1"]
	if activeExecution.CurrentChainingTraceID != "chain-z" {
		t.Fatalf("active execution current chaining trace ID = %q, want chain-z", activeExecution.CurrentChainingTraceID)
	}
	if got := activeExecution.PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("active execution previous chaining trace IDs = %#v, want [chain-a chain-z]", got)
	}
	if got := activeExecution.WorkItems; len(got) != 2 || len(got[0].PreviousChainingTraceIDs) != 0 {
		t.Fatalf("active execution work items = %#v, want projected chaining-aware refs", got)
	}
}

func assertChainingTraceProjectionCompletedView(t *testing.T, view interfaces.FactoryWorldView) {
	t.Helper()

	history := view.Runtime.Session.DispatchHistory
	if len(history) != 1 {
		t.Fatalf("dispatch history = %#v, want one completion", history)
	}
	if history[0].CurrentChainingTraceID != "chain-z" {
		t.Fatalf("dispatch history current chaining trace ID = %q, want chain-z", history[0].CurrentChainingTraceID)
	}
	if got := history[0].PreviousChainingTraceIDs; len(got) != 2 || got[0] != "chain-a" || got[1] != "chain-z" {
		t.Fatalf("dispatch history previous chaining trace IDs = %#v, want [chain-a chain-z]", got)
	}
	if got := history[0].OutputWorkItems; len(got) != 1 || len(got[0].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("dispatch history output work items = %#v, want retained output chaining lineage", got)
	}
	occupancy := view.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:complete"]
	if len(occupancy) != 1 || len(occupancy[0].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("place occupancy work refs = %#v, want output chaining lineage", occupancy)
	}
}
