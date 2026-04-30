package testutil

import (
	"github.com/portpowered/agent-factory/internal/testpath"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type ArtifactClassification = testpath.ArtifactClassification

const (
	ArtifactCheckedIn = testpath.ArtifactCheckedIn
	ArtifactGenerated = testpath.ArtifactGenerated
	ArtifactObsolete  = testpath.ArtifactObsolete
)

type ArtifactContractEntry = testpath.ArtifactContractEntry

func ArtifactContract() []ArtifactContractEntry {
	return testpath.ArtifactContract()
}

func MustArtifactContractEntry(t testing.TB, path string) ArtifactContractEntry {
	t.Helper()
	return testpath.MustArtifactContractEntry(t, path)
}

func MustRepoRoot(t testing.TB) string {
	t.Helper()

	_, callerFile, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("cannot determine caller file path")
	}

	root, err := findRepoRoot(filepath.Dir(callerFile))
	if err != nil {
		t.Fatalf("find repo root from %s: %v", callerFile, err)
	}
	return root
}

func MustRepoPath(t testing.TB, rel string) string {
	t.Helper()
	return filepath.Join(MustRepoRoot(t), filepath.FromSlash(rel))
}

func MustClassifiedArtifactPath(t testing.TB, rel string, allowed ...ArtifactClassification) string {
	t.Helper()
	return testpath.MustClassifiedArtifactPathFromCaller(t, 0, rel, allowed...)
}

func findRepoRoot(startDir string) (string, error) {
	current := filepath.Clean(startDir)
	for {
		goModPath := filepath.Join(current, "go.mod")
		if info, err := os.Stat(goModPath); err == nil && !info.IsDir() {
			return current, nil
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}
