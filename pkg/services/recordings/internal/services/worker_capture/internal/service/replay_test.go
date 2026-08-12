package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkerCaptureLiveProjectionEqualsCompletedReplay(t *testing.T) {
	request, handle, recordingRoot := startCompletedWorkerCapture(t)
	liveReader, ok := handle.(recordings.WorkerRecordingProjectionReader)
	if !ok {
		t.Fatal("capture does not expose its live projection")
	}
	live, err := liveReader.WorkerRecordingProjection()
	if err != nil {
		t.Fatalf("WorkerRecordingProjection() error = %v", err)
	}
	snapshot := loadWorkerRecording(t, recordingRoot, request.RecordingID)
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording() error = %v", err)
	}
	if !reflect.DeepEqual(live, replayed.Projection) {
		t.Fatalf("live projection = %#v, replay projection = %#v", live, replayed.Projection)
	}
	if !live.Complete || len(live.Records) != 3 || live.Records[2].ID.Position != 3 {
		t.Fatalf("live projection = %#v, want complete opening/output/terminal history", live)
	}
}

func TestWorkerCaptureAbortPersistsIncompleteSnapshot(t *testing.T) {
	eventService := newRecordingEventsService()
	recordingRoot := t.TempDir()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), recordingRoot)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(eventService, writer, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:     "recording-aborted",
		WorkerSessionID: "worker-aborted",
		Topic:           events.Topic("worker-session/worker-aborted/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Abort(context.Background(), nil); !errors.Is(err, recordings.ErrWorkerRecordingOpening) {
		t.Fatalf("Abort() error = %v, want opening failure", err)
	}
	reader, ok := writer.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("FileWriter does not expose its durable reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), request.RecordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("aborted snapshot = %#v, want one INCOMPLETE Worker Session", snapshot)
	}
}

func startCompletedWorkerCapture(t *testing.T) (recordings.WorkerSessionRecordingRequest, recordings.WorkerSessionRecording, string) {
	t.Helper()
	eventService := newRecordingEventsService()
	recordingRoot := t.TempDir()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), recordingRoot)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(eventService, writer, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:     "recording-replay",
		WorkerSessionID: "worker-replay",
		Topic:           events.Topic("worker-session/worker-replay/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, appendRequest := range []events.AppendRequest{
		openingAppend(request.Topic, request.WorkerSessionID),
		workerOutputAppend(request.Topic, request.WorkerSessionID, 1, "message-1"),
		terminalAppend(request.Topic, request.WorkerSessionID),
	} {
		if _, err := eventService.Append(context.Background(), appendRequest); err != nil {
			t.Fatal(err)
		}
	}
	if err := handle.AwaitOpening(context.Background()); err != nil {
		t.Fatalf("AwaitOpening() error = %v", err)
	}
	if err := handle.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return request, handle, recordingRoot
}

func loadWorkerRecording(t *testing.T, recordingRoot, recordingID string) recordings.WorkerRecordingSnapshot {
	t.Helper()
	reopenedWriter, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), recordingRoot)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := reopenedWriter.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("FileWriter does not expose its durable reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	return snapshot
}

func TestWorkerCaptureCloseRejectsProviderCompletionWithoutTerminal(t *testing.T) {
	eventService := newRecordingEventsService()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(eventService, writer, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:     "recording-incomplete",
		WorkerSessionID: "worker-incomplete",
		Topic:           events.Topic("worker-session/worker-incomplete/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eventService.Append(context.Background(), openingAppend(request.Topic, request.WorkerSessionID)); err != nil {
		t.Fatal(err)
	}
	if err := handle.AwaitOpening(context.Background()); err != nil {
		t.Fatalf("AwaitOpening() error = %v", err)
	}
	if err := handle.Close(context.Background()); !errors.Is(err, recordings.ErrWorkerRecordingIncomplete) {
		t.Fatalf("Close() error = %v, want ErrWorkerRecordingIncomplete", err)
	}
	reader, ok := writer.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("FileWriter does not expose its durable reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), request.RecordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("failed snapshot = %#v, want one INCOMPLETE Worker Session", snapshot)
	}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording(incomplete) error = %v", err)
	}
	if replayed.Projection.Status != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("ReplayWorkerRecording(incomplete) status = %q, want INCOMPLETE", replayed.Projection.Status)
	}
}

func TestWorkerCaptureCloseCancellationIsNotCompletion(t *testing.T) {
	eventService := newRecordingEventsService()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(eventService, writer, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:     "recording-canceled",
		WorkerSessionID: "worker-canceled",
		Topic:           events.Topic("worker-session/worker-canceled/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := handle.Close(ctx); !errors.Is(err, recordings.ErrWorkerRecordingCanceled) {
		t.Fatalf("Close(canceled) error = %v, want ErrWorkerRecordingCanceled", err)
	}
	reader, ok := writer.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("FileWriter does not expose its durable reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), request.RecordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusIncomplete {
		t.Fatalf("canceled snapshot = %#v, want one INCOMPLETE Worker Session", snapshot)
	}
}

func TestWorkerRecordingDurableLossWithAuthoritativeTerminalReopensAsDegraded(t *testing.T) {
	const (
		recordingID = "recording-degraded"
		sessionID   = "worker-degraded"
	)
	topic := events.Topic("worker-session/worker-degraded/events")
	recordingRoot := t.TempDir()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), recordingRoot)
	if err != nil {
		t.Fatal(err)
	}
	opening := mustRecord(t, openingAppend(topic, sessionID), 1)
	if err := writer.PersistWorkerRecord(context.Background(), recordings.WorkerRecordingRecord{
		RecordingID: recordingID, WorkerSessionID: sessionID, Record: opening,
	}); err != nil {
		t.Fatalf("PersistWorkerRecord(opening) error = %v", err)
	}
	if failureWriter, ok := writer.(recordings.WorkerRecordingFailureWriter); ok {
		if err := failureWriter.PersistWorkerRecordingFailure(context.Background(), recordings.WorkerRecordingFailure{
			RecordingID: recordingID, WorkerSessionID: sessionID, Topic: topic, Code: "PERSISTENCE_FAILED",
			ExecutionTerminal: &recordings.WorkerRecordingTerminal{Phase: workers.PhaseCompleted, Status: "COMPLETED"},
		}); err != nil {
			t.Fatalf("PersistWorkerRecordingFailure() error = %v", err)
		}
	} else {
		t.Fatal("FileWriter does not expose the failure writer contract")
	}

	reopened, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), recordingRoot)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := reopened.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("FileWriter does not expose the durable reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusDegraded {
		t.Fatalf("durable degraded snapshot = %#v, want DEGRADED", snapshot)
	}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording() error = %v", err)
	}
	if replayed.Projection.Status != recordings.WorkerRecordingStatusDegraded || replayed.Projection.Terminal != nil || replayed.Projection.ExecutionTerminal == nil {
		t.Fatalf("replayed degraded projection = %#v, want authoritative terminal without fabricated record", replayed.Projection)
	}
}

func TestWorkerCaptureRejectsRecordAfterTerminal(t *testing.T) {
	eventService := newRecordingEventsService()
	service, err := New(eventService, recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error {
		return nil
	}), logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		WorkerSessionID: "worker-late",
		Topic:           events.Topic("worker-session/worker-late/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	for _, appendRequest := range []events.AppendRequest{
		openingAppend(request.Topic, request.WorkerSessionID),
		terminalAppend(request.Topic, request.WorkerSessionID),
		workerOutputAppend(request.Topic, request.WorkerSessionID, 2, "message-after-terminal"),
	} {
		if _, err := eventService.Append(context.Background(), appendRequest); err != nil {
			t.Fatal(err)
		}
	}
	if err := handle.AwaitOpening(context.Background()); err != nil {
		t.Fatalf("AwaitOpening() error = %v", err)
	}
	if err := handle.Close(context.Background()); !errors.Is(err, recordings.ErrWorkerRecordingTerminal) {
		t.Fatalf("Close() error = %v, want ErrWorkerRecordingTerminal", err)
	}
}

func TestWorkerCaptureDuplicateDeliveryIsIdempotentOnlyWhenIdentical(t *testing.T) {
	topic := events.Topic("worker-session/worker-duplicate/events")
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:     "recording-duplicate",
		WorkerSessionID: "worker-duplicate",
		Topic:           topic,
	}
	writes := 0
	capture := &capture{
		request: request,
		writer: recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error {
			writes++
			return nil
		}),
		runCtx:     context.Background(),
		opening:    make(chan struct{}),
		identities: make(map[events.AppendIdentity]events.Record),
	}
	opening := mustRecord(t, openingAppend(topic, request.WorkerSessionID), 1)
	if err := capture.accept(opening); err != nil {
		t.Fatalf("accept(opening): %v", err)
	}
	if err := capture.accept(opening.Detached()); err != nil {
		t.Fatalf("accept(identical redelivery): %v, want idempotent success", err)
	}
	conflict := opening.Detached()
	conflict.Payload = []byte(`{"kind":"SESSION","phase":"STARTED","payload":{"status":"STARTING","workerSessionId":"different"}}`)
	if err := capture.accept(conflict); !errors.Is(err, recordings.ErrWorkerRecordingDuplicate) {
		t.Fatalf("accept(conflicting redelivery): %v, want ErrWorkerRecordingDuplicate", err)
	}
	if writes != 1 {
		t.Fatalf("durable writes = %d, want one write for one accepted record", writes)
	}
}

func TestReplayWorkerRecordingReturnsIncompleteAndRejectsSkippedHistory(t *testing.T) {
	topic := events.Topic("worker-session/worker-replay-invalid/events")
	opening := mustRecord(t, openingAppend(topic, "worker-replay-invalid"), 1)
	terminal := mustRecord(t, terminalAppend(topic, "worker-replay-invalid"), 2)
	snapshot := recordings.WorkerRecordingSnapshot{
		RecordingID: "recording-invalid",
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: "worker-replay-invalid",
			Topic:           topic,
			Records:         []events.Record{opening, terminal},
		}},
	}
	if _, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot}); err != nil {
		t.Fatalf("ReplayWorkerRecording(valid history) error = %v", err)
	}
	incomplete := snapshot
	incomplete.Sessions = []recordings.WorkerSessionRecordingSnapshot{{
		WorkerSessionID: "worker-replay-invalid",
		Topic:           topic,
		Records:         []events.Record{opening},
	}}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: incomplete})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording(incomplete) error = %v", err)
	}
	if replayed.Projection.Status != recordings.WorkerRecordingStatusIncomplete || replayed.Projection.Complete {
		t.Fatalf("ReplayWorkerRecording(incomplete) projection = %#v, want INCOMPLETE and not complete", replayed.Projection)
	}
	skipped := snapshot
	skipped.Sessions = []recordings.WorkerSessionRecordingSnapshot{{
		WorkerSessionID: "worker-replay-invalid",
		Topic:           topic,
		Records: []events.Record{opening, func() events.Record {
			record := terminal.Detached()
			record.ID.Position = 3
			return record
		}()},
	}}
	if _, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: skipped}); !errors.Is(err, recordings.ErrWorkerRecordingOrder) {
		t.Fatalf("ReplayWorkerRecording(skipped) error = %v, want ErrWorkerRecordingOrder", err)
	}
}

func mustRecord(t *testing.T, request events.AppendRequest, position events.AggregateSequence) events.Record {
	t.Helper()
	return events.Record{
		ID:             events.RecordID{Topic: request.Topic, Position: position},
		SourceType:     request.SourceType,
		SourceID:       request.SourceID,
		SourceSequence: request.SourceSequence,
		SourceEventID:  request.SourceEventID,
		SchemaID:       request.SchemaID,
		Payload:        append([]byte(nil), request.Payload...),
	}
}

func workerOutputAppend(topic events.Topic, sessionID string, sequence events.SourceSequence, eventID string) events.AppendRequest {
	payload, _ := json.Marshal(workers.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workers.ContentBlock{{
			Kind: workers.ContentBlockText,
			Text: "captured",
		}},
	})
	draft, _ := json.Marshal(workers.Draft{
		Kind:  workers.KindMessage,
		Phase: workers.PhaseCompleted,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliveryNativeFinal,
			Fidelity:        workers.FidelityFinalOnly,
			NativeEventType: "message.completed",
			Provider:        "codex",
			Representation:  workers.RepresentationSnapshot,
		},
		Payload:    payload,
		DispatchID: sessionID,
	})
	return events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker_provider",
		SourceID:       events.SourceID(sessionID + "/provider"),
		SourceSequence: sequence,
		SourceEventID:  events.SourceEventID(eventID),
		SchemaID:       "workers.draft.v1",
		Payload:        draft,
	}
}
