package process_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const successStdoutPrimaryResult = "mock worker accepted"

// TestCLISuccessWritesPrimaryResultOnlyToStdout proves a successful one-shot
// built you CLI run writes the primary invocation result to stdout only and
// does not mix diagnostics, lifecycle chatter, or other non-result noise onto
// the primary-result stream.
func TestCLISuccessWritesPrimaryResultOnlyToStdout(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initOutcome := initializeOperatorConfig(t, ctx, session, "success-stdout-purity-config-init")
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
		"--with-mock-workers",
		"--no-record",
		"--quiet",
		mockWorkersPath,
		fmt.Sprintf("success-stdout-purity-%d", time.Now().UnixNano()),
	)

	result, err := session.Run(ctx, args...)
	if err != nil {
		t.Fatalf(
			"successful stdout-purity run failed: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			result.Stdout,
			result.Stderr,
		)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want documented success exit 0", result.ExitCode)
	}
	if result.Stdout != successStdoutPrimaryResult {
		t.Fatalf(
			"stdout = %q, want only primary result %q",
			result.Stdout,
			successStdoutPrimaryResult,
		)
	}
	for _, forbidden := range []string{
		"Factory initiated:",
		"Dashboard URL:",
		"Dashboard server disabled",
		"error:",
		"Error:",
		"traceId:",
		"requestId:",
	} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf(
				"stdout mixed diagnostic or lifecycle noise %q into primary-result stream:\n%s",
				forbidden,
				result.Stdout,
			)
		}
	}
	if strings.TrimSpace(result.Stderr) != "" {
		t.Fatalf(
			"stderr = %q, want empty (success diagnostics must not leak onto stdout or stderr)",
			result.Stderr,
		)
	}
}

// TestCLIFailureWritesDiagnosticToStderr proves a terminal worker failure through
// the public built you CLI writes customer-visible diagnostics to stderr, leaves
// stdout free of a false primary result, and exits unsuccessfully.
func TestCLIFailureWritesDiagnosticToStderr(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initOutcome := initializeOperatorConfig(t, ctx, session, "failure-stderr-config-init")
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
		fmt.Sprintf("failure-stderr-%d", time.Now().UnixNano()),
	)

	result, err := session.Run(ctx, args...)
	if err == nil {
		t.Fatalf("terminal worker failure result = %#v; want process failure", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = %d, want non-zero terminal failure exit", result.ExitCode)
	}

	stderr := strings.TrimSpace(result.Stderr)
	if stderr == "" {
		t.Fatal("terminal worker failure stderr was empty; want actionable diagnostic")
	}
	diagnostic := stderr + "\n" + err.Error()
	if !strings.Contains(diagnostic, "INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf(
			"terminal worker failure diagnostic missing INVOCATION_RUNTIME_FAILURE:\nstdout:\n%s\nstderr:\n%s",
			result.Stdout,
			result.Stderr,
		)
	}
	if strings.Contains(stderr, "Factory initiated:") ||
		strings.Contains(stderr, "Dashboard URL:") {
		t.Fatalf(
			"stderr mixed lifecycle chatter into diagnostic stream:\n%s",
			result.Stderr,
		)
	}

	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf(
			"stdout = %q, want empty (no false primary result on terminal failure)",
			result.Stdout,
		)
	}
	if result.Stdout == successStdoutPrimaryResult ||
		strings.Contains(result.Stdout, successStdoutPrimaryResult) {
		t.Fatalf(
			"stdout = %q, want no success primary-result payload %q",
			result.Stdout,
			successStdoutPrimaryResult,
		)
	}
	for _, forbidden := range []string{
		"Factory initiated:",
		"Dashboard URL:",
		"Dashboard server disabled",
	} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf(
				"stdout mixed lifecycle or diagnostic noise %q into primary-result stream:\n%s",
				forbidden,
				result.Stdout,
			)
		}
	}
}

const quietPrimaryResultSeparator = "--- primary result ---"

// TestCLIQuietModeSuppressesNonResultNoise proves --quiet keeps script-safe
// stdout/stderr at the public built you CLI process boundary: success writes
// only the raw primary result without operator or lifecycle noise, and failure
// keeps stdout empty while diagnostics stay on stderr.
func TestCLIQuietModeSuppressesNonResultNoise(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	initOutcome := initializeOperatorConfig(t, ctx, session, "quiet-mode-config-init")
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
	rejectingMockWorkersPath := writeRejectingGoalMockWorkers(t)

	t.Run("success suppresses stdout lifecycle presentation", func(t *testing.T) {
		prompt := fmt.Sprintf("quiet-mode-stream-baseline-%d", time.Now().UnixNano())
		streamArgs := appendGoalRunArgs(session, mockWorkersPath, prompt,
			"--output", "response-stream",
		)
		streamResult, err := session.Run(ctx, streamArgs...)
		if err != nil {
			t.Fatalf(
				"response-stream success run failed: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				streamResult.Stdout,
				streamResult.Stderr,
			)
		}
		if streamResult.ExitCode != 0 {
			t.Fatalf("response-stream exit code = %d, want success exit 0", streamResult.ExitCode)
		}
		if !strings.Contains(streamResult.Stdout, quietPrimaryResultSeparator) {
			t.Fatalf(
				"response-stream stdout missing primary-result separator %q; want observable non-result noise baseline:\n%s",
				quietPrimaryResultSeparator,
				streamResult.Stdout,
			)
		}
		if !containsHumanLifecycleNoise(streamResult.Stdout) {
			t.Fatalf(
				"response-stream stdout missing human lifecycle noise; want baseline chatter before quiet suppression:\n%s",
				streamResult.Stdout,
			)
		}
		if streamResult.Stdout == successStdoutPrimaryResult {
			t.Fatalf(
				"response-stream stdout = %q, want mixed lifecycle presentation rather than raw primary result only",
				streamResult.Stdout,
			)
		}

		quietPrompt := fmt.Sprintf("quiet-mode-success-%d", time.Now().UnixNano())
		quietArgs := appendGoalRunArgs(session, mockWorkersPath, quietPrompt, "--quiet")
		quietResult, err := session.Run(ctx, quietArgs...)
		if err != nil {
			t.Fatalf(
				"quiet success run failed: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				quietResult.Stdout,
				quietResult.Stderr,
			)
		}
		if quietResult.ExitCode != 0 {
			t.Fatalf("quiet exit code = %d, want success exit 0", quietResult.ExitCode)
		}
		if quietResult.Stdout != successStdoutPrimaryResult {
			t.Fatalf(
				"quiet stdout = %q, want only raw primary result %q",
				quietResult.Stdout,
				successStdoutPrimaryResult,
			)
		}
		for _, forbidden := range []string{
			quietPrimaryResultSeparator,
			"factory started",
			"work accepted",
		} {
			if strings.Contains(quietResult.Stdout, forbidden) {
				t.Fatalf(
					"quiet stdout leaked non-result noise %q into script-safe primary-result stream:\n%s",
					forbidden,
					quietResult.Stdout,
				)
			}
		}
		if strings.TrimSpace(quietResult.Stderr) != "" {
			t.Fatalf(
				"quiet success stderr = %q, want empty (operator chatter must stay off stderr)",
				quietResult.Stderr,
			)
		}
	})

	t.Run("success suppresses verbose stderr operator logs", func(t *testing.T) {
		prompt := fmt.Sprintf("quiet-mode-verbose-baseline-%d", time.Now().UnixNano())
		verboseArgs := appendGoalRunArgs(session, mockWorkersPath, prompt, "--verbose")
		verboseResult, err := session.Run(ctx, verboseArgs...)
		if err != nil {
			t.Fatalf(
				"verbose success run failed: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				verboseResult.Stdout,
				verboseResult.Stderr,
			)
		}
		if verboseResult.ExitCode != 0 {
			t.Fatalf("verbose exit code = %d, want success exit 0", verboseResult.ExitCode)
		}
		if verboseResult.Stdout != successStdoutPrimaryResult {
			t.Fatalf(
				"verbose stdout = %q, want primary result only %q",
				verboseResult.Stdout,
				successStdoutPrimaryResult,
			)
		}
		verboseStderr := strings.TrimSpace(verboseResult.Stderr)
		if verboseStderr == "" {
			t.Fatal("verbose success stderr was empty; want observable operator/runtime log noise baseline")
		}
		if !strings.Contains(verboseStderr, "named factory resolved") &&
			!strings.Contains(verboseStderr, "engine started") {
			t.Fatalf(
				"verbose stderr missing runtime operator logs; want baseline noise before quiet suppression:\n%s",
				verboseResult.Stderr,
			)
		}

		quietPrompt := fmt.Sprintf("quiet-mode-verbose-contrast-%d", time.Now().UnixNano())
		quietArgs := appendGoalRunArgs(session, mockWorkersPath, quietPrompt, "--quiet")
		quietResult, err := session.Run(ctx, quietArgs...)
		if err != nil {
			t.Fatalf(
				"quiet success run failed: %v\nstdout:\n%s\nstderr:\n%s",
				err,
				quietResult.Stdout,
				quietResult.Stderr,
			)
		}
		if quietResult.Stdout != successStdoutPrimaryResult {
			t.Fatalf(
				"quiet stdout = %q, want only raw primary result %q",
				quietResult.Stdout,
				successStdoutPrimaryResult,
			)
		}
		if strings.TrimSpace(quietResult.Stderr) != "" {
			t.Fatalf(
				"quiet success stderr = %q, want empty after suppressing verbose operator logs",
				quietResult.Stderr,
			)
		}
	})

	t.Run("failure keeps quiet stdout script-safe", func(t *testing.T) {
		quietFailureArgs := appendGoalRunArgs(
			session,
			rejectingMockWorkersPath,
			fmt.Sprintf("quiet-mode-failure-%d", time.Now().UnixNano()),
			"--quiet",
		)
		failureResult, err := session.Run(ctx, quietFailureArgs...)
		if err == nil {
			t.Fatalf("quiet terminal failure result = %#v; want process failure", failureResult)
		}
		if failureResult.ExitCode == 0 {
			t.Fatalf("quiet failure exit code = %d, want non-zero terminal failure exit", failureResult.ExitCode)
		}
		if strings.TrimSpace(failureResult.Stdout) != "" {
			t.Fatalf(
				"quiet failure stdout = %q, want empty script-safe stdout without a false primary result",
				failureResult.Stdout,
			)
		}
		if strings.TrimSpace(failureResult.Stderr) == "" {
			t.Fatal("quiet failure stderr was empty; want actionable diagnostic without stdout noise")
		}
	})
}

func appendGoalRunArgs(
	session *builtcliacceptance.Session,
	mockWorkersPath string,
	prompt string,
	extraArgs ...string,
) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", "@you/goal",
		"--with-mock-workers",
		"--no-record",
	)
	args = append(args, extraArgs...)
	args = append(args, mockWorkersPath, prompt)
	return args
}

func containsHumanLifecycleNoise(stdout string) bool {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == quietPrimaryResultSeparator || line == successStdoutPrimaryResult {
			continue
		}
		closingBracket := strings.Index(line, "] ")
		if !strings.HasPrefix(line, "[") || closingBracket < 2 {
			continue
		}
		message := line[closingBracket+2:]
		for _, prefix := range []string{
			"work accepted",
			"work moved",
			"factory started",
			"factory completed",
			"workstation queued",
			"workstation started",
			"workstation completed",
			"final output updated",
		} {
			if strings.HasPrefix(message, prefix) {
				return true
			}
		}
	}
	return false
}
