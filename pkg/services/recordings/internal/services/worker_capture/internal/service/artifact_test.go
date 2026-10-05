package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestCapturedPayloadCapPreservesJournalAndArtifact(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	clock := &captureTimeProbe{now: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)}
	store, err := NewFileWriter(local, local, local, clock, root, "owner")
	if err != nil {
		t.Fatal(err)
	}
	opening := journalRecord(t, "large-capture", "large-worker")
	if err := store.PersistWorkerRecord(t.Context(), opening); err != nil {
		t.Fatal(err)
	}
	output := opening
	output.Record = mustRecord(t, workerOutputAppend(opening.Record.ID.Topic, "large-worker", 1, "output"), 2)
	var draft map[string]any
	if err := json.Unmarshal(output.Record.Payload, &draft); err != nil {
		t.Fatal(err)
	}
	draft["kind"] = "TOOL"
	draft["phase"] = "DELTA"
	draft["payload"] = map[string]any{"toolCallId": "large", "outputDelta": strings.Repeat("\x00☃", 3<<18)}
	output.Record.Payload, err = json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PersistWorkerRecord(t.Context(), output); err != nil {
		t.Fatal(err)
	}
	page, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: "large-worker", BoundPayload: true})
	if err != nil {
		t.Fatal(err)
	}
	captured := page.Records[1]
	assertCapturedSpill(t, store, opening, output, captured)
}

func assertCapturedSpill(t *testing.T, store recordings.WorkerRecordingStore, opening, output recordings.WorkerRecordingRecord, captured recordings.WorkerCapturedRecord) {
	t.Helper()
	if !captured.Truncated || captured.OriginalBytes != int64(len(output.Record.Payload)) || captured.ReturnedBytes != int64(len(captured.Record.Payload)) || len(captured.Record.Payload) > capturedPayloadLimit || !json.Valid(captured.Record.Payload) {
		t.Fatal("escaped UTF-8 payload violates returned cap/metadata")
	}
	reader := store.(recordings.WorkerCapturedArtifactReader)
	stream, err := reader.ReadWorkerCapturedArtifact(t.Context(), "large-worker", captured.ArtifactRef)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil || !bytes.Equal(data, output.Record.Payload) {
		t.Fatal("spill changed canonical bytes")
	}
	snapshot, err := store.LoadWorkerRecording(t.Context(), opening.RecordingID)
	if err != nil || !bytes.Equal(snapshot.Sessions[0].Records[1].Payload, output.Record.Payload) {
		t.Fatal("bounded read changed stored capture")
	}
	for _, ref := range []string{"sibling/2", "large-worker/../2", "large-worker/02"} {
		if _, err := reader.ReadWorkerCapturedArtifact(t.Context(), "large-worker", ref); !errors.Is(err, recordings.ErrInvalidWorkerRecordingRequest) {
			t.Fatalf("invalid ref accepted: %q error=%v", ref, err)
		}
	}
	if _, err := reader.ReadWorkerCapturedArtifact(t.Context(), "large-worker", "large-worker/3"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncommitted artifact exposed: %v", err)
	}
}
