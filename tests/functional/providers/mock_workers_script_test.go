package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestMockWorkers_ScriptDefaultAcceptProducesSuccessfulScriptResult(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow mock-worker script accept sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock script accept payload"))

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithMockWorkersConfig(factoryconfig.NewEmptyMockWorkersConfig()),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	assertTokenPayload(t, h.Marking(), "task:done", "mock worker accepted")
}

func TestMockWorkers_ScriptRejectConfigRoutesFailureAndLogsCommandOutput(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow mock-worker script reject sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock script reject payload"))
	logDir := t.TempDir()
	exitCode := 9

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRuntimeLogDir(logDir),
		testutil.WithRuntimeInstanceID("mock-script-reject"),
		testutil.WithMockWorkersConfig(&factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName:      "script-worker",
				WorkstationName: "run-script",
				RunType:         factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
					Stdout:   "script configured stdout",
					Stderr:   "script configured stderr",
					ExitCode: &exitCode,
				},
			}},
		}),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")

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
	if !strings.Contains(snapshot.DispatchHistory[0].Reason, "script configured stderr") {
		t.Fatalf("dispatch reason = %q, want configured stderr detail", snapshot.DispatchHistory[0].Reason)
	}

	record := findRuntimeLogRecord(t, filepath.Join(logDir, "mock-script-reject.log"), workers.WorkLogEventCommandRunnerCompleted)
	if record["exit_code"] != float64(9) {
		t.Fatalf("logged exit_code = %#v, want 9", record["exit_code"])
	}
	if record["stdout"] != "script configured stdout" || record["stderr"] != "script configured stderr" {
		t.Fatalf("logged stdout/stderr = %#v/%#v, want configured output", record["stdout"], record["stderr"])
	}
}

func TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow mock-worker zero-exit rejection sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock script reject zero exit payload"))
	exitCode := 0

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithMockWorkersConfig(&factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName:      "script-worker",
				WorkstationName: "run-script",
				RunType:         factoryconfig.MockWorkerRunTypeReject,
				RejectConfig: &factoryconfig.MockWorkerRejectConfig{
					Stdout:   "script configured stdout",
					Stderr:   "script configured stderr",
					ExitCode: &exitCode,
				},
			}},
		}),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")

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
	if !strings.Contains(snapshot.DispatchHistory[0].Reason, "script configured stderr") {
		t.Fatalf("dispatch reason = %q, want configured stderr detail", snapshot.DispatchHistory[0].Reason)
	}
}

func TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow mock-worker command-runner side-effect sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock script command payload"))
	sideEffectPath := filepath.Join(t.TempDir(), "mock-script-side-effect.txt")

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithMockWorkersConfig(&factoryconfig.MockWorkersConfig{
			MockWorkers: []factoryconfig.MockWorkerConfig{{
				WorkerName:      "script-worker",
				WorkstationName: "run-script",
				RunType:         factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
					Command: os.Args[0],
					Args: []string{
						"-test.run=TestMockWorkers_ScriptHelper",
						"--",
						"write-file",
						sideEffectPath,
						"script side effect",
					},
				},
			}},
		}),
	)
	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	raw, err := os.ReadFile(sideEffectPath)
	if err != nil {
		t.Fatalf("read mock script side effect: %v", err)
	}
	if string(raw) != "script side effect" {
		t.Fatalf("side effect content = %q, want %q", raw, "script side effect")
	}
	assertTokenPayload(t, h.Marking(), "task:done", "mock script helper wrote file")
}

func TestMockWorkers_ScriptHelper(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow mock-worker helper sweep")
	if len(os.Args) < 4 {
		return
	}

	mode := os.Args[len(os.Args)-3]
	path := os.Args[len(os.Args)-2]
	content := os.Args[len(os.Args)-1]
	if mode != "write-file" {
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write side effect: %v\n", err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "mock script helper wrote file")
	os.Exit(0)
}
