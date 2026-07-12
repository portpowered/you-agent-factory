package logging

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

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
