package wire

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func TestNewServiceConstructsInertReader(t *testing.T) {
	t.Parallel()

	walk := panicWalkDirectory{}
	resolve := panicResolveSymlinks{}
	open := panicOpenDatabase{}
	reader, err := NewService(platformfilesystem.Local{}, walk.walk, resolve.resolve, open.open, t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if reader == nil {
		t.Fatal("NewService returned nil reader")
	}
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	tests := []struct {
		name    string
		files   providersessions.FileSystem
		walk    providersessions.CursorWalkDirectory
		resolve providersessions.CursorResolveSymlinks
		open    providersessions.CursorOpenSQLDatabase
	}{
		{name: "filesystem", walk: filepath.WalkDir, resolve: filepath.EvalSymlinks, open: sql.Open},
		{name: "directory walker", files: platformfilesystem.Local{}, resolve: filepath.EvalSymlinks, open: sql.Open},
		{name: "symlink resolver", files: platformfilesystem.Local{}, walk: filepath.WalkDir, open: sql.Open},
		{name: "database opener", files: platformfilesystem.Local{}, walk: filepath.WalkDir, resolve: filepath.EvalSymlinks},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(test.files, test.walk, test.resolve, test.open, root)
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
		})
	}
}

type panicWalkDirectory struct{}

func (panicWalkDirectory) walk(string, fs.WalkDirFunc) error {
	panic("directory walk during Cursor reader construction")
}

type panicResolveSymlinks struct{}

func (panicResolveSymlinks) resolve(string) (string, error) {
	panic("symlink resolution during Cursor reader construction")
}

type panicOpenDatabase struct{}

func (panicOpenDatabase) open(string, string) (*sql.DB, error) {
	panic("database open during Cursor reader construction")
}
