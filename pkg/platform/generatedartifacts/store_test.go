package generatedartifacts

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type testFileSystem struct{}

func (testFileSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (testFileSystem) Remove(path string) error             { return os.Remove(path) }
func (testFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (testFileSystem) WriteFile(path string, payload []byte, mode fs.FileMode) error {
	return os.WriteFile(path, payload, mode)
}
func (testFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func newTestStore(t *testing.T) LocalStore {
	t.Helper()
	store, err := NewLocalStore(testFileSystem{})
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}
	return store
}

func TestLocalStoreWriteAndCheckPreserveGeneratedArtifactSemantics(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	artifacts := []Artifact{{Path: "generated/example.txt", Payload: []byte("expected\n")}}
	if err := store.Write(root, artifacts); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	drift, err := store.Check(root, artifacts)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check() drift = %#v", drift)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(artifacts[0].Path)), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale fixture: %v", err)
	}
	drift, err = store.Check(root, append(artifacts, Artifact{Path: "generated/missing.txt", Payload: []byte("missing")}))
	if err != nil {
		t.Fatalf("Check(stale) error = %v", err)
	}
	if len(drift.Stale) != 1 || drift.Stale[0] != artifacts[0].Path {
		t.Fatalf("stale = %#v", drift.Stale)
	}
	if len(drift.Missing) != 1 || drift.Missing[0] != "generated/missing.txt" {
		t.Fatalf("missing = %#v", drift.Missing)
	}
}

func TestLocalStoreWriteAndCheckEnforceRetiredArtifactAbsence(t *testing.T) {
	root := t.TempDir()
	store := newTestStore(t)
	retired := Artifact{Path: "generated/retired.json", Absent: true}
	target := filepath.Join(root, filepath.FromSlash(retired.Path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create retired artifact directory: %v", err)
	}
	if err := os.WriteFile(target, []byte("legacy"), 0o644); err != nil {
		t.Fatalf("write retired artifact fixture: %v", err)
	}

	drift, err := store.Check(root, []Artifact{retired})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(drift.Unexpected) != 1 || drift.Unexpected[0] != retired.Path {
		t.Fatalf("unexpected = %#v, want %q", drift.Unexpected, retired.Path)
	}
	if err := store.Write(root, []Artifact{retired}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("retired artifact still exists after Write(): %v", err)
	}
	drift, err = store.Check(root, []Artifact{retired})
	if err != nil {
		t.Fatalf("Check(after Write) error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("Check(after Write) drift = %#v", drift)
	}
}

func TestLocalStoreRequiresInjectedFileSystem(t *testing.T) {
	if _, err := NewLocalStore(nil); err == nil {
		t.Fatal("NewLocalStore(nil) error = nil, want required-filesystem failure")
	}
	store := LocalStore{}
	if _, err := store.Read("missing"); err == nil {
		t.Fatal("zero LocalStore.Read() error = nil, want fail-closed failure")
	}
	if err := store.Write(t.TempDir(), nil); err == nil {
		t.Fatal("zero LocalStore.Write() error = nil, want fail-closed failure")
	}
	if _, err := store.Check(t.TempDir(), nil); err == nil {
		t.Fatal("zero LocalStore.Check() error = nil, want fail-closed failure")
	}
}
