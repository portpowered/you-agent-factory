package process_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	contextCancellationScenarioTimeout = 60 * time.Second
	contextCancellationGoalWorkType    = "goal"
	contextCancellationExecutorWorker  = "goal-executor"
	contextCancellationExecuteStation  = "execute-goal"
)

// TestCLIContextCancellationStopsExternalWork proves cancelling the CLI process
// context stops the injected provider/external worker process attributable to the
// invocation, so cancelled runs do not leave orphaned external work running.
func TestCLIContextCancellationStopsExternalWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("blocking /bin/sh external-work fixture is unavailable on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	providerPIDFile := filepath.Join(t.TempDir(), "provider.pid")
	factoryPath := writeContextCancellationGoalFactory(t, session.WorkDir)
	mockWorkersPath := writeBlockingGoalExecutorMockWorkers(t, providerPIDFile)
	prompt := fmt.Sprintf("context-cancellation-external-work-%d", time.Now().UnixNano())

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--quiet",
		prompt,
	)

	command := harness.Command(args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start root process: %v", err)
	}

	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()

	providerPID := waitForContextCancellationProviderPID(t, providerPIDFile, 45*time.Second)
	t.Cleanup(func() {
		terminateContextCancellationProcess(providerPID)
	})

	command.Cancel()

	select {
	case err := <-waitResult:
		if err == nil {
			t.Fatalf(
				"cancelled root process returned success; want process failure after context cancellation\nstdout:\n%s\nstderr:\n%s",
				stdout.String(),
				stderr.String(),
			)
		}
	case <-time.After(contextCancellationScenarioTimeout):
		command.Cancel()
		<-waitResult
		t.Fatalf(
			"timed out waiting for root process to exit after context cancellation\nstdout:\n%s\nstderr:\n%s",
			stdout.String(),
			stderr.String(),
		)
	}

	if !waitForContextCancellationProcessExit(providerPID, 15*time.Second) {
		t.Fatalf(
			"provider/external worker process %d still running after CLI context cancellation",
			providerPID,
		)
	}
}

// TestCLIContextCancellationEmitsNoSuccessResult proves cancelling the CLI process
// context during an in-flight invocation yields an interrupted terminal public outcome
// without emitting a completed success primary result attributable to the cancelled run.
func TestCLIContextCancellationEmitsNoSuccessResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("blocking /bin/sh external-work fixture is unavailable on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	providerPIDFile := filepath.Join(t.TempDir(), "provider.pid")
	factoryPath := writeContextCancellationGoalFactory(t, session.WorkDir)
	mockWorkersPath := writeBlockingGoalExecutorMockWorkers(t, providerPIDFile)
	prompt := fmt.Sprintf("context-cancellation-no-success-%d", time.Now().UnixNano())

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers=" + mockWorkersPath,
		"--no-record",
		"--quiet",
		prompt,
	)

	command := harness.Command(args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start root process: %v", err)
	}

	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()

	providerPID := waitForContextCancellationProviderPID(t, providerPIDFile, 45*time.Second)
	t.Cleanup(func() {
		terminateContextCancellationProcess(providerPID)
	})

	command.Cancel()

	var waitErr error
	select {
	case waitErr = <-waitResult:
	case <-time.After(contextCancellationScenarioTimeout):
		command.Cancel()
		<-waitResult
		t.Fatalf(
			"timed out waiting for root process to exit after context cancellation\nstdout:\n%s\nstderr:\n%s",
			stdout.String(),
			stderr.String(),
		)
	}

	if waitErr == nil {
		t.Fatalf(
			"cancelled root process returned success; want interrupted terminal outcome\nstdout:\n%s\nstderr:\n%s",
			stdout.String(),
			stderr.String(),
		)
	}
	cancellationDiagnostic := stderr.String() + "\n" + waitErr.Error()
	if !strings.Contains(cancellationDiagnostic, "INVOCATION_CANCELED") {
		t.Fatalf(
			"cancelled root process diagnostic missing INVOCATION_CANCELED:\nstdout:\n%s\nstderr:\n%s",
			stdout.String(),
			stderr.String(),
		)
	}

	if trimmed := strings.TrimSpace(stdout.String()); trimmed != "" {
		t.Fatalf(
			"stdout = %q, want empty (no completed success primary result after cancellation)",
			stdout.String(),
		)
	}
	if stdout.String() == successStdoutPrimaryResult ||
		strings.Contains(stdout.String(), successStdoutPrimaryResult) {
		t.Fatalf(
			"stdout = %q, want no success primary-result payload %q after cancellation",
			stdout.String(),
			successStdoutPrimaryResult,
		)
	}
}

func writeContextCancellationGoalFactory(t *testing.T, workDir string) string {
	t.Helper()

	factoryDir := filepath.Join(workDir, "context-cancellation-goal-factory")
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create context-cancellation goal factory directory: %v", err)
	}

	factoryJSON := fmt.Sprintf(`{
  "name": "context-cancellation-goal",
  "invocationReturn": {
    "policy": "EXPLICIT",
    "workTypeName": %q,
    "terminalState": "complete"
  },
  "workTypes": [{
    "name": %q,
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "execute", "type": "PROCESSING"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": %q}],
  "workstations": [{
    "name": %q,
    "type": "AGENT_RUN",
    "behavior": "REPEATER",
    "worker": %q,
    "inputs": [{"workType": %q, "state": "init"}],
    "outputs": [{"workType": %q, "state": "complete"}],
    "onContinue": [{"workType": %q, "state": "init"}],
    "onRejection": [{"workType": %q, "state": "init"}],
    "onFailure": [{"workType": %q, "state": "failed"}]
  }]
}`,
		contextCancellationGoalWorkType,
		contextCancellationGoalWorkType,
		contextCancellationExecutorWorker,
		contextCancellationExecuteStation,
		contextCancellationExecutorWorker,
		contextCancellationGoalWorkType,
		contextCancellationGoalWorkType,
		contextCancellationGoalWorkType,
		contextCancellationGoalWorkType,
		contextCancellationGoalWorkType,
	)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(factoryJSON), 0o600); err != nil {
		t.Fatalf("write context-cancellation goal factory.json: %v", err)
	}

	workerAgentsPath := filepath.Join(factoryDir, "workers", contextCancellationExecutorWorker, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workerAgentsPath), 0o755); err != nil {
		t.Fatalf("create goal-executor worker directory: %v", err)
	}
	workerAgents := `---
type: MODEL_WORKER
model: gpt-5-codex
modelProvider: codex
stopToken: COMPLETE
---
Process the input task.
`
	if err := os.WriteFile(workerAgentsPath, []byte(workerAgents), 0o644); err != nil {
		t.Fatalf("write goal-executor worker AGENTS.md: %v", err)
	}

	return factoryPath
}

func writeBlockingGoalExecutorMockWorkers(t *testing.T, providerPIDFile string) string {
	t.Helper()

	data, err := json.Marshal(workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      contextCancellationExecutorWorker,
			WorkstationName: contextCancellationExecuteStation,
			RunType:         workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{
				Command: "/bin/sh",
				Args: []string{
					"-c",
					fmt.Sprintf("echo $$ > %q; exec sleep 300", providerPIDFile),
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("marshal blocking goal mock workers: %v", err)
	}
	path := filepath.Join(t.TempDir(), "blocking-goal-mock-workers.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write blocking goal mock workers: %v", err)
	}
	return path
}

func waitForContextCancellationProviderPID(t *testing.T, pidFile string, timeout time.Duration) int {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if parseErr != nil {
				t.Fatalf("parse provider pid %q: %v", raw, parseErr)
			}
			return pid
		}
		if !os.IsNotExist(err) {
			t.Fatalf("read provider pid file: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for provider/external worker process to start")
	return -1
}

func waitForContextCancellationProcessExit(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		if !contextCancellationProcessRunning(pid) {
			return true
		}
		select {
		case <-ctx.Done():
			return !contextCancellationProcessRunning(pid)
		case <-ticker.C:
		}
	}
}
