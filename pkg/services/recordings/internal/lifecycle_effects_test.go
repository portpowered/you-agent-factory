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

func TestNewReplayRecordingSnapshotWriterAppendsPendingV2Records(t *testing.T) {
	var data []byte
	appendCalls := 0
	writer := NewReplayRecordingSnapshotWriter(
		func(string, []byte) error {
			t.Fatal("v2 writer used replacement effect")
			return nil
		},
		func(_ string, payload []byte) error {
			appendCalls++
			data = append(data, payload...)
			return nil
		},
	)
	first := v2LifecycleSnapshot(time.Date(2026, 8, 23, 14, 0, 0, 0, time.UTC), 1, false)
	if err := writer("session.jsonl", first); err != nil {
		t.Fatalf("first v2 flush: %v", err)
	}
	if appendCalls != 2 {
		t.Fatalf("first append calls = %d, want header plus one event", appendCalls)
	}
	second := v2LifecycleSnapshot(first.Events[0].RecordedAt, 2, false)
	if err := writer("session.jsonl", second); err != nil {
		t.Fatalf("second v2 flush: %v", err)
	}
	if appendCalls != 3 {
		t.Fatalf("second append calls = %d, want one pending event", appendCalls)
	}
	finished := v2LifecycleSnapshot(first.Events[0].RecordedAt, 2, true)
	if err := writer("session.jsonl", finished); err != nil {
		t.Fatalf("terminal v2 flush: %v", err)
	}
	if appendCalls != 4 {
		t.Fatalf("terminal append calls = %d, want one terminal record", appendCalls)
	}
	stream, err := replayimpl.ParseReplayV2(data)
	if err != nil {
		t.Fatalf("ParseReplayV2: %v", err)
	}
	if len(stream.Events) != 2 || stream.Terminal == nil || stream.TruncatedTail {
		t.Fatalf("v2 stream = %#v, want two events and one terminal", stream)
	}
}

func TestNewReplayRecordingSnapshotWriterDoesNotAdvanceAfterAppendFailure(t *testing.T) {
	var data []byte
	appendCalls := 0
	fail := true
	writer := NewReplayRecordingSnapshotWriter(
		func(string, []byte) error { return errors.New("replacement must not run") },
		func(_ string, payload []byte) error {
			appendCalls++
			if fail && appendCalls == 3 {
				return errors.New("append unavailable")
			}
			data = append(data, payload...)
			return nil
		},
	)
	startedAt := time.Date(2026, 8, 23, 15, 0, 0, 0, time.UTC)
	if err := writer("failure.jsonl", v2LifecycleSnapshot(startedAt, 1, false)); err != nil {
		t.Fatalf("initial v2 flush: %v", err)
	}
	fail = true
	if err := writer("failure.jsonl", v2LifecycleSnapshot(startedAt, 2, false)); err == nil || !errors.Is(err, recordings.ErrRecordingSnapshotWrite) {
		t.Fatalf("failed v2 flush = %v, want append failure", err)
	}
	fail = false
	if err := writer("failure.jsonl", v2LifecycleSnapshot(startedAt, 2, false)); err != nil {
		t.Fatalf("retry v2 flush: %v", err)
	}
	stream, err := replayimpl.ParseReplayV2(data)
	if err != nil {
		t.Fatalf("ParseReplayV2 after retry: %v", err)
	}
	if len(stream.Events) != 2 || appendCalls != 4 {
		t.Fatalf("retry state = events=%d appendCalls=%d, want two events and one retry", len(stream.Events), appendCalls)
	}
}

func TestNewReplayRecordingSnapshotWriterWritesHeaderAndTerminalForEmptyRecording(t *testing.T) {
	var data []byte
	writer := NewReplayRecordingSnapshotWriter(
		func(string, []byte) error { return errors.New("replacement must not run") },
		func(_ string, payload []byte) error {
			data = append(data, payload...)
			return nil
		},
	)
	finishedAt := time.Date(2026, 8, 23, 16, 0, 0, 0, time.UTC)
	if err := writer("empty.jsonl", recordings.RecordingSnapshot{
		Status: recordings.RecordingStatusFacts{
			Scope:       recordings.CanonicalEventScope{FactorySessionID: "empty-session"},
			State:       recordings.RecordingFinalized,
			FinalizedAt: &finishedAt,
		},
	}); err != nil {
		t.Fatalf("empty v2 flush: %v", err)
	}
	stream, err := replayimpl.ParseReplayV2(data)
	if err != nil {
		t.Fatalf("ParseReplayV2 empty recording: %v", err)
	}
	if len(stream.Events) != 0 || stream.Terminal == nil || !stream.Terminal.FinishedAt.Equal(finishedAt) {
		t.Fatalf("empty stream = %#v, want header and terminal", stream)
	}
}

func v2LifecycleSnapshot(recordedAt time.Time, eventCount int, finalized bool) recordings.RecordingSnapshot {
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-v2"}
	events := make([]recordings.CanonicalEvent, eventCount)
	for index := range events {
		payload := `{"sequence":` + string(rune('0'+index)) + `}`
		kind := recordings.CanonicalEventKind("WORK_REQUEST")
		if index == 0 {
			kind = recordings.CanonicalEventKind("RUN_REQUEST")
			payload = `{"recordedAt":"` + recordedAt.UTC().Format(time.RFC3339Nano) + `","factory":{"id":"factory-v2","name":"test-factory","metadata":{"factory_hash":"sha256:test"}}}`
		}
		events[index] = recordings.CanonicalEvent{
			ID:         recordings.CanonicalEventID("event-v2-" + string(rune('0'+index))),
			Sequence:   recordings.CanonicalEventSequence(index),
			Scope:      scope,
			Cursor:     recordings.CanonicalEventCursor{StreamGenerationID: "generation-v2", Sequence: recordings.CanonicalEventSequence(index)},
			RecordedAt: recordedAt.Add(time.Duration(index) * time.Second),
			Kind:       kind,
			Payload:    payload,
		}
	}
	status := recordings.RecordingStatusFacts{Scope: scope, State: recordings.RecordingActive}
	if finalized {
		finishedAt := recordedAt.Add(time.Minute)
		status.State = recordings.RecordingFinalized
		status.FinalizedAt = &finishedAt
	}
	return recordings.RecordingSnapshot{Status: status, Events: events}
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

	snapshot := recordingSnapshotForWriteTest(t)
	tests := recordingSnapshotWriterTests()

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

func recordingSnapshotForWriteTest(t *testing.T) recordings.RecordingSnapshot {
	t.Helper()
	recordedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "factory-secret-write", "metadata": map[string]any{"factory_hash": "sha256:factory-secret-write"},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	started, err := replayimpl.NewEventLogArtifact(recordedAt, factorySnapshot, nil, recordings.ReplayDiagnostics{})
	if err != nil {
		t.Fatalf("NewEventLogArtifact: %v", err)
	}
	startEvent := canonical.CanonicalEventFromFactory(started.Events[0], "generation-secret-write")
	startEvent.Scope = recordings.CanonicalEventScope{FactorySessionID: "~default"}
	startEvent.Sequence = 0
	startEvent.Cursor.Sequence = 0
	return recordings.RecordingSnapshot{
		Status: recordings.RecordingStatusFacts{RecordingID: "recording-secret-write", State: recordings.RecordingFinalized},
		Events: []recordings.CanonicalEvent{startEvent, {
			ID: "event-secret-write", Kind: "WORK_REQUEST", Sequence: 1,
			Cursor: recordings.CanonicalEventCursor{StreamGenerationID: "generation-secret-write", Sequence: 1},
			Scope:  recordings.CanonicalEventScope{FactorySessionID: "~default"}, RecordedAt: recordedAt.Add(time.Minute),
			Payload: `{"credential":"snapshot-write-secret-002","control":"keep-me"}`,
		}},
		SecretProvenance: map[int][]recordings.RecordingSecret{1: {{
			JSONPointer: "/credential", Provenance: recordings.RecordingSecretProvenanceDeclared,
		}}},
	}
}

func recordingSnapshotWriterTests() []struct {
	name   string
	replay bool
	assert func(*testing.T, []byte)
} {
	return []struct {
		name   string
		replay bool
		assert func(*testing.T, []byte)
	}{
		{name: "canonical snapshot", assert: func(t *testing.T, payload []byte) {
			var decoded recordings.RecordingSnapshot
			if err := json.Unmarshal(payload, &decoded); err != nil {
				t.Fatalf("decode snapshot: %v", err)
			}
			assertSnapshotEventPayloadRedacted(t, decoded.Events[1].Payload)
		}},
		{name: "replay artifact", replay: true, assert: func(t *testing.T, payload []byte) {
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
		}},
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
