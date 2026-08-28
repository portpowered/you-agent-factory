package process_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

// This executable-boundary cell intentionally uses the CLI's serialized
// --with-mock-workers fixture instead of an injected ProviderCommandRunner.
// The latter is an in-process root.BuildProcess edge and cannot cross into the
// built child executable; it therefore cannot prove that cancellation reaps
// the attributable external child process observed through its PID file.

// TestCLIContextCancellationStopsExternalWork proves an interrupt delivered to
// the built CLI cancels its invocation context and stops the attributable
// provider/external worker process, so cancelled runs do not leave orphaned work.
func TestCLIContextCancellationStopsExternalWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("blocking /bin/sh external-work fixture is unavailable on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
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
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
		"--quiet",
		prompt,
	)

	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start built CLI process: %v", err)
	}
	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = command.Wait()
		close(waitDone)
	}()
	providerPID := 0
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
		}
		select {
		case <-waitDone:
		case <-time.After(contextCancellationScenarioTimeout):
			t.Errorf("timed out joining root process during cleanup")
		}
		if providerPID > 0 {
			terminateContextCancellationProcess(providerPID)
			_ = waitForContextCancellationProcessExit(providerPID, 15*time.Second)
		}
	})

	providerPID = waitForContextCancellationProviderPID(t, providerPIDFile, 45*time.Second)

	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built CLI process: %v", err)
	}

	select {
	case <-waitDone:
		if waitErr == nil {
			t.Fatalf(
				"cancelled root process returned success; want process failure after context cancellation\nstdout:\n%s\nstderr:\n%s",
				stdout.String(),
				stderr.String(),
			)
		}
	case <-time.After(contextCancellationScenarioTimeout):
		_ = command.Process.Kill()
		<-waitDone
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

// TestCLIContextCancellationEmitsNoSuccessResult proves an interrupt delivered
// to the built CLI during an in-flight invocation yields a canceled terminal
// public outcome without emitting a completed success primary result.
func TestCLIContextCancellationEmitsNoSuccessResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("blocking /bin/sh external-work fixture is unavailable on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
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
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
		"--quiet",
		prompt,
	)

	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()

	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start built CLI process: %v", err)
	}
	waitDone := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = command.Wait()
		close(waitDone)
	}()
	providerPID := 0
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
		}
		select {
		case <-waitDone:
		case <-time.After(contextCancellationScenarioTimeout):
			t.Errorf("timed out joining root process during cleanup")
		}
		if providerPID > 0 {
			terminateContextCancellationProcess(providerPID)
			_ = waitForContextCancellationProcessExit(providerPID, 15*time.Second)
		}
	})

	providerPID = waitForContextCancellationProviderPID(t, providerPIDFile, 45*time.Second)

	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built CLI process: %v", err)
	}

	select {
	case <-waitDone:
	case <-time.After(contextCancellationScenarioTimeout):
		_ = command.Process.Kill()
		<-waitDone
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

func TestContextCancellationPIDReadinessIgnoresPartialPublication(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
		ok   bool
	}{
		{name: "empty publication", raw: "", want: 0, ok: false},
		{name: "whitespace publication", raw: "\n", want: 0, ok: false},
		{name: "partial publication", raw: "12x", want: 0, ok: false},
		{name: "non numeric publication", raw: "worker", want: 0, ok: false},
		{name: "complete publication", raw: "12345\n", want: 12345, ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseContextCancellationProviderPID([]byte(tc.raw))
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parseContextCancellationProviderPID(%q) = (%d, %t), want (%d, %t)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func parseContextCancellationProviderPID(raw []byte) (int, bool) {
	contents := strings.TrimSpace(string(raw))
	if contents == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(contents)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func waitForContextCancellationProviderPID(t *testing.T, pidFile string, timeout time.Duration) int {
	t.Helper()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// The blocking external worker is a separate OS process and exposes its
	// readiness through a PID file whose shell redirection is not atomic. Poll
	// until the durable contents are a complete numeric PID; the timer is a
	// bounded failure guard, not a readiness delay.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var lastContents string
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			lastContents = strings.TrimSpace(string(raw))
			if pid, ok := parseContextCancellationProviderPID(raw); ok {
				return pid
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("read provider pid file: %v", err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf(
				"timed out waiting for provider/external worker process to start; last pid file contents %q",
				lastContents,
			)
		}
	}
}

func waitForContextCancellationProcessExit(pid int, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Process exit is not exposed through the root-process command once the
	// provider has become an external child, so observe the OS process state
	// until the terminal condition is visible within the bounded guard.
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
