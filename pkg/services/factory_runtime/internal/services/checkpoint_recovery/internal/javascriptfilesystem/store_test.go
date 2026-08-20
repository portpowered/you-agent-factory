package javascriptfilesystem_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/javascriptfilesystem"
)

func TestStoreRoundTripsRecordsAcrossFreshInstances(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	writer, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(writer): %v", err)
	}
	want := validRecord("checkpoint-1", []byte{0x00, 0x01, 0x7f, 0xff})
	writer.Put(want)

	reader, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(reader): %v", err)
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

func TestStoreCreatesRootAndAtomicallyReplacesRecord(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "durable-checkpoints")
	store, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	first := validRecord("checkpoint-1", []byte(`{"version":1}`))
	store.Put(first)
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("durable root stat = %v, info = %#v; want created directory", err, info)
	}

	second := validRecord(first.ID, []byte(`{"version":2,"complete":true}`))
	store.Put(second)
	reader, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(reader): %v", err)
	}
	got, ok := reader.Get(second.ID)
	if !ok {
		t.Fatal("Get(replacement) returned false")
	}
	if !bytes.Equal(got.RawBody, second.RawBody) {
		t.Fatalf("replacement raw body = %q, want %q", got.RawBody, second.RawBody)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir(): %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(recordPath(root, second.ID)) {
		t.Fatalf("durable entries = %#v, want one committed checkpoint and no temporary file", entryNames(entries))
	}
}

func TestStoreSkipsMissingAndCorruptRecordsWhileListingNeighbors(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if _, ok := store.Get("missing"); ok {
		t.Fatal("Get(missing before root creation) = true, want false")
	}
	if records := store.List(); records != nil {
		t.Fatalf("List(missing root) = %#v, want nil", records)
	}
	store.Put(validRecord("checkpoint-b", []byte(`{"id":"b"}`)))
	store.Put(validRecord("checkpoint-a", []byte(`{"id":"a"}`)))
	corruptID := "checkpoint-corrupt"
	if err := os.WriteFile(recordPath(root, corruptID), []byte(`{"formatVersion":1,"id":"checkpoint-corrupt"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}

	reader, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(reader): %v", err)
	}
	if _, ok := reader.Get(corruptID); ok {
		t.Fatal("Get(corrupt) = true, want false")
	}
	records := reader.List()
	if len(records) != 2 || records[0].ID != "checkpoint-a" || records[1].ID != "checkpoint-b" {
		t.Fatalf("List() IDs = %q, want [checkpoint-a checkpoint-b]", recordIDs(records))
	}
}

func TestStoreKeepsHostileIDsInsideRootAndRejectsInvalidRoots(t *testing.T) {
	t.Parallel()

	if _, err := javascriptfilesystem.New(" "); err == nil {
		t.Fatal("New(empty) = nil, want directory error")
	}
	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := javascriptfilesystem.New(root)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	hostileID := "../outside/..\\checkpoint"
	record := validRecord(hostileID, []byte(`{"contained":true}`))
	store.Put(record)
	got, ok := store.Get(hostileID)
	if !ok || got.ID != hostileID {
		t.Fatalf("Get(hostile ID) = (%#v, %t), want stored record", got, ok)
	}
	relative, err := filepath.Rel(root, recordPath(root, hostileID))
	if err != nil {
		t.Fatalf("Rel(): %v", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		t.Fatalf("checkpoint path escaped root: %q", relative)
	}
}

func validRecord(id string, rawBody []byte) factorydefinitions.JavaScriptCheckpointRecord {
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

func recordPath(root, id string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(id)))
	return filepath.Join(root, hex.EncodeToString(digest[:])+".json")
}

func recordIDs(records []factorydefinitions.JavaScriptCheckpointRecord) []string {
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
