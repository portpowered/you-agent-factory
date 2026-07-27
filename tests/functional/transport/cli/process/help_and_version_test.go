package process_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

var expectedPublicRootCommandFamilies = []string{
	"run",
	"docs",
	"session",
	"work",
	"factory",
	"submit",
	"init",
	"server",
}

var forbiddenRootDiscoveryCommands = []string{
	"batch",
	"list",
	"show",
	"serve",
	"validate",
	"save",
	"flatten",
	"expand",
	"query",
	"create",
	"delete",
	"pause",
	"resume",
	"dispatches",
	"move",
	"visualize",
	"inspect",
	"invoke",
	"pull",
	"replace-current",
}

// TestCLIHelpListsPublicCommandFamilies proves bare-root and explicit root help
// through the public built you CLI list supported public command families,
// omit hidden or nested-only discovery surfaces, and do not activate product
// side effects such as Factory load, server start, or dashboard lifecycle noise.
func TestCLIHelpListsPublicCommandFamilies(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "bare_root", args: nil},
		{name: "explicit_help", args: []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			result, err := session.Run(ctx, tc.args...)
			session.RequireSuccess(t, tc.name, result, err)

			if strings.TrimSpace(result.Stderr) != "" {
				t.Fatalf("root help stderr = %q, want empty", result.Stderr)
			}
			for _, forbidden := range []string{
				"Factory initiated:",
				"Dashboard URL:",
				"Dashboard server disabled",
			} {
				if strings.Contains(result.Stdout, forbidden) {
					t.Fatalf("root help stdout contains product activation marker %q:\n%s", forbidden, result.Stdout)
				}
			}

			if !strings.Contains(result.Stdout, "Available Commands:") {
				t.Fatalf("root help stdout omitted Available Commands section:\n%s", result.Stdout)
			}
			if !strings.Contains(result.Stdout, "Run and manage CPN-based workflow factories") {
				t.Fatalf("root help stdout omitted product title:\n%s", result.Stdout)
			}

			listed := parseListedRootCommands(result.Stdout)
			if len(listed) == 0 {
				t.Fatalf("root help did not list any command families:\n%s", result.Stdout)
			}

			for _, family := range expectedPublicRootCommandFamilies {
				if !containsString(listed, family) {
					t.Fatalf("root help missing public family %q; listed=%v\n%s", family, listed, result.Stdout)
				}
			}
			for _, forbidden := range forbiddenRootDiscoveryCommands {
				if containsString(listed, forbidden) {
					t.Fatalf("root help listed hidden or nested-only command %q; listed=%v\n%s", forbidden, listed, result.Stdout)
				}
			}

			if tc.name == "bare_root" && strings.Contains(result.Stdout, "How to use:") {
				t.Fatalf("bare root help emitted long-form discovery text:\n%s", result.Stdout)
			}

			assertRootHelpDiscoveryHasNoProductFilesystemEffects(t, session)
		})
	}

	bareListed := runRootHelpListedCommands(t, session, nil)
	helpListed := runRootHelpListedCommands(t, session, []string{"--help"})
	if !sameStringSet(bareListed, helpListed) {
		t.Fatalf("bare root and --help listed different command families:\nbare=%v\nhelp=%v", bareListed, helpListed)
	}
}

func runRootHelpListedCommands(
	t *testing.T,
	session *builtcliacceptance.Session,
	args []string,
) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx, args...)
	session.RequireSuccess(t, "root-help-listing", result, err)
	return parseListedRootCommands(result.Stdout)
}

func assertRootHelpDiscoveryHasNoProductFilesystemEffects(
	t *testing.T,
	session *builtcliacceptance.Session,
) {
	t.Helper()

	factoryPath := filepath.Join(session.WorkDir, "factory")
	if _, err := os.Stat(factoryPath); err == nil {
		t.Fatalf("root help created factory directory at %s", factoryPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat factory directory after root help: %v", err)
	}

	configPath := filepath.Join(session.HomeDir, ".you-agent-factory", "config.json")
	if _, err := os.Stat(configPath); err == nil {
		t.Fatalf("root help created operator config at %s", configPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat operator config after root help: %v", err)
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
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		names = append(names, fields[0])
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

func sameStringSet(left, right []string) bool {
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
