package root_composition_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	automationRecoveryFixture = "tests/functional/sessions/root_composition/testdata/automation-work-missing-occupancy.replay.json"
	automationRecoveryWorkID  = "work-recovered-after-automation"
	automationRecoveryTimeID  = "automation-time-daily-refresh-20260820"
)

// TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection
// routes a checked-in recording through the assembled replay and resume
// processes. The recording contains a durable cron time Work whose accepted
// dispatch has no current place occupancy, plus another Work whose completed
// dispatch output is intermediate and is superseded by a later canonical state
// change.
func TestAutomationWorkWithoutRecordedOccupancyRestoresThroughRecordingProjection(t *testing.T) {
	fixturePath := testutil.MustRepoPath(t, automationRecoveryFixture)
	fixture := testutil.LoadReplayArtifact(t, fixturePath)
	assertAutomationRecoveryFixture(t, fixture)

	factoryDir := support.ScaffoldFactory(t, automationRecoveryFactoryConfig())
	replay := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--replay", fixturePath, "--no-record"},
	})
	replayed := waitForRecoveredAutomationWork(t, replay.URL())
	assertRecoveredAutomationWorkList(t, replayed)
	replay.Stop(t)

	successorPath := filepath.Join(t.TempDir(), "automation-work-missing-occupancy-successor.replay.json")
	resumed := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Args:                      []string{"--resume", fixturePath, "--record", successorPath},
	})
	resumedWork := waitForRecoveredAutomationWork(t, resumed.URL())
	assertRecoveredAutomationWorkList(t, resumedWork)
	resumed.Stop(t)

	successor := testutil.LoadReplayArtifact(t, successorPath)
	dailyRefreshDispatches := 0
	for _, event := range successor.Events {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest {
			continue
		}
		var payload interfaces.DispatchRequestEventPayload
		if err := event.DecodePayload(&payload); err != nil {
			t.Fatalf("decode successor dispatch request %q: %v", event.Id, err)
		}
		if payload.TransitionID == "daily-refresh" {
			dailyRefreshDispatches++
		}
	}
	if dailyRefreshDispatches != 1 {
		t.Fatalf("successor daily-refresh dispatch count = %d, want the one recorded completion without a redispatch", dailyRefreshDispatches)
	}
}

func waitForRecoveredAutomationWork(t *testing.T, baseURL string) factoryapi.ListWorkResponse {
	t.Helper()
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.Categories.Initial == 0 && status.Categories.Processing == 0 && status.Categories.Terminal == 1
	})
	return support.ListDefaultSessionWork(t, baseURL)
}

func assertAutomationRecoveryFixture(t *testing.T, fixture *interfaces.ReplayArtifact) {
	t.Helper()
	if fixture.SchemaVersion != "agent-factory.replay.v1" {
		t.Fatalf("fixture schema version = %q, want agent-factory.replay.v1", fixture.SchemaVersion)
	}
	if len(fixture.Events) == 0 {
		t.Fatal("fixture has no events")
	}
	seenAutomationWork := false
	seenCompletion := false
	for _, event := range fixture.Events {
		if event.Type == interfaces.FactoryEventTypeWorkRequest {
			var payload work.WorkRequestEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode fixture Work request %q: %v", event.Id, err)
			}
			for _, item := range payload.Works {
				if item.WorkID != automationRecoveryTimeID {
					continue
				}
				seenAutomationWork = item.Tags != nil && item.Tags["agent_factory.source"] == "cron" && item.Tags["agent_factory.cron.workstation"] == "daily-refresh"
			}
		}
		if event.Type == interfaces.FactoryEventTypeDispatchResponse {
			var payload workerexecution.DispatchResponseEventPayload
			if err := event.DecodePayload(&payload); err != nil {
				t.Fatalf("decode fixture dispatch response %q: %v", event.Id, err)
			}
			if event.Context.DispatchID != nil && *event.Context.DispatchID == "dispatch-daily-refresh" && payload.Outcome == "ACCEPTED" {
				seenCompletion = true
			}
		}
	}
	if !seenAutomationWork {
		t.Fatalf("fixture Work %q has no durable cron provenance", automationRecoveryTimeID)
	}
	if !seenCompletion {
		t.Fatal("fixture has no accepted daily-refresh completion")
	}
}

func assertRecoveredAutomationWorkList(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()
	if len(listed.Results) != 1 {
		t.Fatalf("public Work list length = %d, want only the recoverable Work: %#v", len(listed.Results), listed.Results)
	}
	if !support.HasWorkAtCustomerState(listed, automationRecoveryWorkID, "task:complete") {
		locations := make([]string, 0, len(listed.Results))
		for _, item := range listed.Results {
			if item.WorkTypeName != nil && item.State != nil {
				locations = append(locations, support.WorkItemCustomerLocation(item))
			}
		}
		t.Fatalf("public Work list does not contain %q at task:complete: locations=%v listed=%#v", automationRecoveryWorkID, locations, listed.Results)
	}
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == automationRecoveryTimeID {
			t.Fatalf("completed automation Work leaked into public Work list: %#v", item)
		}
	}
}

func automationRecoveryFactoryConfig() map[string]any {
	return map[string]any{
		"name": "automation-occupancy-recovery",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "to-complete", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":      "process-work",
				"worker":    "cron-worker",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "to-complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
			{
				"name":     "daily-refresh",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       "0 0 1 1 *",
					"triggerAtStart": false,
				},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}
