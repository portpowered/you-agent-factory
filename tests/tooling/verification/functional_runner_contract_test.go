//go:build !windows

package verification

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestFunctionalCoverageCommandSmoke_ReportsExplicitTierSelection proves the
// PR and merge tiers expose their trigger, budget, short-mode choice, and
// subtractive quarantine selection through the observable Make command.
func TestFunctionalCoverageCommandSmoke_ReportsExplicitTierSelection(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	goStub := writeMakeEchoScript(t, "stub-go-functional-tier")

	for _, tc := range []struct {
		name    string
		tier    string
		trigger string
		budget  string
		short   string
		wantRun string
	}{
		{
			name:    "pull request short",
			tier:    "pr-short",
			trigger: "pull_request",
			budget:  "35m",
			short:   "true",
		},
		{
			name:    "main merge full",
			tier:    "merge-full",
			trigger: "push-main",
			budget:  "75m",
			short:   "false",
			wantRun: "-short=false",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
				"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
			})
			output, err := runMakefileTargetWithArgs(
				repoRoot,
				makefilePath,
				"FUNCTIONAL_TEST_TIER="+tc.tier,
				"FUNCTIONAL_TEST_TRIGGER="+tc.trigger,
				"FUNCTIONAL_TEST_BUDGET="+tc.budget,
				"FUNCTIONAL_SHORT="+tc.short,
				"FUNCTIONAL_QUARANTINE=tests/functional/functional-quarantine.json",
				"GO="+goStub,
				"test-functional-coverage",
			)
			if err != nil {
				t.Fatalf("run %s tier: %v\n%s", tc.name, err, output)
			}

			wantHeader := fmt.Sprintf(
				"Functional tier: name=%s trigger=%s short=%s budget=%s selection=subtractive quarantine=tests/functional/functional-quarantine.json",
				tc.tier,
				tc.trigger,
				tc.short,
				tc.budget,
			)
			if !strings.Contains(output, wantHeader) {
				t.Fatalf("tier output missing explicit selection header %q:\n%s", wantHeader, output)
			}
			if tc.wantRun != "" && !strings.Contains(output, tc.wantRun) {
				t.Fatalf("full tier did not disable short mode with %q:\n%s", tc.wantRun, output)
			}
			if tc.wantRun == "" && strings.Contains(output, "-short=false") {
				t.Fatalf("short tier unexpectedly disabled short mode:\n%s", output)
			}
		})
	}
}

// TestCoverageTargetsSmoke_UseBlockingFloorPolicy proves unit and functional
// coverage inherit the checker default used by required CI. Any staged
// exceptions are scoped to manifest floor holds, not an advisory invocation.
func TestCoverageTargetsSmoke_UseBlockingFloorPolicy(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	goStub := writeMakeEchoScript(t, "stub-go-coverage-policy")

	for _, target := range []string{"test-unit-coverage", "test-functional-coverage"} {
		t.Run(target, func(t *testing.T) {
			makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
				"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
			})
			output, err := runMakefileTargetWithArgs(
				repoRoot,
				makefilePath,
				fmt.Sprintf("GO=%s", goStub),
				target,
			)
			if err != nil {
				t.Fatalf("run %s: %v\n%s", target, err, output)
			}
			if !strings.Contains(output, "-package-floor-policy blocking") {
				t.Fatalf("%s did not forward the blocking policy to gocoveragecheck:\n%s", target, output)
			}
		})
	}
}

// TestFunctionalCoverageCommandSmoke_DefersOrdinaryGocoverageFailure proves
// the CI-only handoff preserves gocoveragecheck's exact exit code while
// allowing the compact verdict step to own an ordinary exit-1 outcome.
func TestFunctionalCoverageCommandSmoke_DefersOrdinaryGocoverageFailure(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	goStub := writeExecutableScript(t, "stub-go-functional-failure", `#!/bin/sh
printf '%s\n' 'Functional suite inventory: discovered-packages=1 observed-packages=1 (pass=0 fail=1 skip=0) top-level-tests=1 (pass=0 fail=1 skip=0)'
printf '%s\n' 'coverage not evaluated: 1 failed tests observed; package floors were NOT checked because the coverage test run failed' >&2
exit 1
`)
	exitPath := filepath.Join(t.TempDir(), "gocoveragecheck-exit-code.txt")
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
	})

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		fmt.Sprintf("GO=%s", goStub),
		fmt.Sprintf("FUNCTIONAL_GOCOVERAGE_EXIT_FILE=%s", filepath.ToSlash(exitPath)),
		"test-functional-coverage",
	)
	if err != nil {
		t.Fatalf("ordinary gocoveragecheck failure was not deferred: %v\n%s", err, output)
	}
	gotExit, readErr := os.ReadFile(exitPath)
	if readErr != nil {
		t.Fatalf("read recorded gocoveragecheck exit code: %v", readErr)
	}
	if strings.TrimSpace(string(gotExit)) != "1" {
		t.Fatalf("recorded gocoveragecheck exit code = %q, want 1", gotExit)
	}
	if !strings.Contains(output, "coverage not evaluated:") {
		t.Fatalf("deferred gocoveragecheck diagnostics were not preserved:\n%s", output)
	}
}

// TestFunctionalTestVizLaneScriptSmoke_TimesOutAndRetainsDiagnostics proves CI
// bounds the single Make-owned functional report without a second runner.
func TestFunctionalTestVizLaneScriptSmoke_TimesOutAndRetainsDiagnostics(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	workflow, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	body := string(workflow)
	functionalStep := "- name: Run Linux functional coverage with concurrent quarantine verification"
	start := strings.Index(body, functionalStep)
	if start < 0 {
		t.Fatalf("functional coverage workflow step is missing")
	}
	section := body[start:]
	if next := strings.Index(section[len(functionalStep):], "\n      - name:"); next >= 0 {
		section = section[:len(functionalStep)+next]
	}
	for _, required := range []string{
		"timeout-minutes: 75",
		"run: bash scripts/ci/run-functional-coverage-with-quarantine.sh",
	} {
		if !strings.Contains(section, required) {
			t.Fatalf("functional coverage workflow step missing %q:\n%s", required, section)
		}
	}
}
