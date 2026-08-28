package process_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestBuiltCLIGroupHelpRendersExactlyOnce proves the production-built CLI
// emits one complete help document for affected generic groups and controls.
func TestBuiltCLIGroupHelpRendersExactlyOnce(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildContext, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	binaryPath := buildYouBinary(t, buildContext, harness.RepoRoot)

	for _, family := range []string{"factory", "config", "worker-sessions"} {
		family := family
		t.Run(family, func(t *testing.T) {
			long := runGroupHelpInvocation(t, binaryPath, harness, family, "--help")
			short := runGroupHelpInvocation(t, binaryPath, harness, family, "-h")
			if long.Stdout != short.Stdout || long.Stderr != short.Stderr {
				t.Fatalf("%s long and short help differ:\n--help stdout:\n%s\n-h stdout:\n%s\n--help stderr=%q\n-h stderr=%q", family, long.Stdout, short.Stdout, long.Stderr, short.Stderr)
			}
		})
	}

	for _, family := range []string{"models", "docs"} {
		family := family
		t.Run(family, func(t *testing.T) {
			runGroupHelpInvocation(t, binaryPath, harness, family, "--help")
		})
	}
}

func runGroupHelpInvocation(
	t *testing.T,
	binaryPath string,
	harness *builtcliacceptance.Harness,
	family string,
	flag string,
) builtcliacceptance.RunResult {
	t.Helper()

	session := harness.NewSession(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	result, err := runBuiltYouBinary(ctx, binaryPath, session, family, flag)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("you %s %s result=%#v error=%v; want exit 0", family, flag, result, err)
	}
	if result.Stderr != "" {
		t.Fatalf("you %s %s stderr=%q; want empty", family, flag, result.Stderr)
	}
	if strings.TrimSpace(result.Stdout) == "" {
		t.Fatalf("you %s %s stdout is empty; want complete help", family, flag)
	}
	if got := countExactProcessOutputLines(result.Stdout, "Usage:"); got != 1 {
		t.Fatalf("you %s %s Usage: line count = %d, want 1; stdout:\n%s", family, flag, got, result.Stdout)
	}
	if !strings.Contains(result.Stdout, "Usage:\n  you "+family) {
		t.Fatalf("you %s %s help omitted its command usage:\n%s", family, flag, result.Stdout)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("you %s %s help contains product activation marker %q:\n%s", family, flag, forbidden, result.Stdout)
		}
	}
	assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
	return result
}

func countExactProcessOutputLines(output, want string) int {
	count := 0
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if line == want {
			count++
		}
	}
	return count
}
