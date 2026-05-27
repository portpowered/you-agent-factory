package projections_test

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	. "github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this dashboard projection case keeps the active-dispatch observability contract together in one scenario.
func TestBuildSimpleDashboardProjection_TracksActiveDispatchState(t *testing.T) {
	t0 := time.Date(2026, 5, 5, 15, 0, 0, 0, time.UTC)
	state, err := ReconstructFactoryWorldState([]factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), interfaces.FactoryWorkItem{
			ID:          "work-1",
			WorkTypeID:  "task",
			DisplayName: "Write docs",
			TraceID:     "trace-1",
			PlaceID:     "task:init",
		}),
		workstationRequestEvent(2, t0.Add(2*time.Second), interfaces.WorkstationRequestPayload{
			DispatchID:   "dispatch-1",
			TransitionID: "t-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Inputs: []interfaces.WorkstationInput{{
				TokenID: "work-1",
				PlaceID: "task:init",
				WorkItem: &interfaces.FactoryWorkItem{
					ID:          "work-1",
					WorkTypeID:  "task",
					DisplayName: "Write docs",
					TraceID:     "trace-1",
					PlaceID:     "task:init",
				},
			}},
		}),
	}, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}

	projection := BuildSimpleDashboardProjection(state)

	if projection.Runtime.InFlightDispatchCount != 1 {
		t.Fatalf("InFlightDispatchCount = %d, want 1", projection.Runtime.InFlightDispatchCount)
	}
	execution, ok := projection.Runtime.ActiveExecutionsByDispatchID["dispatch-1"]
	if !ok {
		t.Fatalf("ActiveExecutionsByDispatchID = %#v, want dispatch-1", projection.Runtime.ActiveExecutionsByDispatchID)
	}
	if execution.WorkstationNodeID != "t-review" || len(execution.WorkItems) != 1 || execution.WorkItems[0].WorkID != "work-1" {
		t.Fatalf("active execution = %#v, want dispatch-1 for work-1 on t-review", execution)
	}
	activity, ok := projection.Runtime.WorkstationActivityByNodeID["t-review"]
	if !ok {
		t.Fatalf("WorkstationActivityByNodeID = %#v, want t-review activity", projection.Runtime.WorkstationActivityByNodeID)
	}
	if len(activity.ActiveDispatchIDs) != 1 || activity.ActiveDispatchIDs[0] != "dispatch-1" {
		t.Fatalf("active dispatch IDs = %#v, want [dispatch-1]", activity.ActiveDispatchIDs)
	}
	if got := projection.Runtime.CurrentWorkItemsByPlaceID["task:init"]; len(got) != 0 {
		t.Fatalf("current work items at task:init = %#v, want empty after dispatch consumes input", got)
	}
	if got := projection.Runtime.PlaceOccupancyWorkItemsByPlaceID["task:init"]; len(got) != 0 {
		t.Fatalf("place occupancy work items at task:init = %#v, want empty after dispatch consumes input", got)
	}
	if !projection.Runtime.Session.HasData {
		t.Fatalf("session.HasData = false, want true")
	}
	if projection.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("session.DispatchedCount = %d, want 1", projection.Runtime.Session.DispatchedCount)
	}
	if projection.Runtime.Session.CompletedCount != 0 {
		t.Fatalf("session.CompletedCount = %d, want 0", projection.Runtime.Session.CompletedCount)
	}
	if projection.Runtime.Session.FailedCount != 0 {
		t.Fatalf("session.FailedCount = %d, want 0", projection.Runtime.Session.FailedCount)
	}
}

func TestBuildSimpleDashboardProjection_TracksTerminalAndFailedTransitions(t *testing.T) {
	t0 := time.Date(2026, 5, 5, 16, 0, 0, 0, time.UTC)

	tests := []struct {
		name                   string
		outcome                string
		wantCompletedCount     int
		wantFailedCount        int
		wantTerminalPlace      string
		wantTerminalCategory   string
		wantFailureDetailCount int
	}{
		{
			name:                 "accepted terminal",
			outcome:              "ACCEPTED",
			wantCompletedCount:   1,
			wantFailedCount:      0,
			wantTerminalPlace:    "task:complete",
			wantTerminalCategory: "TERMINAL",
		},
		{
			name:                   "failed terminal",
			outcome:                "FAILED",
			wantCompletedCount:     0,
			wantFailedCount:        1,
			wantTerminalPlace:      "task:failed",
			wantTerminalCategory:   "FAILED",
			wantFailureDetailCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := reconstructTerminalDashboardState(t0, tt.outcome)
			if err != nil {
				t.Fatalf("ReconstructFactoryWorldState: %v", err)
			}

			projection := BuildSimpleDashboardProjection(state)
			assertTerminalProjection(t, state, projection, tt.wantCompletedCount, tt.wantFailedCount, tt.wantTerminalPlace, tt.wantTerminalCategory, tt.wantFailureDetailCount, tt.outcome)
		})
	}
}

func TestBuildSimpleDashboardProjection_TracksNonTerminalTransitionRoutes(t *testing.T) {
	t0 := time.Date(2026, 5, 5, 17, 0, 0, 0, time.UTC)

	tests := []struct {
		name            string
		outcome         string
		wantPlaceID     string
		wantCurrentNode string
	}{
		{
			name:            "continue route",
			outcome:         "CONTINUE",
			wantPlaceID:     "task:retry",
			wantCurrentNode: "task:retry",
		},
		{
			name:            "rejection route",
			outcome:         "REJECTED",
			wantPlaceID:     "task:triage",
			wantCurrentNode: "task:triage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := reconstructNonTerminalDashboardState(t0, tt.outcome)
			if err != nil {
				t.Fatalf("ReconstructFactoryWorldState: %v", err)
			}

			projection := BuildSimpleDashboardProjection(state)
			assertNonTerminalProjection(t, projection, tt.outcome, tt.wantCurrentNode, tt.wantPlaceID)
		})
	}
}

func reconstructTerminalDashboardState(t0 time.Time, outcome string) (interfaces.FactoryWorldState, error) {
	return ReconstructFactoryWorldState([]factoryapi.FactoryEvent{
		initialStructureEvent(t0),
		workInputEvent(1, t0.Add(time.Second), dashboardProjectionWorkItem()),
		workstationRequestEvent(2, t0.Add(2*time.Second), dashboardProjectionRequestPayload()),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: outcome},
			DurationMillis: 1500,
			OutputWork:     []interfaces.FactoryWorkItem{dashboardProjectionOutputWorkItem()},
		}),
	}, 3)
}

func reconstructNonTerminalDashboardState(t0 time.Time, outcome string) (interfaces.FactoryWorldState, error) {
	return ReconstructFactoryWorldState([]factoryapi.FactoryEvent{
		initialStructureEventWithNonSuccessRouteArrays(t0),
		workInputEvent(1, t0.Add(time.Second), dashboardProjectionWorkItem()),
		workstationRequestEvent(2, t0.Add(2*time.Second), dashboardProjectionRequestPayload()),
		workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
			DispatchID:     "dispatch-1",
			TransitionID:   "t-review",
			Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
			Result:         interfaces.WorkstationResult{Outcome: outcome},
			DurationMillis: 1500,
			OutputWork:     []interfaces.FactoryWorkItem{dashboardProjectionOutputWorkItem()},
		}),
	}, 3)
}

func dashboardProjectionWorkItem() interfaces.FactoryWorkItem {
	return interfaces.FactoryWorkItem{
		ID:          "work-1",
		WorkTypeID:  "task",
		DisplayName: "Write docs",
		TraceID:     "trace-1",
		PlaceID:     "task:init",
	}
}

func dashboardProjectionOutputWorkItem() interfaces.FactoryWorkItem {
	item := dashboardProjectionWorkItem()
	item.PlaceID = ""
	return item
}

// pkgmaintcheck:ignore-cyclomatic-complexity this helper intentionally validates the full terminal projection contract in one place.
func assertTerminalProjection(
	t *testing.T,
	state interfaces.FactoryWorldState,
	projection SimpleDashboardProjection,
	wantCompletedCount, wantFailedCount int,
	wantTerminalPlace, wantTerminalCategory string,
	wantFailureDetailCount int,
	outcome string,
) {
	t.Helper()

	if projection.Runtime.InFlightDispatchCount != 0 {
		t.Fatalf("InFlightDispatchCount = %d, want 0", projection.Runtime.InFlightDispatchCount)
	}
	if projection.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("session.DispatchedCount = %d, want 1", projection.Runtime.Session.DispatchedCount)
	}
	if projection.Runtime.Session.CompletedCount != wantCompletedCount {
		t.Fatalf("session.CompletedCount = %d, want %d", projection.Runtime.Session.CompletedCount, wantCompletedCount)
	}
	if projection.Runtime.Session.FailedCount != wantFailedCount {
		t.Fatalf("session.FailedCount = %d, want %d", projection.Runtime.Session.FailedCount, wantFailedCount)
	}
	if len(projection.Runtime.Session.DispatchHistory) != 1 {
		t.Fatalf("session.DispatchHistory = %#v, want one completion", projection.Runtime.Session.DispatchHistory)
	}
	if projection.Runtime.Session.DispatchHistory[0].Result.Outcome != outcome {
		t.Fatalf("dispatch history outcome = %q, want %q", projection.Runtime.Session.DispatchHistory[0].Result.Outcome, outcome)
	}
	if got := projection.Runtime.PlaceOccupancyWorkItemsByPlaceID[wantTerminalPlace]; len(got) != 1 || got[0].WorkID != "work-1" {
		t.Fatalf("place occupancy work items at %s = %#v, want work-1", wantTerminalPlace, got)
	}
	if got := projection.Runtime.CurrentWorkItemsByPlaceID[wantTerminalPlace]; len(got) != 0 {
		t.Fatalf("current work items at %s = %#v, want empty because terminal and failed places are excluded", wantTerminalPlace, got)
	}
	if got := projection.Runtime.PlaceTokenCounts[wantTerminalPlace]; got != 1 {
		t.Fatalf("place token count at %s = %d, want 1", wantTerminalPlace, got)
	}
	if wantTerminalCategory == "TERMINAL" {
		terminal, ok := state.TerminalWorkByID["work-1"]
		if !ok {
			t.Fatalf("TerminalWorkByID = %#v, want work-1", state.TerminalWorkByID)
		}
		if terminal.Status != wantTerminalCategory {
			t.Fatalf("terminal status = %q, want %q", terminal.Status, wantTerminalCategory)
		}
		return
	}

	failed, ok := state.FailedWorkItemsByID["work-1"]
	if !ok {
		t.Fatalf("FailedWorkItemsByID = %#v, want work-1", state.FailedWorkItemsByID)
	}
	if failed.PlaceID != wantTerminalPlace {
		t.Fatalf("failed work place = %q, want %q", failed.PlaceID, wantTerminalPlace)
	}
	if len(state.FailureDetailsByWorkID) != wantFailureDetailCount {
		t.Fatalf("FailureDetailsByWorkID = %#v, want %d detail(s)", state.FailureDetailsByWorkID, wantFailureDetailCount)
	}
}

func dashboardProjectionRequestPayload() interfaces.WorkstationRequestPayload {
	workItem := dashboardProjectionWorkItem()
	return interfaces.WorkstationRequestPayload{
		DispatchID:   "dispatch-1",
		TransitionID: "t-review",
		Workstation:  interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
		Inputs: []interfaces.WorkstationInput{{
			TokenID:  "work-1",
			PlaceID:  "task:init",
			WorkItem: &workItem,
		}},
	}
}

func assertNonTerminalProjection(t *testing.T, projection SimpleDashboardProjection, outcome, currentNodeID, placeID string) {
	t.Helper()

	if projection.Runtime.Session.DispatchedCount != 1 {
		t.Fatalf("session.DispatchedCount = %d, want 1", projection.Runtime.Session.DispatchedCount)
	}
	if projection.Runtime.Session.CompletedCount != 0 {
		t.Fatalf("session.CompletedCount = %d, want 0 for non-terminal outcome", projection.Runtime.Session.CompletedCount)
	}
	if projection.Runtime.Session.FailedCount != 0 {
		t.Fatalf("session.FailedCount = %d, want 0 for non-terminal outcome", projection.Runtime.Session.FailedCount)
	}
	if got := projection.Runtime.CurrentWorkItemsByPlaceID[currentNodeID]; len(got) != 1 || got[0].WorkID != "work-1" {
		t.Fatalf("current work items at %s = %#v, want work-1", currentNodeID, got)
	}
	if got := projection.Runtime.PlaceOccupancyWorkItemsByPlaceID[placeID]; len(got) != 1 || got[0].WorkID != "work-1" {
		t.Fatalf("place occupancy work items at %s = %#v, want work-1", placeID, got)
	}
	if got := projection.Runtime.PlaceTokenCounts[placeID]; got != 1 {
		t.Fatalf("place token count at %s = %d, want 1", placeID, got)
	}
	if len(projection.Runtime.Session.DispatchHistory) != 1 || projection.Runtime.Session.DispatchHistory[0].Result.Outcome != outcome {
		t.Fatalf("dispatch history = %#v, want one %s completion", projection.Runtime.Session.DispatchHistory, outcome)
	}

	node := projection.WorkstationNodesByID["t-review"]
	if node.WorkstationName != "Review" {
		t.Fatalf("workstation node = %#v, want Review metadata", node)
	}
	if len(node.OutputPlaces) != 6 {
		t.Fatalf("output places = %#v, want deduped success plus continue, rejection, and failure routes", node.OutputPlaces)
	}
}
