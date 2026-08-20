package processlocal_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/processlocal"
)

func TestDurableOpaqueStoreRoundTripsEnvelopeAcrossFreshInstances(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	writer, err := processlocal.NewDurable(root, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDurable(writer): %v", err)
	}
	envelope := checkpointrecovery.Envelope{
		CheckpointID:  "checkpoint-1",
		SchemaVersion: 7,
		StrategyKind:  "opaque-strategy",
		Payload:       []byte{0x00, 0x01, 0x7f, 0xff},
	}
	if err := writer.Put(envelope); err != nil {
		t.Fatalf("Put(): %v", err)
	}

	reader, err := processlocal.NewDurable(root, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDurable(reader): %v", err)
	}
	got, err := reader.Get(envelope.CheckpointID)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.CheckpointID != envelope.CheckpointID ||
		got.SchemaVersion != envelope.SchemaVersion ||
		got.StrategyKind != envelope.StrategyKind ||
		!bytes.Equal(got.Payload, envelope.Payload) {
		t.Fatalf("Get() = %#v, want %#v", got, envelope)
	}

	envelope.Payload[0] = 0x55
	if got.Payload[0] != 0x00 {
		t.Fatalf("stored payload changed after caller mutation: %x", got.Payload)
	}
}

func TestDurableOpaqueStoreCreatesRootAndAtomicallyReplacesEnvelope(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "durable-checkpoints")
	store, err := processlocal.NewDurable(root, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDurable(): %v", err)
	}
	first := validEnvelope("checkpoint-1", []byte("first"))
	if err := store.Put(first); err != nil {
		t.Fatalf("Put(first): %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("durable root stat = %v, info = %#v; want created directory", err, info)
	}

	second := validEnvelope(first.CheckpointID, []byte("complete replacement"))
	if err := store.Put(second); err != nil {
		t.Fatalf("Put(second): %v", err)
	}
	reader, err := processlocal.NewDurable(root, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDurable(reader): %v", err)
	}
	got, err := reader.Get(second.CheckpointID)
	if err != nil {
		t.Fatalf("Get(replacement): %v", err)
	}
	if !bytes.Equal(got.Payload, second.Payload) {
		t.Fatalf("replacement payload = %q, want %q", got.Payload, second.Payload)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir(): %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(opaqueRecordPath(root, second.CheckpointID)) {
		t.Fatalf("durable entries = %#v, want one committed checkpoint and no temporary file", entryNames(entries))
	}
}

func TestDurableOpaqueStoreIsolatesMissingAndCorruptCheckpoints(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := processlocal.NewDurable(root, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDurable(): %v", err)
	}
	if _, err := store.Get("missing"); !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("Get(missing before root creation) error = %v, want ErrCheckpointNotFound", err)
	}
	neighbor := validEnvelope("neighbor", []byte("valid"))
	if err := store.Put(neighbor); err != nil {
		t.Fatalf("Put(neighbor): %v", err)
	}
	corruptID := "corrupt"
	if err := os.WriteFile(opaqueRecordPath(root, corruptID), []byte(`{"formatVersion":1,"checkpointId":"corrupt","schemaVersion":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}
	if _, err := store.Get(corruptID); !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
		t.Fatalf("Get(corrupt) error = %v, want ErrCorruptCheckpoint", err)
	}
	got, err := store.Get(neighbor.CheckpointID)
	if err != nil {
		t.Fatalf("Get(neighbor after corrupt read): %v", err)
	}
	if !bytes.Equal(got.Payload, neighbor.Payload) {
		t.Fatalf("neighbor payload = %q, want %q", got.Payload, neighbor.Payload)
	}
}

func TestDurableOpaqueStoreRejectsInvalidInputAndKeepsIDsInsideRoot(t *testing.T) {
	t.Parallel()

	if _, err := processlocal.NewDurable(" ", platformfilesystem.Local{}); err == nil {
		t.Fatal("NewDurable(empty) = nil, want directory error")
	}
	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := processlocal.NewDurable(root, platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewDurable(): %v", err)
	}
	if err := store.Put(checkpointrecovery.Envelope{CheckpointID: "invalid", SchemaVersion: 1}); !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
		t.Fatalf("Put(invalid) error = %v, want ErrCorruptCheckpoint", err)
	}
	hostileID := "../outside/..\\checkpoint"
	if err := store.Put(validEnvelope(hostileID, []byte("contained"))); err != nil {
		t.Fatalf("Put(hostile ID): %v", err)
	}
	got, err := store.Get(hostileID)
	if err != nil {
		t.Fatalf("Get(hostile ID): %v", err)
	}
	if got.CheckpointID != hostileID {
		t.Fatalf("hostile ID = %q, want %q", got.CheckpointID, hostileID)
	}
	relative, err := filepath.Rel(root, opaqueRecordPath(root, hostileID))
	if err != nil {
		t.Fatalf("Rel(): %v", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		t.Fatalf("checkpoint path escaped root: %q", relative)
	}
}

func TestDurableOpaqueStoreUsesInjectedFilesystemAndReportsAtomicWriteFailures(t *testing.T) {
	t.Parallel()

	constructor := processlocal.NewDurableCheckpointStore(platformfilesystem.Local{})
	if _, err := constructor(filepath.Join(t.TempDir(), "opaque-checkpoints")); err != nil {
		t.Fatalf("NewDurableCheckpointStore() error = %v", err)
	}
	if _, err := processlocal.NewDurable("root", nil); err == nil {
		t.Fatal("NewDurable(nil filesystem) error = nil, want required-filesystem error")
	}

	tests := []struct {
		name  string
		setup func(*fakeDurableFileSystem)
	}{
		{name: "mkdir", setup: func(files *fakeDurableFileSystem) { files.mkdirErr = errors.New("mkdir failed") }},
		{name: "create temp", setup: func(files *fakeDurableFileSystem) { files.createErr = errors.New("create failed") }},
		{name: "write", setup: func(files *fakeDurableFileSystem) { files.temporary.writeErr = errors.New("write failed") }},
		{name: "short write", setup: func(files *fakeDurableFileSystem) { files.temporary.shortWrite = true }},
		{name: "close", setup: func(files *fakeDurableFileSystem) { files.temporary.closeErr = errors.New("close failed") }},
		{name: "rename", setup: func(files *fakeDurableFileSystem) { files.renameErr = errors.New("rename failed") }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			files := &fakeDurableFileSystem{temporary: &fakeTemporaryFile{name: "checkpoint.tmp"}}
			test.setup(files)
			store, err := processlocal.NewDurable("root", files)
			if err != nil {
				t.Fatalf("NewDurable(): %v", err)
			}
			if err := store.Put(validEnvelope("checkpoint", []byte("payload"))); err == nil {
				t.Fatal("Put() error = nil, want injected filesystem failure")
			}
		})
	}
}

func TestDurableStoresRetainInjectedReadErrors(t *testing.T) {
	t.Parallel()

	files := &fakeDurableFileSystem{readErr: errors.New("read failed")}
	opaque, err := processlocal.NewDurable("root", files)
	if err != nil {
		t.Fatalf("NewDurable(): %v", err)
	}
	if _, err := opaque.Get("checkpoint"); err == nil {
		t.Fatal("opaque Get() error = nil, want injected read error")
	}

	constructor := processlocal.NewDurableJavaScriptCheckpointStore(files)
	javascript, err := constructor("root")
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(): %v", err)
	}
	if _, ok := javascript.Get("checkpoint"); ok {
		t.Fatal("JavaScript Get() = true, want false for injected read error")
	}
}

func TestDurableJavaScriptStoreRoundTripsRecordsAcrossFreshInstances(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	constructor := processlocal.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})
	writer, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(writer): %v", err)
	}
	want := validJavaScriptRecord("checkpoint-1", []byte{0x00, 0x01, 0x7f, 0xff})
	writer.Put(want)

	reader, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(reader): %v", err)
	}
	got, ok := reader.Get(want.ID)
	if !ok {
		t.Fatal("Get() returned false, want stored record")
	}
	if got.ID != want.ID || got.Label != want.Label || got.Summary != want.Summary ||
		!got.Timestamp.Equal(want.Timestamp) || got.ArtifactID != want.ArtifactID ||
		got.ContentHash != want.ContentHash || got.SizeBytes != want.SizeBytes ||
		!bytes.Equal(got.RawBody, want.RawBody) || got.StoragePath != want.StoragePath {
		t.Fatalf("Get() = %#v, want %#v", got, want)
	}

	want.RawBody[0] = 'x'
	if got.RawBody[0] != 0x00 {
		t.Fatalf("stored raw body changed after caller mutation: %q", got.RawBody)
	}
}

func TestDurableJavaScriptStorePreservesWhitespaceDistinctIDs(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	constructor := processlocal.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})
	store, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(): %v", err)
	}
	plain := validJavaScriptRecord("checkpoint", []byte("plain"))
	spaced := validJavaScriptRecord(" checkpoint ", []byte("spaced"))
	store.Put(plain)
	store.Put(spaced)

	reader, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(reader): %v", err)
	}
	for _, want := range []factorydefinitions.JavaScriptCheckpointRecord{plain, spaced} {
		got, ok := reader.Get(want.ID)
		if !ok || got.ID != want.ID || !bytes.Equal(got.RawBody, want.RawBody) {
			t.Fatalf("Get(%q) = (%#v, %t), want exact distinct record", want.ID, got, ok)
		}
	}
	records := reader.List()
	if len(records) != 2 || records[0].ID != spaced.ID || records[1].ID != plain.ID {
		t.Fatalf("List() IDs = %q, want exact stable order [%q %q]", javascriptRecordIDs(records), spaced.ID, plain.ID)
	}
}

func TestDurableJavaScriptStoreCreatesRootAndAtomicallyReplacesRecord(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "durable-checkpoints")
	constructor := processlocal.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})
	store, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(): %v", err)
	}
	first := validJavaScriptRecord("checkpoint-1", []byte(`{"version":1}`))
	store.Put(first)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("durable root stat = %v, info = %#v; want created directory", err, info)
	}

	second := validJavaScriptRecord(first.ID, []byte(`{"version":2,"complete":true}`))
	store.Put(second)
	reader, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(reader): %v", err)
	}
	got, ok := reader.Get(second.ID)
	if !ok {
		t.Fatal("Get(replacement) returned false")
	}
	if !bytes.Equal(got.RawBody, second.RawBody) {
		t.Fatalf("replacement raw body = %q, want %q", got.RawBody, second.RawBody)
	}
	entries, err := os.ReadDir(javascriptStorageRoot(root))
	if err != nil {
		t.Fatalf("ReadDir(): %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(javascriptRecordPath(javascriptStorageRoot(root), second.ID)) {
		t.Fatalf("durable entries = %#v, want one committed checkpoint and no temporary file", entryNames(entries))
	}
}

func TestDurableJavaScriptStoreSkipsMissingAndCorruptRecordsWhileListingNeighbors(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	constructor := processlocal.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})
	store, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(): %v", err)
	}
	if _, ok := store.Get("missing"); ok {
		t.Fatal("Get(missing before root creation) = true, want false")
	}
	if records := store.List(); records != nil {
		t.Fatalf("List(missing root) = %#v, want nil", records)
	}
	store.Put(validJavaScriptRecord("checkpoint-b", []byte(`{"id":"b"}`)))
	store.Put(validJavaScriptRecord("checkpoint-a", []byte(`{"id":"a"}`)))
	corruptID := "checkpoint-corrupt"
	if err := os.WriteFile(javascriptRecordPath(javascriptStorageRoot(root), corruptID), []byte(`{"formatVersion":1,"id":"checkpoint-corrupt"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}

	reader, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(reader): %v", err)
	}
	if _, ok := reader.Get(corruptID); ok {
		t.Fatal("Get(corrupt) = true, want false")
	}
	records := reader.List()
	if len(records) != 2 || records[0].ID != "checkpoint-a" || records[1].ID != "checkpoint-b" {
		t.Fatalf("List() IDs = %q, want [checkpoint-a checkpoint-b]", javascriptRecordIDs(records))
	}
}

func TestDurableJavaScriptStoreKeepsHostileIDsInsideRootAndRejectsInvalidRoots(t *testing.T) {
	t.Parallel()

	constructor := processlocal.NewDurableJavaScriptCheckpointStore(platformfilesystem.Local{})
	if _, err := constructor(" "); err == nil {
		t.Fatal("NewDurableJavaScriptCheckpointStore(empty) = nil, want directory error")
	}
	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := constructor(root)
	if err != nil {
		t.Fatalf("NewDurableJavaScriptCheckpointStore(): %v", err)
	}
	hostileID := "../outside/..\\checkpoint"
	record := validJavaScriptRecord(hostileID, []byte(`{"contained":true}`))
	store.Put(record)
	got, ok := store.Get(hostileID)
	if !ok || got.ID != hostileID {
		t.Fatalf("Get(hostile ID) = (%#v, %t), want stored record", got, ok)
	}
	relative, err := filepath.Rel(root, javascriptRecordPath(javascriptStorageRoot(root), hostileID))
	if err != nil {
		t.Fatalf("Rel(): %v", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		t.Fatalf("checkpoint path escaped root: %q", relative)
	}
}

func validEnvelope(id string, payload []byte) checkpointrecovery.Envelope {
	return checkpointrecovery.Envelope{
		CheckpointID:  id,
		SchemaVersion: 1,
		StrategyKind:  "opaque",
		Payload:       payload,
	}
}

func validJavaScriptRecord(id string, rawBody []byte) factorydefinitions.JavaScriptCheckpointRecord {
	return factorydefinitions.JavaScriptCheckpointRecord{
		ID:          id,
		Label:       "after-plan",
		Summary:     "checkpoint summary",
		Timestamp:   time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC),
		ArtifactID:  "artifact-1",
		ContentHash: "sha256:checkpoint",
		SizeBytes:   int64(len(rawBody)),
		RawBody:     rawBody,
		StoragePath: "checkpoint-1.json",
	}
}

func opaqueRecordPath(root, id string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(id)))
	return filepath.Join(root, hex.EncodeToString(digest[:])+".json")
}

func javascriptRecordPath(root, id string) string {
	digest := sha256.Sum256([]byte(id))
	return filepath.Join(root, hex.EncodeToString(digest[:])+".json")
}

func javascriptStorageRoot(root string) string {
	return filepath.Join(root, "javascript-checkpoints")
}

func javascriptRecordIDs(records []factorydefinitions.JavaScriptCheckpointRecord) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ID)
	}
	return ids
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

type fakeDurableFileSystem struct {
	readErr   error
	mkdirErr  error
	createErr error
	renameErr error
	removeErr error
	temporary *fakeTemporaryFile
}

func (f *fakeDurableFileSystem) ReadFile(string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return nil, os.ErrNotExist
}

func (*fakeDurableFileSystem) ReadDir(string) ([]fs.DirEntry, error) {
	return nil, os.ErrNotExist
}

func (f *fakeDurableFileSystem) MkdirAll(string, fs.FileMode) error {
	return f.mkdirErr
}

func (f *fakeDurableFileSystem) CreateTemp(string, string) (platformfilesystem.TemporaryFile, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.temporary == nil {
		f.temporary = &fakeTemporaryFile{name: "checkpoint.tmp"}
	}
	return f.temporary, nil
}

func (f *fakeDurableFileSystem) Remove(string) error {
	return f.removeErr
}

func (f *fakeDurableFileSystem) Rename(string, string) error {
	return f.renameErr
}

type fakeTemporaryFile struct {
	name       string
	writeErr   error
	closeErr   error
	shortWrite bool
}

func (f *fakeTemporaryFile) Name() string {
	return f.name
}

func (f *fakeTemporaryFile) WriteString(value string) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(value) - 1, nil
	}
	return len(value), nil
}

func (f *fakeTemporaryFile) Close() error {
	return f.closeErr
}
