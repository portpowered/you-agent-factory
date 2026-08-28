package process_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const unknownCommandProbeToken = "not-a-command"

// TestCLIUnknownCommandWritesCobraUsageStderr proves that mistyping a root
// command through the public built you CLI preserves Cobra's correction hint.
func TestCLIUnknownCommandWritesSafeCodedStderr(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := runBuiltYouBinary(ctx, binaryPath, session, unknownCommandProbeToken)
	if err == nil {
		t.Fatalf("unknown root command result = %#v; want process failure", result)
	}

	stderr := strings.TrimSpace(result.Stderr)
	if stderr == "" {
		t.Fatal("unknown command stderr was empty; want actionable diagnostic")
	}

	for _, want := range []string{
		`Error: unknown command "not-a-command" for "you"`,
		"Run 'you --help' for usage.",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("unknown-command stderr = %q, want Cobra text %q", stderr, want)
		}
	}
	if strings.Contains(stderr, "CLI_COMMAND_FAILED") || strings.Contains(stderr, "INTERNAL_SERVER_ERROR") {
		t.Fatalf("unknown-command stderr used an internal failure envelope: %q", stderr)
	}

	for _, forbidden := range forbiddenRootDiscoveryCommands {
		if containsSuggestionToken(stderr, forbidden) {
			t.Fatalf("unknown command stderr leaked hidden or nested-only command %q:\n%s", forbidden, stderr)
		}
	}

	for _, marker := range []string{
		"Factory initiated:",
		"Dashboard URL:",
		"Available Commands:",
	} {
		if strings.Contains(stderr, marker) {
			t.Fatalf("unknown command stderr contains non-diagnostic surface %q:\n%s", marker, stderr)
		}
	}
}

// TestCLIUnknownCommandReturnsUsageExitCode proves that rejecting a mistyped root
// command through the public built you CLI leaves stdout empty, returns the
// documented non-success usage/validation exit code, and does not start Factory
// load, server, or worker dispatch attributable to the rejected invocation.
func TestCLIUnknownCommandReturnsUsageExitCode(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := runBuiltYouBinary(ctx, binaryPath, session, unknownCommandProbeToken)
	if err == nil {
		t.Fatalf("unknown root command result = %#v; want process failure", result)
	}
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want documented usage/validation exit 1", result.ExitCode)
	}

	stdout := strings.TrimSpace(result.Stdout)
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty (no primary result, usage dump, or lifecycle chatter)", result.Stdout)
	}
	for _, forbidden := range []string{
		"Factory initiated:",
		"Dashboard URL:",
		"Dashboard server disabled",
		"Available Commands:",
		"How to use:",
	} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("stdout contains product activation or help surface %q:\n%s", forbidden, result.Stdout)
		}
	}

	assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
}

func containsSuggestionToken(stderr, command string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, field := range strings.Fields(line) {
			if strings.Trim(field, `"'`) == command {
				return true
			}
		}
	}
	return false
}
