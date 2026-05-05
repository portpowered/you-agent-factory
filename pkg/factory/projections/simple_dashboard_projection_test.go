package projections

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
			result := interfaces.WorkstationResult{Outcome: tt.outcome}
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
				workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
					DispatchID:     "dispatch-1",
					TransitionID:   "t-review",
					Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
					Result:         result,
					DurationMillis: 1500,
					OutputWork: []interfaces.FactoryWorkItem{{
						ID:          "work-1",
						WorkTypeID:  "task",
						DisplayName: "Write docs",
						TraceID:     "trace-1",
					}},
				}),
			}, 3)
			if err != nil {
				t.Fatalf("ReconstructFactoryWorldState: %v", err)
			}

			projection := BuildSimpleDashboardProjection(state)

			if projection.Runtime.InFlightDispatchCount != 0 {
				t.Fatalf("InFlightDispatchCount = %d, want 0", projection.Runtime.InFlightDispatchCount)
			}
			if projection.Runtime.Session.DispatchedCount != 1 {
				t.Fatalf("session.DispatchedCount = %d, want 1", projection.Runtime.Session.DispatchedCount)
			}
			if projection.Runtime.Session.CompletedCount != tt.wantCompletedCount {
				t.Fatalf("session.CompletedCount = %d, want %d", projection.Runtime.Session.CompletedCount, tt.wantCompletedCount)
			}
			if projection.Runtime.Session.FailedCount != tt.wantFailedCount {
				t.Fatalf("session.FailedCount = %d, want %d", projection.Runtime.Session.FailedCount, tt.wantFailedCount)
			}
			if len(projection.Runtime.Session.DispatchHistory) != 1 {
				t.Fatalf("session.DispatchHistory = %#v, want one completion", projection.Runtime.Session.DispatchHistory)
			}
			if projection.Runtime.Session.DispatchHistory[0].Result.Outcome != tt.outcome {
				t.Fatalf("dispatch history outcome = %q, want %q", projection.Runtime.Session.DispatchHistory[0].Result.Outcome, tt.outcome)
			}
			if got := projection.Runtime.PlaceOccupancyWorkItemsByPlaceID[tt.wantTerminalPlace]; len(got) != 1 || got[0].WorkID != "work-1" {
				t.Fatalf("place occupancy work items at %s = %#v, want work-1", tt.wantTerminalPlace, got)
			}
			if got := projection.Runtime.CurrentWorkItemsByPlaceID[tt.wantTerminalPlace]; len(got) != 0 {
				t.Fatalf("current work items at %s = %#v, want empty because terminal and failed places are excluded", tt.wantTerminalPlace, got)
			}
			if got := projection.Runtime.PlaceTokenCounts[tt.wantTerminalPlace]; got != 1 {
				t.Fatalf("place token count at %s = %d, want 1", tt.wantTerminalPlace, got)
			}
			if tt.wantTerminalCategory == "TERMINAL" {
				terminal, ok := state.TerminalWorkByID["work-1"]
				if !ok {
					t.Fatalf("TerminalWorkByID = %#v, want work-1", state.TerminalWorkByID)
				}
				if terminal.Status != tt.wantTerminalCategory {
					t.Fatalf("terminal status = %q, want %q", terminal.Status, tt.wantTerminalCategory)
				}
			} else {
				failed, ok := state.FailedWorkItemsByID["work-1"]
				if !ok {
					t.Fatalf("FailedWorkItemsByID = %#v, want work-1", state.FailedWorkItemsByID)
				}
				if failed.PlaceID != tt.wantTerminalPlace {
					t.Fatalf("failed work place = %q, want %q", failed.PlaceID, tt.wantTerminalPlace)
				}
				if len(state.FailureDetailsByWorkID) != tt.wantFailureDetailCount {
					t.Fatalf("FailureDetailsByWorkID = %#v, want %d detail(s)", state.FailureDetailsByWorkID, tt.wantFailureDetailCount)
				}
			}
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
			state, err := ReconstructFactoryWorldState([]factoryapi.FactoryEvent{
				initialStructureEventWithNonSuccessRouteArrays(t0),
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
				workstationResponseEvent(3, t0.Add(3*time.Second), interfaces.WorkstationResponsePayload{
					DispatchID:     "dispatch-1",
					TransitionID:   "t-review",
					Workstation:    interfaces.FactoryWorkstationRef{ID: "t-review", Name: "Review"},
					Result:         interfaces.WorkstationResult{Outcome: tt.outcome},
					DurationMillis: 1500,
					OutputWork: []interfaces.FactoryWorkItem{{
						ID:          "work-1",
						WorkTypeID:  "task",
						DisplayName: "Write docs",
						TraceID:     "trace-1",
					}},
				}),
			}, 3)
			if err != nil {
				t.Fatalf("ReconstructFactoryWorldState: %v", err)
			}

			projection := BuildSimpleDashboardProjection(state)

			if projection.Runtime.Session.DispatchedCount != 1 {
				t.Fatalf("session.DispatchedCount = %d, want 1", projection.Runtime.Session.DispatchedCount)
			}
			if projection.Runtime.Session.CompletedCount != 0 {
				t.Fatalf("session.CompletedCount = %d, want 0 for non-terminal outcome", projection.Runtime.Session.CompletedCount)
			}
			if projection.Runtime.Session.FailedCount != 0 {
				t.Fatalf("session.FailedCount = %d, want 0 for non-terminal outcome", projection.Runtime.Session.FailedCount)
			}
			if got := projection.Runtime.CurrentWorkItemsByPlaceID[tt.wantCurrentNode]; len(got) != 1 || got[0].WorkID != "work-1" {
				t.Fatalf("current work items at %s = %#v, want work-1", tt.wantCurrentNode, got)
			}
			if got := projection.Runtime.PlaceOccupancyWorkItemsByPlaceID[tt.wantPlaceID]; len(got) != 1 || got[0].WorkID != "work-1" {
				t.Fatalf("place occupancy work items at %s = %#v, want work-1", tt.wantPlaceID, got)
			}
			if got := projection.Runtime.PlaceTokenCounts[tt.wantPlaceID]; got != 1 {
				t.Fatalf("place token count at %s = %d, want 1", tt.wantPlaceID, got)
			}
			if len(projection.Runtime.Session.DispatchHistory) != 1 || projection.Runtime.Session.DispatchHistory[0].Result.Outcome != tt.outcome {
				t.Fatalf("dispatch history = %#v, want one %s completion", projection.Runtime.Session.DispatchHistory, tt.outcome)
			}

			node := projection.WorkstationNodesByID["t-review"]
			if node.WorkstationName != "Review" {
				t.Fatalf("workstation node = %#v, want Review metadata", node)
			}
			if len(node.OutputPlaces) != 6 {
				t.Fatalf("output places = %#v, want deduped success plus continue, rejection, and failure routes", node.OutputPlaces)
			}
		})
	}
}
