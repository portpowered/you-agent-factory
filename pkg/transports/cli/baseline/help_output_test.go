package baseline_test

import (
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
)

const (
	rootHelpFixture = "testdata/root_help.txt"
	runHelpFixture  = "testdata/run_help.txt"
	docsHelpFixture = "testdata/docs_help.txt"
)

// FND-12 captured CLI success baseline: customer-visible `you --help` stdout
// matches the checked-in fixture. Invoked by `make fnd-12-cli-behavior-baselines`.
func TestRootHelpBaseline_MatchesFixture(t *testing.T) {
	assertHelpMatchesFixture(t, rootHelpFixture, []string{"--help"})
}

func TestRunHelpBaseline_MatchesFixture(t *testing.T) {
	assertHelpMatchesFixture(t, runHelpFixture, []string{"run", "--help"})
}

func TestDocsHelpBaseline_MatchesFixture(t *testing.T) {
	assertHelpMatchesFixture(t, docsHelpFixture, []string{"docs", "--help"})
}

func TestHelpBaselines_AreStableAcrossRuns(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{name: "root", args: []string{"--help"}},
		{name: "run", args: []string{"run", "--help"}},
		{name: "docs", args: []string{"docs", "--help"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, err := captureProductionHelp(t, tc.args)
			if err != nil {
				t.Fatalf("capture first help output: %v", err)
			}
			second, err := captureProductionHelp(t, tc.args)
			if err != nil {
				t.Fatalf("capture second help output: %v", err)
			}
			if first != second {
				t.Fatalf(
					"help output is not stable across repeated runs\n%s",
					formatHelpDiff(first, second),
				)
			}
		})
	}
}

func assertHelpMatchesFixture(t *testing.T, fixture string, args []string) {
	t.Helper()

	got, err := captureProductionHelp(t, args)
	if err != nil {
		t.Fatalf("capture help output: %v", err)
	}

	want, err := baseline.ReadFixtureText(fixtureSourceStore(), fixture)
	if err != nil {
		t.Fatalf("read help baseline fixture: %v", err)
	}

	if got == want {
		return
	}
	if os.Getenv("UPDATE_CLI_BASELINES") == "1" {
		if err := os.WriteFile(fixture, []byte(got), 0o600); err != nil {
			t.Fatalf("update help baseline fixture: %v", err)
		}
		return
	}

	t.Fatalf(
		"help baseline drift detected; update %s when intentional\n%s",
		fixture,
		formatHelpDiff(want, got),
	)
}

func captureProductionHelp(t testing.TB, args []string) (string, error) {
	t.Helper()
	output, err := executeProductionCLI(t, args...)
	return baseline.NormalizeHelpOutput(output), err
}

func formatHelpDiff(want, got string) string {
	var b strings.Builder
	b.WriteString("--- want ---\n")
	b.WriteString(want)
	if !strings.HasSuffix(want, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString("--- got ---\n")
	b.WriteString(got)
	if !strings.HasSuffix(got, "\n") {
		b.WriteByte('\n')
	}
	return b.String()
}
