package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFactoryRootFromConfigFile_ReturnsParentDirectory(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(`{"id":"test"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ResolveFactoryRootFromConfigFile(factoryPath)
	if err != nil {
		t.Fatalf("ResolveFactoryRootFromConfigFile: %v", err)
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if got != want {
		t.Fatalf("factory root = %q, want %q", got, want)
	}
}

func TestResolveFactoryRootFromConfigFile_RejectsMissingPath(t *testing.T) {
	_, err := ResolveFactoryRootFromConfigFile(filepath.Join(t.TempDir(), "missing.json"))
	if err == nil {
		t.Fatal("expected error for missing factory config file")
	}
	if !strings.Contains(err.Error(), "factory config file not found") {
		t.Fatalf("error = %q, want not-found message", err.Error())
	}
}

func TestResolveFactoryRootFromConfigFile_RejectsDirectoryPath(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveFactoryRootFromConfigFile(dir)
	if err == nil {
		t.Fatal("expected error for directory factory config path")
	}
	if !strings.Contains(err.Error(), "must be a file") {
		t.Fatalf("error = %q, want file requirement message", err.Error())
	}
}

func TestResolveFactoryRootFromConfigFile_RejectsEmptyPath(t *testing.T) {
	_, err := ResolveFactoryRootFromConfigFile("  ")
	if err == nil {
		t.Fatal("expected error for empty factory config path")
	}
	if !strings.Contains(err.Error(), "factory config path is required") {
		t.Fatalf("error = %q, want required-path message", err.Error())
	}
}
