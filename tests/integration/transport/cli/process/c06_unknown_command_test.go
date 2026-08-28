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

// TestCLIUnknownCommandWritesSafeCodedStderr proves the built CLI preserves
// Cobra's safe correction hint without exposing internal failure envelopes.
func TestCLIUnknownCommandWritesSafeCodedStderr(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t)
	result := runIntegrationCLIAllowFailure(t, binaryPath, session, unknownCommandProbeToken)
	if strings.TrimSpace(result.Stderr) == "" {
		t.Fatal("unknown command stderr was empty; want actionable diagnostic")
	}
	for _, want := range []string{
		`Error: unknown command "not-a-command" for "you"`,
		"Run 'you --help' for usage.",
	} {
		if !strings.Contains(result.Stderr, want) {
			t.Fatalf("unknown-command stderr = %q, want Cobra text %q", result.Stderr, want)
		}
	}
	if strings.Contains(result.Stderr, "CLI_COMMAND_FAILED") || strings.Contains(result.Stderr, "INTERNAL_SERVER_ERROR") {
		t.Fatalf("unknown-command stderr used an internal failure envelope: %q", result.Stderr)
	}
	for _, forbidden := range forbiddenRootDiscoveryCommands {
		if containsSuggestionToken(result.Stderr, forbidden) {
			t.Fatalf("unknown command stderr leaked hidden command %q:\n%s", forbidden, result.Stderr)
		}
	}
	for _, marker := range []string{"Factory initiated:", "Dashboard URL:", "Available Commands:"} {
		if strings.Contains(result.Stderr, marker) {
			t.Fatalf("unknown command stderr contains non-diagnostic surface %q:\n%s", marker, result.Stderr)
		}
	}
}

// TestCLIUnknownCommandReturnsUsageExitCode proves a rejected built CLI
// command returns usage exit 1, keeps stdout empty, and avoids activation.
func TestCLIUnknownCommandReturnsUsageExitCode(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t)
	result := runIntegrationCLIAllowFailure(t, binaryPath, session, unknownCommandProbeToken)
	if result.ExitCode != 1 {
		t.Fatalf("exit code = %d, want documented usage/validation exit 1", result.ExitCode)
	}
	if strings.TrimSpace(result.Stdout) != "" {
		t.Fatalf("stdout = %q, want empty after unknown command", result.Stdout)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled", "Available Commands:", "How to use:"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("stdout contains product activation or help surface %q:\n%s", forbidden, result.Stdout)
		}
	}
	assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
}

func runIntegrationCLIAllowFailure(t *testing.T, binaryPath string, session *builtcliacceptance.Session, args ...string) builtcliacceptance.RunResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	if err == nil {
		t.Fatalf("built CLI result = %#v; want process failure", result)
	}
	return result
}

func containsSuggestionToken(stderr, command string) bool {
	for _, line := range strings.Split(stderr, "\n") {
		for _, field := range strings.Fields(line) {
			if strings.Trim(field, `"'`) == command {
				return true
			}
		}
	}
	return false
}
