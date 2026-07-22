package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
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

func TestRunGeneratesCLIFamilyArtifacts(t *testing.T) {
	root := t.TempDir()
	writeProductionManifestFixture(t, root)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	store := testArtifactStore(t)
	if status := run(store, root, false, stdout, stderr); status != 0 {
		t.Fatalf("run() = %d, stderr = %q", status, stderr.String())
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("CLI family metadata generated")) {
		t.Fatalf("stdout = %q, want success message", got)
	}

	artifacts, err := climanifestgen.Artifacts(store, root)
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
	manifestPath := filepath.Join(root, filepath.FromSlash(climanifestgen.RuntimeFamilyManifestsPath))
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write stale artifact: %v", err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(testArtifactStore(t), root, true, stdout, stderr); status == 0 {
		t.Fatalf("run(check) = 0, want failure; stderr = %q", stderr.String())
	}
}

func TestRunCheckNamesAffectedStableCommandIDs(t *testing.T) {
	root := t.TempDir()
	writeProductionManifestFixture(t, root)
	store := testArtifactStore(t)
	artifacts, err := climanifestgen.Artifacts(store, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Write(root, artifacts); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, filepath.FromSlash(climanifestgen.MCPFamilyCommandIDsPath))
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(store, root, true, stdout, stderr); status == 0 {
		t.Fatal("run(check) = 0, want stale-artifact failure")
	}
	for _, id := range climanifestgen.MCPFamilyCommandIDs {
		if !bytes.Contains(stderr.Bytes(), []byte(id)) {
			t.Errorf("stderr = %q, want affected stable ID %q", stderr.String(), id)
		}
	}
}

func writeProductionManifestFixture(t *testing.T, root string) {
	t.Helper()
	repositoryRoot := testutil.MustRepoPath(t, ".")
	for _, relativePath := range []string{
		climanifestgen.ProductionManifestPath,
		climanifestgen.CompatibilityManifestPath,
	} {
		manifest, err := os.ReadFile(filepath.Join(repositoryRoot, filepath.FromSlash(relativePath)))
		if err != nil {
			t.Fatalf("read %s fixture: %v", relativePath, err)
		}
		manifestPath := filepath.Join(root, filepath.FromSlash(relativePath))
		if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
			t.Fatalf("create manifest directory: %v", err)
		}
		if err := os.WriteFile(manifestPath, manifest, 0o644); err != nil {
			t.Fatalf("write %s: %v", relativePath, err)
		}
	}
}
