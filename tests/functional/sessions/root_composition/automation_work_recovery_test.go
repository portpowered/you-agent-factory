package root_composition_test

import (
	"encoding/json"
	"os"
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
	acquireRootCompositionFixtureSlot(t)

	fixturePath := testutil.MustRepoPath(t, automationRecoveryFixture)
	replayFixture := testutil.LoadReplayArtifact(t, fixturePath)
	assertAutomationRecoveryFixture(t, replayFixture)

	factoryDir := support.ScaffoldFactory(t, automationRecoveryFactoryConfig())
	reusable := newAutomationRecoveryProcess(t)
	successorPath := filepath.Join(t.TempDir(), "automation-work-missing-occupancy-successor.replay.json")
	fixture := ensureRootCompositionFixture(t)
	fixture.withRootCompositionRoute(t, rootCompositionRouteSpec{
		label:      "automation-work-recovery",
		homeDir:    t.TempDir(),
		workingDir: factoryDir,
		extraPaths: []string{fixturePath, filepath.Dir(successorPath), filepath.Dir(reusable.mockWorkersPath)},
	}, func() {
		replay := reusable.run(
			t,
			factoryDir,
			support.NewProcessAPIServer(),
			"--replay", fixturePath, "--no-record",
		)
		replayed := waitForRecoveredAutomationWork(t, replay.url)
		assertRecoveredAutomationWorkList(t, replayed)
		replay.daemon.Stop(t)

		resumed := reusable.run(
			t,
			factoryDir,
			support.NewProcessAPIServer(),
			"--resume", fixturePath, "--record", successorPath,
		)
		resumedWork := waitForRecoveredAutomationWork(t, resumed.url)
		assertRecoveredAutomationWorkList(t, resumedWork)
		resumed.daemon.Stop(t)

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
	})
}

type automationRecoveryProcess struct {
	mockWorkersPath string
	fixture         *rootCompositionFixture
}

type automationRecoveryRun struct {
	url    string
	daemon *rootCompositionServer
}

func newAutomationRecoveryProcess(t *testing.T) *automationRecoveryProcess {
	t.Helper()
	fixture := ensureRootCompositionFixture(t)
	mockWorkersPath := filepath.Join(t.TempDir(), "mock-workers.json")
	payload, err := json.Marshal(workerexecution.NewEmptyMockWorkersConfig())
	if err != nil {
		t.Fatalf("marshal empty mock-worker configuration: %v", err)
	}
	if err := os.WriteFile(mockWorkersPath, payload, 0o600); err != nil {
		t.Fatalf("write empty mock-worker configuration: %v", err)
	}
	return &automationRecoveryProcess{
		fixture:         fixture,
		mockWorkersPath: mockWorkersPath,
	}
}

func (reusable *automationRecoveryProcess) run(
	t *testing.T,
	factoryDir string,
	api *support.ProcessAPIServer,
	extraArgs ...string,
) automationRecoveryRun {
	t.Helper()
	args := append([]string{
		"you", "run",
		"--continuously", "--with-server", "--quiet", "--dir", factoryDir,
		"--with-mock-workers", reusable.mockWorkersPath,
	}, extraArgs...)
	daemon := startRootCompositionServer(t, reusable.fixture, api, args, nil, factoryDir)
	baseURL := daemon.URL(t)
	support.WaitForStatus(t, baseURL, 15*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.RuntimeStatus != ""
	})
	return automationRecoveryRun{url: baseURL, daemon: daemon}
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
