package wire

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	providersessionsinternal "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	modulePrefix                    = "github.com/portpowered/infinite-you/"
	providerSessionsWirePackage     = modulePrefix + "pkg/services/provider_sessions/wire"
	providerSessionsOwnerImportPath = modulePrefix + "pkg/services/provider_sessions"
)

// TestWireDoesNotImportUnexpectedPublicSiblingsBeyondService seals
// pss-cln-pses-legacy-packages-003: provider_sessions/wire composition must not
// introduce or retain imports of unexpected public sibling packages beyond
// service/.

func TestNewForRootsConstructsInertRoot(t *testing.T) {
	t.Parallel()

	cursorWalk := &recordingCursorWalkDirectory{}
	cursorSymlinks := &recordingCursorResolveSymlinks{}
	cursorDatabase := &recordingCursorOpenDatabase{}

	service, err := NewForRoots(
		platformfilesystem.Local{},
		cursorWalk.walk,
		cursorSymlinks.resolve,
		cursorDatabase.open,
		t.TempDir(),
		emptyCapturedReader{},
	)
	if err != nil {
		t.Fatalf("NewForRoots() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewForRoots() returned nil service")
	}
	var root providersessions.Service = service
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
}

func TestNewServiceConstructsInertRoot(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor", "chats"), 0o755); err != nil {
		t.Fatalf("mkdir cursor chats: %v", err)
	}
	cursorWalk := &recordingCursorWalkDirectory{}
	cursorSymlinks := &recordingCursorResolveSymlinks{}
	cursorDatabase := &recordingCursorOpenDatabase{}

	service, err := NewService(
		platformfilesystem.Local{},
		func() (string, error) { return home, nil },
		cursorWalk.walk,
		cursorSymlinks.resolve,
		cursorDatabase.open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
		emptyCapturedReader{},
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil service")
	}
	var root providersessions.Service = service
	if cursorWalk.calls != 0 || cursorSymlinks.calls != 0 || cursorDatabase.calls != 0 {
		t.Fatal("construction performed Cursor storage effects")
	}
	if _, err := root.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providersessions.SessionIDKind,
		ID:       "missing-from-default-root",
	}}); !errors.Is(err, providersessions.ErrSessionNotFound) {
		t.Fatalf("Inspect() = %v, want ErrSessionNotFound", err)
	}
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

	resolveHome := providersessionsinternal.ResolveHomeDirectory(func() (string, error) { return t.TempDir(), nil })
	tests := []struct {
		name                  string
		files                 FileSystem
		home                  ResolveHomeDirectory
		cursorWalk            CursorWalkDirectory
		cursorSymlinks        CursorResolveSymlinks
		cursorDatabase        CursorOpenSQLDatabase
		cursorOperatingSystem OperatingSystem
	}{
		{name: "filesystem", home: resolveHome, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: OperatingSystem(runtime.GOOS)},
		{name: "home", files: platformfilesystem.Local{}, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: OperatingSystem(runtime.GOOS)},
		{name: "cursor walk", files: platformfilesystem.Local{}, home: resolveHome, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open, cursorOperatingSystem: OperatingSystem(runtime.GOOS)},
		{name: "cursor symlinks", files: platformfilesystem.Local{}, home: resolveHome, cursorWalk: filepath.WalkDir, cursorDatabase: sql.Open, cursorOperatingSystem: OperatingSystem(runtime.GOOS)},
		{name: "cursor database", files: platformfilesystem.Local{}, home: resolveHome, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorOperatingSystem: OperatingSystem(runtime.GOOS)},
		{name: "OS", files: platformfilesystem.Local{}, home: resolveHome, cursorWalk: filepath.WalkDir, cursorSymlinks: filepath.EvalSymlinks, cursorDatabase: sql.Open},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, err := NewService(
				test.files,
				test.home,
				test.cursorWalk,
				test.cursorSymlinks,
				test.cursorDatabase,
				test.cursorOperatingSystem,
				emptyCapturedReader{},
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
		sql.Open,
		providersessionsinternal.OperatingSystem(runtime.GOOS),
		emptyCapturedReader{},
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

func TestNewForRootsReturnsUnsupportedProviderForUnknownProvider(t *testing.T) {
	t.Parallel()

	service, err := NewForRoots(
		platformfilesystem.Local{},
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		t.TempDir(),
		emptyCapturedReader{},
	)
	if err != nil {
		t.Fatalf("NewForRoots() error = %v", err)
	}
	var root providersessions.Service = service

	_, err = root.Inspect(providersessions.InspectRequest{Session: providers.SessionRef{
		Provider: "openai",
		Kind:     providersessions.SessionIDKind,
		ID:       "session-1",
	}})
	if !errors.Is(err, providersessions.ErrUnsupportedProvider) {
		t.Fatalf("Inspect() = %v, want ErrUnsupportedProvider", err)
	}
}

func TestNewForRootsRejectsMissingProcessEdges(t *testing.T) {
	t.Parallel()

	service, err := NewForRoots(
		nil,
		filepath.WalkDir,
		filepath.EvalSymlinks,
		sql.Open,
		t.TempDir(),
		emptyCapturedReader{},
	)
	if err == nil {
		t.Fatal("NewForRoots() error = nil, want missing filesystem dependency")
	}
	if service != nil {
		t.Fatalf("NewForRoots() = %#v, want nil service", service)
	}
}

type emptyCapturedReader struct{}

func (emptyCapturedReader) ListWorkerSessionCaptures(context.Context, recordings.WorkerCapturedCatalogRequest) (recordings.WorkerCapturedCatalogPage, error) {
	return recordings.WorkerCapturedCatalogPage{}, nil
}
func (emptyCapturedReader) ReadWorkerCapturedActivity(context.Context, recordings.WorkerCapturedActivityRequest) (recordings.WorkerCapturedActivityPage, error) {
	panic("unexpected activity read")
}
func (emptyCapturedReader) LookupWorkerSessionCapture(context.Context, string) (recordings.WorkerSessionCatalogEntry, error) {
	panic("unexpected lookup")
}
