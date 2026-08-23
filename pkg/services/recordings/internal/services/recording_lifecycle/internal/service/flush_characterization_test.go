package service_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingsinternal "github.com/portpowered/infinite-you/pkg/services/recordings/internal"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/canonical"
	replayimpl "github.com/portpowered/infinite-you/pkg/services/recordings/internal/replay"
)

type characterizedFlushWrite struct {
	target  string
	payload []byte
}

type characterizedReplayEnvelope struct {
	SchemaVersion string `json:"schemaVersion"`
	Events        []struct {
		ID      string `json:"id"`
		Context struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
	} `json:"events"`
}

const characterizedDefaultFlushInterval = 250 * time.Millisecond

func TestActiveFlushCharacterizesDefaultCadenceAndCompleteV1Rewrites(t *testing.T) {
	t.Parallel()

	ticker := newManualFlushTicker()
	events := characterizedFlushEvents(t)
	writes := make(chan characterizedFlushWrite, 4)
	var observedInterval time.Duration
	root := newActiveFlushRoot(
		recordingsinternal.NewReplayRecordingSnapshotWriter(
			func(target string, payload []byte) error {
				writes <- characterizedFlushWrite{
					target:  target,
					payload: append([]byte(nil), payload...),
				}
				return nil
			},
		),
		func(interval time.Duration) recordings.RecordingFlushTicker {
			observedInterval = interval
			return manualTickerHandle(ticker)
		},
	)
	recordingID := startActiveRecording(t, root, "recording-flush-characterization", 0)
	t.Cleanup(func() { stopRecording(t, root, recordingID) })

	if observedInterval != characterizedDefaultFlushInterval {
		t.Fatalf("default flush interval = %s, want 250ms", observedInterval)
	}

	recordEvent(t, root, recordingID, events[0])
	ticker.ticks <- time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC)
	first := requireCharacterizedFlushWrite(t, writes)
	assertCharacterizedReplayWrite(t, first, []recordings.CanonicalEvent{events[0]})

	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	}); err != nil {
		t.Fatalf("clean FlushRecording: %v", err)
	}
	if len(writes) != 0 {
		t.Fatalf("clean flush queued %d additional writes, want none", len(writes))
	}

	recordEvent(t, root, recordingID, events[1])
	ticker.ticks <- time.Date(2026, 2, 3, 18, 45, 13, 0, time.UTC)
	second := requireCharacterizedFlushWrite(t, writes)
	assertCharacterizedReplayWrite(t, second, events[:2])
	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	}); err != nil {
		t.Fatalf("second clean FlushRecording: %v", err)
	}

	status := recordingStatus(t, root, recordingID)
	if status.FlushedThrough == nil || *status.FlushedThrough != events[1].Cursor {
		t.Fatalf("flushed status = %#v, want second event cursor", status)
	}
}

func TestFinishRecordingCharacterizesJoinedFinalPersistenceAndNoPostStopWrite(t *testing.T) {
	t.Parallel()

	ticker := newManualFlushTicker()
	events := characterizedFlushEvents(t)
	writes := make(chan characterizedFlushWrite, 4)
	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	writeCount := 0
	root := newActiveFlushRoot(
		recordingsinternal.NewReplayRecordingSnapshotWriter(
			func(target string, payload []byte) error {
				writeCount++
				if writeCount == 1 {
					close(writeStarted)
					<-releaseWrite
				}
				writes <- characterizedFlushWrite{
					target:  target,
					payload: append([]byte(nil), payload...),
				}
				return nil
			},
		),
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(ticker)
		},
	)
	recordingID := startActiveRecording(t, root, "recording-final-flush-characterization", time.Second)
	recordEvent(t, root, recordingID, events[0])
	ticker.ticks <- time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC)
	<-writeStarted

	// The second event is accepted while the first complete document is still
	// being persisted. Finalization must join that write before its final one.
	recordEvent(t, root, recordingID, events[1])
	finished := make(chan recordings.FinishRecordingResult, 1)
	finishErrors := make(chan error, 1)
	go func() {
		result, err := root.FinishRecording(recordings.FinishRecordingRequest{
			RecordingID: recordingID,
			FinishedAt:  time.Date(2026, 2, 3, 18, 46, 0, 0, time.UTC),
		})
		finished <- result
		finishErrors <- err
	}()

	select {
	case <-finished:
		t.Fatal("FinishRecording returned while the periodic write was blocked")
	default:
	}
	close(releaseWrite)
	result := <-finished
	if err := <-finishErrors; err != nil {
		t.Fatalf("FinishRecording: %v", err)
	}
	if writeCount != 2 || result.Status.FlushedThrough == nil ||
		*result.Status.FlushedThrough != events[1].Cursor ||
		result.Status.State != recordings.RecordingFinalized {
		t.Fatalf("final status = %#v with %d writes, want latest event durably finalized", result.Status, writeCount)
	}

	first := requireCharacterizedFlushWrite(t, writes)
	assertCharacterizedReplayWrite(t, first, []recordings.CanonicalEvent{events[0]})
	second := requireCharacterizedFlushWrite(t, writes)
	assertCharacterizedReplayWrite(t, second, events[:2])
	if len(writes) != 0 {
		t.Fatalf("finalization queued %d unexpected writes", len(writes))
	}

	// FinishRecording has stopped and joined the ticker loop. A tick delivered
	// after that boundary must not create another persistence effect.
	ticker.ticks <- time.Date(2026, 2, 3, 18, 46, 1, 0, time.UTC)
	if len(writes) != 0 || writeCount != 2 {
		t.Fatalf("post-stop writes = %d buffered / %d total, want none", len(writes), writeCount)
	}
}

func TestFailedFlushCharacterizesUnadvancedDurablePosition(t *testing.T) {
	t.Parallel()

	writeErr := errors.New("storage unavailable")
	root := newActiveFlushRoot(
		func(string, recordings.RecordingSnapshot) error { return writeErr },
		func(time.Duration) recordings.RecordingFlushTicker {
			return manualTickerHandle(newManualFlushTicker())
		},
	)
	recordingID := startActiveRecording(t, root, "recording-failed-flush-characterization", 0)
	t.Cleanup(func() { stopRecording(t, root, recordingID) })
	recordEvent(t, root, recordingID, characterizedFlushEvents(t)[0])

	if _, err := root.FlushRecording(recordings.FlushRecordingRequest{
		RecordingID: recordingID,
	}); !errors.Is(err, writeErr) {
		t.Fatalf("FlushRecording error = %v, want storage error", err)
	}
	status := recordingStatus(t, root, recordingID)
	if status.FlushedThrough != nil {
		t.Fatalf("failed flush advanced durable position: %#v", status)
	}
}

func requireCharacterizedFlushWrite(
	t *testing.T,
	writes <-chan characterizedFlushWrite,
) characterizedFlushWrite {
	t.Helper()
	select {
	case write := <-writes:
		return write
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for controlled recording flush")
		return characterizedFlushWrite{}
	}
}

func assertCharacterizedReplayWrite(
	t *testing.T,
	write characterizedFlushWrite,
	wantEvents []recordings.CanonicalEvent,
) {
	t.Helper()
	if write.target != "artifact:active" {
		t.Fatalf("flush target = %q, want artifact:active", write.target)
	}
	var envelope characterizedReplayEnvelope
	if err := json.Unmarshal(write.payload, &envelope); err != nil {
		t.Fatalf("decode persisted replay document: %v", err)
	}
	if !bytes.HasSuffix(write.payload, []byte("}\n")) ||
		bytes.HasSuffix(write.payload, []byte("}\n\n")) {
		t.Fatal("persisted replay document does not have exactly one terminal newline")
	}
	if envelope.SchemaVersion != "agent-factory.replay.v1" {
		t.Fatalf("persisted schema version = %q, want agent-factory.replay.v1", envelope.SchemaVersion)
	}
	if len(envelope.Events) != len(wantEvents) {
		t.Fatalf("persisted event count = %d, want %d", len(envelope.Events), len(wantEvents))
	}
	for index, want := range wantEvents {
		if envelope.Events[index].ID != string(want.ID) ||
			envelope.Events[index].Context.Sequence != int(want.Sequence) {
			t.Fatalf("persisted event %d = %#v, want id=%q sequence=%d", index, envelope.Events[index], want.ID, want.Sequence)
		}
	}
}

func characterizedFlushEvents(t *testing.T) []recordings.CanonicalEvent {
	t.Helper()
	startedAt := time.Date(2026, 2, 3, 18, 45, 12, 0, time.UTC)
	factorySnapshot, err := factorydefinitions.NewFactorySnapshot(map[string]any{
		"id": "factory-flush-characterization",
		"metadata": map[string]any{
			"factory_hash": "sha256:factory-flush",
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
		Id:   "work-request-flush-1",
		Type: recordings.FactoryEventTypeWorkRequest,
		Context: recordings.FactoryEventContext{
			EventTime: startedAt.Add(time.Minute),
			Sequence:  1,
			Tick:      1,
		},
		Payload: []byte(`{"workId":"work-flush-1"}`),
	}
	events := []recordings.CanonicalEvent{
		canonical.CanonicalEventFromFactory(started.Events[0], "generation-flush-characterization"),
		canonical.CanonicalEventFromFactory(work, "generation-flush-characterization"),
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: "session-active"}
	for index := range events {
		events[index].Scope = scope
		events[index].Sequence = recordings.CanonicalEventSequence(index)
		events[index].Cursor.Sequence = recordings.CanonicalEventSequence(index)
	}
	return events
}
