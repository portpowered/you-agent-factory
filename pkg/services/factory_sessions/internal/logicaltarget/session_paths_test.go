package logicaltarget

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

type directoryInspectionStub struct {
	stat    func(string) (fs.FileInfo, error)
	readDir func(string) ([]fs.DirEntry, error)
}

func (s directoryInspectionStub) Stat(path string) (fs.FileInfo, error) {
	if s.stat == nil {
		return nil, fs.ErrNotExist
	}
	return s.stat(path)
}

func (s directoryInspectionStub) ReadDir(path string) ([]fs.DirEntry, error) {
	if s.readDir == nil {
		return nil, nil
	}
	return s.readDir(path)
}

type directoryInfo struct{ fs.FileInfo }

func (directoryInfo) IsDir() bool { return true }

func TestValidateInitNewFactoryNestedDir(t *testing.T) {
	t.Run("missing nested directory", func(t *testing.T) {
		if err := ValidateInitNewFactoryNestedDir(t.TempDir(), platformfilesystem.Local{}); err != nil {
			t.Fatalf("ValidateInitNewFactoryNestedDir(missing) = %v, want nil", err)
		}
	})
	t.Run("empty nested directory", func(t *testing.T) {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, factorydefinitions.FactoryDir), 0o755); err != nil {
			t.Fatalf("Mkdir(nested): %v", err)
		}
		if err := ValidateInitNewFactoryNestedDir(root, platformfilesystem.Local{}); err != nil {
			t.Fatalf("ValidateInitNewFactoryNestedDir(empty) = %v, want nil", err)
		}
	})
	t.Run("conflicting file", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, factorydefinitions.FactoryDir), []byte("file\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(nested): %v", err)
		}
		assertPathValidation(t, ValidateInitNewFactoryNestedDir(root, platformfilesystem.Local{}), factorysessions.ValidationReasonConflict)
	})
	t.Run("conflicting content", func(t *testing.T) {
		root := t.TempDir()
		nested := filepath.Join(root, factorydefinitions.FactoryDir)
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatalf("MkdirAll(nested): %v", err)
		}
		if err := os.WriteFile(filepath.Join(nested, "notes.txt"), []byte("notes\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(notes): %v", err)
		}
		assertPathValidation(t, ValidateInitNewFactoryNestedDir(root, platformfilesystem.Local{}), factorysessions.ValidationReasonConflict)
	})
	t.Run("injected failures", func(t *testing.T) {
		assertPathValidation(t, ValidateInitNewFactoryNestedDir(t.TempDir(), nil), factorysessions.ValidationReasonUnreadable)
		statErr := errors.New("stat unavailable")
		if err := ValidateInitNewFactoryNestedDir("project", directoryInspectionStub{stat: func(string) (fs.FileInfo, error) { return nil, statErr }}); !errors.Is(err, statErr) {
			t.Fatalf("stat error = %v, want %v", err, statErr)
		}
		readErr := errors.New("read unavailable")
		err := ValidateInitNewFactoryNestedDir("project", directoryInspectionStub{
			stat:    func(string) (fs.FileInfo, error) { return directoryInfo{}, nil },
			readDir: func(string) ([]fs.DirEntry, error) { return nil, readErr },
		})
		if !errors.Is(err, readErr) {
			t.Fatalf("read error = %v, want %v", err, readErr)
		}
	})
}

func assertPathValidation(t *testing.T, err error, wantReason string) {
	t.Helper()
	reason, field, ok := ValidationReasonFromError(err)
	if !ok || reason != wantReason || field != "folderPath" {
		t.Fatalf("validation = (%q, %q, %v), want (%q, folderPath, true)", reason, field, ok, wantReason)
	}
}

func TestSessionFactoryRoots(t *testing.T) {
	serviceRoot := filepath.Join("workspace", factorydefinitions.FactoryDir)
	defaultSession := &livesession.LiveSession{IsDefault: true, SessionState: livesession.SessionState{FolderPath: "workspace", FactoryDir: serviceRoot}}
	if got := SessionFactoryRootDir(serviceRoot, defaultSession); got != "workspace" {
		t.Fatalf("SessionFactoryRootDir(default) = %q, want workspace", got)
	}
	named := &livesession.LiveSession{SessionState: livesession.SessionState{FolderPath: "named", FactoryDir: filepath.Join("named", factorydefinitions.FactoryDir)}}
	if got := SessionFactoryPersistRoot(serviceRoot, named); got != "named" {
		t.Fatalf("SessionFactoryPersistRoot(named) = %q, want named", got)
	}
}
