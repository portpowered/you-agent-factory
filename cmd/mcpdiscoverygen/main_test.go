package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	"github.com/portpowered/infinite-you/pkg/transports/mcp/discoverygen"
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

func testArtifactStore(t *testing.T) generatedartifacts.LocalStore {
	t.Helper()
	store, err := generatedartifacts.NewLocalStore(testFileSystem{})
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}
	return store
}

func TestRunGeneratesDiscoveryArtifact(t *testing.T) {
	root := t.TempDir()
	writeAuthoredCatalogFixture(t, root)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	store := testArtifactStore(t)
	if status := run(store, root, false, stdout, stderr); status != 0 {
		t.Fatalf("run() = %d, stderr = %q", status, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("MCP discovery metadata generated")) {
		t.Fatalf("stdout = %q, want success message", got)
	}

	artifacts, err := discoverygen.Artifacts(root)
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	drift, err := store.Check(root, artifacts)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !drift.Empty() {
		t.Fatalf("drift after generation = %#v", drift)
	}
}

func TestRunCheckFailsOnStaleArtifact(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, filepath.FromSlash(discoverygen.DiscoveryJSONPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(testArtifactStore(t), root, true, stdout, stderr); status == 0 {
		t.Fatalf("run(check) = 0, want failure; stderr = %q", stderr.String())
	}
}

func writeAuthoredCatalogFixture(t *testing.T, root string) {
	t.Helper()
	catalog, err := os.ReadFile(filepath.Join("..", "..", discoverygen.AuthoredCatalogPath))
	if err != nil {
		t.Fatalf("read authored catalog fixture: %v", err)
	}
	target := filepath.Join(root, filepath.FromSlash(discoverygen.AuthoredCatalogPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("create catalog directory: %v", err)
	}
	if err := os.WriteFile(target, catalog, 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}
