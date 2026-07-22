package inboxgitkeep

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type localTestFileSystem struct{}

func (localTestFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (localTestFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }
func (localTestFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	return os.WriteFile(path, data, mode)
}

func TestLocalEnsuresInboxSentinelAndPreservesExistingFile(t *testing.T) {
	root := t.TempDir()
	relative := filepath.Join("inputs", "WORK", "default", ".gitkeep")
	local := NewLocal(localTestFileSystem{})
	if err := local.EnsureInputInboxGitkeep(root, relative); err != nil {
		t.Fatalf("EnsureInputInboxGitkeep() error = %v", err)
	}
	path := filepath.Join(root, relative)
	if err := os.WriteFile(path, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing): %v", err)
	}
	if err := local.EnsureInputInboxGitkeep(root, relative); err != nil {
		t.Fatalf("EnsureInputInboxGitkeep(existing) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "preserve" {
		t.Fatalf("existing sentinel = %q, %v", data, err)
	}
}

func TestLocalFailsClosedWithoutFileSystem(t *testing.T) {
	err := NewLocal(nil).EnsureInputInboxGitkeep("unused", ".gitkeep")
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("EnsureInputInboxGitkeep() error = %v", err)
	}
}
