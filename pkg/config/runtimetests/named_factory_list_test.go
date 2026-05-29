package runtimetests

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
)

func TestListNamedFactories_ReturnsPersistedFactoriesSorted(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	entries, err := ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].Name != "alpha" || entries[1].Name != "beta" {
		t.Fatalf("entries = %#v, want alpha then beta", entries)
	}
	if entries[0].FactoryDir != filepath.Join(rootDir, "alpha") {
		t.Fatalf("alpha directory = %q, want %q", entries[0].FactoryDir, filepath.Join(rootDir, "alpha"))
	}
}

func TestListNamedFactories_MarksCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := PersistNamedFactory(rootDir, "beta", namedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	entries, err := ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}

	currentCount := 0
	for _, entry := range entries {
		if entry.Current {
			currentCount++
			if entry.Name != "beta" {
				t.Fatalf("unexpected current entry %#v", entry)
			}
		}
	}
	if currentCount != 1 {
		t.Fatalf("current count = %d, want 1", currentCount)
	}
}

func TestListNamedFactories_NoPointerMarksNothingCurrent(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := PersistNamedFactory(rootDir, "alpha", namedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}

	entries, err := ListNamedFactories(rootDir)
	if err != nil {
		t.Fatalf("ListNamedFactories: %v", err)
	}
	for _, entry := range entries {
		if entry.Current {
			t.Fatalf("expected no current entry, got %#v", entry)
		}
	}
}

func TestListNamedFactories_RejectsMissingRoot(t *testing.T) {
	_, err := ListNamedFactories(filepath.Join(t.TempDir(), "missing-root"))
	if err == nil {
		t.Fatal("expected missing factory root to fail")
	}
}

func TestListNamedFactories_RejectsNonDirectoryRoot(t *testing.T) {
	rootPath := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(rootPath, []byte("factory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ListNamedFactories(rootPath)
	if err == nil {
		t.Fatal("expected non-directory factory root to fail")
	}
}

func TestListNamedFactories_RejectsInvalidCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, ".current-factory"), []byte("../beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ListNamedFactories(rootDir)
	if err == nil {
		t.Fatal("expected malformed current-factory pointer to fail")
	}
}
