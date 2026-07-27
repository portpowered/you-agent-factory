package wire

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestNewForRootsConstructsInertRoot(t *testing.T) {
	t.Parallel()

	codexWalk := &recordingCodexWalkDirectory{}
	codexSymlinks := &recordingCodexResolveSymlinks{}
	cursorWalk := &recordingCursorWalkDirectory{}
	cursorSymlinks := &recordingCursorResolveSymlinks{}
	cursorDatabase := &recordingCursorOpenDatabase{}

	service, err := NewForRoots(
		platformfilesystem.Local{},
		codexWalk.walk,
		codexSymlinks.resolve,
		cursorWalk.walk,
		cursorSymlinks.resolve,
		cursorDatabase.open,
		t.TempDir(),
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("NewForRoots() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewForRoots() returned nil service")
	}
	var root providersessions.Service = service
	if codexWalk.calls != 0 {
		t.Fatalf("construction invoked Codex walk %d times, want no session discovery", codexWalk.calls)
	}
	if codexSymlinks.calls != 0 {
		t.Fatalf("construction invoked Codex symlink resolution %d times, want no filesystem activity", codexSymlinks.calls)
	}
	if cursorWalk.calls != 0 {
		t.Fatalf("construction invoked Cursor walk %d times, want no session discovery", cursorWalk.calls)
	}
	if cursorSymlinks.calls != 0 {
		t.Fatalf("construction invoked Cursor symlink resolution %d times, want no filesystem activity", cursorSymlinks.calls)
	}
	if cursorDatabase.calls != 0 {
		t.Fatalf("construction opened Cursor SQL %d times, want no database activity", cursorDatabase.calls)
	}
	if _, err := root.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-session",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Inspect() = %v, want ErrSessionNotFound after inert construction", err)
	}
	if codexWalk.calls == 0 {
		t.Fatal("Inspect() did not invoke Codex walk, want runtime session lookup")
	}
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir cursor chats: %v", err)
	}
	codexWalk := &recordingCodexWalkDirectory{}
	codexSymlinks := &recordingCodexResolveSymlinks{}
	cursorWalk := &recordingCursorWalkDirectory{}
	cursorSymlinks := &recordingCursorResolveSymlinks{}
	cursorDatabase := &recordingCursorOpenDatabase{}

	service, err := NewService(
		platformfilesystem.Local{},
		func() (string, error) { return home, nil },
		codexWalk.walk,
		codexSymlinks.resolve,
		cursorWalk.walk,
		cursorSymlinks.resolve,
		cursorDatabase.open,
		providersessions.OperatingSystem(runtime.GOOS),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providersessions.Service = service
	if codexWalk.calls != 0 || codexSymlinks.calls != 0 ||
		cursorWalk.calls != 0 || cursorSymlinks.calls != 0 || cursorDatabase.calls != 0 {
		t.Fatalf(
			"construction invoked walk/symlink/sql stubs (codex walk=%d symlinks=%d cursor walk=%d symlinks=%d sql=%d), want inert construction",
			codexWalk.calls, codexSymlinks.calls, cursorWalk.calls, cursorSymlinks.calls, cursorDatabase.calls,
		)
	}
	if _, err := root.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-from-default-root",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Inspect() = %v, want ErrSessionNotFound", err)
	}
}

type recordingCodexWalkDirectory struct{ calls int }

func (r *recordingCodexWalkDirectory) walk(string, fs.WalkDirFunc) error {
	r.calls++
	return nil
}

type recordingCodexResolveSymlinks struct{ calls int }

func (r *recordingCodexResolveSymlinks) resolve(string) (string, error) {
	r.calls++
	return "", nil
}

type recordingCursorWalkDirectory struct{ calls int }

func (r *recordingCursorWalkDirectory) walk(string, fs.WalkDirFunc) error {
	r.calls++
	return nil
}

type recordingCursorResolveSymlinks struct{ calls int }

func (r *recordingCursorResolveSymlinks) resolve(string) (string, error) {
	r.calls++
	return "", nil
}

type recordingCursorOpenDatabase struct{ calls int }

func (r *recordingCursorOpenDatabase) open(string, string) (*sql.DB, error) {
	r.calls++
	return nil, errors.New("database open during construction")
}

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
