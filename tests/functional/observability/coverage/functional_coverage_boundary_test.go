//go:build !windows

package coverage

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestFunctionalCoverageCommandSmoke_RunsBoundaryBeforeCoverage proves the
// required functional coverage Make surface runs functional-boundary-check
// successfully before invoking gocoveragecheck, without executing the full
// functional suite.
func TestFunctionalCoverageCommandSmoke_RunsBoundaryBeforeCoverage(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
	})
	goStub := writeMakeEchoScript(t, "stub-go")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"test-functional-coverage",
		"GO="+goStub,
	)
	if err != nil {
		t.Fatalf("run test-functional-coverage wrapper: %v\n%s", err, output)
	}

	assertOutputOrder(t, output,
		"stub:boundary-ok",
		"stub-go:",
	)
	if !strings.Contains(output, "run ./cmd/gocoveragecheck") {
		t.Fatalf("test-functional-coverage did not invoke gocoveragecheck after boundary:\n%s", output)
	}
	if !strings.Contains(output, "-suite functional") {
		t.Fatalf("test-functional-coverage missing functional suite flag:\n%s", output)
	}
	if count := strings.Count(output, "stub:boundary-ok"); count != 1 {
		t.Fatalf("expected boundary check exactly once, found %d:\n%s", count, output)
	}
}

// TestFunctionalCoverageCommandSmoke_BoundaryFailureStopsCoverage proves a
// failing functional-boundary-check exits non-zero and does not start the
// coverage suite.
func TestFunctionalCoverageCommandSmoke_BoundaryFailureStopsCoverage(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-fail'\n\t@exit 23\n",
	})
	goStub := writeMakeEchoScript(t, "stub-go")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"test-functional-coverage",
		"GO="+goStub,
	)
	if err == nil {
		t.Fatalf("test-functional-coverage unexpectedly succeeded after boundary failure:\n%s", output)
	}
	if !strings.Contains(output, "stub:boundary-fail") {
		t.Fatalf("test-functional-coverage output missing boundary failure marker:\n%s", output)
	}
	if strings.Contains(output, "stub-go:") {
		t.Fatalf("test-functional-coverage continued into gocoveragecheck after boundary failure:\n%s", output)
	}
}

// TestBackendCoverageAliasesSmoke_UnitLaneDoesNotRequireFunctionalBoundary
// proves the unit coverage matrix alias remains independent of the functional
// boundary prerequisite.
func TestBackendCoverageAliasesSmoke_UnitLaneDoesNotRequireFunctionalBoundary(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-unexpected'\n\t@exit 99\n",
		"test-unit-coverage":        "@printf '%s\\n' 'stub:test-unit-coverage'\n",
		"test-functional-coverage":  "@printf '%s\\n' 'stub:test-functional-coverage'\n",
	})

	coverageOutput, err := runMakefileTarget(repoRoot, makefilePath, "test-backend-coverage")
	if err != nil {
		t.Fatalf("run test-backend-coverage wrapper: %v\n%s", err, coverageOutput)
	}
	if count := strings.Count(coverageOutput, "stub:test-unit-coverage"); count != 1 {
		t.Fatalf("test-backend-coverage should delegate to unit coverage exactly once, found %d:\n%s", count, coverageOutput)
	}
	if strings.Contains(coverageOutput, "stub:boundary-unexpected") {
		t.Fatalf("unit coverage alias unexpectedly ran functional-boundary-check:\n%s", coverageOutput)
	}
	if strings.Contains(coverageOutput, "stub:test-functional-coverage") {
		t.Fatalf("test-backend-coverage unexpectedly ran functional coverage:\n%s", coverageOutput)
	}
}
