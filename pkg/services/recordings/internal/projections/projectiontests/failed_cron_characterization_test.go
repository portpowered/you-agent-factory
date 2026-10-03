package projections_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldState_ConsumedFailedCronRetainsHistoryWithoutLiveOwnership(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	cron := work.FactoryWorkItem{
		ID: "synthetic-cron-input", WorkTypeID: interfaces.SystemTimeWorkTypeID,
		State: interfaces.SystemTimePendingState,
		Tags: map[string]string{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: "synthetic-cron-route",
		},
	}
	output := work.FactoryWorkItem{ID: "synthetic-downstream-task", WorkTypeID: "task", State: "failed"}
	events := []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeInitialStructureRequest, "synthetic-structure", 0, base,
			factoryapi.FactoryEventContext{}, factoryapi.InitialStructureRequestEventPayload{Factory: factoryapi.Factory{
				WorkTypes: &[]factoryapi.WorkType{
					{Name: "task", States: []factoryapi.WorkState{{Name: "init", Type: factoryapi.WorkStateTypeINITIAL}, {Name: "failed", Type: factoryapi.WorkStateTypeFAILED}}},
					{Name: interfaces.SystemTimeWorkTypeID, States: []factoryapi.WorkState{{Name: interfaces.SystemTimePendingState, Type: factoryapi.WorkStateTypePROCESSING}}},
				},
			}}),
		workInputEvent(1, base.Add(time.Second), cron),
		workstationRequestEvent(2, base.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID: "synthetic-failed-dispatch", TransitionID: "synthetic-cron-route",
			Inputs: []interfaces.WorkstationInput{{TokenID: cron.ID, PlaceID: interfaces.SystemTimePendingPlaceID, WorkItem: &cron}},
		}),
		workstationResponseEvent(3, base.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID: "synthetic-failed-dispatch", TransitionID: "synthetic-cron-route",
			Result:     interfaces.WorkstationResult{Outcome: "FAILED", Error: "synthetic failure"},
			OutputWork: []work.FactoryWorkItem{output},
		}),
	}
	state, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if _, exists := state.ActiveWorkItemsByID[cron.ID]; exists {
		t.Fatal("consumed failed cron input remains falsely active")
	}
	if _, exists := state.WorkItemsByID[cron.ID]; !exists || len(state.ActiveDispatches) != 0 {
		t.Fatal("projection lost historical input or retained completed dispatch ownership")
	}
	if pending := state.PlaceOccupancyByID[interfaces.SystemTimePendingPlaceID]; len(pending.WorkItemIDs) != 0 {
		t.Fatalf("consumed input still occupies pending place: %#v", pending)
	}
	if placed := state.PlaceOccupancyByID["task:failed"].WorkItemIDs; len(placed) != 1 || placed[0] != output.ID {
		t.Fatalf("downstream placement = %v, want %q", placed, output.ID)
	}
	if len(state.CompletedDispatches) != 1 || len(state.FailedDispatches) != 1 || state.FailedDispatches[0].Result.Error != "synthetic failure" {
		t.Fatal("projection did not retain completed dispatch and its failure history")
	}
}
