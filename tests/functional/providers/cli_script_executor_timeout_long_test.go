//go:build functionallong

package providers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestScriptExecutor_RuntimeWorkerTimeoutFromLoadedConfigRequeuesAndRetriesOnLaterTick(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	agentsMD := "---\ntype: SCRIPT_WORKER\ncommand: echo\ntimeout: 10ms\n---\nExecute the script.\n"
	if err := os.WriteFile(workerAgentsPath, []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedFile(t, dir, "task", []byte("input-payload"))

	runner := newTimeoutThenSuccessCommandRunner()
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	h.RunUntilComplete(t, 5*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	if runner.CallCount() < 2 {
		t.Fatalf("expected script runner to be called at least twice, got %d", runner.CallCount())
	}

	engineState, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if len(engineState.DispatchHistory) < 2 {
		t.Fatalf("DispatchHistory length = %d, want at least 2", len(engineState.DispatchHistory))
	}
	if engineState.DispatchHistory[0].Reason != "execution timeout" {
		t.Fatalf("first DispatchHistory reason = %q, want %q", engineState.DispatchHistory[0].Reason, "execution timeout")
	}
}
