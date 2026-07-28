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

// TestCLIUnknownCommandWritesActionableStderr proves that mistyping a root
// command through the public built you CLI writes stderr that names the invalid
// token and keeps any command guidance limited to customer-visible surfaces
// without leaking hidden or internal discovery commands.
func TestCLIUnknownCommandWritesActionableStderr(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx, unknownCommandProbeToken)
	if err == nil {
		t.Fatalf("unknown root command result = %#v; want process failure", result)
	}

	stderr := strings.TrimSpace(result.Stderr)
	if stderr == "" {
		t.Fatal("unknown command stderr was empty; want actionable diagnostic")
	}

	wantDiagnostic := `unknown command "` + unknownCommandProbeToken + `" for "you"`
	if strings.Count(stderr, wantDiagnostic) != 1 {
		t.Fatalf("stderr = %q, want exactly one diagnostic naming %q", stderr, unknownCommandProbeToken)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("stderr = %q, want unknown-command guidance", stderr)
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
