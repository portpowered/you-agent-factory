package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
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
	if snapshot.DispatchHistory[0].Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, workerexecution.OutcomeAccepted)
	}
}

func TestMockWorkers_AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_with_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock reject payload"))
	logDir := t.TempDir()
	exitCode := 7

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRuntimeFileLoggingEnabled(true),
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
	if snapshot.DispatchHistory[0].Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, workerexecution.OutcomeFailed)
	}
	if snapshot.DispatchHistory[0].FailureMetadata == nil || snapshot.DispatchHistory[0].FailureMetadata.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("FailureMetadata.Type = %#v, want %q", snapshot.DispatchHistory[0].FailureMetadata, workerexecution.WorkFailureTypeUnknown)
	}
	if !strings.Contains(snapshot.DispatchHistory[0].Reason, "provider error: unknown: Codex reported a terminal error.") {
		t.Fatalf("dispatch reason = %q, want stable unknown code with audited message", snapshot.DispatchHistory[0].Reason)
	}

	record := findRuntimeLogRecord(t, requireRuntimeLogPath(t, logDir, "mock-reject"), workers.WorkLogEventCommandRunnerCompleted)
	if record["exit_code"] != float64(7) {
		t.Fatalf("logged exit_code = %#v, want 7", record["exit_code"])
	}
	if _, ok := record["stdout"]; ok {
		t.Fatalf("failure record unexpectedly included stdout: %#v", record)
	}
	if _, ok := record["stderr"]; ok {
		t.Fatalf("failure record unexpectedly included stderr: %#v", record)
	}
}

func TestMockWorkers_AgentRejectConfigWithZeroExitCodeStillRoutesFailure(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_with_arcs"))
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
	if snapshot.DispatchHistory[0].Outcome != workerexecution.OutcomeFailed {
		t.Fatalf("dispatch outcome = %s, want %s", snapshot.DispatchHistory[0].Outcome, workerexecution.OutcomeFailed)
	}
	if snapshot.DispatchHistory[0].FailureMetadata == nil || snapshot.DispatchHistory[0].FailureMetadata.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("FailureMetadata.Type = %#v, want %q", snapshot.DispatchHistory[0].FailureMetadata, workerexecution.WorkFailureTypeUnknown)
	}
	if !strings.Contains(snapshot.DispatchHistory[0].Reason, "provider error: unknown: Codex reported a terminal error.") {
		t.Fatalf("dispatch reason = %q, want stable unknown code with audited message", snapshot.DispatchHistory[0].Reason)
	}
}
