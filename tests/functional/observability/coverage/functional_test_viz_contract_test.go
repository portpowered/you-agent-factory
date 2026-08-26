//go:build !windows

package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	functionalTestVizDefaultProfile  = ".artifacts/functional-test-viz/coverage.out"
	functionalTestVizDefaultJSON     = ".artifacts/functional-test-viz/coverage-summary.json"
	functionalTestVizDefaultMarkdown = ".artifacts/functional-test-viz/functional-tests.md"
	functionalTestVizDefaultLog      = ".artifacts/functional-test-viz/command.log"
)

// TestFunctionalTestVizContract_DefaultWiringDryRun proves make functional-test-viz
// wires boundary → one functional coverage run with the planned default profile and
// -json-output paths → Markdown generator, without executing the full suite.
func TestFunctionalTestVizContract_DefaultWiringDryRun(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := filepath.ToSlash(filepath.Join(repoRoot, "Makefile"))

	output, err := runMakefileTargetWithArgs(repoRoot, makefilePath, "functional-test-viz", "-n")
	if err != nil {
		t.Fatalf("dry-run functional-test-viz: %v\n%s", err, output)
	}

	for _, marker := range []string{
		"run ./cmd/functionaltestviz",
		"-run-suite",
		"-coverage-summary \"" + functionalTestVizDefaultJSON + "\"",
		"-profile \"" + functionalTestVizDefaultProfile + "\"",
		"-output \"" + functionalTestVizDefaultMarkdown + "\"",
		"-log \"" + functionalTestVizDefaultLog + "\"",
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("functional-test-viz dry-run missing wiring marker %q:\n%s", marker, output)
		}
	}
	if count := strings.Count(output, "run ./cmd/functionaltestviz"); count != 1 {
		t.Fatalf("functional-test-viz should invoke one Go runner, found %d:\n%s", count, output)
	}
}

// TestFunctionalTestVizContract_WiresBoundarySingleCoverageThenMarkdown proves the
// live Make composition runs boundary, exactly one stubbed coverage invocation with
// the planned artifact env paths, then the Markdown generator—without the full
// functional suite.
func TestFunctionalTestVizContract_WiresBoundarySingleCoverageThenMarkdown(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := filepath.ToSlash(filepath.Join(repoRoot, "Makefile"))
	artifactDir := t.TempDir()
	jobSummaryPath := filepath.Join(artifactDir, "job-summary.md")
	goStub := writeFunctionalTestGoStub(t, "success")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_GO="+goStub,
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
		"GITHUB_STEP_SUMMARY="+filepath.ToSlash(jobSummaryPath),
	)
	if err != nil {
		t.Fatalf("run functional-test-viz wiring contract: %v\n%s", err, output)
	}

	for _, expected := range []string{
		"Functional coverage for pkg/:\npkg/alpha 80.0%",
		"Functional package latencies:\ntests/functional/work/submit 0.125s",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("functional-test-viz console summary missing %q:\n%s", expected, output)
		}
	}
	for _, noisy := range []string{"stub:boundary-ok", "stub:coverage-once", "stub-go:", "outcome=pass", "TestSubmit"} {
		if strings.Contains(output, noisy) {
			t.Fatalf("functional-test-viz console summary contains noisy output %q:\n%s", noisy, output)
		}
	}

	logBody, readErr := os.ReadFile(filepath.Join(artifactDir, "command.log"))
	if readErr != nil {
		t.Fatalf("read functional-test-viz command log: %v", readErr)
	}
	log := string(logBody)
	assertOutputOrder(t, log, "stub:boundary-ok", "Functional suite inventory:", "stub:catalog-ok")
	jobSummary, readErr := os.ReadFile(jobSummaryPath)
	if readErr != nil {
		t.Fatalf("read integrated functional job summary: %v", readErr)
	}
	if !strings.Contains(string(jobSummary), "# Functional tests") {
		t.Fatalf("integrated functional job summary missing rendered Markdown:\n%s", jobSummary)
	}
}

func writeFunctionalTestGoStub(t *testing.T, outcome string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "functional-go-stub")
	body := `#!/bin/sh
case "$*" in
  *functionalboundarycheck*)
    printf '%s\n' 'stub:boundary-ok'
    exit 0
    ;;
  *gocoveragecheck*)
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -profile) profile="$2"; shift 2 ;;
        -json-output) coverage="$2"; shift 2 ;;
        -timing-output) timing="$2"; shift 2 ;;
        *) shift ;;
      esac
    done
    mkdir -p "$(dirname "$coverage")"
    printf '%s\n' 'mode: set' > "$profile"
    printf '%s\n' '{"packages":[{"package":"github.com/portpowered/infinite-you/pkg/alpha","coveragePercent":80}]}' > "$coverage"
    printf '%s\n' '{"version":1,"complete":true,"wallSeconds":0.125,"packageElapsedSecondsSum":0.125,"expectedPackageCount":1,"packageCount":1,"testCount":1,"testPassCount":1,"testFailCount":0,"testSkipCount":0,"packages":[{"package":"github.com/portpowered/infinite-you/tests/functional/work/submit","seconds":0.125,"outcome":"pass"}],"tests":[{"package":"github.com/portpowered/infinite-you/tests/functional/work/submit","test":"TestSubmit","seconds":0.125,"outcome":"pass"}]}' > "$timing"
    printf '%s\n' 'Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=1 fail=0 skip=0) top-level-tests=1 (pass=1 fail=0 skip=0) deferred-short-tests=0 wall=0.125s complete=true'
    printf '%s\n' 'Go coverage 80.0% meets minimum 33.1%.'
    exit 0
    ;;
  *functionaltestviz*)
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -output) output="$2"; shift 2 ;;
        *) shift ;;
      esac
    done
    printf '%s\n' '# Functional tests' > "$output"
    printf '%s\n' 'stub:catalog-ok'
    exit 0
    ;;
esac
printf '%s\n' 'unexpected Go command' >&2
exit 99
`
	if outcome == "boundary-fail" {
		body = strings.Replace(body, "printf '%s\\n' 'stub:boundary-ok'\n    exit 0", "printf '%s\\n' 'stub:boundary-"+outcome+"'\n    exit 23", 1)
	}
	if outcome == "coverage-fail" {
		body = strings.Replace(body, "printf '%s\\n' 'Go coverage 80.0% meets minimum 33.1%.'\n    exit 0", "printf '%s\\n' 'stub:coverage-floor-fail'\n    exit 17", 1)
	}
	if outcome == "render-fail" {
		body = strings.Replace(body, "printf '%s\\n' '# Functional tests' > \"$output\"\n    printf '%s\\n' 'stub:catalog-ok'\n    exit 0", "printf '%s\\n' 'stub:catalog-fail'\n    exit 19", 1)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write functional Go stub: %v", err)
	}
	return filepath.ToSlash(path)
}

// TestFunctionalTestVizContract_DefaultWiringIgnoresAmbientCIVizDir proves the
// default-path contract still holds when the suite process inherits
// FUNCTIONAL_TEST_VIZ_DIR from required Backend Functional Coverage CI.
func TestFunctionalTestVizContract_DefaultWiringIgnoresAmbientCIVizDir(t *testing.T) {
	t.Setenv("FUNCTIONAL_TEST_VIZ_DIR", ".artifacts/backend-functional-coverage")
	t.Setenv("GO_FUNCTIONAL_COVERAGE_PROFILE", ".artifacts/backend-functional-coverage/coverage.out")
	t.Setenv("GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT", ".artifacts/backend-functional-coverage/coverage-summary.json")

	// Reuse the dry-run contract assertions; helpers scrub ambient overrides.
	TestFunctionalTestVizContract_DefaultWiringDryRun(t)
}
