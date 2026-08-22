package runtimepersist_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
)

type failingFileSystem struct {
	mkdirErr error
	readErr  error
	writeErr error
}

func (f failingFileSystem) MkdirAll(string, fs.FileMode) error { return f.mkdirErr }
func (f failingFileSystem) ReadFile(string) ([]byte, error)    { return nil, f.readErr }
func (f failingFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	return f.writeErr
}

func TestNewProjectStore_ConstructsSnapshotBoundaryAndRoundTrips(t *testing.T) {
	projectRoot := t.TempDir()
	store, err := runtimepersist.NewProjectStore(projectRoot, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewProjectStore: %v", err)
	}
	if got, want := store.(runtimepersist.DirectoryStore).Dir, runtimepersist.DirForProjectRoot(projectRoot); got != want {
		t.Fatalf("store directory = %q, want %q", got, want)
	}
	sessionID := "dur-sess-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	payload := []byte(`{"status":"COMPLETED"}`)
	if err := store.Save(sessionID, payload); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(loaded) != string(payload) {
		t.Fatalf("loaded payload = %s, want %s", loaded, payload)
	}
}

func TestNewProjectStore_RejectsMissingAndUnavailableRoots(t *testing.T) {
	if _, err := runtimepersist.NewProjectStore("   ", platformfilesystem.Local{}); err == nil {
		t.Fatal("NewProjectStore(blank) error = nil")
	}
	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := runtimepersist.NewProjectStore(blockedRoot, platformfilesystem.Local{}); err == nil || !strings.Contains(err.Error(), "initialize durable session persistence directory") {
		t.Fatalf("NewProjectStore(blocked) error = %v, want actionable initialization failure", err)
	}
}

func TestSaveLoadBytes_RoundTripsSnapshotPayload(t *testing.T) {
	dir := t.TempDir()
	sessionID := "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	payload := map[string]string{"sessionId": sessionID}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if err := runtimepersist.SaveBytes(dir, sessionID, encoded, platformfilesystem.Local{}); err != nil {
		t.Fatalf("SaveBytes: %v", err)
	}
	loaded, err := runtimepersist.LoadBytes(dir, sessionID, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if string(loaded) != string(encoded) {
		t.Fatalf("loaded payload = %s, want %s", loaded, encoded)
	}
	if _, err := os.Stat(filepath.Join(dir, sessionID+".json")); err != nil {
		t.Fatalf("stat persisted snapshot: %v", err)
	}
}

func TestSaveLoadBytes_AcceptsCanonicalFactorySessionIdentifiers(t *testing.T) {
	for _, sessionID := range []string{
		"~default",
		"12345678-1234-1234-1234-1234567890ab",
	} {
		t.Run(sessionID, func(t *testing.T) {
			dir := t.TempDir()
			if err := runtimepersist.SaveBytes(dir, sessionID, []byte(`{"status":"RUNNING"}`), platformfilesystem.Local{}); err != nil {
				t.Fatalf("SaveBytes: %v", err)
			}
			if _, err := runtimepersist.LoadBytes(dir, sessionID, platformfilesystem.Local{}); err != nil {
				t.Fatalf("LoadBytes: %v", err)
			}
		})
	}
}

func TestSaveBytes_RejectsUnsafeSessionIdentifiers(t *testing.T) {
	for _, sessionID := range []string{"../escape", "session/child", "arbitrary"} {
		if err := runtimepersist.SaveBytes(t.TempDir(), sessionID, []byte(`{}`), platformfilesystem.Local{}); err == nil {
			t.Fatalf("SaveBytes(%q) succeeded", sessionID)
		}
	}
}

func TestStoreFailsClosedWithoutFileSystem(t *testing.T) {
	if _, err := runtimepersist.NewProjectStore(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewProjectStore(nil filesystem) error = %v", err)
	}
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := runtimepersist.SaveBytes(t.TempDir(), sessionID, nil, nil); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("SaveBytes(nil filesystem) error = %v", err)
	}
	if _, err := runtimepersist.LoadBytes(t.TempDir(), sessionID, nil); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("LoadBytes(nil filesystem) error = %v", err)
	}
}

func TestStorePropagatesInjectedFileSystemFailures(t *testing.T) {
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	mkdirErr := errors.New("mkdir unavailable")
	if _, err := runtimepersist.NewProjectStore(t.TempDir(), failingFileSystem{mkdirErr: mkdirErr}); !errors.Is(err, mkdirErr) || !strings.Contains(err.Error(), "initialize durable session persistence directory") {
		t.Fatalf("NewProjectStore injected mkdir error = %v", err)
	}
	if err := runtimepersist.SaveBytes(t.TempDir(), sessionID, nil, failingFileSystem{mkdirErr: mkdirErr}); !errors.Is(err, mkdirErr) || !strings.Contains(err.Error(), "create durable session persistence directory") {
		t.Fatalf("SaveBytes injected mkdir error = %v", err)
	}
	writeErr := errors.New("write unavailable")
	if err := runtimepersist.SaveBytes(t.TempDir(), sessionID, nil, failingFileSystem{writeErr: writeErr}); !errors.Is(err, writeErr) || !strings.Contains(err.Error(), "write durable session snapshot") {
		t.Fatalf("SaveBytes injected write error = %v", err)
	}
	readErr := errors.New("read unavailable")
	if _, err := runtimepersist.LoadBytes(t.TempDir(), sessionID, failingFileSystem{readErr: readErr}); !errors.Is(err, readErr) || !strings.Contains(err.Error(), "read durable session snapshot") {
		t.Fatalf("LoadBytes injected read error = %v", err)
	}
}

type interruptingStorage struct {
	directories platformfilesystem.Local
	delegate    platformreplay.Storage
	failWrite   bool
}

var errSnapshotInterrupted = errors.New("injected snapshot interruption")

func (s *interruptingStorage) MkdirAll(path string, mode fs.FileMode) error {
	return s.directories.MkdirAll(path, mode)
}

func (s *interruptingStorage) ReadFile(path string) ([]byte, error) {
	return s.delegate.ReadFile(path)
}

func (s *interruptingStorage) WriteFile(path string, data []byte, _ fs.FileMode) error {
	if s.failWrite {
		return errSnapshotInterrupted
	}
	return s.delegate.WriteFile(path, data)
}

func TestDirectoryStore_InterruptedSavePreservesPriorSnapshotAndSuccessfulSaveReplacesIt(t *testing.T) {
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	storage := &interruptingStorage{delegate: platformreplay.NewLocal(runtime.GOOS)}
	store, err := runtimepersist.NewDirectoryStore(t.TempDir(), storage)
	if err != nil {
		t.Fatalf("NewDirectoryStore: %v", err)
	}
	previous := []byte(`{"status":"RUNNING","sequence":1}`)
	next := []byte(`{"status":"COMPLETED","sequence":2}`)
	if err := store.Save(sessionID, previous); err != nil {
		t.Fatalf("Save(previous): %v", err)
	}

	storage.failWrite = true
	if err := store.Save(sessionID, next); err == nil || !errors.Is(err, errSnapshotInterrupted) || !strings.Contains(err.Error(), "write durable session snapshot") {
		t.Fatalf("Save(interrupted) error = %v, want wrapped interruption", err)
	}
	loaded, err := store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load after interrupted save: %v", err)
	}
	if !json.Valid(loaded) {
		t.Fatalf("snapshot after interrupted save is invalid JSON: %s", loaded)
	}
	if string(loaded) != string(previous) {
		t.Fatalf("snapshot after interrupted save = %s, want previous %s", loaded, previous)
	}

	storage.failWrite = false
	if err := store.Save(sessionID, next); err != nil {
		t.Fatalf("Save(next): %v", err)
	}
	loaded, err = store.Load(sessionID)
	if err != nil {
		t.Fatalf("Load after successful save: %v", err)
	}
	if !json.Valid(loaded) || string(loaded) != string(next) {
		t.Fatalf("snapshot after successful save = %s, want complete new payload %s", loaded, next)
	}
}
