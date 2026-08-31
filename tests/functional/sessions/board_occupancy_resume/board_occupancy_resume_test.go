package board_occupancy_resume_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	boardOccupancyFixture = "tests/functional/sessions/board_occupancy_resume/testdata/stale-work-move.replay.json"
	currentTerminalWorkID = "idea-work-current-terminal-after-move"
	unaffectedWorkID      = "idea-work-unaffected"
)

func TestResumeUsesCurrentWorkOccupancyOverStaleMoveHistory(t *testing.T) {
	fixturePath := testutil.MustRepoPath(t, boardOccupancyFixture)
	fixture := testutil.LoadReplayArtifact(t, fixturePath)
	assertBoardOccupancyFixture(t, fixture)

	factoryDir := support.ScaffoldFactory(t, boardOccupancyFactoryConfig())
	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	successorPath := filepath.Join(t.TempDir(), "stale-work-move-successor.replay.json")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--resume", fixturePath, "--record", successorPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	support.WaitForStatus(t, server.URL(), 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial == 1 &&
			status.Categories.Processing == 0 &&
			status.Categories.Terminal == 1 &&
			status.Categories.Failed == 0
	})

	listed := listWorkThroughPublicCLI(t, server, factoryDir)
	assertBoardOccupancyWorkList(t, listed)
	if got := runner.CallCount(); got != 0 {
		t.Fatalf("resume provider calls = %d, want no live dispatch for completed recording", got)
	}
}

func listWorkThroughPublicCLI(t *testing.T, server *support.FunctionalAPIServer, factoryDir string) factoryapi.ListWorkResponse {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--server", strings.TrimSuffix(server.URL(), "/"), "--json", "work", "list",
	})
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = factoryDir
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("you work list: %v; stderr=%q", err, inputs.Stderr())
	}
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &listed); err != nil {
		t.Fatalf("decode you work list JSON %q: %v", inputs.Stdout(), err)
	}
	return listed
}

func assertBoardOccupancyFixture(t *testing.T, fixture *interfaces.ReplayArtifact) {
	t.Helper()
	if fixture.SchemaVersion != "agent-factory.replay.v1" || len(fixture.Events) != 8 {
		t.Fatalf("fixture schema/events = %q/%d, want replay v1 with eight events", fixture.SchemaVersion, len(fixture.Events))
	}
	seenMove, seenAccepted := false, false
	for _, event := range fixture.Events {
		switch event.Type {
		case interfaces.FactoryEventTypeWorkStateChange:
			var payload interfaces.WorkStateChangeEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode fixture move %q: %v", event.Id, err)
			}
			seenMove = payload.WorkID == currentTerminalWorkID && payload.ToPlaceID == "idea:to-complete"
		case interfaces.FactoryEventTypeDispatchResponse:
			var payload workerexecution.DispatchResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode fixture response %q: %v", event.Id, err)
			}
			seenAccepted = payload.Outcome == "ACCEPTED" && payload.OutputWork != nil &&
				len(*payload.OutputWork) == 1 && (*payload.OutputWork)[0].WorkID == currentTerminalWorkID &&
				(*payload.OutputWork)[0].State.Name == "complete"
		}
	}
	if !seenMove || !seenAccepted {
		t.Fatalf("fixture stale move/accepted terminal response = %t/%t", seenMove, seenAccepted)
	}
}

func assertBoardOccupancyWorkList(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if len(listed.Results) != 2 {
		t.Fatalf("public Work list length = %d, want two Work items: %#v", len(listed.Results), listed.Results)
	}
	locations := make(map[string]factoryapi.WorkState, len(listed.Results))
	for _, item := range listed.Results {
		if item.WorkId == nil || item.WorkTypeName == nil || item.State == nil {
			t.Fatalf("public Work item missing identity/type/state: %#v", item)
		}
		if _, duplicate := locations[*item.WorkId]; duplicate {
			t.Fatalf("public Work list contains duplicate Work %q: %#v", *item.WorkId, listed.Results)
		}
		if *item.WorkTypeName != "idea" {
			t.Fatalf("public Work type = %q, want idea", *item.WorkTypeName)
		}
		locations[*item.WorkId] = *item.State
	}
	if locations[currentTerminalWorkID].Name != "complete" || locations[currentTerminalWorkID].Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("current Work location = %#v, want idea:complete/TERMINAL", locations[currentTerminalWorkID])
	}
	if locations[unaffectedWorkID].Name != "init" || locations[unaffectedWorkID].Type != factoryapi.WorkStateTypeINITIAL {
		t.Fatalf("unaffected Work location = %#v, want idea:init/INITIAL", locations[unaffectedWorkID])
	}
	if len(locations) != 2 {
		t.Fatalf("public Work IDs = %#v, want exactly current and unaffected Work", locations)
	}
}

func boardOccupancyFactoryConfig() map[string]any {
	return map[string]any{
		"name": "board-occupancy-resume",
		"workTypes": []map[string]any{{
			"name": "idea",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "to-complete", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process-idea",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "idea", "state": "to-complete"}},
			"outputs":   []map[string]string{{"workType": "idea", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "idea", "state": "failed"}},
		}},
	}
}
