package process_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// TestCLIContextCancellationStopsExternalWork proves an interrupt delivered
// to the built CLI stops the attributable provider/external worker process.
func TestCLIContextCancellationStopsExternalWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("blocking /bin/sh external-work fixture is unavailable on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	run := startContextCancellationCLI(t, harness, "context-cancellation-external-work", "--quiet")
	run.providerPID = waitForContextCancellationProviderPID(t, run.providerPIDFile, 45*time.Second)
	if err := run.command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built CLI process: %v", err)
	}
	if err := run.wait(t); err == nil {
		t.Fatalf("cancelled root process returned success; stdout=%q stderr=%q", run.stdout.String(), run.stderr.String())
	}
	if !waitForContextCancellationProcessExit(run.providerPID, 15*time.Second) {
		t.Fatalf("provider/external worker process %d still running after CLI context cancellation", run.providerPID)
	}
}

// TestCLIContextCancellationEmitsNoSuccessResult proves an interrupted
// built-CLI invocation reports cancellation without a completed success
// result. Process cleanup is owned by the integration run fixture.
func TestCLIContextCancellationEmitsNoSuccessResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("blocking /bin/sh external-work fixture is unavailable on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	run := startContextCancellationCLI(t, harness, "context-cancellation-no-success", "--quiet")
	run.providerPID = waitForContextCancellationProviderPID(t, run.providerPIDFile, 45*time.Second)
	if err := run.command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built CLI process: %v", err)
	}
	waitErr := run.wait(t)
	if waitErr == nil {
		t.Fatalf("cancelled root process returned success; stdout=%q stderr=%q", run.stdout.String(), run.stderr.String())
	}
	assertCancellationHasNoSuccessResult(t, run.stdout.String(), run.stderr.String(), waitErr)
}

// TestBuiltCLIInterruptedResponseStreamExitCode proves the response-stream
// executable maps a real interrupt to exit 130 and emits only cancellation.
func TestBuiltCLIInterruptedResponseStreamExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt is not supported for child processes on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	run := startContextCancellationCLI(t, harness, "built-response-stream-cancellation", "--output", "response-stream")
	run.providerPID = waitForContextCancellationProviderPID(t, run.providerPIDFile, 45*time.Second)
	if err := run.command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built response-stream CLI: %v", err)
	}
	waitErr := run.wait(t)
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) || exitErr.ExitCode() != 130 {
		t.Fatalf("interrupted response-stream CLI exit = %v, want exit code 130\nstdout:\n%s\nstderr:\n%s", waitErr, run.stdout.String(), run.stderr.String())
	}
	output := run.stdout.String()
	if !strings.Contains(output, "status: CANCELED") {
		t.Fatalf("response-stream stdout missing canceled status:\n%s", output)
	}
	for _, forbidden := range []string{
		"--- primary result ---",
		successStdoutPrimaryResult,
		"final output updated: FINAL",
		"factory completed: SUCCEEDED",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("response-stream stdout contains forbidden success claim %q:\n%s", forbidden, output)
		}
	}
	if !strings.Contains(run.stderr.String(), "INVOCATION_CANCELED") {
		t.Fatalf("response-stream stderr missing INVOCATION_CANCELED:\n%s", run.stderr.String())
	}
	if !waitForContextCancellationProcessExit(run.providerPID, 15*time.Second) {
		t.Fatalf("provider/external worker process %d still running after response-stream cancellation", run.providerPID)
	}
}

type contextCancellationCLI struct {
	command         *exec.Cmd
	stdout          *bytes.Buffer
	stderr          *bytes.Buffer
	providerPIDFile string
	providerPID     int
	waitDone        chan struct{}
	waitErr         error
}

func startContextCancellationCLI(
	t *testing.T,
	harness *builtcliacceptance.Harness,
	prompt string,
	extraArgs ...string,
) *contextCancellationCLI {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	binaryPath := buildYouBinary(t, ctx, harness.RepoRoot)
	session := harness.NewSession(t).WithNoExternalServer(t)
	providerPIDFile := filepath.Join(t.TempDir(), "provider.pid")
	factoryPath := writeContextCancellationGoalFactory(t, session.WorkDir)
	mockWorkersPath := writeBlockingGoalExecutorMockWorkers(t, providerPIDFile)
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args, "run", "--factory", factoryPath, "--with-mock-workers="+mockWorkersPath, "--no-record")
	args = append(args, extraArgs...)
	args = append(args, prompt)
	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	run := &contextCancellationCLI{
		command: command, stdout: new(bytes.Buffer), stderr: new(bytes.Buffer),
		providerPIDFile: providerPIDFile, waitDone: make(chan struct{}),
	}
	command.Stdout = run.stdout
	command.Stderr = run.stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built CLI process: %v", err)
	}
	go func() {
		run.waitErr = command.Wait()
		close(run.waitDone)
	}()
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
		}
		select {
		case <-run.waitDone:
		case <-time.After(contextCancellationScenarioTimeout):
			t.Errorf("timed out joining root process during cleanup")
		}
		if run.providerPID > 0 {
			terminateContextCancellationProcess(run.providerPID)
			_ = waitForContextCancellationProcessExit(run.providerPID, 15*time.Second)
		}
	})
	return run
}

func (run *contextCancellationCLI) wait(t testing.TB) error {
	t.Helper()
	select {
	case <-run.waitDone:
		return run.waitErr
	case <-time.After(contextCancellationScenarioTimeout):
		_ = run.command.Process.Kill()
		<-run.waitDone
		t.Fatalf("built CLI did not exit within %s\nstdout:\n%s\nstderr:\n%s", contextCancellationScenarioTimeout, run.stdout.String(), run.stderr.String())
		return run.waitErr
	}
}

func assertCancellationHasNoSuccessResult(t testing.TB, stdout, stderr string, waitErr error) {
	t.Helper()
	diagnostic := stderr + "\n" + waitErr.Error()
	if !strings.Contains(diagnostic, "INVOCATION_CANCELED") {
		t.Fatalf("cancelled root process diagnostic missing INVOCATION_CANCELED:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("stdout = %q, want empty after cancellation", stdout)
	}
	if strings.Contains(stdout, successStdoutPrimaryResult) {
		t.Fatalf("stdout = %q, want no success primary-result payload %q after cancellation", stdout, successStdoutPrimaryResult)
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
  "invocationReturn": {"policy": "EXPLICIT", "workTypeName": %q, "terminalState": "complete"},
  "workTypes": [{"name": %q, "handlingBehavior": ["DEFAULT"], "states": [
    {"name": "init", "type": "INITIAL"}, {"name": "execute", "type": "PROCESSING"},
    {"name": "complete", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}
  ]}],
  "workers": [{"name": %q}],
  "workstations": [{"name": %q, "type": "AGENT_RUN", "behavior": "REPEATER", "worker": %q,
    "inputs": [{"workType": %q, "state": "init"}], "outputs": [{"workType": %q, "state": "complete"}],
    "onContinue": [{"workType": %q, "state": "init"}], "onRejection": [{"workType": %q, "state": "init"}],
    "onFailure": [{"workType": %q, "state": "failed"}]
  }]
}`,
		contextCancellationGoalWorkType, contextCancellationGoalWorkType,
		contextCancellationExecutorWorker, contextCancellationExecuteStation,
		contextCancellationExecutorWorker, contextCancellationGoalWorkType,
		contextCancellationGoalWorkType, contextCancellationGoalWorkType,
		contextCancellationGoalWorkType, contextCancellationGoalWorkType,
	)
	factoryPath := filepath.Join(factoryDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(factoryJSON), 0o600); err != nil {
		t.Fatalf("write context-cancellation goal factory.json: %v", err)
	}
	workerAgentsPath := filepath.Join(factoryDir, "workers", contextCancellationExecutorWorker, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workerAgentsPath), 0o755); err != nil {
		t.Fatalf("create goal-executor worker directory: %v", err)
	}
	workerAgents := "---\ntype: MODEL_WORKER\nmodel: gpt-5-codex\nmodelProvider: codex\nstopToken: COMPLETE\n---\nProcess the input task.\n"
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
			WorkerName: contextCancellationExecutorWorker, WorkstationName: contextCancellationExecuteStation,
			RunType: workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{Command: "/bin/sh", Args: []string{
				"-c", fmt.Sprintf("echo $$ > %q; exec sleep 300", providerPIDFile),
			}},
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
	// The shell writes its PID through a non-atomic redirection. Poll until a
	// complete numeric publication is visible; the timer only bounds a hang.
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
			t.Fatalf("timed out waiting for provider process to start; last pid file contents %q", lastContents)
		}
	}
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
