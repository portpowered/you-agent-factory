//go:build !windows

package coverage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	functionalTestVizDefaultProfile  = ".artifacts/functional-test-viz/coverage.out"
	functionalTestVizDefaultJSON     = ".artifacts/functional-test-viz/coverage-summary.json"
	functionalTestVizDefaultMarkdown = ".artifacts/functional-test-viz/functional-tests.md"
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

	if !strings.Contains(output, "functional-boundary-check") && !strings.Contains(output, "functionalboundarycheck") {
		t.Fatalf("functional-test-viz dry-run missing functional-boundary-check:\n%s", output)
	}
	if count := strings.Count(output, "run ./cmd/gocoveragecheck"); count != 1 {
		t.Fatalf("functional-test-viz must invoke gocoveragecheck exactly once, found %d:\n%s", count, output)
	}
	if !strings.Contains(output, "-suite functional") {
		t.Fatalf("functional-test-viz dry-run missing functional suite flag:\n%s", output)
	}
	for _, marker := range []string{
		"GO_FUNCTIONAL_COVERAGE_PROFILE=" + functionalTestVizDefaultProfile,
		"GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT=" + functionalTestVizDefaultJSON,
		"-profile " + functionalTestVizDefaultProfile,
		"-json-output " + functionalTestVizDefaultJSON,
		"run ./cmd/functionaltestviz",
		"-coverage-summary " + functionalTestVizDefaultJSON,
		"-output " + functionalTestVizDefaultMarkdown,
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("functional-test-viz dry-run missing wiring marker %q:\n%s", marker, output)
		}
	}
	assertOutputOrder(t, output,
		"functional-boundary-check",
		"run ./cmd/gocoveragecheck",
		"run ./cmd/functionaltestviz",
	)
}

// TestFunctionalTestVizContract_WiresBoundarySingleCoverageThenMarkdown proves the
// live Make composition runs boundary, exactly one stubbed coverage invocation with
// the planned artifact env paths, then the Markdown generator—without the full
// functional suite.
func TestFunctionalTestVizContract_WiresBoundarySingleCoverageThenMarkdown(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
		"test-functional-coverage": "@printf '%s\\n' 'stub:coverage-once'\n" +
			"\t@printf '%s\\n' 'profile=$(GO_FUNCTIONAL_COVERAGE_PROFILE)'\n" +
			"\t@printf '%s\\n' 'json=$(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT)'\n",
	})
	goStub := writeMakeEchoScript(t, "stub-go")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"GO="+goStub,
	)
	if err != nil {
		t.Fatalf("run functional-test-viz wiring contract: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"stub:boundary-ok",
		"stub:coverage-once",
		"stub-go:",
	)
	if count := strings.Count(output, "stub:boundary-ok"); count != 1 {
		t.Fatalf("expected boundary check exactly once at the viz surface, found %d:\n%s", count, output)
	}
	if count := strings.Count(output, "stub:coverage-once"); count != 1 {
		t.Fatalf("expected a single functional coverage invocation, found %d:\n%s", count, output)
	}
	if !strings.Contains(output, "run ./cmd/functionaltestviz") {
		t.Fatalf("functional-test-viz did not invoke Markdown generator:\n%s", output)
	}
	if !strings.Contains(output, "profile="+functionalTestVizDefaultProfile) {
		t.Fatalf("coverage invocation missing default profile path:\n%s", output)
	}
	if !strings.Contains(output, "json="+functionalTestVizDefaultJSON) {
		t.Fatalf("coverage invocation missing default JSON path:\n%s", output)
	}
	if !strings.Contains(output, "-coverage-summary "+functionalTestVizDefaultJSON) {
		t.Fatalf("Markdown generator missing default coverage-summary path:\n%s", output)
	}
	if !strings.Contains(output, "-output "+functionalTestVizDefaultMarkdown) {
		t.Fatalf("Markdown generator missing default output path:\n%s", output)
	}
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
