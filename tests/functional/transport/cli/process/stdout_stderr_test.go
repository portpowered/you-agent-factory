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

// These tests intentionally use the CLI's serialized --with-mock-workers
// fixture. ProviderCommandRunner is an in-process edges.Edges dependency and
// cannot enter the separately built `you` executable; using it would move the
// assertions off the stdout/stderr and exit-code process boundary this cell
// exists to prove. The fixture supplies deterministic provider-shaped results
// inside that real child process without weakening the public stream checks.

// TestCLISuccessWritesPrimaryResultOnlyToStdout proves a successful one-shot
// built you CLI run writes the primary invocation result to stdout only and
// does not mix diagnostics, lifecycle chatter, or other non-result noise onto
// the primary-result stream.
func TestCLISuccessWritesPrimaryResultOnlyToStdout(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initOutcome := initializeOperatorConfig(t, ctx, binaryPath, session, "success-stdout-purity-config-init")
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
		fmt.Sprintf("success-stdout-purity-%d", time.Now().UnixNano()),
	)

	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
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
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initOutcome := initializeOperatorConfig(t, ctx, binaryPath, session, "failure-stderr-config-init")
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
		fmt.Sprintf("failure-stderr-%d", time.Now().UnixNano()),
	)

	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
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
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func TestCLIQuietModeSuppressesNonResultNoise(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	t.Run("success suppresses stdout lifecycle presentation", func(t *testing.T) {
		streamSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-stream-config")
		streamMockWorkersPath := writeAcceptingGoalMockWorkers(t)
		streamPrompt := fmt.Sprintf("quiet-mode-stream-baseline-%d", time.Now().UnixNano())
		streamArgs := appendGoalRunArgs(streamSession, streamMockWorkersPath, streamPrompt,
			"--output", "response-stream",
		)
		streamResult, err := runBuiltYouBinary(ctx, binaryPath, streamSession, streamArgs...)
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

		quietSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-success-config")
		quietMockWorkersPath := writeAcceptingGoalMockWorkers(t)
		quietPrompt := fmt.Sprintf("quiet-mode-success-%d", time.Now().UnixNano())
		quietArgs := appendGoalRunArgs(quietSession, quietMockWorkersPath, quietPrompt, "--quiet")
		quietResult, err := runBuiltYouBinary(ctx, binaryPath, quietSession, quietArgs...)
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
		verboseSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-verbose-config")
		verboseMockWorkersPath := writeAcceptingGoalMockWorkers(t)
		prompt := fmt.Sprintf("quiet-mode-verbose-baseline-%d", time.Now().UnixNano())
		verboseArgs := appendGoalRunArgs(verboseSession, verboseMockWorkersPath, prompt, "--verbose")
		verboseResult, err := runBuiltYouBinary(ctx, binaryPath, verboseSession, verboseArgs...)
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
		if !strings.HasSuffix(verboseResult.Stdout, successStdoutPrimaryResult) {
			t.Fatalf(
				"verbose stdout = %q, want primary result suffix %q",
				verboseResult.Stdout,
				successStdoutPrimaryResult,
			)
		}
		quietSession := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-verbose-contrast-config")
		quietMockWorkersPath := writeAcceptingGoalMockWorkers(t)
		quietPrompt := fmt.Sprintf("quiet-mode-verbose-contrast-%d", time.Now().UnixNano())
		quietArgs := appendGoalRunArgs(quietSession, quietMockWorkersPath, quietPrompt, "--quiet")
		quietResult, err := runBuiltYouBinary(ctx, binaryPath, quietSession, quietArgs...)
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
		session := newConfiguredGoalSession(t, ctx, binaryPath, harness, "quiet-mode-failure-config")
		rejectingMockWorkersPath := writeRejectingGoalMockWorkers(t)
		quietFailureArgs := appendGoalRunArgs(
			session,
			rejectingMockWorkersPath,
			fmt.Sprintf("quiet-mode-failure-%d", time.Now().UnixNano()),
			"--quiet",
		)
		failureResult, err := runBuiltYouBinary(ctx, binaryPath, session, quietFailureArgs...)
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

func newConfiguredGoalSession(
	t testing.TB,
	ctx context.Context,
	binaryPath string,
	harness *builtcliacceptance.Harness,
	scenario string,
) *builtcliacceptance.Session {
	t.Helper()
	session := harness.NewSession(t).WithNoExternalServer(t)
	initOutcome := initializeOperatorConfig(t, ctx, binaryPath, session, scenario)
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if err := os.WriteFile(initOutcome.ConfigPath, configBody, 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", initOutcome.ConfigPath, err)
	}
	return session
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
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
	)
	args = append(args, extraArgs...)
	args = append(args, prompt)
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
