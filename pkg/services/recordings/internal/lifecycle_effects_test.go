package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	recordingevents "github.com/portpowered/infinite-you/pkg/services/recordings/internal/events"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

func TestNewRecordingSnapshotWriterNilWriteReturnsNil(t *testing.T) {
	t.Parallel()

	if writer := NewRecordingSnapshotWriter(nil); writer != nil {
		t.Fatalf("writer = %#v, want nil", writer)
	}
}

func TestNewRecordingSnapshotWriterPersistsEncodedSnapshot(t *testing.T) {
	t.Parallel()

	var writtenPath string
	var writtenPayload []byte
	writer := NewRecordingSnapshotWriter(func(path string, payload []byte) error {
		writtenPath = path
		writtenPayload = append([]byte(nil), payload...)
		return nil
	})
	snapshot := recordings.RecordingSnapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "recording-snapshot-writer",
			State:       recordings.RecordingFinalized,
		},
		Events: []recordings.CanonicalEvent{
			{
				ID:       "event-1",
				Kind:     "WORK_REQUEST",
				Sequence: 0,
				Payload:  `{"type":"WORK_REQUEST"}`,
			},
		},
	}
	if err := writer("target/snapshot.json", snapshot); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	if writtenPath != "target/snapshot.json" {
		t.Fatalf("written path = %q", writtenPath)
	}
	var decoded recordings.RecordingSnapshot
	if err := json.Unmarshal(writtenPayload, &decoded); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if decoded.Status.RecordingID != snapshot.Status.RecordingID ||
		len(decoded.Events) != 1 {
		t.Fatalf("decoded snapshot = %#v", decoded)
	}
}

func TestNewRecordingSnapshotWriterWriteError(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("write failed")
	writer := NewRecordingSnapshotWriter(func(string, []byte) error { return writeErr })
	err := writer("target/snapshot.json", recordings.RecordingSnapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: "recording-snapshot-writer"},
	})
	if !errors.Is(err, recordings.ErrRecordingSnapshotWrite) || !errors.Is(err, writeErr) {
		t.Fatalf("write error = %v, want snapshot write identity", err)
	}
}

func TestNewReplayRecordingSnapshotWriterNilWriteReturnsNil(t *testing.T) {
	t.Parallel()

	if writer := NewReplayRecordingSnapshotWriter(nil); writer != nil {
		t.Fatalf("writer = %#v, want nil", writer)
	}
}

const replayV1GoldenJSON = `{
  "schemaVersion": "agent-factory.replay.v1",
  "recordedAt": "2026-02-03T18:45:12Z",
  "events": [
    {
      "context": {
        "eventTime": "2026-02-03T18:45:12Z",
        "sequence": 0,
        "sessionId": "~default",
        "tick": 0
      },
      "id": "factory-event/run-started",
      "payload": {
        "diagnostics": {},
        "factory": {
          "id": "factory-1",
          "metadata": {
            "factory_hash": "sha256:factory",
            "runtime_config_hash": "sha256:runtime",
            "workers_hash": "sha256:workers",
            "workstations_hash": "sha256:workstations"
          }
        },
        "recordedAt": "2026-02-03T18:45:12Z",
        "wallClock": {
          "startedAt": "2026-02-03T18:45:12Z"
        }
      },
      "schemaVersion": "agent-factory.event.v1",
      "type": "RUN_REQUEST"
    },
    {
      "context": {
        "eventTime": "2026-02-03T18:46:12Z",
        "sequence": 1,
        "sessionId": "~default",
        "tick": 3
      },
      "id": "work-request-1",
      "payload": {
        "workId": "work-1",
        "contentHash": "sha256:work"
      },
      "schemaVersion": "agent-factory.event.v1",
      "type": "WORK_REQUEST"
    },
    {
      "context": {
        "eventTime": "2026-02-03T18:47:12Z",
        "sequence": 2,
        "sessionId": "~default",
        "tick": 0
      },
      "id": "factory-event/run-finished",
      "payload": {
        "state": "COMPLETED",
        "wallClock": {
          "finishedAt": "2026-02-03T18:47:12Z",
          "startedAt": "2026-02-03T18:45:12Z"
        }
      },
      "schemaVersion": "agent-factory.event.v1",
      "type": "RUN_RESPONSE"
    }
  ]
}
`

func TestNewReplayRecordingSnapshotWriterPinsReplayV1JSONBytes(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC)
	finishedAt := startedAt.Add(2 * time.Minute)
	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "factory-1",
		"metadata": map[string]any{
			"factory_hash":        "sha256:factory",
			"workers_hash":        "sha256:workers",
			"workstations_hash":   "sha256:workstations",
			"runtime_config_hash": "sha256:runtime",
		},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	started, err := replayimpl.NewEventLogArtifact(
		startedAt,
		factorySnapshot,
		&recordings.ReplayWallClockMetadata{StartedAt: startedAt},
		recordings.ReplayDiagnostics{},
	)
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}

	work := recordings.FactoryEvent{
		Id:   "work-request-1",
		Type: recordings.FactoryEventTypeWorkRequest,
		Context: recordings.FactoryEventContext{
			EventTime: startedAt.Add(time.Minute),
			Sequence:  1,
			Tick:      3,
		},
		Payload: []byte(`{"workId":"work-1","contentHash":"sha256:work"}`),
	}
	finished := recordingevents.RunFinishedFactoryEvent(startedAt, finishedAt)
	finished.Context.Sequence = 2
	events := []recordings.CanonicalEvent{
		canonical.CanonicalEventFromFactory(started.Events[0], "generation-1"),
		canonical.CanonicalEventFromFactory(work, "generation-1"),
		canonical.CanonicalEventFromFactory(finished, "generation-1"),
	}
	for sequence := range events {
		events[sequence].Scope = recordings.CanonicalEventScope{FactorySessionID: "~default"}
		events[sequence].Sequence = recordings.CanonicalEventSequence(sequence)
		events[sequence].Cursor.Sequence = recordings.CanonicalEventSequence(sequence)
	}
	var writtenPayload []byte
	writer := NewReplayRecordingSnapshotWriter(func(_ string, payload []byte) error {
		writtenPayload = append([]byte(nil), payload...)
		return nil
	})
	finalizedAt := finishedAt
	if err := writer("recording.json", recordings.RecordingSnapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID:    "recording-byte-golden",
			State:          recordings.RecordingFinalized,
			AcceptedEvents: len(events),
			FinalizedAt:    &finalizedAt,
		},
		Events: events,
	}); err != nil {
		t.Fatalf("write replay snapshot: %v", err)
	}

	want := []byte(replayV1GoldenJSON)
	if !bytes.Equal(writtenPayload, want) {
		t.Fatalf("replay v1 bytes =\n%s\nwant =\n%s", writtenPayload, want)
	}
	if !json.Valid(bytes.TrimSuffix(writtenPayload, []byte{'\n'})) {
		t.Fatal("replay v1 payload without terminal newline is not valid JSON")
	}
	if !bytes.HasSuffix(writtenPayload, []byte("}\n")) ||
		bytes.HasSuffix(writtenPayload, []byte("}\n\n")) {
		t.Fatal("replay v1 payload does not have exactly one terminal newline")
	}
}

func TestNewRecordingFlushTickerFactoryStopsTicker(t *testing.T) {
	t.Parallel()

	factory := NewRecordingFlushTickerFactory()
	ticker := factory(10 * time.Millisecond)
	defer ticker.Stop()
	select {
	case <-ticker.Ticks:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected flush ticker to emit")
	}
}

func TestRecordingSnapshotWritersRedactBeforeSerialization(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id":       "factory-secret-write",
		"metadata": map[string]any{"factory_hash": "sha256:factory-secret-write"},
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
	startEvent := canonical.CanonicalEventFromFactory(started.Events[0], "generation-secret-write")
	startEvent.Scope = recordings.CanonicalEventScope{FactorySessionID: "~default"}
	startEvent.Sequence = 0
	startEvent.Cursor.Sequence = 0
	event := recordings.CanonicalEvent{
		ID:       "event-secret-write",
		Kind:     "WORK_REQUEST",
		Sequence: 1,
		Cursor: recordings.CanonicalEventCursor{
			StreamGenerationID: "generation-secret-write",
			Sequence:           1,
		},
		Scope:      recordings.CanonicalEventScope{FactorySessionID: "~default"},
		RecordedAt: recordedAt.Add(time.Minute),
		Payload:    `{"credential":"snapshot-write-secret-002","control":"keep-me"}`,
	}
	snapshot := recordings.RecordingSnapshot{
		Status: recordings.RecordingStatusFacts{
			RecordingID: "recording-secret-write",
			State:       recordings.RecordingFinalized,
		},
		Events: []recordings.CanonicalEvent{startEvent, event},
		SecretProvenance: map[int][]recordings.RecordingSecret{
			1: {{
				JSONPointer: "/credential",
				Provenance:  recordings.RecordingSecretProvenanceDeclared,
			}},
		},
	}

	tests := []struct {
		name   string
		replay bool
		assert func(*testing.T, []byte)
	}{
		{
			name: "canonical snapshot",
			assert: func(t *testing.T, payload []byte) {
				var decoded recordings.RecordingSnapshot
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatalf("decode snapshot: %v", err)
				}
				assertSnapshotEventPayloadRedacted(t, decoded.Events[1].Payload)
			},
		},
		{
			name:   "replay artifact",
			replay: true,
			assert: func(t *testing.T, payload []byte) {
				var decoded struct {
					Events []struct {
						Payload json.RawMessage `json:"payload"`
					} `json:"events"`
				}
				if err := json.Unmarshal(payload, &decoded); err != nil {
					t.Fatalf("decode replay artifact: %v", err)
				}
				if len(decoded.Events) != 2 {
					t.Fatalf("replay events = %d, want 2", len(decoded.Events))
				}
				assertJSONPayloadRedacted(t, decoded.Events[1].Payload)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var written []byte
			var writer recordings.RecordingSnapshotWriter
			if !test.replay {
				writer = NewRecordingSnapshotWriter(func(_ string, payload []byte) error {
					written = append([]byte(nil), payload...)
					return nil
				})
			} else {
				writer = NewReplayRecordingSnapshotWriter(func(_ string, payload []byte) error {
					written = append([]byte(nil), payload...)
					return nil
				})
			}
			if err := writer("recording.json", snapshot); err != nil {
				t.Fatalf("write %s: %v", test.name, err)
			}
			if len(written) == 0 {
				t.Fatalf("write %s produced no bytes", test.name)
			}
			test.assert(t, written)
		})
	}
}

func TestRecordingSnapshotWriterRejectsClassifiedPathBeforeWrite(t *testing.T) {
	t.Parallel()

	const declaredSecret = "snapshot-write-failure-secret-002"
	writes := 0
	writer := NewRecordingSnapshotWriter(func(string, []byte) error {
		writes++
		return nil
	})
	err := writer("recording.json", recordings.RecordingSnapshot{
		Events: []recordings.CanonicalEvent{{
			ID:         "event-secret-write-failure",
			Kind:       "WORK_REQUEST",
			RecordedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
			Payload:    `{"credential":"snapshot-write-failure-secret-002"}`,
		}},
		SecretProvenance: map[int][]recordings.RecordingSecret{
			0: {{
				JSONPointer: "/missing",
				Provenance:  recordings.RecordingSecretProvenanceDeclared,
			}},
		},
	})
	if !errors.Is(err, recordings.ErrRecordingSnapshotEncoding) {
		t.Fatalf("writer error = %v, want encoding error", err)
	}
	if writes != 0 {
		t.Fatalf("storage writes = %d, want none", writes)
	}
	if strings.Contains(err.Error(), declaredSecret) {
		t.Fatalf("writer error exposed declared secret: %v", err)
	}
}

func assertSnapshotEventPayloadRedacted(t *testing.T, payload string) {
	t.Helper()
	assertJSONPayloadRedacted(t, json.RawMessage(payload))
}

func assertJSONPayloadRedacted(t *testing.T, payload json.RawMessage) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	var marker recordings.RecordingRedactedValue
	if err := json.Unmarshal(fields["credential"], &marker); err != nil {
		t.Fatalf("decode credential marker: %v", err)
	}
	if err := marker.Validate(); err != nil {
		t.Fatalf("credential marker: %v", err)
	}
	var control string
	if err := json.Unmarshal(fields["control"], &control); err != nil || control != "keep-me" {
		t.Fatalf("control = %q, want keep-me (err=%v)", control, err)
	}
}
