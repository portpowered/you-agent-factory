//go:build !windows

package verification

import (
	"fmt"
	"os"
	"os/exec"
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

// TestFunctionalTestVizLaneScriptSmoke_TimesOutAndRetainsDiagnostics proves a
// budget expiry fails the process while preserving output produced before the
// interruption, including inventory and quarantine diagnostics.
func TestFunctionalTestVizLaneScriptSmoke_TimesOutAndRetainsDiagnostics(t *testing.T) {
	if _, err := exec.LookPath("timeout"); err != nil {
		t.Skipf("GNU timeout is required for the Linux runner smoke: %v", err)
	}

	repoRoot := testutil.MustRepoPath(t, ".")
	makePath := writeExecutableScript(t, "fake-make-functional-viz-timeout", `#!/bin/sh
printf '%s\n' 'inventory: discovered-packages=147 observed-packages=12'
printf '%s\n' 'quarantine: selector=./tests/functional/example bucket=ENVIRONMENT-DEPENDENT observed=skip'
trap 'exit 143' TERM INT
sleep 2
`)
	artifactRoot := filepath.Join(t.TempDir(), "functional-test-viz-timeout-artifacts")

	output, err := runScript(
		repoRoot,
		filepath.Join(repoRoot, "scripts", "ci", "run-functional-test-viz.sh"),
		fmt.Sprintf("FUNCTIONAL_TEST_VIZ_DIR=%s", artifactRoot),
		fmt.Sprintf("FUNCTIONAL_TEST_BUDGET=%s", "0.1s"),
		"FUNCTIONAL_TEST_TIER=pr-short",
		"FUNCTIONAL_TEST_TRIGGER=pull_request",
		fmt.Sprintf("MAKE_BIN=%s", makePath),
	)
	if err == nil {
		t.Fatalf("functional runner unexpectedly succeeded after budget expiry:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != 124 {
		t.Fatalf("functional runner exit = %v, want timeout exit code 124\n%s", err, output)
	}

	logBody, readErr := os.ReadFile(filepath.Join(artifactRoot, "command.log"))
	if readErr != nil {
		t.Fatalf("read timeout command log: %v", readErr)
	}
	log := string(logBody)
	for _, expected := range []string{
		"tier=pr-short trigger=pull_request short=true budget=0.1s selection=subtractive",
		"inventory: discovered-packages=147 observed-packages=12",
		"quarantine: selector=./tests/functional/example bucket=ENVIRONMENT-DEPENDENT observed=skip",
		"tier timed out after budget=0.1s",
		"partial diagnostics retained",
	} {
		if !strings.Contains(log, expected) {
			t.Fatalf("timeout command log missing %q:\n%s", expected, log)
		}
	}
}
