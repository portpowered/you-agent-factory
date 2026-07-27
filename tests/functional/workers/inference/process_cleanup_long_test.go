//go:build functionallong

package inference_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProviderCancellationTerminatesCompanionProcesses proves a timed-out script-worker
// invocation that spawned companion command processes terminates those companions
// before the public timeout failure settles, then requeues and completes so no
// companion process remains running after cancellation cleanup.
func TestProviderCancellationTerminatesCompanionProcesses(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	attemptFile := filepath.Join(t.TempDir(), "companion-attempts.txt")
	companionPIDFile := attemptFile + ".companion.pid"
	traceID := "trace-script-timeout-companion-001"
	workID := "work-script-timeout-companion-001"

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestProcessTreeHelper'
  - '--'
  - 'companion-timeout-once'
  - %s
timeout: 1500ms
---
Spawn a companion process on the first attempt and succeed after timeout requeue.
`, yamlSingleQuoted(os.Args[0]), yamlSingleQuoted(attemptFile))
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     workID,
		WorkTypeID: "task",
		TraceID:    traceID,
		Payload:    []byte("timeout companion payload"),
	})

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		processCleanupScriptEdges(t),
		20*time.Second,
	)

	if _, err := os.Stat(companionPIDFile); err != nil {
		t.Fatalf("companion pid file missing: %v", err)
	}
	companionPID := readProcessCleanupPID(t, companionPIDFile)
	t.Cleanup(func() {
		processCleanupTerminateProcess(companionPID)
	})
	if processCleanupProcessRunning(companionPID) {
		t.Fatalf("companion process %d is still running after timeout cancellation", companionPID)
	}

	assertProcessCleanupSessionPlaces(t, listed, map[string]int{
		"task:done":   1,
		"task:init":   0,
		"task:failed": 0,
	})
	assertProcessCleanupListedWorkIdentity(t, listed, "done", workID, "task", traceID, nil)
	assertProcessCleanupDispatchOutcomeSequence(t, events, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
	if session.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one terminal work item after companion timeout requeue",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf(
			"session processing count = %d, want 0 after companion cancellation cleanup",
			session.Runtime.Progress.Categories.Processing,
		)
	}
}
