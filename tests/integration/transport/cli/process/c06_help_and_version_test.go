package process_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

var expectedPublicRootCommandFamilies = []string{
	"run", "docs", "session", "work", "factory", "submit", "init", "server",
}

var machineReadableVersionLinePattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.+-]+)*$|^dev$`)

var forbiddenRootDiscoveryCommands = []string{
	"batch", "list", "show", "validate", "save", "flatten", "expand", "query", "create", "delete",
	"pause", "resume", "dispatches", "move", "render", "inspect", "invoke", "pull", "replace-current",
}

// TestCLIHelpListsPublicCommandFamilies proves built-CLI root help lists the
// public command families without activating product runtime side effects.
func TestCLIHelpListsPublicCommandFamilies(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "bare_root"}, {name: "explicit_help", args: []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := harness.NewSession(t)
			result := runIntegrationCLI(t, binaryPath, session, tc.args...)
			if strings.TrimSpace(result.Stderr) != "" {
				t.Fatalf("root help stderr = %q, want empty", result.Stderr)
			}
			for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled"} {
				if strings.Contains(result.Stdout, forbidden) {
					t.Fatalf("root help stdout contains product activation marker %q:\n%s", forbidden, result.Stdout)
				}
			}
			if !strings.Contains(result.Stdout, "Available Commands:") || !strings.Contains(result.Stdout, "Run and manage CPN-based workflow factories") {
				t.Fatalf("root help omitted its public heading or title:\n%s", result.Stdout)
			}
			listed := parseListedRootCommands(result.Stdout)
			if len(listed) == 0 {
				t.Fatalf("root help did not list command families:\n%s", result.Stdout)
			}
			for _, family := range expectedPublicRootCommandFamilies {
				if !containsString(listed, family) {
					t.Fatalf("root help missing public family %q; listed=%v", family, listed)
				}
			}
			for _, forbidden := range forbiddenRootDiscoveryCommands {
				if containsString(listed, forbidden) {
					t.Fatalf("root help listed hidden command %q; listed=%v", forbidden, listed)
				}
			}
			if tc.name == "bare_root" && strings.Contains(result.Stdout, "How to use:") {
				t.Fatalf("bare root help emitted long-form discovery text:\n%s", result.Stdout)
			}
			assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
		})
	}

	bareListed := runRootHelpListedCommands(t, binaryPath, harness)
	helpListed := runRootHelpListedCommands(t, binaryPath, harness, "--help")
	if !sameHelpStringSet(bareListed, helpListed) {
		t.Fatalf("bare root and --help listed different command families:\nbare=%v\nhelp=%v", bareListed, helpListed)
	}
}

// TestCLISubcommandHelpUsesStableUsageAndExitZero proves representative built
// CLI nested help prints stable usage and avoids runtime or filesystem effects.
func TestCLISubcommandHelpUsesStableUsageAndExitZero(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "docs_help_flag", args: []string{"docs", "--help"}},
		{name: "docs_short_help", args: []string{"docs", "-h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			session := harness.NewSession(t)
			result := runIntegrationCLI(t, binaryPath, session, tc.args...)
			if strings.TrimSpace(result.Stderr) != "" {
				t.Fatalf("nested help stderr = %q, want empty", result.Stderr)
			}
			for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled"} {
				if strings.Contains(result.Stdout, forbidden) {
					t.Fatalf("nested help stdout contains product activation marker %q:\n%s", forbidden, result.Stdout)
				}
			}
			for _, marker := range []string{
				"Usage:\n  you docs [topic] [flags]",
				"Print packaged markdown reference topics from the installed binary.",
				"Flags:", "-h, --help", "help for docs",
			} {
				if !strings.Contains(result.Stdout, marker) {
					t.Fatalf("nested docs help omitted %q:\n%s", marker, result.Stdout)
				}
			}
			assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
		})
	}
}

// TestCLIVersionWritesOneMachineReadableVersion proves built-CLI version
// discovery emits one machine-readable token and no startup or help noise.
func TestCLIVersionWritesOneMachineReadableVersion(t *testing.T) {
	t.Parallel()
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	binaryPath := buildYouBinary(t, t.Context(), harness.RepoRoot)
	session := harness.NewSession(t)
	result := runIntegrationCLI(t, binaryPath, session, "--version")
	if strings.TrimSpace(result.Stderr) != "" {
		t.Fatalf("version stderr = %q, want empty", result.Stderr)
	}
	for _, forbidden := range []string{"Factory initiated:", "Dashboard URL:", "Dashboard server disabled", "Available Commands:", "How to use:"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Fatalf("version stdout contains startup or help noise %q:\n%s", forbidden, result.Stdout)
		}
	}
	versionLine := strings.TrimSpace(result.Stdout)
	if versionLine == "" || strings.Contains(versionLine, "\n") || !machineReadableVersionLinePattern.MatchString(versionLine) {
		t.Fatalf("version stdout = %q, want one machine-readable version token", result.Stdout)
	}
	assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
}

func runIntegrationCLI(t *testing.T, binaryPath string, session *builtcliacceptance.Session, args ...string) builtcliacceptance.RunResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
	session.RequireSuccess(t, "integration-cli", result, err)
	return result
}

func runRootHelpListedCommands(t *testing.T, binaryPath string, harness *builtcliacceptance.Harness, args ...string) []string {
	t.Helper()
	session := harness.NewSession(t)
	result := runIntegrationCLI(t, binaryPath, session, args...)
	return parseListedRootCommands(result.Stdout)
}

func assertRootHelpDiscoveryHasNoProductFilesystemEffects(t testing.TB, session *builtcliacceptance.Session) {
	t.Helper()
	for _, path := range []string{
		filepath.Join(session.WorkDir, "factory"),
		filepath.Join(session.HomeDir, ".you-agent-factory", "config.json"),
	} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("help created product filesystem path %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat product filesystem path %s: %v", path, err)
		}
	}
}

func parseListedRootCommands(help string) []string {
	const section = "Available Commands:"
	start := strings.Index(help, section)
	if start < 0 {
		return nil
	}
	rest := help[start+len(section):]
	if flagsIdx := strings.Index(rest, "\n\nFlags:"); flagsIdx >= 0 {
		rest = rest[:flagsIdx]
	} else if flagsIdx := strings.Index(rest, "\nFlags:"); flagsIdx >= 0 {
		rest = rest[:flagsIdx]
	}
	var names []string
	for _, line := range strings.Split(rest, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 {
			names = append(names, fields[0])
		}
	}
	return names
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameHelpStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftSet := make(map[string]struct{}, len(left))
	for _, value := range left {
		leftSet[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := leftSet[value]; !ok {
			return false
		}
	}
	return true
}
