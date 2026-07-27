package inference_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestProviderTimeoutTerminatesChildProcessTree proves a timed-out script-worker
// invocation tears down its spawned descendant process tree and clears active
// execution so the public Work listing and Factory Event stream show a terminal
// timeout failure with no lingering in-progress dispatch.
func TestProviderTimeoutTerminatesChildProcessTree(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout process-tree cleanup proof")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	childPIDFile := filepath.Join(t.TempDir(), "descendant.pid")

	support.UpdateFactoryConfig(t, dir, func(cfg map[string]any) {
		cfg["workstations"] = append(cfg["workstations"].([]any), map[string]any{
			"name":     "timeout-cleanup-loop-breaker",
			"behavior": "STANDARD",
			"type":     "LOGICAL_MOVE",
			"inputs":   []map[string]any{{"workType": "task", "state": "init"}},
			"outputs":  []map[string]any{{"workType": "task", "state": "failed"}},
			"guards": []map[string]any{{
				"type":        "VISIT_COUNT",
				"workstation": "run-script",
				"maxVisits":   float64(1),
			}},
		})
	})

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestProcessTreeHelper'
  - '--'
  - 'spawn-child'
  - %s
timeout: 1500ms
---
Spawn a descendant and wait for the factory timeout to cancel it.
`, yamlSingleQuoted(os.Args[0]), yamlSingleQuoted(childPIDFile))
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-timeout-cleanup-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-cleanup-smoke",
		Payload:    []byte("spawn a descendant process"),
	})

	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		processCleanupScriptEdges(t),
		20*time.Second,
	)

	childPID := readProcessCleanupPID(t, childPIDFile)
	t.Cleanup(func() {
		processCleanupTerminateProcess(childPID)
	})
	if !waitForProcessCleanupExit(childPID, 3*time.Second) {
		t.Fatalf("spawned descendant process %d is still running after factory timeout", childPID)
	}

	assertProcessCleanupSessionPlaces(t, listed, map[string]int{
		"task:failed": 1,
		"task:init":   0,
		"task:done":   0,
	})
	assertProcessCleanupDispatchOutcomeSequence(t, events, []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
	}, "execution timeout")
	if session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"session progress categories = %+v, want one failed work item and cleared active execution",
			session.Runtime.Progress.Categories,
		)
	}
	if session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf(
			"session processing count = %d, want 0 after timeout cleanup",
			session.Runtime.Progress.Categories.Processing,
		)
	}
}

// TestProcessTreeHelper is invoked as the external script command for timeout
// cleanup proofs. It spawns a descendant process, records its PID, and blocks
// until the factory timeout cancels the process tree.
func TestProcessTreeHelper(t *testing.T) {
	if len(os.Args) < 2 {
		return
	}

	mode := os.Args[len(os.Args)-2]
	pidFile := os.Args[len(os.Args)-1]
	switch mode {
	case "spawn-child":
		spawnProcessCleanupChild(pidFile)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "pid-sleep":
		writeProcessCleanupPID(pidFile)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	default:
		return
	}
}

func spawnProcessCleanupChild(pidFile string) {
	child := exec.Command(os.Args[0],
		"-test.run=TestProcessTreeHelper",
		"--",
		"pid-sleep",
		pidFile,
	)
	child.Env = os.Environ()
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start child: %v\n", err)
		os.Exit(2)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "child did not write pid file")
	os.Exit(2)
}

func writeProcessCleanupPID(pidFile string) {
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		os.Exit(2)
	}
}

func readProcessCleanupPID(t *testing.T, pidFile string) int {
	t.Helper()

	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("read descendant pid file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("parse descendant pid %q: %v", raw, err)
	}
	return pid
}

func waitForProcessCleanupExit(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !processCleanupProcessRunning(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return !processCleanupProcessRunning(pid)
		case <-ticker.C:
		}
	}
}

func assertProcessCleanupSessionPlaces(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
}

func assertProcessCleanupDispatchOutcomeSequence(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wants []factoryapi.WorkOutcome,
	firstError string,
) {
	t.Helper()
	responses := processCleanupDispatchResponses(t, events)
	if len(responses) < len(wants) {
		t.Fatalf("dispatch response count = %d, want at least %d", len(responses), len(wants))
	}
	for index, want := range wants {
		if responses[index].Outcome != want {
			t.Errorf("dispatch response %d outcome = %s, want %s", index, responses[index].Outcome, want)
		}
	}
	if firstError != "" && (responses[0].Error == nil || !strings.Contains(*responses[0].Error, firstError)) {
		t.Errorf("first dispatch error = %#v, want text %q", responses[0].Error, firstError)
	}
}

func processCleanupDispatchResponses(t *testing.T, events []factoryapi.FactoryEvent) []factoryapi.DispatchResponseEventPayload {
	t.Helper()
	var responses []factoryapi.DispatchResponseEventPayload
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		responses = append(responses, payload)
	}
	return responses
}

func processCleanupScriptEdges(t *testing.T) serviceedges.Edges {
	t.Helper()
	runner, err := platformprocess.NewExecCommandRunner(exec.Command, platformclock.Real{}, nil)
	if err != nil {
		t.Fatalf("construct process cleanup script command runner: %v", err)
	}
	return serviceedges.Edges{ScriptCommandRunner: runner}
}

func yamlSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
