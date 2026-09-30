package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovePiACPTempLeavesReplacedDirectory(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "bridge")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(directory, filepath.Join(parent, "original")); err != nil {
		_ = root.Close()
		t.Skipf("platform prevents replacing an open directory: %v", err)
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(directory, "bridge.mjs")
	if err := os.WriteFile(marker, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	removePiACPTemp(directory, root)
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("replacement file was removed: %v", err)
	}
}

func TestRemovePiACPTempDoesNotRecursivelyRemoveUnexpectedChild(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "bridge")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bridge.mjs", "THIRD_PARTY_NOTICES.txt", "unexpected"} {
		if err := root.WriteFile(name, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	removePiACPTemp(directory, root)
	if _, err := os.Stat(filepath.Join(directory, "unexpected")); err != nil {
		t.Fatalf("unexpected child was removed: %v", err)
	}
	for _, name := range []string{"bridge.mjs", "THIRD_PARTY_NOTICES.txt"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatalf("owned file %q remains: %v", name, err)
		}
	}
}

func TestRemovePiACPTempRemovesOwnedDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "bridge")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("bridge.mjs", []byte("bridge"), 0600); err != nil {
		t.Fatal(err)
	}
	removePiACPTemp(directory, root)
	if _, err := os.Lstat(directory); !os.IsNotExist(err) {
		t.Fatalf("owned directory remains: %v", err)
	}
}
