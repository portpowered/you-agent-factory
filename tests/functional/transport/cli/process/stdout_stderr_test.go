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
