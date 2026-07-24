package providers

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
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestIntegrationSmoke_TimeoutCancelsProcessTreeAndClearsActiveExecution(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout cleanup smoke")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	childPIDFile := filepath.Join(t.TempDir(), "descendant.pid")

	updateScriptFixtureFactory(t, dir, func(cfg map[string]any) {
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

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-timeout-cleanup-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-cleanup-smoke",
		Payload:    []byte("spawn a descendant process"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)

	childPID := readTimeoutCleanupPID(t, childPIDFile)
	t.Cleanup(func() {
		timeoutCleanupTerminateProcess(childPID)
	})
	if !waitForTimeoutCleanupProcessExit(childPID, 3*time.Second) {
		t.Fatalf("spawned descendant process %d is still running after factory timeout", childPID)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	assertSessionPlaces(t, listed, map[string]int{"task:failed": 1, "task:init": 0, "task:done": 0})
	assertDispatchOutcomeSequence(t, server.GetFactoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
	}, "execution timeout")
	server.Stop(t)
}

func TestIntegrationSmoke_TimeoutRequeuesWorkAndSucceedsOnLaterAttempt(t *testing.T) {
	support.SkipLongFunctional(t, "slow timeout retry smoke")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
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

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     "work-timeout-requeue-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-timeout-requeue-smoke",
		Payload:    []byte("timeout once and retry"),
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	assertSessionPlaces(t, listed, map[string]int{"task:done": 1, "task:init": 0, "task:failed": 0})
	assertListedWorkIdentity(t, support.ListDefaultSessionWork(t, server.URL()), "done", "work-timeout-requeue-smoke", "task", "trace-timeout-requeue-smoke", nil)
	assertDispatchOutcomeSequence(t, server.GetFactoryEvents(t), []factoryapi.WorkOutcome{
		factoryapi.WorkOutcomeFailed,
		factoryapi.WorkOutcomeAccepted,
	}, "execution timeout")
	server.Stop(t)
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
