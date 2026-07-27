package process_test

import (
	"context"
	"fmt"
	"os"
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
		"--with-mock-workers",
		"--no-record",
		"--quiet",
		mockWorkersPath,
		fmt.Sprintf("worker-failure-exit-%d", time.Now().UnixNano()),
	)

	result, err := session.Run(ctx, args...)
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

	command, lines, scanErr, stderr := startBuiltCLIServerCommand(t, session, harness.BinaryPath)
	stopped := false
	defer func() {
		if !stopped {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()

	_ = waitForDashboardURL(t, lines, scanErr, stderr, 30*time.Second)
	interruptAndAssertCancellationExit(t, command, 10*time.Second)
	stopped = true
}
