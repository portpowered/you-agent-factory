package dispatch

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestProjectCycleCurrentCycleSelectionPreservesHistoricalCycle(t *testing.T) {
	enterSharedPetriScenario(t)
	dir := support.ScaffoldFactory(t, projectCycleSelectionFactoryConfig())
	testutil.WriteSeedBatchFile(t, dir, work.WorkRequest{
		RequestID: "project-cycle-selection-seed",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{
				Name:       "resume-project",
				WorkID:     "project-work",
				WorkTypeID: "project",
				State:      "init",
			},
			{
				Name:       "reopen-once",
				WorkID:     "reopen-once",
				WorkTypeID: "reopen-gate",
				State:      "ready",
			},
		},
	})

	selection := openSharedPetriSessionWithRoute(t, dir, sharedPetriRouteConfig{script: sharedPetriScriptSequence(
		sharedPetriCommandResult(platformprocess.CommandResult{
			Stdout: []byte(projectCycleWorkerOutput(t, "cycle-request-44", []map[string]any{
				{
					"name":         "resume-project",
					"workId":       "cycle-44",
					"workTypeName": "project-cycle",
					"state":        "blocked",
				},
			})),
		}),
		sharedPetriCommandResult(platformprocess.CommandResult{
			Stdout: []byte(projectCycleWorkerOutput(t, "cycle-request-93", []map[string]any{
				{
					"name":         "resume-project",
					"workId":       "cycle-93",
					"workTypeName": "project-cycle",
					"state":        "blocked",
				},
				{
					"name":         "block-gate",
					"workId":       "block-gate",
					"workTypeName": "block-gate",
					"state":        "ready",
				},
			})),
		}),
	)})
	status := support.WaitForSessionTerminalStatus(t, selection.fixture.baseURL, selection.sessionID, 15*time.Second)
	listed := listSharedPetriSessionWork(t, selection.fixture.baseURL, selection.sessionID)
	project := getSharedPetriWork(t, selection.fixture.baseURL, selection.sessionID, "project-work")
	historical := getSharedPetriWork(t, selection.fixture.baseURL, selection.sessionID, "cycle-44")
	events := support.GetFactoryEventsForSessionAt(t, selection.fixture.baseURL, selection.sessionID)
	selection.close(t)
	assertSharedPetriRouteRequests(t, dir)

	if status.RuntimeStatus == "" {
		t.Fatal("terminal Factory Session status is empty")
	}
	assertProjectCycleWorkState(t, project, "project-work", "project", "blocked")
	assertProjectCycleWorkState(t, historical, "cycle-44", "project-cycle", "blocked")
	assertProjectCycleListedState(t, listed, "block-gate", "block-gate", "complete")

	dispatches := assertPublicDispatchEvents(t, events, 6)
	assertDispatchTransitionOutcomeCount(t, dispatches, "block-project", factoryapi.WorkOutcomeAccepted, 1)
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != "block-project" {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, "cycle-93") {
			t.Fatalf("block-project dispatch did not consume the current cycle: %#v", dispatch)
		}
		if support.DispatchObservationIncludesWork(dispatch, "cycle-44") {
			t.Fatalf("block-project dispatch consumed historical cycle-44: %#v", dispatch)
		}
	}
}

func getSharedPetriWork(t testing.TB, baseURL, sessionID, workID string) factoryapi.Work {
	t.Helper()
	return support.GetJSON[factoryapi.Work](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work/"+url.PathEscape(workID),
	)
}

func projectCycleWorkerOutput(t *testing.T, requestID string, works []map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"request": map[string]any{
			"requestId": requestID,
			"type":      string(work.WorkRequestTypeFactoryRequestBatch),
			"works":     works,
		},
	})
	if err != nil {
		t.Fatalf("marshal project-cycle worker output: %v", err)
	}
	return string(encoded)
}

func assertProjectCycleListedState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workID string,
	workType string,
	state string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkId == nil || *item.WorkId != workID {
			continue
		}
		if item.WorkTypeName == nil || *item.WorkTypeName != workType {
			t.Fatalf("Work %q type = %#v, want %q", workID, item.WorkTypeName, workType)
		}
		if item.State == nil || item.State.Name != state {
			t.Fatalf("Work %q state = %#v, want %q", workID, item.State, state)
		}
		return
	}
	t.Fatalf("listed Work missing %s (%s:%s)", workID, workType, state)
}

func assertProjectCycleWorkState(
	t *testing.T,
	item factoryapi.Work,
	workID string,
	workType string,
	state string,
) {
	t.Helper()
	if item.WorkId == nil || *item.WorkId != workID {
		t.Fatalf("Work response ID = %#v, want %q", item.WorkId, workID)
	}
	if item.WorkTypeName == nil || *item.WorkTypeName != workType {
		t.Fatalf("Work %q type = %#v, want %q", workID, item.WorkTypeName, workType)
	}
	if item.State == nil || item.State.Name != state {
		t.Fatalf("Work %q state = %#v, want %q", workID, item.State, state)
	}
}
