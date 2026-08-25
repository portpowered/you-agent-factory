package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

func TestRepoRootDirFindsNearestAncestorWithGoMod(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	nestedDir := filepath.Join(repoRoot, "pkg", "service", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chdir(nestedDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	got, err := repoRootDir()
	if err != nil {
		t.Fatalf("repoRootDir() error = %v", err)
	}
	if testutil.CanonicalPath(got) != testutil.CanonicalPath(repoRoot) {
		t.Fatalf("repoRootDir() = %q, want %q", got, repoRoot)
	}
}

func tempDirOutsideRepo(t *testing.T) string {
	t.Helper()

	repoRoot := testutil.CanonicalPath(testutil.MustRepoRoot(t))
	candidates := []string{os.TempDir()}
	if runtime.GOOS != "windows" {
		candidates = append(candidates, "/tmp")
	}

	for _, base := range candidates {
		tempRoot, err := os.MkdirTemp(base, "gocoveragecheck-*")
		if err != nil {
			continue
		}
		canonicalTemp := testutil.CanonicalPath(tempRoot)
		if isPathWithin(canonicalTemp, repoRoot) {
			if removeErr := os.RemoveAll(tempRoot); removeErr != nil {
				t.Fatalf("remove in-repo temp dir %q: %v", tempRoot, removeErr)
			}
			continue
		}
		t.Cleanup(func() {
			if removeErr := os.RemoveAll(tempRoot); removeErr != nil {
				t.Fatalf("remove temp dir %q: %v", tempRoot, removeErr)
			}
		})
		return tempRoot
	}

	t.Fatal("could not create temp dir outside repository")
	return ""
}

func isPathWithin(child, parent string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func TestRepoRootDirFailsWhenNoGoModExists(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalDir); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	workingDir := filepath.Join(tempDirOutsideRepo(t), "pkg", "service")
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	_, err = repoRootDir()
	if err == nil {
		t.Fatal("repoRootDir() unexpectedly succeeded")
	}
	if err.Error() != "resolve repository root: go.mod not found" {
		t.Fatalf("repoRootDir() error = %q, want missing go.mod error", err.Error())
	}
}

func TestFindZeroCoveragePackagesFromSummaries(t *testing.T) {
	t.Parallel()

	zeroCoveragePackages := findZeroCoveragePackagesFromSummaries(
		[]packageCoverageSummary{
			{importPath: modulePath + "/pkg/config", coverage: 0},
			{importPath: modulePath + "/pkg/service", coverage: 100},
		},
		map[string]struct{}{
			modulePath + "/pkg/config": {},
		},
	)
	if len(zeroCoveragePackages) != 0 {
		t.Fatalf("findZeroCoveragePackagesFromSummaries() = %v, want none", zeroCoveragePackages)
	}
}
