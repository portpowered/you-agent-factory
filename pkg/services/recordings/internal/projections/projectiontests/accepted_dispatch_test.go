package projections_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldState_AcceptedDispatchRemainsUnappliedWhilePaused(t *testing.T) {
	t.Parallel()
	events := pausedAcceptedDispatchEvents()
	state, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatal(err)
	}
	if state.FactoryState != "PAUSED" || len(state.ActiveDispatches) != 1 || len(state.CompletedDispatches) != 0 {
		t.Fatalf("acceptance was confused with application: state=%s active=%d completed=%d", state.FactoryState, len(state.ActiveDispatches), len(state.CompletedDispatches))
	}
	if state.WorkItemsByID["work-1"].State != "init" || len(state.WorkItemsByID) != 1 {
		t.Fatalf("acceptance fabricated Work output: %#v", state.WorkItemsByID)
	}
	if state.PlaceOccupancyByID["agent-slot:available"].TokenCount != 0 {
		t.Fatal("acceptance released an in-flight resource before application")
	}
	if len(state.AgentRunResponsesByDispatchID["dispatch-agent-1"]) != 1 {
		t.Fatal("projection lost the recorded agent acceptance")
	}
}

func TestReconstructFactoryWorldState_AcceptedDispatchCompletionWhilePausedRetainsOutputAndLineage(t *testing.T) {
	t.Parallel()
	events := pausedAcceptedDispatchEvents()
	base := events[0].Context.EventTime
	events = append(events, workstationResponseEvent(5, base.Add(5*time.Second), interfaces.WorkstationResponsePayload{
		DispatchID: "dispatch-agent-1", TransitionID: "t-review", Result: interfaces.WorkstationResult{Outcome: "ACCEPTED", Output: "complete response"},
		Outputs: []interfaces.WorkstationOutput{
			{Type: string(interfaces.MutationMove), TokenID: "work-1", ToPlace: "task:complete", WorkItem: &work.FactoryWorkItem{
				ID: "work-1", WorkTypeID: "task", State: "complete", TraceID: "trace-1",
			}},
			{Type: string(interfaces.MutationCreate), TokenID: "child-1", ToPlace: "task:init", WorkItem: &work.FactoryWorkItem{
				ID: "child-1", WorkTypeID: "task", State: "init", TraceID: "trace-1",
			}},
		},
		OutputResources: []interfaces.FactoryResourceUnit{{ResourceID: "agent-slot"}},
	}))
	for _, tick := range []int{5, 6} {
		state, err := ReconstructFactoryWorldState(events, tick)
		if err != nil {
			t.Fatal(err)
		}
		if state.FactoryState != "PAUSED" || len(state.ActiveDispatches) != 0 || len(state.CompletedDispatches) != 1 {
			t.Fatalf("completion projection state=%s active=%d completed=%d", state.FactoryState, len(state.ActiveDispatches), len(state.CompletedDispatches))
		}
		if len(state.WorkItemsByID) != 2 || state.WorkItemsByID["work-1"].State != "complete" {
			t.Fatalf("applied Work = %#v, want one original and one child", state.WorkItemsByID)
		}
		if ids := state.PlaceOccupancyByID["task:init"].WorkItemIDs; len(ids) != 1 || ids[0] != "child-1" {
			t.Fatalf("child occupancy = %v", ids)
		}
		if state.PlaceOccupancyByID["agent-slot:available"].TokenCount != 1 {
			t.Fatal("completed dispatch did not release exactly one resource")
		}
		if completion := state.CompletedDispatches[0]; completion.DispatchID != "dispatch-agent-1" || len(completion.InputWorkItems) != 1 || completion.InputWorkItems[0].ID != "work-1" {
			t.Fatalf("dispatch input lineage = %#v", completion)
		}
	}
}

func pausedAcceptedDispatchEvents() []factoryapi.FactoryEvent {
	base := time.Date(2026, time.October, 9, 0, 0, 0, 0, time.UTC)
	return []factoryapi.FactoryEvent{
		initialStructureEventWithResources(base, []factoryapi.Resource{{Name: "agent-slot", Capacity: 1}}),
		workInputEventWithToken(1, base.Add(time.Second), "work-1", work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", State: "init", TraceID: "trace-1"}),
		workstationRequestEvent(2, base.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID: "dispatch-agent-1", TransitionID: "t-review", Workstation: interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs:    []interfaces.WorkstationInput{{TokenID: "work-1", PlaceID: "task:init", WorkItem: &work.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", State: "init", TraceID: "trace-1"}}},
			Resources: []interfaces.FactoryResourceUnit{{ResourceID: "agent-slot"}},
		}),
		agentRunResponseEvent(3, base.Add(3*time.Second), factoryapi.AgentRunResponseEventPayload{AgentRunId: "dispatch-agent-1/agent-run/1", Outcome: factoryapi.WorkOutcomeAccepted}),
		factoryStateEvent(4, base.Add(4*time.Second), "RUNNING", "PAUSED"),
	}
}

func TestReconstructFactoryWorldState_ProviderOnlyDispatchInterruptionReleasesOriginalWork(t *testing.T) {
	t.Parallel()
	events := pausedAcceptedDispatchEvents()[:3]
	base := events[0].Context.EventTime
	response := modelResponseEvent(3, base.Add(3*time.Second), factoryapi.ModelResponseEventPayload{
		ModelRequestId: "dispatch-agent-1/model-request/1", Attempt: 1,
		Outcome: factoryapi.InferenceOutcomeSucceeded, OutputPreview: stringPtrForProjectionTest("provider succeeded"),
	})
	events = append(events, response)
	pending, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending.ActiveDispatches) != 1 || len(pending.CompletedDispatches) != 0 || len(pending.AgentRunResponsesByDispatchID) != 0 {
		t.Fatal("provider success was interpreted as agent acceptance or applied completion")
	}
	events = append(events, generatedProjectionEvent(factoryapi.FactoryEventTypeDispatchInterrupted, "restart-interruption", 4, base.Add(4*time.Second),
		factoryapi.FactoryEventContext{DispatchId: stringPtrForProjectionTest("dispatch-agent-1")}, factoryapi.DispatchInterruptedEventPayload{
			InterruptedAt: base.Add(4 * time.Second), ObservedStatus: factoryapi.FactoryDispatchStatus("RUNNING"),
			Reason: "daemon restart interrupted process-bound attempt", RetryPlanned: true,
		}))
	restored, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.ActiveDispatches) != 0 || len(restored.CompletedDispatches) != 0 || len(restored.WorkItemsByID) != 1 {
		t.Fatal("interruption fabricated completion or output Work")
	}
	if ids := restored.PlaceOccupancyByID["task:init"].WorkItemIDs; len(ids) != 1 || ids[0] != "work-1" {
		t.Fatalf("interrupted Work was not restored for retry: %v", ids)
	}
	if restored.PlaceOccupancyByID["agent-slot:available"].TokenCount != 1 {
		t.Fatal("interruption did not release the original resource")
	}
}
