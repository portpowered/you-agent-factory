package dashboardrender

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestSimpleDashboardRenderDataFromWorldState_CountsFailedWorkItemsForCustomerSummary(t *testing.T) {
	worldState := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		FailedWorkItemsByID: map[string]interfaces.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		FailedDispatches: []interfaces.FactoryWorldDispatchCompletion{
			{
				DispatchID:   "dispatch-1",
				TransitionID: "review",
				WorkItemIDs:  []string{"work-1", "work-2", "work-3"},
				Workstation:  interfaces.FactoryWorkstationRef{Name: "Review"},
				Result:       interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeFailed)},
			},
		},
	}

	renderData := SimpleDashboardRenderDataFromWorldState(worldState)

	if renderData.Session.FailedCount != 3 {
		t.Fatalf("FailedCount = %d, want 3 failed work items", renderData.Session.FailedCount)
	}
	if got := renderData.Session.FailedByWorkType["story"]; got != 3 {
		t.Fatalf("FailedByWorkType[story] = %d, want 3", got)
	}
}
