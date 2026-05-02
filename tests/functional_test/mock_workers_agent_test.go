package functional_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "executor_success"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock accept payload"))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithMockWorkersConfig(factoryconfig.NewEmptyMockWorkersConfig()),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("DispatchHistory count = %d, want 1", len(snapshot.DispatchHistory))
	}
	if snapshot.DispatchHistory[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, interfaces.OutcomeAccepted)
	}
}

func TestMockWorkers_AgentRejectConfigRoutesFailureAndLogsCommandOutput(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "rejection_with_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock reject payload"))
	logDir := t.TempDir()
	exitCode := 7

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRuntimeLogDir(logDir),
		testutil.WithRuntimeInstanceID("mock-reject"),
		testutil.WithMockWorkersConfig(&factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName:      "worker",
				WorkstationName: "process",
				RunType:         factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
					Stdout:   "configured stdout",
					Stderr:   "configured stderr",
					ExitCode: &exitCode,
				},
			}},
		}),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("DispatchHistory count = %d, want 1", len(snapshot.DispatchHistory))
	}
	if snapshot.DispatchHistory[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, interfaces.OutcomeFailed)
	}
	if !strings.Contains(snapshot.DispatchHistory[0].Reason, "code 7") {
		t.Fatalf("dispatch reason = %q, want exit code detail", snapshot.DispatchHistory[0].Reason)
	}

	record := findRuntimeLogRecord(t, filepath.Join(logDir, "mock-reject.log"), workers.WorkLogEventCommandRunnerCompleted)
	if record["exit_code"] != float64(7) {
		t.Fatalf("logged exit_code = %#v, want 7", record["exit_code"])
	}
	if record["stdout"] != "configured stdout" || record["stderr"] != "configured stderr" {
		t.Fatalf("logged stdout/stderr = %#v/%#v, want configured output", record["stdout"], record["stderr"])
	}
}

func TestMockWorkers_AgentRejectConfigWithZeroExitCodeStillRoutesFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "rejection_with_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock reject zero exit payload"))
	exitCode := 0

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithMockWorkersConfig(&factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName:      "worker",
				WorkstationName: "process",
				RunType:         factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
					Stdout:   "configured stdout",
					Stderr:   "configured stderr",
					ExitCode: &exitCode,
				},
			}},
		}),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snapshot.DispatchHistory) != 1 {
		t.Fatalf("DispatchHistory count = %d, want 1", len(snapshot.DispatchHistory))
	}
	if snapshot.DispatchHistory[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, interfaces.OutcomeFailed)
	}
	if !strings.Contains(snapshot.DispatchHistory[0].Reason, "code 1") {
		t.Fatalf("dispatch reason = %q, want defensive non-zero exit code detail", snapshot.DispatchHistory[0].Reason)
	}
}

func findRuntimeLogRecord(t *testing.T, path, eventName string) map[string]any {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open runtime log %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode runtime log record: %v", err)
		}
		if record["event_name"] == eventName {
			return record
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan runtime log %s: %v", path, err)
	}
	t.Fatalf("runtime log %s did not contain event_name %q", path, eventName)
	return nil
}
