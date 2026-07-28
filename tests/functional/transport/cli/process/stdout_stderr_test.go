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
