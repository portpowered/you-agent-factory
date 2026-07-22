//go:build functionallong

package providers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	server, session := runScriptFactory(t, dir, runner, 5*time.Second)
	assertSessionPlaces(t, session, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})

	if runner.CallCount() < 2 {
		t.Fatalf("expected script runner to be called at least twice, got %d", runner.CallCount())
	}

	assertDispatchOutcomeSequence(t, server.GetFactoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
	server.Stop(t)
}
