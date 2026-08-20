package filesystem_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	checkpointrecovery "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/checkpoint_recovery/internal/filesystem"
)

func TestStoreRoundTripsEnvelopeAcrossFreshInstances(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	writer, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New(writer): %v", err)
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

	reader, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New(reader): %v", err)
	}
	got, err := reader.Get(envelope.CheckpointID)
	if err != nil {
		t.Fatalf("Get(): %v", err)
	}
	if got.CheckpointID != envelope.CheckpointID ||
		got.SchemaVersion != envelope.SchemaVersion ||
		got.StrategyKind != envelope.StrategyKind ||
		!equalBytes(got.Payload, envelope.Payload) {
		t.Fatalf("Get() = %#v, want %#v", got, envelope)
	}

	envelope.Payload[0] = 0x55
	if got.Payload[0] != 0x00 {
		t.Fatalf("stored payload changed after caller mutation: %x", got.Payload)
	}
}

func TestStoreCreatesRootAndAtomicallyReplacesEnvelope(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "nested", "durable-checkpoints")
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New(): %v", err)
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
	reader, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New(reader): %v", err)
	}
	got, err := reader.Get(second.CheckpointID)
	if err != nil {
		t.Fatalf("Get(replacement): %v", err)
	}
	if !equalBytes(got.Payload, second.Payload) {
		t.Fatalf("replacement payload = %q, want %q", got.Payload, second.Payload)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("ReadDir(): %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(checkpointPath(root, second.CheckpointID)) {
		t.Fatalf("durable entries = %#v, want one committed checkpoint and no temporary file", entryNames(entries))
	}
}

func TestStoreIsolatesMissingAndCorruptCheckpoints(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if _, err := store.Get("missing"); !errors.Is(err, checkpointrecovery.ErrCheckpointNotFound) {
		t.Fatalf("Get(missing before root creation) error = %v, want ErrCheckpointNotFound", err)
	}
	neighbor := validEnvelope("neighbor", []byte("valid"))
	if err := store.Put(neighbor); err != nil {
		t.Fatalf("Put(neighbor): %v", err)
	}
	corruptID := "corrupt"
	if err := os.WriteFile(checkpointPath(root, corruptID), []byte(`{"formatVersion":1,"checkpointId":"corrupt","schemaVersion":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile(corrupt): %v", err)
	}
	if _, err := store.Get(corruptID); !errors.Is(err, checkpointrecovery.ErrCorruptCheckpoint) {
		t.Fatalf("Get(corrupt) error = %v, want ErrCorruptCheckpoint", err)
	}
	got, err := store.Get(neighbor.CheckpointID)
	if err != nil {
		t.Fatalf("Get(neighbor after corrupt read): %v", err)
	}
	if !equalBytes(got.Payload, neighbor.Payload) {
		t.Fatalf("neighbor payload = %q, want %q", got.Payload, neighbor.Payload)
	}
}

func TestStoreRejectsInvalidInputAndKeepsIDsInsideRoot(t *testing.T) {
	t.Parallel()

	if _, err := filesystem.New(" "); err == nil {
		t.Fatal("New(empty) = nil, want directory error")
	}
	root := filepath.Join(t.TempDir(), "durable-checkpoints")
	store, err := filesystem.New(root)
	if err != nil {
		t.Fatalf("New(): %v", err)
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
	relative, err := filepath.Rel(root, checkpointPath(root, hostileID))
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

func checkpointPath(root, id string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(id)))
	return filepath.Join(root, hex.EncodeToString(digest[:])+".json")
}

func equalBytes(left, right []byte) bool {
	return string(left) == string(right)
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
