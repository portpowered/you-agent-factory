package internal

import (
	"bytes"
	"encoding/json"
	"errors"
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
