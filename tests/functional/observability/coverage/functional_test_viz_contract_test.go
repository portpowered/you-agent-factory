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

	if !strings.Contains(output, "test-functional-coverage") {
		t.Fatalf("functional-test-viz dry-run missing the canonical coverage target:\n%s", output)
	}
	for _, marker := range []string{
		"GO_FUNCTIONAL_COVERAGE_PROFILE=" + functionalTestVizDefaultProfile,
		"GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT=" + functionalTestVizDefaultJSON,
		"run ./cmd/functionaltestviz",
		"-coverage-summary " + functionalTestVizDefaultJSON,
		"-output " + functionalTestVizDefaultMarkdown,
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("functional-test-viz dry-run missing wiring marker %q:\n%s", marker, output)
		}
	}
	assertOutputOrder(t, output,
		"test-functional-coverage",
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
		"test-functional-coverage": "@$(MAKE) functional-boundary-check\n" +
			"\t@mkdir -p $(FUNCTIONAL_TEST_VIZ_DIR)\n" +
			"\t@printf '%s\\n' '{\"packages\":[{\"package\":\"github.com/portpowered/infinite-you/pkg/alpha\",\"coveragePercent\":80}]}' > $(FUNCTIONAL_TEST_VIZ_JSON)\n" +
			"\t@printf '%s\\n' '{\"tests\":[{\"package\":\"github.com/portpowered/infinite-you/tests/functional/work/submit\",\"test\":\"TestSubmit\",\"seconds\":0.125,\"outcome\":\"pass\"}]}' > $(FUNCTIONAL_TEST_VIZ_TIMING)\n" +
			"\t@printf '%s\\n' 'stub:coverage-once'\n" +
			"\t@printf '%s\\n' 'profile=$(GO_FUNCTIONAL_COVERAGE_PROFILE)'\n" +
			"\t@printf '%s\\n' 'json=$(GO_FUNCTIONAL_COVERAGE_JSON_OUTPUT)'\n",
	})
	goStub := writeMakeEchoScript(t, "stub-go")
	nodeStub := filepath.Join(t.TempDir(), "stub-node")
	if err := os.WriteFile(nodeStub, []byte("#!/bin/sh\nprintf '%s\\n' 'Functional coverage for pkg/:' 'pkg/alpha 80.0%' '' 'Functional test latencies:' 'tests/functional/work/submit TestSubmit 0.125s'\n"), 0o755); err != nil {
		t.Fatalf("write console summary stub: %v", err)
	}

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"GO="+goStub,
		"NODE="+filepath.ToSlash(nodeStub),
	)
	if err != nil {
		t.Fatalf("run functional-test-viz wiring contract: %v\n%s", err, output)
	}

	for _, expected := range []string{
		"Functional coverage for pkg/:\npkg/alpha 80.0%",
		"Functional test latencies:\ntests/functional/work/submit TestSubmit 0.125s",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("functional-test-viz console summary missing %q:\n%s", expected, output)
		}
	}
	for _, noisy := range []string{"stub:boundary-ok", "stub:coverage-once", "stub-go:", "outcome=pass"} {
		if strings.Contains(output, noisy) {
			t.Fatalf("functional-test-viz console summary contains noisy output %q:\n%s", noisy, output)
		}
	}

	logBody, readErr := os.ReadFile(filepath.Join(repoRoot, functionalTestVizDefaultLog))
	if readErr != nil {
		t.Fatalf("read functional-test-viz command log: %v", readErr)
	}
	log := string(logBody)
	assertOutputOrder(t, log, "stub:boundary-ok", "stub:coverage-once", "stub-go:")
	if !strings.Contains(log, "profile="+functionalTestVizDefaultProfile) ||
		!strings.Contains(log, "json="+functionalTestVizDefaultJSON) {
		t.Fatalf("coverage invocation log missing default artifact paths:\n%s", log)
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
