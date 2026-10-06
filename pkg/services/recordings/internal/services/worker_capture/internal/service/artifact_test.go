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
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCapturedPayloadCapPreservesJournalAndArtifact(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	root := t.TempDir()
	clock := &captureTimeProbe{now: time.Date(2026, 10, 5, 9, 0, 0, 0, time.UTC)}
	store, err := NewFileWriter(local, local, local, clock, root, "owner", nil)
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

// Storage consumes the already-redacted Events payload. This witness covers
// disk, spill and portable replay; publication classification is tested by
// Worker Sessions, and public CLI/HTTP privacy needs its own functional proof.
func TestCapturedRedactedPayloadSurvivesStorageExportAndSpill(t *testing.T) {
	t.Parallel()
	local := platformreplay.NewLocal(runtime.GOOS)
	store := journalWriter(t, local)
	opening := journalRecord(t, "private-capture", "private-worker")
	var lifecycle workers.Draft
	if err := json.Unmarshal(opening.Record.Payload, &lifecycle); err != nil {
		t.Fatal(err)
	}
	lifecycle.Provenance.Provider = "codex"
	var err error
	opening.Record.Payload, err = json.Marshal(lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	output := opening
	output.Record = mustRecord(t, workerOutputAppend(opening.Record.ID.Topic, opening.WorkerSessionID, 1, "tool"), 2)
	var draft workers.Draft
	if err := json.Unmarshal(output.Record.Payload, &draft); err != nil {
		t.Fatal(err)
	}
	draft.Kind, draft.Phase = workers.KindTool, workers.PhaseCompleted
	payload, err := json.Marshal(map[string]any{
		"toolCallId": "private-tool", "toolName": "visible",
		"argumentsSummary": map[string]string{"credential": "environment-secret"},
		"resultSummary":    map[string]string{"secret": "tool-secret", "neighbor": strings.Repeat("visible", 1<<18)},
	})
	if err != nil {
		t.Fatal(err)
	}
	safe, err := recordings.RedactDeclaredSecretText(recordings.RecordingRedactionRequest{
		Payload: payload, Secrets: []recordings.RecordingSecret{
			{JSONPointer: "/argumentsSummary/credential", Provenance: recordings.RecordingSecretProvenanceDeclared},
			{JSONPointer: "/resultSummary/secret", Provenance: recordings.RecordingSecretProvenanceDeclared},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	draft.Payload = safe.Payload
	if err := workers.ValidateDraft(draft); err != nil {
		t.Fatal(err)
	}
	output.Record.Payload, err = json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	terminal := opening
	terminal.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, opening.WorkerSessionID), 3)
	for _, record := range []recordings.WorkerRecordingRecord{opening, output, terminal} {
		if err := store.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	journal, err := os.ReadFile(store.path(opening.RecordingID) + "l")
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPrivateBytes(t, journal)
	reopened, err := newTestFileWriter(local, store.root)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPrivateExport(t, reopened, opening, output)
}

func assertCapturedPrivateBytes(t *testing.T, data []byte) {
	t.Helper()
	for _, secret := range []string{"environment-secret", "tool-secret", "DeclaredSecretJSONPointers"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatal("captured storage or export exposed classified content")
		}
	}
	if !bytes.Contains(data, []byte("visible")) || !bytes.Contains(data, []byte("<redacted>")) && !bytes.Contains(data, []byte(`\u003credacted\u003e`)) {
		t.Fatal("captured storage or export lost public content or redaction marker")
	}
}

func assertCapturedPrivateExport(t *testing.T, store recordings.WorkerRecordingStore, opening, output recordings.WorkerRecordingRecord) {
	t.Helper()
	page, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: opening.WorkerSessionID, BoundPayload: true})
	if err != nil || len(page.Records) != 3 {
		t.Fatalf("reopened private capture: records=%d error=%v", len(page.Records), err)
	}
	captured := page.Records[1]
	if !captured.Truncated || len(captured.Record.Payload) > capturedPayloadLimit {
		t.Fatal("private large tool output bypassed the payload cap")
	}
	stream, err := store.(recordings.WorkerCapturedArtifactReader).ReadWorkerCapturedArtifact(t.Context(), opening.WorkerSessionID, captured.ArtifactRef)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(stream)
	_ = stream.Close()
	if err != nil || !bytes.Equal(data, output.Record.Payload) {
		t.Fatal("private spill changed canonical safe bytes")
	}
	assertCapturedPrivateBytes(t, data)
	snapshot, err := store.LoadWorkerRecording(t.Context(), opening.RecordingID)
	if err != nil {
		t.Fatal(err)
	}
	codec := recordings.WorkerRecordingCodec{}
	portable, err := codec.ExportWorkerPortableRecording(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := codec.EncodeWorkerPortableRecording(portable)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPrivateBytes(t, encoded)
	decoded, err := codec.DecodeWorkerPortableRecording(encoded)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := codec.ReplayWorkerPortableRecording(decoded)
	if err != nil || !bytes.Equal(replayed.Projection.Records[1].Payload, output.Record.Payload) {
		t.Fatal("portable replay changed canonical private payload")
	}
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
