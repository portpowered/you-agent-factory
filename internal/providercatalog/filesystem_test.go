package providercatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCheckReportsDriftWithoutChangingTheWorktree(t *testing.T) {
	root := t.TempDir()
	copyFixtureToDisk(t, root, repositoryFixture(t))
	if err := Generate(root); err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	stalePath := filepath.Join(root, filepath.FromSlash(CatalogPath))
	stale := []byte("{\"stale\":true}\n")
	if err := os.WriteFile(stalePath, stale, 0o644); err != nil {
		t.Fatalf("write stale catalog: %v", err)
	}
	drift, err := Check(root)
	if err != nil {
		t.Fatalf("Check(): %v", err)
	}
	if got := strings.Join(drift.Stale, ","); got != CatalogPath {
		t.Fatalf("stale paths = %q, want %q", got, CatalogPath)
	}
	after, err := os.ReadFile(stalePath)
	if err != nil {
		t.Fatalf("read stale catalog after check: %v", err)
	}
	if !bytes.Equal(after, stale) {
		t.Fatal("Check() modified the stale generated catalog")
	}
}

func copyFixtureToDisk(t *testing.T, root string, fixture fstest.MapFS) {
	t.Helper()
	for name, file := range fixture {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(target, file.Data, 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}
