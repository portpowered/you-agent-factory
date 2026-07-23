package logicaltarget

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
)

func TestResolveSessionFolderRequiresPath(t *testing.T) {
	_, err := ResolveSessionFolder("   ", os.UserHomeDir, platformfilesystem.Local{})
	if err == nil {
		t.Fatal("ResolveSessionFolder(empty) error = nil, want validation failure")
	}
}

func TestResolveSessionFolderRejectsMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := ResolveSessionFolder(missing, os.UserHomeDir, platformfilesystem.Local{})
	if err == nil {
		t.Fatal("ResolveSessionFolder(missing) error = nil, want failure")
	}
}

func TestExpandFolderHomeExpandsTildePrefix(t *testing.T) {
	home := t.TempDir()
	got, err := ExpandFolderHome("~/sessions/workspace", func() (string, error) { return home, nil })
	if err != nil {
		t.Fatalf("ExpandFolderHome: %v", err)
	}
	want := filepath.Join(home, "sessions", "workspace")
	if got != want {
		t.Fatalf("ExpandFolderHome = %q, want %q", got, want)
	}
}

func TestFactorySessionPathResolutionRequiresInjectedEffects(t *testing.T) {
	if _, err := ExpandFolderHome("/absolute/session", nil); err == nil {
		t.Fatal("ExpandFolderHome without home resolver error = nil")
	}
	if _, err := ResolveSessionFolder(t.TempDir(), os.UserHomeDir, nil); err == nil {
		t.Fatal("ResolveSessionFolder without directory inspection error = nil")
	}
}

func TestSameFactoryDirNormalizesEquivalentPaths(t *testing.T) {
	left := filepath.Join("tmp", "factory", "..", "factory")
	right := filepath.Join("tmp", "factory")
	if !SameFactoryDir(left, right) {
		t.Fatal("SameFactoryDir did not treat equivalent paths as equal")
	}
	if SameFactoryDir("", right) {
		t.Fatal("SameFactoryDir accepted empty left path")
	}
}

func TestResolveSessionFolderExpandsTildeBeforeStat(t *testing.T) {
	home := t.TempDir()
	sessionDir := filepath.Join(home, "session-root")
	if err := os.Mkdir(sessionDir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	got, err := ResolveSessionFolder("~/session-root", func() (string, error) { return home, nil }, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("ResolveSessionFolder: %v", err)
	}
	if got != sessionDir {
		t.Fatalf("ResolveSessionFolder = %q, want %q", got, sessionDir)
	}
}

func TestExpandFolderHomePropagatesResolverFailure(t *testing.T) {
	want := errors.New("home unavailable")
	_, err := ExpandFolderHome("~/session", func() (string, error) { return "", want })
	if !errors.Is(err, want) {
		t.Fatalf("ExpandFolderHome error = %v, want wrapped %v", err, want)
	}
}
