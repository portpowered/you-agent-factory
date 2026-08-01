package projections_test

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"testing"

	factoryeventprojection "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryeventprojection"

	"github.com/portpowered/infinite-you/internal/testpath"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordingprojections "github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	. "github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections/dashboard"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestSimpleDashboardRenderDataFromWorldState_CountsFailedWorkItemsForCustomerSummary(t *testing.T) {
	failedDispatch := interfaces.FactoryWorldDispatchCompletion{
		DispatchID:   "dispatch-1",
		TransitionID: "review",
		WorkItemIDs:  []string{"work-1", "work-2", "work-3"},
		Workstation:  interfaces.FactoryWorkstationRef{Name: "Review"},
		Result:       interfaces.WorkstationResult{Outcome: string(workerexecution.OutcomeFailed)},
	}
	worldState := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]work.FactoryWorkItem{
			"work-1": {ID: "work-1", WorkTypeID: "story", DisplayName: "Blocked Story"},
			"work-2": {ID: "work-2", WorkTypeID: "story", DisplayName: "Rejected Story"},
			"work-3": {ID: "work-3", WorkTypeID: "story", DisplayName: "Reworked Story"},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{failedDispatch},
		FailedWorkItemsByID: map[string]work.FactoryWorkItem{
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

	worldState, err := factoryeventprojection.ReconstructFactoryWorldState(recordingprojections.ReconstructCanonicalFactoryWorldState, events, 4)
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
	if len(renderData.Session.DispatchHistory) != 1 || renderData.Session.DispatchHistory[0].Result.Outcome != string(workerexecution.OutcomeFailed) {
		t.Fatalf("DispatchHistory = %#v, want retained failed dispatch", renderData.Session.DispatchHistory)
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
