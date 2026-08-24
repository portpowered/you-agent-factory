package process_test

import (
	"bufio"
	"bytes"
	"context"
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
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIValidationFailureExitCode proves invalid customer input exits the
// documented validation-failure code through the public built you CLI process.
func TestCLIValidationFailureExitCode(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))

	for _, tc := range []struct {
		name string
		args []string
	}{
		{
			name: "default",
			args: []string{"run", "--named", "@you/missing", "--no-record", "invalid-goal-prompt"},
		},
		{
			name: "quiet",
			args: []string{"run", "--named", "@you/missing", "--no-record", "--quiet", "invalid-goal-prompt"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := harness.NewSession(t)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			result, err := session.Run(ctx, tc.args...)
			if err == nil {
				t.Fatalf("invalid input result = %#v; want process failure", result)
			}
			if result.ExitCode != 1 {
				t.Fatalf("exit code = %d, want documented validation-failure exit 1", result.ExitCode)
			}
		})
	}
}

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

// TestCLIInterruptedExitCode proves delivering the normal process interrupt to
// an in-flight built you CLI command exits the documented cancellation code.
func TestCLIInterruptedExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt is not supported for child processes on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	command, lines, scanErr, stderr := startRootProcessServerCommand(t, session, harness)
	stopped := false
	defer func() {
		if !stopped {
			command.Cancel()
			_ = command.Wait()
		}
	}()

	_ = waitForDashboardURL(t, lines, scanErr, stderr, 30*time.Second)
	interruptAndAssertCancellationExit(t, command, 10*time.Second)
	waitForScannerCompletion(t, scanErr, "interrupted root process", 5*time.Second)
	stopped = true
}

// TestBuiltCLIDeclaredCancellationExitCodes proves the executable boundary
// preserves exit 130 for both the standalone server and the continuous run
// command. Process.Execute cannot expose the final OS status, so this test
// intentionally starts the built CLI and sends the platform interrupt.
func TestBuiltCLIDeclaredCancellationExitCodes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt is not supported for child processes on Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, testutil.MustRepoRoot(t))

	for _, test := range []struct {
		name string
		args func(*builtcliacceptance.Session) []string
	}{
		{
			name: "server",
			args: func(session *builtcliacceptance.Session) []string {
				return append(session.ServerFlags(), "server")
			},
		},
		{
			name: "continuous run",
			args: func(session *builtcliacceptance.Session) []string {
				return append(session.ServerFlags(), "run", "--continuously", "--with-server", "--no-record")
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			session := harness.NewSession(t).WithNoExternalServer(t)
			writeIdleCurrentFactory(t, session.WorkDir)

			command := exec.Command(binaryPath, test.args(session)...)
			command.Dir = session.WorkDir
			command.Env = session.ProcessEnv()
			stdout, err := command.StdoutPipe()
			if err != nil {
				t.Fatalf("open built CLI stdout: %v", err)
			}
			var stderr bytes.Buffer
			command.Stderr = &stderr
			if err := command.Start(); err != nil {
				t.Fatalf("start built CLI %s: %v", test.name, err)
			}
			stopped := false
			defer func() {
				if !stopped {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			}()

			lines := make(chan string, 8)
			scanErr := make(chan error, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					lines <- scanner.Text()
				}
				scanErr <- scanner.Err()
			}()

			dashboardURL := waitForDashboardURL(t, lines, scanErr, &stderr, 30*time.Second)
			if test.name == "continuous run" {
				assertContinuousRunReady(t, dashboardURL)
			}
			interruptBuiltCLIAndAssertExit130(t, command, 15*time.Second)
			waitForScannerCompletion(t, scanErr, test.name, 5*time.Second)
			stopped = true
		})
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
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, testutil.MustRepoRoot(t))
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

// assertContinuousRunReady waits on the public runtime status observation after
// listener binding. The dashboard URL proves only that HTTP is readable; a
// RUNNING Factory state proves the continuous runtime loop reached its active
// lifecycle boundary before the process receives SIGINT. The deadline only
// bounds failure when that lifecycle signal is missing.
func assertContinuousRunReady(t testing.TB, dashboardURL string) {
	t.Helper()

	baseURL, ok := strings.CutSuffix(dashboardURL, "/dashboard/ui")
	if !ok || baseURL == "" {
		t.Fatalf("dashboard URL = %q, want /dashboard/ui suffix", dashboardURL)
	}
	support.WaitForStatus(t, baseURL, 30*time.Second, func(status factoryapi.StatusResponse) bool {
		return status.FactoryState == "RUNNING"
	})
}

func interruptBuiltCLIAndAssertExit130(t testing.TB, command *exec.Cmd, waitTimeout time.Duration) {
	t.Helper()
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built CLI: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()
	select {
	case err := <-waitResult:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
			t.Fatalf("interrupted built CLI exit = %v, want exit code 130", err)
		}
	case <-time.After(waitTimeout):
		_ = command.Process.Kill()
		<-waitResult
		t.Fatalf("interrupted built CLI did not exit within %s", waitTimeout)
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

// TestBuiltCLIExitStatusForNonWorkerCommands proves the OS status mapping that
// Process.Execute cannot expose, using deterministic commands that do not need
// a worker or the MockWorkers feature.
func TestBuiltCLIExitStatusForNonWorkerCommands(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, testutil.MustRepoRoot(t))

	for _, tc := range []struct {
		name       string
		args       []string
		wantStatus int
		wantError  bool
	}{
		{name: "help success", args: []string{"--help"}, wantStatus: 0},
		{name: "unknown command failure", args: []string{"definitely-not-a-command"}, wantStatus: 1, wantError: true},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			session := harness.NewSession(t)
			result, err := runBuiltYouBinary(ctx, binaryPath, session, tc.args...)
			if result.ExitCode != tc.wantStatus {
				t.Fatalf("built CLI exit code = %d, want %d; err=%v stdout=%q stderr=%q", result.ExitCode, tc.wantStatus, err, result.Stdout, result.Stderr)
			}
			if (err != nil) != tc.wantError {
				t.Fatalf("built CLI error = %v, want error=%t", err, tc.wantError)
			}
		})
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
