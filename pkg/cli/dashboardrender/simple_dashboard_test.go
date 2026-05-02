package dashboardrender

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testpath"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
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

func TestSimpleDashboardRenderDataFromWorldState_ReplaysWeirdNumberSummaryFixture(t *testing.T) {
	events := loadReplayFixtureEvents(t, "ui", "integration", "fixtures", "weird-number-summary-replay.jsonl")

	worldState, err := projections.ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
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
	if len(renderData.Session.DispatchHistory) != 1 ||
		renderData.Session.DispatchHistory[0].Result.FailureReason != "throttled" {
		t.Fatalf("DispatchHistory = %#v, want retained failed dispatch details", renderData.Session.DispatchHistory)
	}
}

func loadReplayFixtureEvents(t *testing.T, rel ...string) []factoryapi.FactoryEvent {
	t.Helper()

	path := testpath.MustRepoPathFromCaller(t, 0, rel...)
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open replay fixture %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	events := make([]factoryapi.FactoryEvent, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event factoryapi.FactoryEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse replay fixture line %q: %v", line, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan replay fixture %s: %v", path, err)
	}
	return events
}
