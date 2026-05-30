package factorysessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSessionFolder_RequiresPath(t *testing.T) {
	_, err := ResolveSessionFolder("   ")
	if err == nil {
		t.Fatal("ResolveSessionFolder(empty) error = nil, want validation failure")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonRequired || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want required folderPath", reason, field, ok)
	}
}

func TestResolveSessionFolder_RejectsMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-session-folder")
	_, err := ResolveSessionFolder(missing)
	if err == nil {
		t.Fatal("ResolveSessionFolder(missing) error = nil, want failure")
	}
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != ValidationReasonMissing || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want missing folderPath", reason, field, ok)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error = %q, want resolved path %q", err, missing)
	}
}

func TestExpandFolderHome_ExpandsTildePrefix(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ExpandFolderHome("~/sessions/workspace")
	if err != nil {
		t.Fatalf("ExpandFolderHome: %v", err)
	}
	want := filepath.Join(home, "sessions", "workspace")
	if got != want {
		t.Fatalf("ExpandFolderHome = %q, want %q", got, want)
	}
}

func TestSameFactoryDir_NormalizesEquivalentPaths(t *testing.T) {
	left := filepath.Join("/tmp", "factory", ".", "named")
	right := filepath.Join("/tmp", "factory", "named")
	if !SameFactoryDir(left, right) {
		t.Fatal("SameFactoryDir did not treat equivalent paths as equal")
	}
	if SameFactoryDir("", right) {
		t.Fatal("SameFactoryDir accepted empty left path")
	}
}

func TestResolveSessionFolder_ExpandsTildeBeforeStat(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sessionDir := filepath.Join(home, "session-root")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	got, err := ResolveSessionFolder("~/session-root")
	if err != nil {
		t.Fatalf("ResolveSessionFolder: %v", err)
	}
	if got != sessionDir {
		t.Fatalf("ResolveSessionFolder = %q, want %q", got, sessionDir)
	}
}
