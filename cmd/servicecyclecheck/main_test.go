package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runFixture executes the checker against a synthetic repository root and
// returns its stdout, stderr and error.
func runFixture(t *testing.T, files []fixtureFile, ceiling int) (string, string, error) {
	t.Helper()

	root := writeFixtureRepo(t, ratchetFixtureServices, files)
	var stdout, stderr bytes.Buffer
	err := run(config{root: root, ceilingFile: writeFixtureCeiling(t, root, ceiling)}, &stdout, &stderr)
	return stdout.String(), stderr.String(), err
}

func TestRunPassesWhenTheMeasuredWeightMatchesTheCeiling(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runFixture(t, ratchetFixtureFiles(), 2)
	if err != nil {
		t.Fatalf("run returned an error: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("passing run wrote to stderr:\n%s", stderr)
	}
	for _, want := range []string{diagnosticPrefix, "minimum feedback arc weight 2", "matches ceiling 2", "6 service(s)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout %q does not contain %q", stdout, want)
		}
	}
	if strings.Contains(stdout, "LINT_VIOLATION_COUNT") {
		t.Fatalf("a passing run must not emit a violation count, got:\n%s", stdout)
	}
}

func TestRunFailsWhenAnAddedBackEdgeDeepensTheCycle(t *testing.T) {
	t.Parallel()

	_, stderr, err := runFixture(t, withAddedBackEdge(), 2)
	if err == nil {
		t.Fatalf("expected a regression failure, got success:\n%s", stderr)
	}

	for _, want := range []string{
		"cross-service cycle regression",
		"minimum feedback arc weight is 3",
		"above the recorded ceiling of 2",
		"zeta -> epsilon (weight 1)",
		"carrier package: pkg/services/zeta/internal/adapter",
		"LINT_VIOLATION_COUNT: 1",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("regression output does not contain %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "Lower the ceiling") {
		t.Fatalf("a regression must not tell the maintainer to lower the ceiling:\n%s", stderr)
	}
	if !strings.Contains(err.Error(), "does not match the recorded ceiling 2") {
		t.Fatalf("summary error = %v", err)
	}
}

func TestRunFailsWhenARemovedBackEdgeLeavesTheCeilingUnclaimed(t *testing.T) {
	t.Parallel()

	_, stderr, err := runFixture(t, withRemovedBackEdge(), 2)
	if err == nil {
		t.Fatalf("expected an unclaimed-improvement failure, got success:\n%s", stderr)
	}

	for _, want := range []string{
		"cross-service cycle improved and the gain is not captured",
		"minimum feedback arc weight is 1",
		"below the recorded ceiling of 2",
		"Lower the ceiling to 1",
		"LINT_VIOLATION_COUNT: 1",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("unclaimed-improvement output does not contain %q:\n%s", want, stderr)
		}
	}
	if !strings.Contains(stderr, "cut set (1 edge(s), total weight 1)") {
		t.Fatalf("improvement output must still print the remaining cut set:\n%s", stderr)
	}
}

func TestRunReportsTheDriftMagnitudeAsTheViolationCount(t *testing.T) {
	t.Parallel()

	_, stderr, err := runFixture(t, withAddedBackEdge(), 0)
	if err == nil {
		t.Fatal("expected a regression failure, got success")
	}
	if !strings.Contains(stderr, "LINT_VIOLATION_COUNT: 3") {
		t.Fatalf("violation count must be the drift magnitude (3 - 0), got:\n%s", stderr)
	}
}

func TestRunRejectsAnUnreadableOrInvalidCeilingBaseline(t *testing.T) {
	t.Parallel()

	root := writeFixtureRepo(t, ratchetFixtureServices, ratchetFixtureFiles())
	var stdout, stderr bytes.Buffer

	missing := filepath.Join(root, "absent.json")
	if err := run(config{root: root, ceilingFile: missing}, &stdout, &stderr); err == nil {
		t.Fatal("expected an error for a missing ceiling baseline, got none")
	}

	malformed := filepath.Join(root, "malformed.json")
	if err := os.WriteFile(malformed, []byte("{"), 0o600); err != nil {
		t.Fatalf("write malformed ceiling: %v", err)
	}
	if err := run(config{root: root, ceilingFile: malformed}, &stdout, &stderr); err == nil {
		t.Fatal("expected an error for a malformed ceiling baseline, got none")
	}

	negative := filepath.Join(root, "negative.json")
	if err := os.WriteFile(negative, []byte(`{"ceiling":-1}`), 0o600); err != nil {
		t.Fatalf("write negative ceiling: %v", err)
	}
	if err := run(config{root: root, ceilingFile: negative}, &stdout, &stderr); err == nil {
		t.Fatal("expected an error for a negative ceiling, got none")
	}
}

func TestCeilingPathDefaultsToTheRepositoryBaseline(t *testing.T) {
	t.Parallel()

	got := config{root: "somewhere"}.ceilingPath()
	want := filepath.Join("somewhere", "docs", "internal", "baselines", "service-cycle-ceiling.json")
	if got != want {
		t.Fatalf("default ceiling path = %q, want %q", got, want)
	}
	if override := (config{root: "somewhere", ceilingFile: "explicit.json"}).ceilingPath(); override != "explicit.json" {
		t.Fatalf("explicit ceiling path = %q, want %q", override, "explicit.json")
	}
}

// TestRunAgainstTheRepositoryMatchesTheRecordedCeiling is the live gate: it
// runs the checker over this repository with the committed baseline, which is
// exactly what `make service-cycle` does in CI.
func TestRunAgainstTheRepositoryMatchesTheRecordedCeiling(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if err := run(config{root: repositoryRoot(t)}, &stdout, &stderr); err != nil {
		t.Fatalf("service cycle ratchet failed against the repository: %v\n%s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "matches ceiling") {
		t.Fatalf("repository run did not report the ceiling comparison:\n%s", stdout.String())
	}
}

// repositoryRoot walks up from the test's working directory to the module root.
func repositoryRoot(t *testing.T) string {
	t.Helper()

	directory, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("could not locate the module root from the test working directory")
		}
		directory = parent
	}
}

// TestRunFailsOnASingleBackEdgeAtAZeroCeiling proves a ceiling of 0 is a real
// gate rather than a value the checker degenerates on: one back-edge anywhere
// in the service graph fails the run and is named in the cut set.
func TestRunFailsOnASingleBackEdgeAtAZeroCeiling(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runFixture(t, withSingleBackEdge(), 0)
	if err == nil {
		t.Fatalf("a single back-edge must fail at ceiling 0, got success:\n%s", stdout)
	}

	for _, want := range []string{
		"cross-service cycle regression",
		"minimum feedback arc weight is 1",
		"above the recorded ceiling of 0",
		"cut set (1 edge(s), total weight 1)",
		"gamma -> alpha (weight 1)",
		"carrier package: pkg/services/gamma/transports/http",
		"LINT_VIOLATION_COUNT: 1",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("zero-ceiling regression output does not contain %q:\n%s", want, stderr)
		}
	}
	if strings.Contains(stderr, "Lower the ceiling") {
		t.Fatalf("a back-edge at ceiling 0 must not be reported as an improvement:\n%s", stderr)
	}
}

// TestRunPassesOnAnAcyclicGraphAtAZeroCeiling is the other half of the
// boundary: at 0/0 the checker must succeed outright. A ratchet that reported
// an acyclic graph as an unclaimed improvement would be unsatisfiable once the
// program reached zero, because there would be no lower ceiling to move to.
func TestRunPassesOnAnAcyclicGraphAtAZeroCeiling(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := runFixture(t, acyclicFixtureFiles(), 0)
	if err != nil {
		t.Fatalf("an acyclic graph must pass at ceiling 0: %v\nstderr:\n%s", err, stderr)
	}
	if stderr != "" {
		t.Fatalf("a passing zero-ceiling run wrote to stderr:\n%s", stderr)
	}

	for _, want := range []string{
		diagnosticPrefix,
		"minimum feedback arc weight 0",
		"in 0 cut edge(s)",
		"matches ceiling 0",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("zero-ceiling success output does not contain %q:\n%s", want, stdout)
		}
	}
	for _, unwanted := range []string{"Lower the ceiling", "LINT_VIOLATION_COUNT", "cut set"} {
		if strings.Contains(stdout, unwanted) {
			t.Fatalf("a 0/0 run must not take the unclaimed-improvement branch, got %q in:\n%s", unwanted, stdout)
		}
	}
}
