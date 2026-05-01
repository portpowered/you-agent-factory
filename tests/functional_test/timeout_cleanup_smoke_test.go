package functional_test

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

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow timeout cleanup smoke")
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	childPIDFile := filepath.Join(t.TempDir(), "descendant.pid")

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
		cfg["workstations"] = append(cfg["workstations"].([]any), map[string]any{
			"name":    "timeout-cleanup-loop-breaker",
			"type":    "LOGICAL_MOVE",
			"inputs":  []map[string]any{{"workType": "task", "state": "init"}},
			"outputs": []map[string]any{{"workType": "task", "state": "failed"}},
			"guards": []map[string]any{{
				"type":        "visit_count",
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
  - '-test.run=TestIntegrationSmoke_ProcessTreeHelper'
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

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-timeout-cleanup-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-cleanup-smoke",
		Payload:    []byte("spawn a descendant process"),
	})

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithFullWorkerPoolAndScriptWrap())
	h.RunUntilComplete(t, 10*time.Second)

	childPID := readTimeoutCleanupPID(t, childPIDFile)
	t.Cleanup(func() {
		timeoutCleanupTerminateProcess(childPID)
	})
	if !waitForTimeoutCleanupProcessExit(childPID, 3*time.Second) {
		t.Fatalf("spawned descendant process %d is still running after factory timeout", childPID)
	}

	h.Assert().
		PlaceTokenCount("task:failed", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:done")

	engineState, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	if engineState.InFlightCount != 0 {
		t.Fatalf("InFlightCount = %d, want 0", engineState.InFlightCount)
	}
	if len(engineState.Dispatches) != 0 {
		t.Fatalf("active dispatch count = %d, want 0", len(engineState.Dispatches))
	}
	if len(engineState.DispatchHistory) == 0 {
		t.Fatal("DispatchHistory is empty, want completed timeout dispatch")
	}
	if engineState.DispatchHistory[0].Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first dispatch outcome = %s, want %s", engineState.DispatchHistory[0].Outcome, interfaces.OutcomeFailed)
	}
	if engineState.DispatchHistory[0].Reason != "execution timeout" {
		t.Fatalf("first dispatch reason = %q, want execution timeout", engineState.DispatchHistory[0].Reason)
	}
}

func TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt(t *testing.T) {
	skipSlowFunctionalSmokeInShort(t, "slow timeout retry smoke")
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "script_executor_dir"))
	attemptFile := filepath.Join(t.TempDir(), "timeout-attempts.txt")

	workerAgentsPath := filepath.Join(dir, "workers", "script-worker", "AGENTS.md")
	workerAgents := fmt.Sprintf(`---
type: SCRIPT_WORKER
command: %s
args:
  - '-test.run=TestIntegrationSmoke_ProcessTreeHelper'
  - '--'
  - 'timeout-once'
  - %s
timeout: 1500ms
---
Timeout once, then succeed after the Agent Factory requeues the work.
`, yamlSingleQuoted(os.Args[0]), yamlSingleQuoted(attemptFile))
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkID:     "work-timeout-requeue-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-requeue-smoke",
		Payload:    []byte("timeout once and retry"),
	})

	h := testutil.NewServiceTestHarness(t, dir, testutil.WithFullWorkerPoolAndScriptWrap())
	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")

	engineState, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot() error = %v", err)
	}
	dispatches := dispatchesForWorkID(engineState.DispatchHistory, "work-timeout-requeue-smoke")
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count for timeout work = %d, want 2", len(dispatches))
	}
	first := dispatches[0]
	if first.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("first dispatch outcome = %s, want %s", first.Outcome, interfaces.OutcomeFailed)
	}
	if first.Reason != "execution timeout" {
		t.Fatalf("first dispatch reason = %q, want execution timeout", first.Reason)
	}
	if first.ProviderFailure == nil {
		t.Fatal("first dispatch ProviderFailure is nil, want timeout metadata")
	}
	if first.ProviderFailure.Type != interfaces.ProviderErrorTypeTimeout {
		t.Fatalf("first dispatch provider failure type = %s, want %s", first.ProviderFailure.Type, interfaces.ProviderErrorTypeTimeout)
	}
	if first.ProviderFailure.Family != interfaces.ProviderErrorFamilyRetryable {
		t.Fatalf("first dispatch provider failure family = %s, want %s", first.ProviderFailure.Family, interfaces.ProviderErrorFamilyRetryable)
	}
	if len(first.OutputMutations) == 0 || first.OutputMutations[0].ToPlace != "task:init" {
		t.Fatalf("first dispatch mutations = %#v, want requeue to task:init", first.OutputMutations)
	}
	if first.OutputMutations[0].Token == nil || first.OutputMutations[0].Token.History.LastError != "execution timeout" {
		t.Fatalf("first dispatch requeued token = %#v, want timeout history", first.OutputMutations[0].Token)
	}

	second := dispatches[1]
	if second.Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("second dispatch outcome = %s, want %s", second.Outcome, interfaces.OutcomeAccepted)
	}
	if second.ProviderFailure != nil {
		t.Fatalf("second dispatch ProviderFailure = %#v, want nil", second.ProviderFailure)
	}
}

func TestIntegrationSmoke_ProcessTreeHelper(t *testing.T) {
	if len(os.Args) < 2 {
		return
	}

	mode := os.Args[len(os.Args)-2]
	pidFile := os.Args[len(os.Args)-1]
	switch mode {
	case "spawn-child":
		spawnTimeoutCleanupChild(pidFile)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "pid-sleep":
		writeTimeoutCleanupPID(pidFile)
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "timeout-once":
		runTimeoutOnceHelper(pidFile)
	default:
		return
	}
}

func spawnTimeoutCleanupChild(pidFile string) {
	child := exec.Command(os.Args[0],
		"-test.run=TestIntegrationSmoke_ProcessTreeHelper",
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

func writeTimeoutCleanupPID(pidFile string) {
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write pid file: %v\n", err)
		os.Exit(2)
	}
}

func runTimeoutOnceHelper(attemptFile string) {
	attempt := readTimeoutAttempt(attemptFile) + 1
	if err := os.WriteFile(attemptFile, []byte(strconv.Itoa(attempt)), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write attempt file: %v\n", err)
		os.Exit(2)
	}
	if attempt == 1 {
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	fmt.Println("recovered after timeout")
	os.Exit(0)
}

func readTimeoutAttempt(attemptFile string) int {
	raw, err := os.ReadFile(attemptFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		fmt.Fprintf(os.Stderr, "read attempt file: %v\n", err)
		os.Exit(2)
	}
	attempt, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse attempt file %q: %v\n", raw, err)
		os.Exit(2)
	}
	return attempt
}

func readTimeoutCleanupPID(t *testing.T, pidFile string) int {
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

func waitForTimeoutCleanupProcessExit(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !timeoutCleanupProcessRunning(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return !timeoutCleanupProcessRunning(pid)
		case <-ticker.C:
		}
	}
}

func yamlSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func dispatchesForWorkID(history []interfaces.CompletedDispatch, workID string) []interfaces.CompletedDispatch {
	dispatches := make([]interfaces.CompletedDispatch, 0, len(history))
	for _, dispatch := range history {
		for _, token := range dispatch.ConsumedTokens {
			if token.Color.WorkID == workID {
				dispatches = append(dispatches, dispatch)
				break
			}
		}
	}
	return dispatches
}
