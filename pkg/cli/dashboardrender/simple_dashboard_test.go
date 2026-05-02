package dashboardrender

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSimpleDashboardRenderDataFromWorldState_CountsFailedWorkItemsForCustomerSummary(t *testing.T) {
	failedDispatch := interfaces.FactoryWorldDispatchCompletion{
		DispatchID:   "dispatch-1",
		TransitionID: "review",
		WorkItemIDs:  []string{"work-1", "work-2", "work-3"},
		Workstation:  interfaces.FactoryWorkstationRef{Name: "Review"},
		Result:       interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeFailed)},
	}
	worldState := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{failedDispatch},
		FailedWorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{failedDispatch},
	}

	renderData := SimpleDashboardRenderDataFromWorldState(worldState)

	if renderData.Session.DispatchedCount != 1 {
		t.Fatalf("DispatchedCount = %d, want 1 failed dispatch", renderData.Session.DispatchedCount)
	}
	if renderData.Session.CompletedCount != 0 {
		t.Fatalf("CompletedCount = %d, want 0 accepted completions", renderData.Session.CompletedCount)
	}
	if renderData.Session.FailedCount != 3 {
		t.Fatalf("FailedCount = %d, want 3 failed work items", renderData.Session.FailedCount)
	}
	if got := renderData.Session.FailedByWorkType["story"]; got != 3 {
		t.Fatalf("FailedByWorkType[story] = %d, want 3", got)
	}
}
