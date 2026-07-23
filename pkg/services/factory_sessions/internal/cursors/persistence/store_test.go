package persistence_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors/persistence"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

func TestFileStoreRestartResumesAtNextObservableEvent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cursors")
	identity := testIdentity()
	first, err := persistence.NewFileStore(dir, platformfilesystem.Local{}, createTemporaryFile)
	if err != nil {
		t.Fatalf("NewFileStore(first): %v", err)
	}
	tracker, err := factorysessioncursors.NewTracker(first, identity)
	if err != nil {
		t.Fatalf("NewTracker(first): %v", err)
	}
	acknowledgedSequence := 2
	if err := tracker.Advance(context.Background(), factorysessioncursors.Checkpoint{
		AfterEventID: "event-2", AfterSequence: &acknowledgedSequence,
	}); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first): %v", err)
	}

	reconstructed, err := persistence.NewFileStore(dir, platformfilesystem.Local{}, createTemporaryFile)
	if err != nil {
		t.Fatalf("NewFileStore(reconstructed): %v", err)
	}
	t.Cleanup(func() { _ = reconstructed.Close() })
	restoredTracker, err := factorysessioncursors.NewTracker(reconstructed, identity)
	if err != nil {
		t.Fatalf("NewTracker(reconstructed): %v", err)
	}
	checkpoint, found, err := restoredTracker.Restore(context.Background())
	if err != nil || !found {
		t.Fatalf("Restore = %#v, %v, %v; want persisted checkpoint", checkpoint, found, err)
	}

	events := []json.RawMessage{
		canonicalEvent("event-1", 1),
		canonicalEvent("event-2", 2),
		canonicalEvent("event-3", 3),
	}
	resumed, err := factorysessionexecution.FilterEventsAfterReconnect(events, factorysessionexecution.EventReconnectRequest{
		AfterEventID: checkpoint.AfterEventID, AfterSequence: checkpoint.AfterSequence,
	}, identity.FactorySessionID)
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect: %v", err)
	}
	if len(resumed) != 1 || !strings.Contains(string(resumed[0]), `"id":"event-3"`) {
		t.Fatalf("resumed events = %s, want event-3 exactly once", resumed)
	}
}

func TestFileStoreMissingCorruptUnreadableAndClosedOutcomes(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "cursors")
	store, err := persistence.NewFileStore(dir, platformfilesystem.Local{}, createTemporaryFile)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	identity := testIdentity()
	if checkpoint, found, err := store.Load(ctx, identity); err != nil || found || checkpoint != (factorysessioncursors.Checkpoint{}) {
		t.Fatalf("missing Load = %#v, %v, %v; want empty start", checkpoint, found, err)
	}
	if err := store.Save(ctx, identity, factorysessioncursors.Checkpoint{AfterEventID: "event-1"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path := onlyCheckpointPath(t, dir)
	if err := os.WriteFile(path, []byte("not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt checkpoint: %v", err)
	}
	if _, _, err := store.Load(ctx, identity); err == nil || !strings.Contains(err.Error(), "decode factory session reconnect cursor") {
		t.Fatalf("corrupt Load error = %v, want actionable decode error", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove corrupt checkpoint: %v", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("replace checkpoint with unreadable directory: %v", err)
	}
	if _, _, err := store.Load(ctx, identity); err == nil || !strings.Contains(err.Error(), "read factory session reconnect cursor") {
		t.Fatalf("unreadable Load error = %v, want actionable read error", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := store.Load(ctx, identity); !errors.Is(err, factorysessioncursors.ErrStoreClosed) {
		t.Fatalf("Load after Close error = %v, want ErrStoreClosed", err)
	}
}

func TestFileStoreWriteFailureDoesNotAdvanceTracker(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "cursors")
	store, err := persistence.NewFileStore(dir, platformfilesystem.Local{}, createTemporaryFile)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	tracker, err := factorysessioncursors.NewTracker(store, testIdentity())
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.Advance(ctx, factorysessioncursors.Checkpoint{AfterEventID: "event-1"}); err != nil {
		t.Fatalf("Advance(first): %v", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("remove persistence directory: %v", err)
	}
	if err := os.WriteFile(dir, []byte("blocks directory recreation"), 0o600); err != nil {
		t.Fatalf("replace persistence directory: %v", err)
	}
	if err := tracker.Advance(ctx, factorysessioncursors.Checkpoint{AfterEventID: "event-2"}); err == nil || !strings.Contains(err.Error(), "create factory session reconnect cursor persistence directory") {
		t.Fatalf("Advance(second) error = %v, want actionable write failure", err)
	}
	current, found := tracker.Current()
	if !found || current.AfterEventID != "event-1" {
		t.Fatalf("Current = %#v, %v; want unadvanced event-1", current, found)
	}
}

func TestFileStoreCommitFailureRollsBackTemporaryFileAndPreservesCheckpoint(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cursors")
	files := &commitFailingFileSystem{}
	store, err := persistence.NewFileStore(dir, files, createTemporaryFile)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	identity := testIdentity()
	if err := store.Save(context.Background(), identity, factorysessioncursors.Checkpoint{AfterEventID: "event-1"}); err != nil {
		t.Fatalf("Save(first): %v", err)
	}
	files.failCommit = true
	if err := store.Save(context.Background(), identity, factorysessioncursors.Checkpoint{AfterEventID: "event-2"}); err == nil || !strings.Contains(err.Error(), "commit factory session reconnect cursor") {
		t.Fatalf("Save(failed commit) error = %v, want actionable commit failure", err)
	}
	files.failCommit = false
	checkpoint, found, err := store.Load(context.Background(), identity)
	if err != nil || !found || checkpoint.AfterEventID != "event-1" {
		t.Fatalf("Load after failed commit = %#v, %v, %v; want prior checkpoint", checkpoint, found, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || strings.HasSuffix(entries[0].Name(), ".tmp") {
		t.Fatalf("checkpoint entries = %#v, want rollback to remove temporary file", entries)
	}
}

func TestFileStoreValidatesConstructionAndRequests(t *testing.T) {
	if _, err := persistence.NewFileStore(" ", platformfilesystem.Local{}, createTemporaryFile); err == nil {
		t.Fatal("NewFileStore(empty) = nil, want directory error")
	}
	if _, err := persistence.NewFileStore("cursors", nil, createTemporaryFile); err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("NewFileStore(nil filesystem) error = %v", err)
	}
	if _, err := persistence.NewFileStore("cursors", platformfilesystem.Local{}, nil); err == nil || !strings.Contains(err.Error(), "temporary-file creator is required") {
		t.Fatalf("NewFileStore(nil temporary-file creator) error = %v", err)
	}
	store := newTestFileStore(t, filepath.Join(t.TempDir(), "cursors"))
	identity := testIdentity()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := store.Load(canceled, identity); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load(canceled) error = %v", err)
	}
	invalidIdentity := identity
	invalidIdentity.ConsumerID = ""
	if err := store.Save(context.Background(), invalidIdentity, factorysessioncursors.Checkpoint{AfterEventID: "event-1"}); err == nil {
		t.Fatal("Save(invalid identity) = nil, want identity error")
	}
	if err := store.Save(context.Background(), identity, factorysessioncursors.Checkpoint{}); err == nil {
		t.Fatal("Save(empty checkpoint) = nil, want checkpoint error")
	}
}

func TestFileStoreConcurrentSavesPublishOnlyCompleteCheckpoints(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cursors")
	store := newTestFileStore(t, dir)
	identity := testIdentity()

	const saves = 32
	errorsBySave := make(chan error, saves)
	var wait sync.WaitGroup
	for sequence := 0; sequence < saves; sequence++ {
		sequence := sequence
		wait.Add(1)
		go func() {
			defer wait.Done()
			eventID := fmt.Sprintf("event-%d", sequence)
			errorsBySave <- store.Save(context.Background(), identity, factorysessioncursors.Checkpoint{
				AfterEventID: eventID, AfterSequence: &sequence,
			})
		}()
	}
	wait.Wait()
	close(errorsBySave)
	for err := range errorsBySave {
		if err != nil {
			t.Fatalf("concurrent Save: %v", err)
		}
	}

	checkpoint, found, err := store.Load(context.Background(), identity)
	if err != nil || !found || checkpoint.AfterSequence == nil {
		t.Fatalf("Load after concurrent saves = %#v, %v, %v", checkpoint, found, err)
	}
	wantEventID := fmt.Sprintf("event-%d", *checkpoint.AfterSequence)
	if checkpoint.AfterEventID != wantEventID {
		t.Fatalf("checkpoint = %#v, want complete matching event/sequence pair", checkpoint)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || strings.HasSuffix(entries[0].Name(), ".tmp") {
		t.Fatalf("checkpoint entries = %#v, want one committed file and no temporary files", entries)
	}
}

func TestFileStoreReplacesAndValidatesPersistedEnvelope(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cursors")
	store := newTestFileStore(t, dir)
	identity := testIdentity()
	if err := store.Save(context.Background(), identity, factorysessioncursors.Checkpoint{AfterEventID: "event-1"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	secondSequence := 2
	if err := store.Save(context.Background(), identity, factorysessioncursors.Checkpoint{
		AfterEventID: "event-2", AfterSequence: &secondSequence,
	}); err != nil {
		t.Fatalf("Save(replacement): %v", err)
	}
	checkpoint, found, err := store.Load(context.Background(), identity)
	if err != nil || !found || checkpoint.AfterEventID != "event-2" {
		t.Fatalf("Load(replacement) = %#v, %v, %v", checkpoint, found, err)
	}
	path := onlyCheckpointPath(t, dir)
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("Unmarshal envelope: %v", err)
	}
	envelope["schemaVersion"] = "future/v2"
	writeEnvelope(t, path, envelope)
	if _, _, err := store.Load(context.Background(), identity); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("Load(unsupported version) error = %v", err)
	}
	envelope["schemaVersion"] = "factory-session-reconnect-cursor/v1"
	envelope["identity"].(map[string]any)["consumerId"] = "other-consumer"
	writeEnvelope(t, path, envelope)
	if _, _, err := store.Load(context.Background(), identity); err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("Load(identity mismatch) error = %v", err)
	}
}

func TestFileStoreRejectsSaveAfterClose(t *testing.T) {
	store := newTestFileStore(t, filepath.Join(t.TempDir(), "cursors"))
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Save(context.Background(), testIdentity(), factorysessioncursors.Checkpoint{AfterEventID: "event-2"}); !errors.Is(err, factorysessioncursors.ErrStoreClosed) {
		t.Fatalf("Save after Close error = %v, want ErrStoreClosed", err)
	}
}

func newTestFileStore(t *testing.T, dir string) *persistence.FileStore {
	t.Helper()
	store, err := persistence.NewFileStore(dir, platformfilesystem.Local{}, createTemporaryFile)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return store
}

func createTemporaryFile(dir, pattern string) (roles.CursorPersistenceTemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

type commitFailingFileSystem struct {
	platformfilesystem.Local
	failCommit bool
}

func (f *commitFailingFileSystem) Rename(oldPath, newPath string) error {
	if f.failCommit {
		return errors.New("injected commit failure")
	}
	return f.Local.Rename(oldPath, newPath)
}

func canonicalEvent(id string, sessionSequence int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"id":%q,"context":{"sequence":%d,"sessionSequence":%d}}`,
		id,
		sessionSequence,
		sessionSequence,
	))
}

func onlyCheckpointPath(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("checkpoint entries = %d, want 1", len(entries))
	}
	return filepath.Join(dir, entries[0].Name())
}

func writeEnvelope(t *testing.T, path string, envelope map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal envelope: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("WriteFile envelope: %v", err)
	}
}

func testIdentity() factorysessioncursors.StorageIdentity {
	return factorysessioncursors.StorageIdentity{
		BackendScopeID:     "backend-a",
		FactorySessionID:   "session-a",
		StreamGenerationID: "generation-a",
		ConsumerID:         "dashboard-a",
	}
}
