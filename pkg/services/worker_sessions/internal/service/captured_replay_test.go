package service

import (
	"context"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type replayCaptureReader struct {
	recordings.WorkerSessionRecordingService
	snapshot recordings.WorkerRecordingSnapshot
}

func (f replayCaptureReader) LoadWorkerRecording(context.Context, string) (recordings.WorkerRecordingSnapshot, error) {
	return f.snapshot, nil
}

func TestCapturedAtReplayKeepsStoredTimeAndOmitsUnknown(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 10, 5, 10, 0, 0, 0, time.UTC)
	topic := workersessions.Topic("worker-1")
	reader := &observationEventReaderFake{readResults: []events.ReadResult{{
		Outcome:  events.ReadOutcomeProgress,
		Records:  []events.Record{replayObservationRecord(topic, 1, "one"), replayObservationRecord(topic, 2, "two")},
		Next:     events.Cursor{Topic: topic, Position: 2},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 2},
	}}}
	r := newObservationRegistry(observationProjectorFake{}, reader)
	r.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	r.publications = map[string]*publication{"worker-1": {recordingID: "recording"}}
	times := map[string]time.Time{"1": stamp}
	r.recording = replayCaptureReader{snapshot: recordings.WorkerRecordingSnapshot{
		RecordingID: "recording", Sessions: []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: "worker-1", CapturedAt: times}},
	}}
	stream, err := r.StreamObservationsByWorkerSessionID(t.Context(), workersessions.StreamObservationsByWorkerSessionIDRequest{WorkerSessionID: "worker-1", ReplayOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	times["1"] = time.Time{}
	first := stream.Next(t.Context())
	if first.Event.CapturedAt == nil || !first.Event.CapturedAt.Equal(stamp) {
		t.Fatalf("replay lost detached committed timestamp: %+v", first.Event)
	}
	second := stream.Next(t.Context())
	if second.Event.CapturedAt != nil {
		t.Fatal("replay invented a timestamp for an unknown record")
	}
}
