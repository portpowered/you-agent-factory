package process_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestCLIValidationFailureExitCode proves invalid customer input exits the
// documented validation-failure code through the public built you CLI process.
func TestCLIValidationFailureExitCode(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

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

// TestCLIWorkerFailureExitCode proves a terminal worker failure exits the
// documented runtime-failure code through the public built you CLI process.
func TestCLIWorkerFailureExitCode(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, testutil.MustRepoRoot(t))

	initOutcome := initializeOperatorConfig(t, ctx, session, "worker-failure-exit-config-init")
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if err := os.WriteFile(initOutcome.ConfigPath, configBody, 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", initOutcome.ConfigPath, err)
	}

	mockWorkersPath := writeRejectingGoalMockWorkers(t)
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", "@you/goal",
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
		"--quiet",
		fmt.Sprintf("worker-failure-exit-%d", time.Now().UnixNano()),
	)

	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err == nil {
		t.Fatalf("worker failure result = %#v; want process failure", result)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want documented worker-failure exit 1", result.ExitCode)
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

			waitForDashboardURL(t, lines, scanErr, &stderr, 30*time.Second)
			interruptBuiltCLIAndAssertExit130(t, command, 15*time.Second)
			stopped = true
		})
	}
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

// TestCLISuccessExitCode proves a successful one-shot run that reaches normal
// quiescence exits the documented success code through the public built you CLI process.
func TestCLISuccessExitCode(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, ctx, testutil.MustRepoRoot(t))

	initOutcome := initializeOperatorConfig(t, ctx, session, "success-exit-config-init")
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if err := os.WriteFile(initOutcome.ConfigPath, configBody, 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", initOutcome.ConfigPath, err)
	}

	mockWorkersPath := writeAcceptingGoalMockWorkers(t)
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", "@you/goal",
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
		"--quiet",
		fmt.Sprintf("success-exit-%d", time.Now().UnixNano()),
	)

	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err != nil {
		t.Fatalf("successful quiescence run failed: %v; stdout=%q stderr=%q", err, result.Stdout, result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want documented success exit 0", result.ExitCode)
	}
}
