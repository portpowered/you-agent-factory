package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
)

func assertRuntimeArtifactRootLacksCalendarDirectories(t *testing.T, rootDir string) {
	t.Helper()
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", rootDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() && regexp.MustCompile(`^\d{4}$`).MatchString(entry.Name()) {
			t.Fatalf("root %q unexpectedly contains calendar directory %q", rootDir, entry.Name())
		}
	}
}

func assertRuntimeArtifactDatedDirPresent(t *testing.T, datedDir string) {
	t.Helper()
	info, err := os.Stat(datedDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("dated dir %q is unavailable: %v", datedDir, err)
	}
}

func assertPathUsesPlatformSeparators(t *testing.T, path string) {
	t.Helper()
	altSep := '/'
	if os.PathSeparator == '/' {
		altSep = '\\'
	}
	if strings.ContainsRune(path, altSep) {
		t.Fatalf("path %q contains non-platform separator %q", path, altSep)
	}
}

func assertRuntimeArtifactCollisionPath(t *testing.T, path, rootDir string, at time.Time, kind runtimeartifact.RuntimeArtifactKind, suffix string, collisionIndex int) {
	t.Helper()
	want := runtimeartifact.RuntimeArtifactPathWithCollision(rootDir, at, kind, suffix, collisionIndex)
	if path != want {
		t.Fatalf("collision path = %q, want %q", path, want)
	}
	if filepath.Dir(path) != runtimeartifact.RuntimeLogsDatedDir(rootDir, at) {
		t.Fatalf("collision path parent = %q, want dated directory", filepath.Dir(path))
	}
}
