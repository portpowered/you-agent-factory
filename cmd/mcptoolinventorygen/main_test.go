package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	factorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp/inventorygen"
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

type recordingStore struct {
	writeErr  error
	artifacts []generatedartifacts.Artifact
}

func (store *recordingStore) Read(string) ([]byte, error) { return nil, os.ErrNotExist }

func (store *recordingStore) Write(root string, artifacts []generatedartifacts.Artifact) error {
	store.artifacts = artifacts
	return store.writeErr
}

func (store *recordingStore) Check(string, []generatedartifacts.Artifact) (generatedartifacts.Drift, error) {
	return generatedartifacts.Drift{}, nil
}

func testArtifactStore(t *testing.T) generatedartifacts.LocalStore {
	t.Helper()
	store, err := generatedartifacts.NewLocalStore(testFileSystem{})
	if err != nil {
		t.Fatalf("NewLocalStore() error = %v", err)
	}
	return store
}

func TestRunWritesOnlyTheProductionS11Artifact(t *testing.T) {
	root := t.TempDir()
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(testArtifactStore(t), root, stdout, stderr); status != 0 {
		t.Fatalf("run() = %d, stderr = %q", status, stderr.String())
	}
	if stdout.String() != successMessage+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout.String(), successMessage+"\n")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	artifact, err := inventorygen.Artifact()
	if err != nil {
		t.Fatalf("inventorygen.Artifact() error = %v", err)
	}
	target := filepath.Join(root, filepath.FromSlash(factorysession.ToolInventoryBaselineRelativePath))
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read generated S-11: %v", err)
	}
	if !bytes.Equal(got, artifact.Payload) {
		t.Fatal("generated S-11 differs from the production inventory artifact")
	}
	files := filesUnderRoot(t, root)
	if len(files) != 1 || files[0] != factorysession.ToolInventoryBaselineRelativePath {
		t.Fatalf("generated files = %#v, want only %q", files, factorysession.ToolInventoryBaselineRelativePath)
	}
}

func TestRunIsByteStableAcrossWrites(t *testing.T) {
	root := t.TempDir()
	store := testArtifactStore(t)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(store, root, stdout, stderr); status != 0 {
		t.Fatalf("first run() = %d, stderr = %q", status, stderr.String())
	}
	path := filepath.Join(root, filepath.FromSlash(factorysession.ToolInventoryBaselineRelativePath))
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read first S-11: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if status := run(store, root, stdout, stderr); status != 0 {
		t.Fatalf("second run() = %d, stderr = %q", status, stderr.String())
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read second S-11: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("S-11 bytes changed across unchanged writes")
	}
	if files := filesUnderRoot(t, root); len(files) != 1 || files[0] != factorysession.ToolInventoryBaselineRelativePath {
		t.Fatalf("generated files after repeat = %#v, want only %q", files, factorysession.ToolInventoryBaselineRelativePath)
	}
}

func TestRunFailsClosedWhenStoreIsMissing(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	if status := run(nil, t.TempDir(), stdout, stderr); status == 0 {
		t.Fatal("run(nil) = 0, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("artifact store is required")) {
		t.Fatalf("stderr = %q, want missing-store context", stderr.String())
	}
}

func TestRunFailsClosedWhenStoreWriteFails(t *testing.T) {
	root := t.TempDir()
	store := &recordingStore{writeErr: errors.New("forced write failure")}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(store, root, stdout, stderr); status == 0 {
		t.Fatal("run() = 0, want write failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("generation failed: forced write failure")) {
		t.Fatalf("stderr = %q, want write failure context", stderr.String())
	}
	if len(store.artifacts) != 1 || store.artifacts[0].Path != factorysession.ToolInventoryBaselineRelativePath {
		t.Fatalf("store artifacts = %#v, want exactly S-11", store.artifacts)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(factorysession.ToolInventoryBaselineRelativePath))); !os.IsNotExist(err) {
		t.Fatalf("S-11 exists after failed store write: %v", err)
	}
}

func TestRunFailsClosedForInvalidRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(root, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write invalid root fixture: %v", err)
	}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}

	if status := run(testArtifactStore(t), root, stdout, stderr); status == 0 {
		t.Fatal("run(invalid root) = 0, want failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !bytes.Contains(stderr.Bytes(), []byte("generation failed")) {
		t.Fatalf("stderr = %q, want generation failure context", stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(factorysession.ToolInventoryBaselineRelativePath))); !os.IsNotExist(err) {
		t.Fatalf("S-11 unexpectedly exists beneath invalid root: %v", err)
	}
}

func filesUnderRoot(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	if err != nil {
		t.Fatalf("walk generated root: %v", err)
	}
	return files
}
