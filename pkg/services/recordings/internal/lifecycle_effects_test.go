package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	first.CanonicalSessionID = "550e8400-e29b-41d4-a716-446655440000"
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
	if stream.Header.SessionID != first.CanonicalSessionID {
		t.Fatalf("v2 header session ID = %q, want canonical %q", stream.Header.SessionID, first.CanonicalSessionID)
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

func TestReplayRecordingSnapshotWriterRejectsNonEmptyV2Target(t *testing.T) {
	var data []byte
	appendFile := func(_ string, payload []byte) error {
		data = append(data, payload...)
		return nil
	}
	prepare := func(_ string) error {
		if len(data) != 0 {
			return errors.New("replay v2 target already contains data")
		}
		return nil
	}
	firstWriter := newReplayRecordingSnapshotWriter(
		func(string, []byte) error { return errors.New("replacement must not run") },
		appendFile,
		prepare,
	)
	startedAt := time.Date(2026, 8, 23, 14, 30, 0, 0, time.UTC)
	if err := firstWriter("explicit.jsonl", v2LifecycleSnapshot(startedAt, 1, true)); err != nil {
		t.Fatalf("first explicit v2 recording: %v", err)
	}
	original := append([]byte(nil), data...)
	secondWriter := newReplayRecordingSnapshotWriter(
		func(string, []byte) error { return errors.New("replacement must not run") },
		appendFile,
		prepare,
	)
	if err := secondWriter("explicit.jsonl", v2LifecycleSnapshot(startedAt, 1, true)); err == nil || !errors.Is(err, recordings.ErrRecordingSnapshotWrite) {
		t.Fatalf("second explicit v2 recording error = %v, want collision error", err)
	}
	if !bytes.Equal(data, original) {
		t.Fatal("collision attempt changed the existing v2 recording")
	}
	stream, err := replayimpl.ParseReplayV2(data)
	if err != nil {
		t.Fatalf("ParseReplayV2 after collision: %v", err)
	}
	if len(stream.Events) != 1 || stream.Terminal == nil {
		t.Fatalf("stream after collision = %#v, want one event and one terminal", stream)
	}
}

func TestReplayRecordingSnapshotWriterRejectsWhitespaceV2Target(t *testing.T) {
	original := []byte(" \n")
	data := append([]byte(nil), original...)
	appendCalled := false
	writer := newReplayRecordingSnapshotWriter(
		func(string, []byte) error { return errors.New("replacement must not run") },
		func(_ string, payload []byte) error {
			appendCalled = true
			data = append(data, payload...)
			return nil
		},
		replayV2TargetPreparation(func(string) ([]byte, error) {
			return append([]byte(nil), data...), nil
		}),
	)

	if err := writer("whitespace.jsonl", v2LifecycleSnapshot(time.Date(2026, 8, 23, 14, 45, 0, 0, time.UTC), 1, true)); err == nil || !errors.Is(err, recordings.ErrRecordingSnapshotWrite) {
		t.Fatalf("whitespace-only v2 target error = %v, want collision error", err)
	}
	if appendCalled {
		t.Fatal("collision attempt appended to the whitespace-only target")
	}
	if !bytes.Equal(data, original) {
		t.Fatalf("collision attempt changed target bytes = %q, want %q", data, original)
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
			Scope:       recordings.CanonicalEventScope{FactorySessionID: "00000000-0000-4000-8000-000000000003"},
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
	scope := recordings.CanonicalEventScope{FactorySessionID: "00000000-0000-4000-8000-000000000002"}
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

const (
	measurementRecordedAt = "2026-08-23T17:00:00Z"
	measurementSessionID  = "00000000-0000-4000-8000-000000000004"
	measurementGeneration = "measurement-generation"
	measurementEventBody  = `{"fixed":"payload-with-a-deterministic-size"}`
)

type writeAmplificationMeasurement struct {
	eventCount    int
	v1Bytes       int64
	v2Bytes       int64
	v1Growth      float64
	v2Growth      float64
	improvement   float64
	v1FinalBytes  int
	v2FinalBytes  int
	v1WriteCalls  int
	v2AppendCalls int
}

type successfulWriteCapture struct {
	data                []byte
	bytes               int64
	writeCalls          int
	appendCalls         int
	forbidReplaceWrites bool
}

func TestReplayWriteAmplificationMeasurement(t *testing.T) {
	t.Parallel()

	counts := []int{10, 100, 300}
	measurements := make([]writeAmplificationMeasurement, 0, len(counts))
	for _, eventCount := range counts {
		v1 := measureReplaySnapshotWrites(t, eventCount, false)
		v2 := measureReplaySnapshotWrites(t, eventCount, true)
		if v2.writeCalls != 0 {
			t.Fatalf("N=%d v2 replacement write calls = %d, want 0", eventCount, v2.writeCalls)
		}
		if v2.appendCalls != eventCount+2 {
			t.Fatalf("N=%d v2 append calls = %d, want header, %d events, and terminal", eventCount, v2.appendCalls, eventCount)
		}
		if v2.bytes != int64(len(v2.data)) {
			t.Fatalf("N=%d v2 submitted bytes = %d, final artifact bytes = %d; append-only writes must equal final size", eventCount, v2.bytes, len(v2.data))
		}
		if v1.bytes <= int64(len(v1.data)) {
			t.Fatalf("N=%d v1 submitted bytes = %d, final artifact bytes = %d; v1 must rewrite accumulated history", eventCount, v1.bytes, len(v1.data))
		}

		measurement := writeAmplificationMeasurement{
			eventCount:    eventCount,
			v1Bytes:       v1.bytes,
			v2Bytes:       v2.bytes,
			v1FinalBytes:  len(v1.data),
			v2FinalBytes:  len(v2.data),
			v1WriteCalls:  v1.writeCalls,
			v2AppendCalls: v2.appendCalls,
			improvement:   float64(v1.bytes) / float64(v2.bytes),
		}
		if len(measurements) > 0 {
			previous := measurements[len(measurements)-1]
			measurement.v1Growth = float64(v1.bytes) / float64(previous.v1Bytes)
			measurement.v2Growth = float64(v2.bytes) / float64(previous.v2Bytes)
		}
		measurements = append(measurements, measurement)

		assertMeasuredReplayArtifacts(t, eventCount, v1.data, v2.data)
	}

	// Growth is measured against the preceding corpus size. V1 rewrites the
	// whole history after every event and must grow quadratically, while v2
	// appends each event once and must stay near linear growth. Using a bounded
	// final corpus keeps this unit regression test fast under coverage; larger
	// throughput measurements belong in the performance lane.
	for index := 1; index < len(measurements); index++ {
		scale := float64(measurements[index].eventCount) / float64(measurements[index-1].eventCount)
		if measurements[index].v1Growth <= scale*scale/2 {
			t.Fatalf("N=%d v1 growth ratio = %.1fx, want quadratic amplification for %.1fx corpus growth", measurements[index].eventCount, measurements[index].v1Growth, scale)
		}
		if measurements[index].v2Growth >= scale*2 {
			t.Fatalf("N=%d v2 growth ratio = %.1fx, want linear growth for %.1fx corpus growth", measurements[index].eventCount, measurements[index].v2Growth, scale)
		}
	}
	for _, measurement := range measurements {
		if measurement.improvement <= 1 {
			t.Fatalf("N=%d improvement ratio = %.1fx, want v2 below v1", measurement.eventCount, measurement.improvement)
		}
	}

	t.Log("flush schedule: one snapshot flush after every event; final flush adds one terminal record")
	t.Log("growth columns: total submitted bytes divided by the preceding corpus total; first row is baseline")
	t.Log("N | v1 bytes written | v2 bytes written | v1 growth | v2 growth | improvement ratio")
	for index, measurement := range measurements {
		v1Growth := "baseline"
		v2Growth := "baseline"
		if index > 0 {
			v1Growth = fmt.Sprintf("%.1fx", measurement.v1Growth)
			v2Growth = fmt.Sprintf("%.1fx", measurement.v2Growth)
		}
		t.Logf("%d | %d | %d | %s | %s | %.1fx", measurement.eventCount, measurement.v1Bytes, measurement.v2Bytes, v1Growth, v2Growth, measurement.improvement)
	}
}

func measureReplaySnapshotWrites(t *testing.T, eventCount int, v2 bool) successfulWriteCapture {
	t.Helper()

	capture := successfulWriteCapture{forbidReplaceWrites: v2}
	writer := NewReplayRecordingSnapshotWriter(
		func(_ string, data []byte) error {
			capture.writeCalls++
			if capture.forbidReplaceWrites {
				return errors.New("v2 measurement forbids replacement writes")
			}
			capture.bytes += int64(len(data))
			capture.data = append([]byte(nil), data...)
			return nil
		},
		func(_ string, data []byte) error {
			capture.appendCalls++
			capture.bytes += int64(len(data))
			capture.data = append(capture.data, data...)
			return nil
		},
	)
	path := fmt.Sprintf("measurement-%d", eventCount)
	if v2 {
		path += ".jsonl"
	} else {
		path += ".json"
	}
	for index := 1; index <= eventCount; index++ {
		finalized := index == eventCount
		if err := writer(path, measurementSnapshot(t, index, finalized)); err != nil {
			t.Fatalf("measure N=%d v2=%t flush %d: %v", eventCount, v2, index, err)
		}
	}
	if capture.bytes == 0 || len(capture.data) == 0 {
		t.Fatalf("measure N=%d v2=%t produced no successful recording bytes", eventCount, v2)
	}
	return capture
}

func measurementSnapshot(t *testing.T, eventCount int, finalized bool) recordings.RecordingSnapshot {
	t.Helper()

	recordedAt, err := time.Parse(time.RFC3339, measurementRecordedAt)
	if err != nil {
		t.Fatalf("parse measurement time: %v", err)
	}
	factory, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"name":         "measurement-factory",
		"workTypes":    []map[string]any{{"name": "task"}},
		"workers":      []map[string]any{{"name": "worker"}},
		"workstations": []map[string]any{{"name": "process", "worker": "worker"}},
	})
	if err != nil {
		t.Fatalf("build measurement Factory snapshot: %v", err)
	}
	artifact, err := replayimpl.NewEventLogArtifact(
		recordedAt,
		factory,
		&recordings.ReplayWallClockMetadata{StartedAt: recordedAt},
		factorydefinitions.ReplayDiagnostics{},
	)
	if err != nil {
		t.Fatalf("build measurement replay shell: %v", err)
	}

	events := make([]recordings.CanonicalEvent, eventCount)
	sessionID := measurementSessionID
	for index := 0; index < eventCount; index++ {
		event := artifact.Events[0]
		if index > 0 {
			event = factorydefinitions.FactoryEvent{
				Id:            fmt.Sprintf("measurement-event-%04d", index),
				SchemaVersion: factorydefinitions.FactoryEventSchemaVersionV1,
				Type:          factorydefinitions.FactoryEventTypeWorkRequest,
				Context: factorydefinitions.FactoryEventContext{
					EventTime: recordedAt,
					Sequence:  index,
					SessionID: &sessionID,
					Tick:      1,
				},
				Payload: []byte(measurementEventBody),
			}
		} else {
			event.Context.Sequence = 0
			event.Context.SessionID = &sessionID
		}
		events[index] = canonical.CanonicalEventFromFactory(event, measurementGeneration)
	}

	status := recordings.RecordingStatusFacts{
		RecordingID:    recordings.RecordingID("measurement-recording"),
		Scope:          recordings.CanonicalEventScope{FactorySessionID: sessionID},
		State:          recordings.RecordingActive,
		AcceptedEvents: eventCount,
	}
	if finalized {
		finishedAt := recordedAt.Add(time.Minute)
		status.State = recordings.RecordingFinalized
		status.FinalizedAt = &finishedAt
	}
	return recordings.RecordingSnapshot{Status: status, Events: events}
}

func assertMeasuredReplayArtifacts(t *testing.T, eventCount int, v1Data, v2Data []byte) {
	t.Helper()

	var v1Artifact factorydefinitions.ReplayArtifact
	if err := json.Unmarshal(v1Data, &v1Artifact); err != nil {
		t.Fatalf("decode measured v1 artifact N=%d: %v", eventCount, err)
	}
	if len(v1Artifact.Events) != eventCount {
		t.Fatalf("measured v1 event count N=%d = %d, want %d", eventCount, len(v1Artifact.Events), eventCount)
	}
	v2Stream, err := replayimpl.ParseReplayV2(v2Data)
	if err != nil {
		t.Fatalf("decode measured v2 artifact N=%d: %v", eventCount, err)
	}
	if len(v2Stream.Events) != eventCount || v2Stream.Terminal == nil || v2Stream.TruncatedTail {
		t.Fatalf("measured v2 stream N=%d = events=%d terminal=%t truncated=%t", eventCount, len(v2Stream.Events), v2Stream.Terminal != nil, v2Stream.TruncatedTail)
	}
}
