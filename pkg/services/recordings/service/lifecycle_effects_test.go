package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
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
