package service_test

import (
	"encoding/json"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

func TestRecordingLifecycleRedactsEveryStreamingAndFinalSnapshot(t *testing.T) {
	t.Parallel()

	var writes [][]byte
	root := newActiveFlushRoot(
		recordingsinternal.NewReplayRecordingSnapshotWriter(func(_ string, payload []byte) error {
			writes = append(writes, append([]byte(nil), payload...))
			return nil
		}),
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(newManualFlushTicker())
		},
	)
	recordingID := startActiveRecording(t, root, "recording-streaming-secret", time.Hour)
	finished := false
	t.Cleanup(func() {
		if !finished {
			stopRecording(t, root, recordingID)
		}
	})

	startEvent, first, second := secretLifecycleEvents(t)
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       startEvent,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent start: %v", err)
	}
	first.Payload = `{"credential":"lifecycle-streaming-secret-002","control":"stream-control"}`
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       first,
		SecretProvenance: []recordings.RecordingSecret{{
			JSONPointer: "/credential",
			Provenance:  recordings.RecordingSecretProvenanceDeclared,
		}},
	}); err != nil {
		t.Fatalf("RecordRecordingEvent first: %v", err)
	}
	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{RecordingID: recordingID}); err != nil {
		t.Fatalf("streaming FlushRecording: %v", err)
	}
	if len(writes) != 1 {
		t.Fatalf("streaming writes = %d, want 1", len(writes))
	}
	assertReplaySnapshotEventRedacted(t, writes[0], 1, "stream-control")

	second.Payload = `{"control":"final-control"}`
	if _, err := root.RecordRecordingEvent(recordings.RecordRecordingEventRequest{
		RecordingID: recordingID,
		Event:       second,
	}); err != nil {
		t.Fatalf("RecordRecordingEvent second: %v", err)
	}
	if _, err := root.FinishRecording(recordings.FinishRecordingRequest{
		RecordingID: recordingID,
		FinishedAt:  time.Date(2026, 8, 24, 12, 5, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	finished = true
	if len(writes) != 2 {
		t.Fatalf("streaming/final writes = %d, want 2", len(writes))
	}
	assertReplaySnapshotEventRedacted(t, writes[1], 1, "stream-control")
	assertReplaySnapshotEventControl(t, writes[1], 2, "final-control")
}

func secretLifecycleEvents(t *testing.T) (
	recordings.CanonicalEvent,
	recordings.CanonicalEvent,
	recordings.CanonicalEvent,
) {
	t.Helper()
	recordedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id":       "factory-lifecycle-secret",
		"metadata": map[string]any{"factory_hash": "sha256:factory-lifecycle-secret"},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	started, err := replayimpl.NewEventLogArtifact(
		recordedAt,
		factorySnapshot,
		nil,
		recordings.ReplayDiagnostics{},
	)
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}
	start := canonical.CanonicalEventFromFactory(started.Events[0], "generation-lifecycle-secret")
	start.Scope = recordings.CanonicalEventScope{FactorySessionID: "session-active"}
	start.Sequence = 0
	start.Cursor.Sequence = 0
	first := activeFlushEvent(1)
	first.RecordedAt = recordedAt.Add(time.Minute)
	first.Cursor.StreamGenerationID = "generation-lifecycle-secret"
	second := activeFlushEvent(2)
	second.RecordedAt = recordedAt.Add(2 * time.Minute)
	second.Cursor.StreamGenerationID = "generation-lifecycle-secret"
	return start, first, second
}

func assertReplaySnapshotEventRedacted(
	t *testing.T,
	payload []byte,
	eventIndex int,
	wantControl string,
) {
	t.Helper()
	var artifact struct {
		Events []struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("decode replay snapshot: %v", err)
	}
	if eventIndex < 0 || eventIndex >= len(artifact.Events) {
		t.Fatalf("event index %d is absent from %d events", eventIndex, len(artifact.Events))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(artifact.Events[eventIndex].Payload, &fields); err != nil {
		t.Fatalf("decode replay event payload: %v", err)
	}
	var marker recordings.RecordingRedactedValue
	if err := json.Unmarshal(fields["credential"], &marker); err != nil {
		t.Fatalf("decode replay redaction marker: %v", err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("replay redaction marker: %v", err)
	}
	assertReplaySnapshotEventControlValue(t, fields, wantControl)
}

func assertReplaySnapshotEventControl(
	t *testing.T,
	payload []byte,
	eventIndex int,
	wantControl string,
) {
	t.Helper()
	var artifact struct {
		Events []struct {
			Payload json.RawMessage `json:"payload"`
		} `json:"events"`
	}
	if err := json.Unmarshal(payload, &artifact); err != nil {
		t.Fatalf("decode replay snapshot: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(artifact.Events[eventIndex].Payload, &fields); err != nil {
		t.Fatalf("decode replay event payload: %v", err)
	}
	assertReplaySnapshotEventControlValue(t, fields, wantControl)
}

func assertReplaySnapshotEventControlValue(
	t *testing.T,
	fields map[string]json.RawMessage,
	wantControl string,
) {
	t.Helper()
	var control string
	if err := json.Unmarshal(fields["control"], &control); err != nil || control != wantControl {
		t.Fatalf("control = %q, want %q (err=%v)", control, wantControl, err)
	}
}
