package wire

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	resolveHome := providersessions.ResolveHomeDirectory(func() (string, error) { return t.TempDir(), nil })
	tests := []struct {
		name                  string
		files                 providersessions.FileSystem
		home                  providersessions.ResolveHomeDirectory
		codexWalk             providersessions.CodexWalkDirectory
		codexSymlinks         providersessions.CodexResolveSymlinks
		cursorWalk            providersessions.CursorWalkDirectory
		cursorSymlinks        providersessions.CursorResolveSymlinks
		cursorDatabase        providersessions.CursorOpenSQLDatabase
		cursorOperatingSystem providersessions.OperatingSystem
	}{
		{name: "filesystem", home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "home resolver", files: platformfilesystem.Local{}, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Codex walker", files: platformfilesystem.Local{}, home: resolveHome, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Codex symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Cursor walker", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Cursor symlink resolver", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorDatabase: sql.Open, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "Cursor database opener", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorOperatingSystem: providersessions.OperatingSystem(runtime.GOOS)},
		{name: "operating system", files: platformfilesystem.Local{}, home: resolveHome, codexWalk: filepath.WalkDir, codexSymlinks: filepath.EvalSymlinks, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(
				test.files,
				test.home,
				test.codexWalk,
				test.codexSymlinks,
				test.cursorWalk,
				test.cursorSymlinks,
				test.cursorDatabase,
				test.cursorOperatingSystem,
			)
			if err == nil {
				t.Fatalf("NewService() error = nil, want missing %s dependency", test.name)
			}
			if service != nil {
				t.Fatalf("NewService() = %#v, want nil service", service)
			}
		})
	}
}

func TestNewServiceConstructsPublishedRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir cursor chats: %v", err)
	}
	service, err := NewService(
		platformfilesystem.Local{},
		func() (string, error) { return home, nil },
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		providersessions.OperatingSystem(runtime.GOOS),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providersessions.Service = service
	if _, err := root.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-from-default-root",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Inspect() = %v, want ErrSessionNotFound", err)
	}
}

func TestNewForRootsRejectsMissingProcessEdges(t *testing.T) {
	t.Parallel()

	service, err := NewForRoots(
		nil,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		t.TempDir(),
		t.TempDir(),
	)
	if err == nil {
		t.Fatal("NewForRoots() error = nil, want missing filesystem dependency")
	}
	if service != nil {
		t.Fatalf("NewForRoots() = %#v, want nil service", service)
	}
}
