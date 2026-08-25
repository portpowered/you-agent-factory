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
	makefilePath := filepath.ToSlash(filepath.Join(repoRoot, "Makefile"))
	goStub := writeFunctionalTestGoStub(t, "boundary-fail")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
		"FUNCTIONAL_TEST_GO="+goStub,
	)
	if err == nil {
		t.Fatalf("functional-test-viz unexpectedly succeeded after boundary failure:\n%s", output)
	}
	log := readFunctionalTestVizLog(t, artifactDir)
	if !strings.Contains(log, "stub:boundary-boundary-fail") {
		t.Fatalf("functional-test-viz log missing boundary failure marker:\n%s", log)
	}
	for _, unexpected := range []string{"Functional suite inventory:", "stub:catalog"} {
		if strings.Contains(log, unexpected) {
			t.Fatalf("functional-test-viz continued after boundary failure (%q present):\n%s", unexpected, log)
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
	makefilePath := filepath.ToSlash(filepath.Join(repoRoot, "Makefile"))
	goStub := writeFunctionalTestGoStub(t, "coverage-fail")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
		"FUNCTIONAL_TEST_GO="+goStub,
	)
	if err == nil {
		t.Fatalf("functional-test-viz unexpectedly succeeded after coverage failure:\n%s", output)
	}
	log := readFunctionalTestVizLog(t, artifactDir)
	if !strings.Contains(log, "stub:boundary-ok") {
		t.Fatalf("functional-test-viz log missing boundary success marker:\n%s", log)
	}
	if !strings.Contains(log, "stub:coverage-floor-fail") {
		t.Fatalf("functional-test-viz log missing coverage failure marker:\n%s", log)
	}
	if strings.Contains(log, "stub:catalog") {
		t.Fatalf("functional-test-viz rendered Markdown after coverage failure:\n%s", log)
	}

	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage.out", "mode: set")
	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage-summary.json", `"coveragePercent":80`)
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "functional-tests.md")
}

// TestFunctionalTestVizFailClosed_RenderFailurePreservesEarlierArtifacts proves a
// Markdown/metadata rendering failure exits non-zero while leaving the coverage
// profile and JSON that earlier steps already wrote.
func TestFunctionalTestVizFailClosed_RenderFailurePreservesEarlierArtifacts(t *testing.T) {
	repoRoot := testutil.MustRepoPath(t, ".")
	artifactDir := t.TempDir()
	makefilePath := filepath.ToSlash(filepath.Join(repoRoot, "Makefile"))
	goStub := writeFunctionalTestGoStub(t, "render-fail")

	output, err := runMakefileTargetWithArgs(
		repoRoot,
		makefilePath,
		"functional-test-viz",
		"FUNCTIONAL_TEST_VIZ_DIR="+filepath.ToSlash(artifactDir),
		"FUNCTIONAL_TEST_GO="+goStub,
	)
	if err == nil {
		t.Fatalf("functional-test-viz unexpectedly succeeded after render failure:\n%s", output)
	}
	log := readFunctionalTestVizLog(t, artifactDir)
	if !strings.Contains(log, "stub:boundary-ok") || !strings.Contains(log, "stub:catalog-fail") {
		t.Fatalf("functional-test-viz log missing earlier step markers:\n%s", log)
	}
	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage.out", "mode: set")
	assertFunctionalTestVizArtifactContains(t, artifactDir, "coverage-summary.json", `"coveragePercent":80`)
	assertFunctionalTestVizArtifactAbsent(t, artifactDir, "functional-tests.md")
}

func readFunctionalTestVizLog(t *testing.T, artifactDir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(artifactDir, "command.log"))
	if err != nil {
		t.Fatalf("read functional-test-viz command log: %v", err)
	}
	return string(data)
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
