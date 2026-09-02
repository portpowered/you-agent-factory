package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkerCapturePersistsOpeningBeforeBarrierRelease(t *testing.T) {
	eventService := newRecordingEventsService()

	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeOnce sync.Once
	var mu sync.Mutex
	var persisted []recordings.WorkerRecordingRecord
	writer := recordings.WorkerRecordingWriterFunc(func(ctx context.Context, record recordings.WorkerRecordingRecord) error {
		writeOnce.Do(func() { close(writeEntered) })
		select {
		case <-releaseWrite:
		case <-ctx.Done():
			return ctx.Err()
		}
		mu.Lock()
		persisted = append(persisted, record)
		mu.Unlock()
		return nil
	})
	service, err := New(eventService, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:      "recording-1",
		FactorySessionID: "factory-session-1",
		WorkerSessionID:  "worker-1",
		Topic:            events.Topic("worker-session/worker-1/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := eventService.Append(context.Background(), openingAppend(request.Topic, request.WorkerSessionID)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writeEntered:
	case <-time.After(time.Second):
		t.Fatal("opening was not offered to the durable writer")
	}

	close(releaseWrite)
	if err := handle.AwaitOpening(context.Background()); err != nil {
		t.Fatalf("AwaitOpening() error = %v", err)
	}
	if _, err := eventService.Append(context.Background(), terminalAppend(request.Topic, request.WorkerSessionID)); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(persisted) != 2 || persisted[0].Record.ID.Position != 1 || persisted[1].Record.ID.Position != 2 {
		t.Fatalf("persisted Worker history = %#v, want positions 1 and 2", persisted)
	}
	for _, record := range persisted {
		if record.FactorySessionID != request.FactorySessionID {
			t.Fatalf("persisted Factory Session ID = %q, want %q", record.FactorySessionID, request.FactorySessionID)
		}
	}
}

func TestWorkerCaptureRejectsNonOpeningPositionBeforeBarrier(t *testing.T) {
	eventService := newRecordingEventsService()
	var writes int
	service, err := New(eventService, recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error {
		writes++
		return nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		WorkerSessionID: "worker-2",
		Topic:           events.Topic("worker-session/worker-2/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	bad := openingAppend(request.Topic, request.WorkerSessionID)
	bad.SourceType = "not-worker-lifecycle"
	if _, err := eventService.Append(context.Background(), bad); err != nil {
		t.Fatal(err)
	}
	if err := handle.AwaitOpening(context.Background()); !errors.Is(err, recordings.ErrWorkerRecordingOpening) {
		t.Fatalf("AwaitOpening() error = %v, want ErrWorkerRecordingOpening", err)
	}
	if writes != 0 {
		t.Fatalf("writes = %d, want no durable write for an invalid opening", writes)
	}
	if err := handle.Close(context.Background()); !errors.Is(err, recordings.ErrWorkerRecordingOpening) {
		t.Fatalf("Close() error = %v, want the opening failure", err)
	}
}

func TestWorkerCapturePersistenceFailureBlocksOpening(t *testing.T) {
	eventService := newRecordingEventsService()
	writeErr := errors.New("disk full")
	service, err := New(eventService, recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error {
		return writeErr
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		WorkerSessionID: "worker-3",
		Topic:           events.Topic("worker-session/worker-3/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventService.Append(context.Background(), openingAppend(request.Topic, request.WorkerSessionID)); err != nil {
		t.Fatal(err)
	}
	if err := handle.AwaitOpening(context.Background()); !errors.Is(err, recordings.ErrWorkerRecordingPersistence) {
		t.Fatalf("AwaitOpening() error = %v, want persistence failure", err)
	}
}

func TestWorkerCaptureForwardsPostTerminalRecordAndFailure(t *testing.T) {
	writer := &captureForwardingWriter{}
	serviceValue, err := New(newRecordingEventsService(), writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := serviceValue.(*Service)
	record := recordings.WorkerRecordingRecord{RecordingID: "recording-1", WorkerSessionID: "worker-1"}
	failure := recordings.WorkerRecordingFailure{RecordingID: "recording-1", WorkerSessionID: "worker-1", Code: "LINEAGE_LOST"}
	if err := service.PersistWorkerRecord(context.Background(), record); err != nil {
		t.Fatalf("PersistWorkerRecord() error = %v", err)
	}
	if err := service.PersistWorkerRecordingFailure(context.Background(), failure); err != nil {
		t.Fatalf("PersistWorkerRecordingFailure() error = %v", err)
	}
	if len(writer.records) != 1 || writer.records[0].RecordingID != record.RecordingID || len(writer.failures) != 1 || writer.failures[0].Code != failure.Code {
		t.Fatalf("forwarded post-terminal evidence = records=%#v failures=%#v", writer.records, writer.failures)
	}

	var nilService *Service
	if err := nilService.PersistWorkerRecord(context.Background(), record); !errors.Is(err, recordings.ErrMissingWorkerRecordingWriter) {
		t.Fatalf("nil Service PersistWorkerRecord() = %v, want missing writer", err)
	}
	writerOnly, err := New(newRecordingEventsService(), recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error { return nil }), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := writerOnly.(*Service).PersistWorkerRecordingFailure(context.Background(), failure); !errors.Is(err, recordings.ErrMissingWorkerRecordingWriter) {
		t.Fatalf("writer without failure capability = %v, want missing writer", err)
	}
}

type captureForwardingWriter struct {
	records  []recordings.WorkerRecordingRecord
	failures []recordings.WorkerRecordingFailure
}

func (writer *captureForwardingWriter) PersistWorkerRecord(_ context.Context, record recordings.WorkerRecordingRecord) error {
	writer.records = append(writer.records, record)
	return nil
}

func (writer *captureForwardingWriter) PersistWorkerRecordingFailure(_ context.Context, failure recordings.WorkerRecordingFailure) error {
	writer.failures = append(writer.failures, failure)
	return nil
}

func openingAppend(topic events.Topic, sessionID string) events.AppendRequest {
	payload, _ := json.Marshal(workers.SessionPayload{Status: "STARTING", WorkerSessionID: sessionID})
	draft, _ := json.Marshal(workers.Draft{
		Kind:  workers.KindSession,
		Phase: workers.PhaseStarted,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliverySynthesized,
			Fidelity:        workers.FidelityLifecycleOnly,
			NativeEventType: "worker_session_lifecycle",
			Representation:  workers.RepresentationNotification,
		},
		Payload: payload,
	})
	return events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker_session_lifecycle",
		SourceID:       events.SourceID(sessionID),
		SourceSequence: 1,
		SourceEventID:  "started",
		SchemaID:       "workers.draft.v1",
		Payload:        draft,
	}
}

func terminalAppend(topic events.Topic, sessionID string) events.AppendRequest {
	payload, _ := json.Marshal(map[string]string{"status": "COMPLETED"})
	draft, _ := json.Marshal(workers.Draft{
		Kind:  workers.KindSession,
		Phase: workers.PhaseCompleted,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliverySynthesized,
			Fidelity:        workers.FidelityLifecycleOnly,
			NativeEventType: "worker_session_lifecycle",
			Representation:  workers.RepresentationNotification,
		},
		Payload: payload,
	})
	return events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker_session_lifecycle",
		SourceID:       events.SourceID(sessionID),
		SourceSequence: 2,
		SourceEventID:  "terminal",
		SchemaID:       "workers.draft.v1",
		Payload:        draft,
	}
}
