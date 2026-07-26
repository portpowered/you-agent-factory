//go:build !windows

package coverage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestFunctionalTestVizFailClosed_BoundaryFailureStopsCoverageAndViz proves a
// failing functional-boundary-check exits non-zero and does not proceed into
// the coverage lane or Markdown generator.
func TestFunctionalTestVizFailClosed_BoundaryFailureStopsCoverageAndViz(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	artifactDir := t.TempDir()
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-fail'\n\t@exit 23\n",
		"test-functional-coverage":  "@printf '%s\\n' 'stub:coverage-unexpected'\n\t@exit 99\n",
	})

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
	)
	if err == nil {
		t.Fatalf("functional-test-viz unexpectedly succeeded after boundary failure:\n%s", output)
	}
	if !strings.Contains(output, "stub:boundary-fail") {
		t.Fatalf("functional-test-viz output missing boundary failure marker:\n%s", output)
	}
	for _, unexpected := range []string{"stub:coverage-unexpected", "wrote catalog"} {
		if strings.Contains(output, unexpected) {
			t.Fatalf("functional-test-viz continued after boundary failure (%q present):\n%s", unexpected, output)
		}
	}
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "coverage.out")
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "coverage-summary.json")
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "functional-tests.md")
}

// TestFunctionalTestVizFailClosed_CoverageFailurePreservesWrittenArtifacts proves
// a failing coverage lane exits non-zero, leaves already-written profile/JSON
// diagnostics, and does not treat the viz run as successful by rendering Markdown.
func TestFunctionalTestVizFailClosed_CoverageFailurePreservesWrittenArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	artifactDir := t.TempDir()
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
		"test-functional-coverage": "@mkdir -p $(FUNCTIONAL_TEST_VIZ_DIR)\n" +
			"\t@printf '%s\\n' 'mode: set' > $(FUNCTIONAL_TEST_VIZ_PROFILE)\n" +
			"\t@printf '%s\\n' '{\"schemaVersion\":1,\"coveragePercent\":12.5}' > $(FUNCTIONAL_TEST_VIZ_JSON)\n" +
			"\t@printf '%s\\n' 'stub:coverage-floor-fail'\n" +
			"\t@exit 17\n",
	})

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
	)
	if err == nil {
		t.Fatalf("functional-test-viz unexpectedly succeeded after coverage failure:\n%s", output)
	}
	if !strings.Contains(output, "stub:boundary-ok") {
		t.Fatalf("functional-test-viz output missing boundary success marker:\n%s", output)
	}
	if !strings.Contains(output, "stub:coverage-floor-fail") {
		t.Fatalf("functional-test-viz output missing coverage failure marker:\n%s", output)
	}
	if strings.Contains(output, "wrote catalog") {
		t.Fatalf("functional-test-viz rendered Markdown after coverage failure:\n%s", output)
	}

	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage.out", "mode: set")
	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage-summary.json", `"coveragePercent":12.5`)
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "functional-tests.md")
}

// TestFunctionalTestVizFailClosed_RenderFailurePreservesEarlierArtifacts proves a
// Markdown/metadata rendering failure exits non-zero while leaving the coverage
// profile and JSON that earlier steps already wrote.
func TestFunctionalTestVizFailClosed_RenderFailurePreservesEarlierArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	artifactDir := t.TempDir()
	makefilePath := writeVerifyFastWrapperMakefile(t, repoRoot, map[string]string{
		"functional-boundary-check": "@printf '%s\\n' 'stub:boundary-ok'\n",
		"test-functional-coverage": "@mkdir -p $(FUNCTIONAL_TEST_VIZ_DIR)\n" +
			"\t@printf '%s\\n' 'mode: set' > $(FUNCTIONAL_TEST_VIZ_PROFILE)\n" +
			"\t@printf '%s\\n' 'not-valid-coverage-summary-json' > $(FUNCTIONAL_TEST_VIZ_JSON)\n" +
			"\t@printf '%s\\n' 'stub:coverage-ok'\n",
	})

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
	)
	if err == nil {
		t.Fatalf("functional-test-viz unexpectedly succeeded after render failure:\n%s", output)
	}
	if !strings.Contains(output, "stub:boundary-ok") || !strings.Contains(output, "stub:coverage-ok") {
		t.Fatalf("functional-test-viz output missing earlier step markers:\n%s", output)
	}
	if strings.Contains(output, "wrote catalog") {
		t.Fatalf("functional-test-viz reported catalog success after render failure:\n%s", output)
	}

	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage.out", "mode: set")
	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage-summary.json", "not-valid-coverage-summary-json")
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "functional-tests.md")
}

func assertFunctionalTestVizArtifactContains(t *testing.T, artifactDir, name, wantSubstring string) {
	t.Helper()
	path := filepath.Join(artifactDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected diagnostic artifact %s to remain after failure: %v", path, err)
	}
	if !strings.Contains(string(data), wantSubstring) {
		t.Fatalf("artifact %s = %q, want substring %q", path, data, wantSubstring)
	}
}

func assertFunctionalTestVizArtifactAbsent(t *testing.T, artifactDir, name string) {
	t.Helper()
	path := filepath.Join(artifactDir, name)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("did not expect artifact %s after this failure mode", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat artifact %s: %v", path, err)
	}
}
