package logging

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

func TestReserveAvailableRuntimeArtifactPathCreatesDatedParentDirectories(t *testing.T) {
	tests := []struct {
		name string
		kind defaultpaths.RuntimeArtifactKind
	}{
		{name: "runtime log", kind: defaultpaths.RuntimeArtifactKindLog},
		{name: "runtime metrics", kind: defaultpaths.RuntimeArtifactKindMetrics},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rootDir := t.TempDir()
			at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
			suffix := defaultpaths.RuntimeArtifactPathComponents("runtime-dated-dir", "fixed-token")
			datedDir := defaultpaths.RuntimeLogsDatedDir(rootDir, at)

			assertRuntimeArtifactDatedDirAbsent(t, datedDir)

			path, err := reserveAvailableRuntimeArtifactPath(rootDir, at, tc.kind, suffix)
			if err != nil {
				t.Fatalf("reserveAvailableRuntimeArtifactPath: %v", err)
			}

			assertRuntimeArtifactDatedDirPresent(t, datedDir)
			assertPathUsesPlatformSeparators(t, path)
			want := defaultpaths.RuntimeArtifactPathWithCollision(rootDir, at, tc.kind, suffix, 0)
			if path != want {
				t.Fatalf("reserved path = %q, want %q", path, want)
			}
			if filepath.Dir(path) != datedDir {
				t.Fatalf("reserved path parent = %q, want dated dir %q", filepath.Dir(path), datedDir)
			}
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("reserved runtime artifact file %q should exist: %v", path, err)
			}
		})
	}
}

func TestReserveAvailableRuntimeArtifactPathUsesPlatformSeparators(t *testing.T) {
	rootDir := t.TempDir()
	at := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	suffix := defaultpaths.RuntimeArtifactPathComponents("runtime-platform-sep")

	path, err := reserveAvailableRuntimeArtifactPath(rootDir, at, defaultpaths.RuntimeArtifactKindLog, suffix)
	if err != nil {
		t.Fatalf("reserveAvailableRuntimeArtifactPath: %v", err)
	}

	assertPathUsesPlatformSeparators(t, path)
	want := defaultpaths.RuntimeArtifactPath(rootDir, at, defaultpaths.RuntimeArtifactKindLog, suffix)
	if path != want {
		t.Fatalf("reserved path = %q, want %q", path, want)
	}
}

func TestReserveAvailableRuntimeArtifactPathAvoidsExistingFile(t *testing.T) {
	rootDir := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
	suffix := defaultpaths.RuntimeArtifactPathComponents("runtime-collision", "fixed-token")

	firstPath, err := reserveAvailableRuntimeArtifactPath(rootDir, at, defaultpaths.RuntimeArtifactKindLog, suffix)
	if err != nil {
		t.Fatalf("reserveAvailableRuntimeArtifactPath first: %v", err)
	}
	if err := os.WriteFile(firstPath, []byte("first-log"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", firstPath, err)
	}

	secondPath, err := reserveAvailableRuntimeArtifactPath(rootDir, at, defaultpaths.RuntimeArtifactKindLog, suffix)
	if err != nil {
		t.Fatalf("reserveAvailableRuntimeArtifactPath second: %v", err)
	}
	if firstPath == secondPath {
		t.Fatalf("collision paths must differ, both %q", firstPath)
	}

	assertRuntimeArtifactCollisionPath(t, secondPath, rootDir, at, defaultpaths.RuntimeArtifactKindLog, suffix, 1)
	assertFileContents(t, firstPath, "first-log")
}

func TestReserveAvailableRuntimeArtifactPathAvoidsConcurrentCollisions(t *testing.T) {
	rootDir := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
	suffix := defaultpaths.RuntimeArtifactPathComponents("runtime-concurrent-collision", "fixed-token")

	const workers = 16
	paths := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, err := reserveAvailableRuntimeArtifactPath(rootDir, at, defaultpaths.RuntimeArtifactKindMetrics, suffix)
			if err != nil {
				errs <- err
				return
			}
			paths <- path
		}()
	}
	wg.Wait()
	close(paths)
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("reserveAvailableRuntimeArtifactPath concurrent: %v", err)
		}
	}

	seen := map[string]struct{}{}
	for path := range paths {
		if _, ok := seen[path]; ok {
			t.Fatalf("duplicate concurrent runtime artifact path %q", path)
		}
		seen[path] = struct{}{}
	}
	if len(seen) != workers {
		t.Fatalf("reserved %d unique runtime artifact paths, want %d", len(seen), workers)
	}
}

func TestReserveAvailableRuntimeArtifactPathPreservesMetricsCollisionShape(t *testing.T) {
	metricsDir := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
	suffix := defaultpaths.RuntimeArtifactPathComponents("session-same-ts", "runtime-same-ts", "fixed-token")

	firstPath, err := reserveAvailableRuntimeArtifactPath(metricsDir, at, defaultpaths.RuntimeArtifactKindMetrics, suffix)
	if err != nil {
		t.Fatalf("reserveAvailableRuntimeArtifactPath first: %v", err)
	}
	if err := os.WriteFile(firstPath, []byte(`{"metric":"preserved"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", firstPath, err)
	}

	secondPath, err := reserveAvailableRuntimeArtifactPath(metricsDir, at, defaultpaths.RuntimeArtifactKindMetrics, suffix)
	if err != nil {
		t.Fatalf("reserveAvailableRuntimeArtifactPath second: %v", err)
	}
	if firstPath == secondPath {
		t.Fatalf("collision paths must differ, both %q", firstPath)
	}

	assertFileContents(t, firstPath, `{"metric":"preserved"}`+"\n")
	assertRuntimeArtifactCollisionPath(t, secondPath, metricsDir, at, defaultpaths.RuntimeArtifactKindMetrics, suffix, 1)
}

func assertRuntimeArtifactDatedDirAbsent(t *testing.T, datedDir string) {
	t.Helper()

	if _, err := os.Stat(datedDir); !os.IsNotExist(err) {
		t.Fatalf("dated dir %q should not exist before sink creation, stat err = %v", datedDir, err)
	}
}

func assertRuntimeArtifactRootLacksCalendarDirectories(t *testing.T, rootDir string) {
	t.Helper()

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", rootDir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if ok, err := regexp.MatchString(`^\d{4}$`, entry.Name()); err != nil {
			t.Fatalf("match year directory name %q: %v", entry.Name(), err)
		} else if ok {
			t.Fatalf("root %q should not contain calendar year directory %q before sink creation", rootDir, entry.Name())
		}
	}
}

func assertRuntimeArtifactDatedDirPresent(t *testing.T, datedDir string) {
	t.Helper()

	info, err := os.Stat(datedDir)
	if err != nil {
		t.Fatalf("dated dir %q should exist after sink creation: %v", datedDir, err)
	}
	if !info.IsDir() {
		t.Fatalf("dated dir %q is not a directory", datedDir)
	}
}

func assertPathUsesPlatformSeparators(t *testing.T, path string) {
	t.Helper()

	altSep := '/'
	if os.PathSeparator == '/' {
		altSep = '\\'
	}
	if strings.Contains(path, string(altSep)) {
		t.Fatalf("path %q contains %q, want only %q on this host", path, altSep, os.PathSeparator)
	}
}

func assertRuntimeArtifactCollisionPath(
	t *testing.T,
	path string,
	rootDir string,
	at time.Time,
	kind defaultpaths.RuntimeArtifactKind,
	suffix string,
	collisionIndex int,
) {
	t.Helper()

	want := defaultpaths.RuntimeArtifactPathWithCollision(rootDir, at, kind, suffix, collisionIndex)
	if path != want {
		t.Fatalf("collision path = %q, want %q", path, want)
	}

	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		t.Fatalf("Rel(%q): %v", path, err)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 4 {
		t.Fatalf("relative path = %q, want YYYY/MM/DD/filename", rel)
	}
	if got, want := parts[0]+"/"+parts[1]+"/"+parts[2], at.UTC().Format("2006/01/02"); got != want {
		t.Fatalf("dated directory = %q, want %q", got, want)
	}
	if !strings.HasPrefix(parts[3], at.UTC().Format(defaultpaths.RuntimeArtifactTimeLayout)+"-"+string(kind)+"-") {
		t.Fatalf("filename = %q, want time-kind prefix", parts[3])
	}
}
