package process_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIWorkerFailureExitCode proves a terminal worker failure crosses the
// public CLI process boundary as a typed failure without using the MockWorkers
// feature outside its workers/mock functional cell.
func TestCLIWorkerFailureExitCode(t *testing.T) {
	factoryDir, factoryPath := scaffoldCLIExitCodeFactory(t)
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("provider process failed with private detail"),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--no-record", "--quiet",
		"worker-failure-exit",
	})
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatal("worker failure Process.Execute error = nil; want typed process failure")
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want one worker dispatch", runner.CallCount())
	}
	if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
		t.Fatalf("worker failure stdout = %q, want no false success output", stdout)
	}
	var diagnostic struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	stderr := strings.TrimSpace(inputs.Stderr())
	if err := json.Unmarshal([]byte(stderr), &diagnostic); err != nil {
		t.Fatalf("decode worker failure diagnostic: %v; stderr=%q", err, stderr)
	}
	if diagnostic.Code != "INVOCATION_RUNTIME_FAILURE" || diagnostic.Message == "" {
		t.Fatalf("worker failure diagnostic = %#v, want coded runtime failure", diagnostic)
	}
	if strings.Count(stderr, diagnostic.Code) != 1 || strings.Contains(stderr, "private detail") {
		t.Fatalf("worker failure stderr = %q, want one sanitized coded diagnostic", stderr)
	}
}

// TestBuiltCLIInterruptedResponseStreamExitCode proves the one-shot human
// response-stream path preserves the canonical cancellation exit at the OS
// process boundary. The invocation error is already rendered as
// INVOCATION_CANCELED by Process.Execute; this assertion covers the final
// executable mapping that cannot be observed from an in-process root.
func TestBuiltCLIInterruptedResponseStreamExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt is not supported for child processes on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := packageArtifact(t).Path
	session := harness.NewSession(t).WithNoExternalServer(t)

	providerPIDFile := filepath.Join(t.TempDir(), "provider.pid")
	factoryPath := writeContextCancellationGoalFactory(t, session.WorkDir)
	mockWorkersPath := writeBlockingGoalExecutorMockWorkers(t, providerPIDFile)
	prompt := fmt.Sprintf("built-response-stream-cancellation-%d", time.Now().UnixNano())
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--factory", factoryPath,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record", "--output", "response-stream",
		prompt,
	)

	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = session.ProcessEnv()
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built response-stream CLI: %v", err)
	}
	providerPID := 0
	t.Cleanup(func() {
		if command.ProcessState == nil || !command.ProcessState.Exited() {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
		if providerPID > 0 {
			terminateContextCancellationProcess(providerPID)
			_ = waitForContextCancellationProcessExit(providerPID, 15*time.Second)
		}
	})

	providerPID = waitForContextCancellationProviderPID(t, providerPIDFile, 45*time.Second)
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built response-stream CLI: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()
	select {
	case err := <-waitResult:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
			t.Fatalf("interrupted response-stream CLI exit = %v, want exit code 130\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
		}
	case <-time.After(contextCancellationScenarioTimeout):
		_ = command.Process.Kill()
		<-waitResult
		t.Fatalf("interrupted response-stream CLI did not exit within %s", contextCancellationScenarioTimeout)
	}

	output := stdout.String()
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
	if !strings.Contains(stderr.String(), "INVOCATION_CANCELED") {
		t.Fatalf("response-stream stderr missing INVOCATION_CANCELED:\n%s", stderr.String())
	}
	if !waitForContextCancellationProcessExit(providerPID, 15*time.Second) {
		t.Fatalf("provider/external worker process %d still running after response-stream cancellation", providerPID)
	}
}

// TestCLISuccessExitCode proves a successful one-shot worker run reaches the
// public CLI process boundary through an injected provider command runner.
func TestCLISuccessExitCode(t *testing.T) {
	factoryDir, factoryPath := scaffoldCLIExitCodeFactory(t)
	const wantPrimaryResult = "worker success exit COMPLETE"
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(wantPrimaryResult),
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", factoryPath,
		"--provider", "codex", "--no-record", "--quiet",
		"worker-success-exit",
	})
	inputs.Input.WorkingDirectory = factoryDir
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	err := process.Execute(inputs.Input)
	if err != nil {
		t.Fatalf("successful worker Process.Execute failed: %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want one worker dispatch", runner.CallCount())
	}
	if got := strings.TrimSpace(inputs.Stdout()); got != wantPrimaryResult {
		t.Fatalf("worker success stdout = %q, want primary result %q", got, wantPrimaryResult)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("worker success stderr = %q, want empty diagnostics", inputs.Stderr())
	}
}

func scaffoldCLIExitCodeFactory(t *testing.T) (string, string) {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "cli-exit-code-factory",
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{"DEFAULT"},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir, filepath.Join(dir, "factory.json")
}
