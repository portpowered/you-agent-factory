package runtimeartifact

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type localFileSystem struct{}

func (localFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (localFileSystem) OpenFile(path string, flag int, mode fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, flag, mode)
}

func TestNewReserverRequiresFileSystem(t *testing.T) {
	reserver, err := NewReserver(nil)
	if err == nil || reserver != nil {
		t.Fatalf("NewReserver(nil) = (%v, %v), want nil and error", reserver, err)
	}
}

func TestReserverCreatesDatedPathAndAvoidsExistingArtifact(t *testing.T) {
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	first, err := reserver.Reserve(root, at, "log", "runtime-fixed")
	if err != nil {
		t.Fatalf("Reserve first: %v", err)
	}
	if err := os.WriteFile(first, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("WriteFile(first): %v", err)
	}
	second, err := reserver.Reserve(root, at, "log", "runtime-fixed")
	if err != nil {
		t.Fatalf("Reserve second: %v", err)
	}
	if first == second {
		t.Fatalf("collision returned the same path %q", first)
	}
	if got, err := os.ReadFile(first); err != nil || string(got) != "preserve" {
		t.Fatalf("first artifact = %q, %v; want preserved contents", got, err)
	}
	wantParent := filepath.Join(root, "2026", "05", "29")
	if filepath.Dir(first) != wantParent || filepath.Dir(second) != wantParent {
		t.Fatalf("reserved parents = %q, %q; want %q", filepath.Dir(first), filepath.Dir(second), wantParent)
	}
}

func TestReserverAvoidsConcurrentCollisions(t *testing.T) {
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	const count = 16
	paths := make(chan string, count)
	errs := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			path, reserveErr := reserver.Reserve(root, at, "metrics", "session-runtime-fixed")
			if reserveErr != nil {
				errs <- reserveErr
				return
			}
			paths <- path
		}()
	}
	group.Wait()
	close(paths)
	close(errs)
	for reserveErr := range errs {
		t.Fatalf("Reserve concurrent: %v", reserveErr)
	}
	seen := map[string]struct{}{}
	for path := range paths {
		if _, exists := seen[path]; exists {
			t.Fatalf("duplicate reserved path %q", path)
		}
		seen[path] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("reserved %d unique paths, want %d", len(seen), count)
	}
}
