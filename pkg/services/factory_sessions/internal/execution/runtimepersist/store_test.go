package runtimepersist_test

import (
	"bytes"
	"context"
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

type boardReferenceFiles struct {
	platformfilesystem.Local
	boundedLimit          int64
	boundedErr            error
	data                  []byte
	readErr, writeErr     error
	afterRead             func()
	reads, writes, mkdirs int
	path                  string
	mode                  fs.FileMode
}

func (files *boardReferenceFiles) MkdirAll(string, fs.FileMode) error {
	files.mkdirs++
	return nil
}

func (files *boardReferenceFiles) ReadFile(path string) ([]byte, error) {
	files.reads++
	files.path = path
	if files.afterRead != nil {
		files.afterRead()
	}
	return append([]byte(nil), files.data...), files.readErr
}

func (files *boardReferenceFiles) ReadFileBounded(path string, limit int64) ([]byte, error) {
	files.boundedLimit = limit
	if files.boundedErr != nil {
		return nil, files.boundedErr
	}
	return files.ReadFile(path)
}

func (files *boardReferenceFiles) WriteFile(path string, data []byte, mode fs.FileMode) error {
	files.writes++
	if files.writeErr != nil {
		return files.writeErr
	}
	files.path, files.mode = path, mode
	files.data = append([]byte(nil), data...)
	return nil
}

func TestSnapshotLoadUsesInclusiveBoundAndRetainsTypedFailure(t *testing.T) {
	t.Parallel()
	files := &boardReferenceFiles{data: []byte("snapshot")}
	store, err := runtimepersist.NewLazyProjectStore(t.TempDir(), files)
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Load("~default")
	if err != nil || !bytes.Equal(got, files.data) || files.boundedLimit != 67108864 {
		t.Fatalf("load = %q, %v; bound = %d", got, err, files.boundedLimit)
	}
	files.boundedErr = &platformfilesystem.ReadSizeLimitError{Limit: files.boundedLimit}
	got, err = store.Load("~default")
	var sizeErr *platformfilesystem.ReadSizeLimitError
	if got != nil || !errors.As(err, &sizeErr) || sizeErr.ReadSizeLimit() != 67108864 {
		t.Fatalf("size failure lost its typed cause: %q, %v", got, err)
	}
	if files.writes != 0 || files.mkdirs != 0 {
		t.Fatal("bounded load changed persisted evidence")
	}
}

func newBoardReferenceStore(t *testing.T, files *boardReferenceFiles) (runtimepersist.CurrentBoardStore, string) {
	t.Helper()
	root := t.TempDir()
	store, err := runtimepersist.NewLazyProjectStore(root, files)
	if err != nil {
		t.Fatal(err)
	}
	return store.(runtimepersist.CurrentBoardStore), root
}

func TestCurrentBoardReferenceRoundTripAndScope(t *testing.T) {
	t.Parallel()
	files := &boardReferenceFiles{readErr: fs.ErrNotExist}
	store, root := newBoardReferenceStore(t, files)
	factory := filepath.Join(root, "factory")
	artifact := filepath.Join(t.TempDir(), "recording-§-—.json")
	path, err := store.LoadCurrentBoard(t.Context(), factory)
	if err != nil || path != "" || files.writes != 0 || files.mkdirs != 0 {
		t.Fatalf("absent reference = %q/%v; writes=%d mkdirs=%d", path, err, files.writes, files.mkdirs)
	}
	if err := store.SaveCurrentBoard(t.Context(), factory, artifact); err != nil {
		t.Fatal(err)
	}
	if files.path != filepath.Join(root, ".you-agent-factory", "current-board.json") || files.mode != 0o600 {
		t.Fatalf("reference path/mode = %s/%o", files.path, files.mode)
	}
	files.readErr = nil
	path, err = store.LoadCurrentBoard(t.Context(), filepath.Join(factory, "."))
	if err != nil || path != artifact {
		t.Fatalf("round trip = %q/%v", path, err)
	}
	prior := append([]byte(nil), files.data...)
	if _, err := store.LoadCurrentBoard(t.Context(), filepath.Join(root, "sibling", "factory")); err == nil {
		t.Fatal("foreign Factory reference was accepted")
	}
	if !bytes.Equal(prior, files.data) || files.writes != 1 {
		t.Fatal("read rewrote the reference")
	}
}

func TestCurrentBoardReferenceRejectsInvalidContractWithoutWriting(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"duplicate", "unknown", "case", "null", "missing", "version", "session", "relative", "remote", "trailing", "malformed"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := &boardReferenceFiles{}
			store, root := newBoardReferenceStore(t, files)
			factory := filepath.Join(root, "factory")
			if err := store.SaveCurrentBoard(t.Context(), factory, filepath.Join(root, "recording.json")); err != nil {
				t.Fatal(err)
			}
			data := string(files.data)
			data = invalidBoardReferenceData(t, name, data)
			files.data = []byte(data)
			path, err := store.LoadCurrentBoard(t.Context(), factory)
			if err == nil || path != "" || files.writes != 1 || string(files.data) != data {
				t.Fatalf("invalid reference = %q/%v; writes=%d", path, err, files.writes)
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("validation leaked contents: %v", err)
			}
		})
	}
}

func TestCurrentBoardReferenceCancellationAndWriteFailure(t *testing.T) {
	t.Parallel()
	files := &boardReferenceFiles{}
	store, root := newBoardReferenceStore(t, files)
	factory, artifact := filepath.Join(root, "factory"), filepath.Join(root, "recording.json")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.LoadCurrentBoard(ctx, factory); !errors.Is(err, context.Canceled) || files.reads != 0 {
		t.Fatalf("canceled read = %v", err)
	}
	if err := store.SaveCurrentBoard(ctx, factory, artifact); !errors.Is(err, context.Canceled) || files.writes != 0 {
		t.Fatalf("canceled write = %v", err)
	}
	if err := store.SaveCurrentBoard(t.Context(), factory, artifact); err != nil {
		t.Fatal(err)
	}
	prior := append([]byte(nil), files.data...)
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	files.afterRead = cancel
	if _, err := store.LoadCurrentBoard(ctx, factory); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel during read = %v", err)
	}
	fault := errors.New("credential=secret")
	files.writeErr = fault
	err := store.SaveCurrentBoard(t.Context(), factory, filepath.Join(root, "successor.json"))
	var diagnostic interface{ CLIErrorMessage() string }
	if !errors.Is(err, fault) || !errors.As(err, &diagnostic) || strings.Contains(diagnostic.CLIErrorMessage(), "secret") || !bytes.Equal(prior, files.data) {
		t.Fatalf("failed replacement lost prior bytes/cause/safe message: %v", err)
	}
}

type failingFileSystem struct {
	mkdirErr error
	readErr  error
	writeErr error
}

func TestPersistenceFailurePreservesCauseAndPublishesSafeOperation(t *testing.T) {
	t.Parallel()
	fault := errors.New("private-path and credential=secret")
	for _, test := range []struct {
		name    string
		files   failingFileSystem
		read    bool
		message string
	}{
		{"mkdir", failingFileSystem{mkdirErr: fault}, false, "create durable session persistence directory failed"},
		{"write", failingFileSystem{writeErr: fault}, false, "write durable session snapshot failed"},
		{"read", failingFileSystem{readErr: fault}, true, "read durable session snapshot failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := runtimepersist.NewLazyProjectStore("project", test.files)
			if err != nil {
				t.Fatal(err)
			}
			if test.read {
				_, err = store.Load("~default")
			} else {
				err = store.Save("~default", []byte(`{}`))
			}
			var diagnostic interface {
				CLIErrorCode() string
				CLIErrorMessage() string
			}
			if !errors.Is(err, fault) || !errors.As(err, &diagnostic) {
				t.Fatalf("persistence error lost cause or safe diagnostic: %v", err)
			}
			if diagnostic.CLIErrorCode() != "DURABLE_SESSION_PERSISTENCE_FAILED" || diagnostic.CLIErrorMessage() != test.message {
				t.Fatalf("diagnostic = %s/%s", diagnostic.CLIErrorCode(), diagnostic.CLIErrorMessage())
			}
		})
	}
}

func (f failingFileSystem) MkdirAll(string, fs.FileMode) error            { return f.mkdirErr }
func (f failingFileSystem) ReadFile(string) ([]byte, error)               { return nil, f.readErr }
func (f failingFileSystem) ReadFileBounded(string, int64) ([]byte, error) { return nil, f.readErr }
func (f failingFileSystem) RenameNoReplace(string, string) error          { return f.writeErr }
func (f failingFileSystem) WriteFile(string, []byte, fs.FileMode) error {
	return f.writeErr
}

func TestNewLazyProjectStore_ConstructsSnapshotBoundaryAndRoundTrips(t *testing.T) {
	projectRoot := t.TempDir()
	store, err := runtimepersist.NewLazyProjectStore(projectRoot, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewLazyProjectStore: %v", err)
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

func TestNewLazyProjectStore_SaveRejectsUnavailableRoot(t *testing.T) {
	if _, err := runtimepersist.NewLazyProjectStore("   ", platformfilesystem.Local{}); err == nil {
		t.Fatal("NewLazyProjectStore(blank) error = nil")
	}
	blockedRoot := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blockedRoot, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store, err := runtimepersist.NewLazyProjectStore(blockedRoot, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("lazy construction: %v", err)
	}
	if err := store.Save("~default", []byte(`{}`)); err == nil || !strings.Contains(err.Error(), "create durable session persistence directory") {
		t.Fatalf("Save(blocked root) error = %v, want actionable initialization failure", err)
	}
}

func TestNewLazyProjectStore_DefersInitializationAndReportsSnapshotPath(t *testing.T) {
	projectRoot := t.TempDir()
	store, err := runtimepersist.NewLazyProjectStore(projectRoot, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewLazyProjectStore: %v", err)
	}
	dir := runtimepersist.DirForProjectRoot(projectRoot)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persistence directory before Save: %v, want not exist", err)
	}
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pathResolver, ok := store.(runtimepersist.SnapshotPathResolver)
	if !ok {
		t.Fatal("lazy store does not report its snapshot path")
	}
	if got, want := pathResolver.SnapshotPath(sessionID), filepath.Join(dir, sessionID+".json"); got != want {
		t.Fatalf("snapshot path = %q, want %q", got, want)
	}
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

func TestNewLazyProjectStore_RejectsMissingDependencies(t *testing.T) {
	if _, err := runtimepersist.NewLazyProjectStore("   ", platformfilesystem.Local{}); err == nil || !strings.Contains(err.Error(), "project root is required") {
		t.Fatalf("NewLazyProjectStore(blank root) error = %v", err)
	}
	if _, err := runtimepersist.NewLazyProjectStore(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewLazyProjectStore(nil filesystem) error = %v", err)
	}
}

func TestDirectoryPersistence_RejectsBlankDirectory(t *testing.T) {
	files := platformfilesystem.Local{}
	const sessionID = "dur-sess-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := runtimepersist.SaveBytes("   ", sessionID, []byte(`{}`), files); err == nil || !strings.Contains(err.Error(), "directory is required") {
		t.Fatalf("SaveBytes(blank directory) error = %v", err)
	}
	if _, err := runtimepersist.LoadBytes("   ", sessionID, files); err == nil || !strings.Contains(err.Error(), "directory is required") {
		t.Fatalf("LoadBytes(blank directory) error = %v", err)
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
	if _, err := runtimepersist.NewLazyProjectStore(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewLazyProjectStore(nil filesystem) error = %v", err)
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
	store, err := runtimepersist.NewLazyProjectStore(t.TempDir(), failingFileSystem{mkdirErr: mkdirErr})
	if err != nil {
		t.Fatalf("lazy construction: %v", err)
	}
	if err := store.Save(sessionID, nil); !errors.Is(err, mkdirErr) || !strings.Contains(err.Error(), "create durable session persistence directory") {
		t.Fatalf("Save injected mkdir error = %v", err)
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

func (s *interruptingStorage) ReadFileBounded(path string, limit int64) ([]byte, error) {
	return s.directories.ReadFileBounded(path, limit)
}

func (s *interruptingStorage) RenameNoReplace(source, destination string) error {
	return s.directories.RenameNoReplace(source, destination)
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
	store, err := runtimepersist.NewLazyProjectStore(t.TempDir(), storage)
	if err != nil {
		t.Fatalf("NewLazyProjectStore: %v", err)
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

func invalidBoardReferenceData(t *testing.T, name, data string) string {
	t.Helper()
	switch name {
	case "duplicate":
		data = strings.TrimSuffix(data, "}") + `,"schemaVersion":"secret"}`
	case "unknown":
		data = strings.TrimSuffix(data, "}") + `,"secret":"private"}`
	case "case":
		data = strings.Replace(data, `"schemaVersion"`, `"SchemaVersion"`, 1)
	case "null":
		data = strings.Replace(data, `"factory-sessions.current-board.v1"`, `null`, 1)
	case "missing":
		data = `{}`
	case "version":
		data = strings.Replace(data, "current-board.v1", "current-board.secret", 1)
	case "session":
		data = strings.Replace(data, "~default", "foreign-secret", 1)
	case "relative", "remote":
		var fields map[string]any
		if err := json.Unmarshal([]byte(data), &fields); err != nil {
			t.Fatal(err)
		}
		fields["artifactReference"] = "secret.json"
		if name == "remote" {
			fields["artifactReference"] = "https://secret.example/board.json"
		}
		encoded, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		data = string(encoded)
	case "trailing":
		data += ` {"secret":true}`
	case "malformed":
		data = `{"secret":`
	}
	return data
}

func TestCurrentBoardReferenceClassifiesLocalDamageButRejectsForeignSelection(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"malformed", "unknown", "session", "relative", "limit", "read"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := &boardReferenceFiles{}
			store, root := newBoardReferenceStore(t, files)
			factory := filepath.Join(root, "factory")
			if err := store.SaveCurrentBoard(t.Context(), factory, filepath.Join(root, "history.json")); err != nil {
				t.Fatal(err)
			}
			if name == "limit" {
				files.boundedErr = &platformfilesystem.ReadSizeLimitError{Limit: 64 << 20}
			} else if name == "read" {
				files.readErr = fs.ErrPermission
			} else {
				files.data = []byte(invalidBoardReferenceData(t, name, string(files.data)))
			}
			_, err := store.LoadCurrentBoard(t.Context(), factory)
			var classified interface {
				SnapshotFailureCause() string
				CurrentBoardReferenceFailure()
			}
			local := errors.As(err, &classified)
			expected := map[string]string{"malformed": "INVALID_JSON", "unknown": "INVALID_SCHEMA", "limit": "SIZE_LIMIT", "read": "READ_FAILED"}[name]
			if err == nil || local != (expected != "") || (local && classified.SnapshotFailureCause() != expected) || files.boundedLimit != 64<<20 || files.writes != 1 {
				t.Fatalf("reference classification %v, local=%v, bound=%d, writes=%d", err, local, files.boundedLimit, files.writes)
			}
		})
	}
}

func TestCurrentBoardReferenceAbsentOnlyPublication(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"absent", "existing invalid", "unreadable", "cancelled", "cancel during read", "write failure"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			files := &boardReferenceFiles{data: []byte("invalid § —"), readErr: fs.ErrNotExist}
			store, root := newBoardReferenceStore(t, files)
			publisher := store.(interface {
				SaveCurrentBoardIfAbsent(context.Context, string, string) error
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch name {
			case "existing invalid":
				files.readErr = nil
			case "unreadable":
				files.readErr = fs.ErrPermission
			case "cancelled":
				cancel()
			case "cancel during read":
				files.afterRead = cancel
			case "write failure":
				files.writeErr = fs.ErrPermission
			}
			err := publisher.SaveCurrentBoardIfAbsent(ctx, filepath.Join(root, "factory"), filepath.Join(root, "board.json"))
			if (err != nil) != (name != "absent" && name != "existing invalid") {
				t.Fatalf("publication: %v", err)
			}
			if name == "absent" {
				if files.writes != 1 {
					t.Fatal("absent reference was not published")
				}
			} else if string(files.data) != "invalid § —" || files.mkdirs != 0 && name != "write failure" {
				t.Fatal("publication changed existing bytes")
			}
		})
	}
}
