package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
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

func TestFileWriterPersistsV2JSONLAndCatalogResolvesWorkerID(t *testing.T) {
	const (
		recordingID = "recording-v2-catalog"
		sessionID   = "worker-v2-catalog"
	)
	root := t.TempDir()
	storage := platformreplay.NewLocal(runtime.GOOS)
	writer, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	fileWriter := writer.(*FileWriter)
	topic := events.Topic("worker-session/" + sessionID + "/events")
	for _, record := range []events.Record{
		mustRecord(t, openingAppend(topic, sessionID), 1),
		mustRecord(t, workerOutputAppend(topic, sessionID, 1, "message-1"), 2),
		mustRecord(t, terminalAppend(topic, sessionID), 3),
	} {
		if err := writer.PersistWorkerRecord(context.Background(), recordings.WorkerRecordingRecord{
			RecordingID:      recordingID,
			WorkerSessionID:  sessionID,
			FactorySessionID: "factory-v2",
			WorkIDs:          []string{"work-v2"},
			AttemptID:        "attempt-v2",
			Record:           record,
		}); err != nil {
			t.Fatalf("PersistWorkerRecord(%d): %v", record.ID.Position, err)
		}
	}

	data, err := storage.ReadFile(fileWriter.v2Path(recordingID))
	if err != nil {
		t.Fatalf("read v2 artifact: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Fatalf("v2 artifact lines = %d, want header, three records, and health: %s", len(lines), string(data))
	}
	wantKinds := []string{"header", "record", "record", "record", "health"}
	for index, line := range lines {
		var envelope struct {
			SchemaVersion string `json:"schemaVersion"`
			Kind          string `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatalf("decode v2 line %d: %v", index+1, err)
		}
		if envelope.SchemaVersion != workerRecordingV2SchemaVersion || envelope.Kind != wantKinds[index] {
			t.Fatalf("v2 line %d = %#v, want schema %q kind %q", index+1, envelope, workerRecordingV2SchemaVersion, wantKinds[index])
		}
	}
	if _, err := storage.ReadFile(fileWriter.path(recordingID)); err == nil {
		t.Fatal("new Worker recording unexpectedly wrote a v1 snapshot")
	}

	reopened, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	historyReader := reopened.(recordings.WorkerRecordingHistoryReader)
	snapshot, err := historyReader.LoadWorkerRecordingByWorkerSessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadWorkerRecordingByWorkerSessionID(): %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].WorkerSessionID != sessionID ||
		snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusComplete ||
		len(snapshot.Sessions[0].Records) != 3 {
		t.Fatalf("Worker-ID snapshot = %#v, want complete three-record history", snapshot)
	}
	page, err := historyReader.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 1})
	if err != nil {
		t.Fatalf("ListWorkerRecordingProjections(): %v", err)
	}
	if len(page.Projections) != 1 || page.NextToken != "" || len(page.Diagnostics) != 0 ||
		page.Projections[0].WorkerSessionID != sessionID || page.Projections[0].FactorySessionID != "factory-v2" ||
		!containsString(page.Projections[0].WorkIDs, "work-v2") {
		t.Fatalf("catalog page = %#v, want one complete Worker-ID projection without diagnostics", page)
	}
}

func TestFileWriterCatalogPaginatesWorkerIDLookup(t *testing.T) {
	root := t.TempDir()
	storage := platformreplay.NewLocal(runtime.GOOS)
	writer, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	fileWriter := writer.(*FileWriter)
	const count = defaultWorkerRecordingPageSize + 7
	for index := 0; index < count; index++ {
		sessionID := fmt.Sprintf("worker-page-%03d", index)
		recordingID := fmt.Sprintf("recording-page-%03d", index)
		topic := events.Topic("worker-session/" + sessionID + "/events")
		record := mustRecord(t, openingAppend(topic, sessionID), 1)
		header, err := json.Marshal(workerRecordingV2Header{
			SchemaVersion:   workerRecordingV2SchemaVersion,
			Kind:            "header",
			RecordingID:     recordingID,
			WorkerSessionID: sessionID,
			Topic:           topic,
			Status:          recordings.WorkerRecordingStatusActive,
		})
		if err != nil {
			t.Fatalf("encode catalog header %d: %v", index, err)
		}
		body, err := json.Marshal(workerRecordingV2Record{
			SchemaVersion:   workerRecordingV2SchemaVersion,
			Kind:            "record",
			WorkerSessionID: sessionID,
			Record:          record,
		})
		if err != nil {
			t.Fatalf("encode catalog record %d: %v", index, err)
		}
		data := append(header, '\n')
		data = append(data, body...)
		data = append(data, '\n')
		if err := storage.WriteFile(fileWriter.v2Path(recordingID), data); err != nil {
			t.Fatalf("seed catalog artifact %d: %v", index, err)
		}
	}

	reopened, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	historyReader := reopened.(recordings.WorkerRecordingHistoryReader)
	snapshot, err := historyReader.LoadWorkerRecordingByWorkerSessionID(context.Background(), "worker-page-056")
	if err != nil {
		t.Fatalf("paginated Worker-ID lookup: %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].WorkerSessionID != "worker-page-056" {
		t.Fatalf("paginated Worker-ID snapshot = %#v, want worker-page-056", snapshot)
	}
}

func TestFileWriterCatalogReportsMalformedTailWithValidPrefix(t *testing.T) {
	const (
		recordingID = "recording-v2-malformed-tail"
		sessionID   = "worker-v2-malformed-tail"
	)
	root := t.TempDir()
	storage := platformreplay.NewLocal(runtime.GOOS)
	writer, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	topic := events.Topic("worker-session/" + sessionID + "/events")
	for _, record := range []events.Record{
		mustRecord(t, openingAppend(topic, sessionID), 1),
		mustRecord(t, terminalAppend(topic, sessionID), 2),
	} {
		if err := writer.PersistWorkerRecord(context.Background(), recordings.WorkerRecordingRecord{
			RecordingID: recordingID, WorkerSessionID: sessionID, Record: record,
		}); err != nil {
			t.Fatalf("seed Worker recording: %v", err)
		}
	}
	fileWriter := writer.(*FileWriter)
	if err := storage.AppendFile(fileWriter.v2Path(recordingID), []byte("{malformed-tail\n")); err != nil {
		t.Fatalf("append malformed tail: %v", err)
	}

	reopened, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	reader := reopened.(recordings.WorkerRecordingReader)
	snapshot, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("read valid prefix: %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusDegraded ||
		snapshot.Sessions[0].Failure != "MALFORMED_TAIL" || len(snapshot.Sessions[0].Records) != 2 {
		t.Fatalf("malformed-tail snapshot = %#v, want degraded valid prefix", snapshot)
	}
	historyReader := reopened.(recordings.WorkerRecordingHistoryReader)
	page, err := historyReader.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 1})
	if err != nil {
		t.Fatalf("catalog malformed tail: %v", err)
	}
	if len(page.Projections) != 1 || page.Projections[0].Status != recordings.WorkerRecordingStatusDegraded || len(page.Diagnostics) != 1 ||
		page.Diagnostics[0].Code != recordings.WorkerRecordingCatalogMalformedTail || page.Diagnostics[0].RecordingID != recordingID {
		t.Fatalf("malformed-tail catalog = %#v, want degraded projection plus MALFORMED_TAIL diagnostic", page)
	}
}

func TestFileWriterLoadsContinuationLineageAppendedAfterExecutionTerminal(t *testing.T) {
	const (
		recordingID = "recording-post-terminal-lineage"
		sessionID   = "worker-post-terminal-lineage"
	)
	root := t.TempDir()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), root)
	if err != nil {
		t.Fatal(err)
	}
	topic := events.Topic("worker-session/" + sessionID + "/events")
	opening := mustRecord(t, openingAppend(topic, sessionID), 1)
	terminal := mustRecord(t, terminalAppend(topic, sessionID), 2)
	lineagePayload, _ := json.Marshal(workers.SessionPayload{
		Status:          "COMPLETED",
		WorkerSessionID: sessionID,
		DispatchID:      "dispatch-post-terminal",
		AttemptID:       "dispatch-post-terminal",
		Continuation: &workers.SessionContinuation{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "opaque-provider-session",
		},
		Lineage: &workers.SessionLineage{SuccessorWorkerSessionID: "successor-post-terminal"},
	})
	lineageDraft, _ := json.Marshal(workers.Draft{
		Kind:  workers.KindSession,
		Phase: workers.PhaseUpdated,
		Provenance: workers.Provenance{
			Delivery: workers.DeliverySynthesized, Fidelity: workers.FidelityLifecycleOnly,
			NativeEventType: "worker_session_lineage", Representation: workers.RepresentationNotification,
		},
		Payload:    lineagePayload,
		DispatchID: "dispatch-post-terminal",
	})
	lineage := mustRecord(t, events.AppendRequest{
		Topic:          topic,
		SourceType:     "worker_session_lineage",
		SourceID:       "worker-post-terminal-lineage/successor/successor-post-terminal",
		SourceSequence: 1,
		SourceEventID:  "successor",
		SchemaID:       "workers.draft.v1",
		Payload:        lineageDraft,
	}, 3)
	persistWorkerRecoveryPrefix(t, writer, recordingID, sessionID, opening, terminal, lineage)

	reopened, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), root)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := reopened.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("reopened FileWriter does not expose the durable reader")
	}
	snapshot, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(): %v", err)
	}
	if len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Records) != 3 ||
		snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusComplete ||
		snapshot.Sessions[0].Records[2].SourceType != events.SourceType("worker_session_lineage") {
		t.Fatalf("loaded snapshot = %#v, want complete opening/terminal/lineage history", snapshot)
	}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording(): %v", err)
	}
	if len(replayed.Projection.Records) != 3 || replayed.Projection.Records[2].ID.Position != 3 {
		t.Fatalf("replayed projection = %#v, want chronological post-terminal lineage", replayed.Projection)
	}
}

func TestWorkerRecordingRecoveryAfterRestartPreservesDurablePrefix(t *testing.T) {
	const (
		recordingID = "recording-interrupted-recovery"
		sessionID   = "worker-interrupted-recovery"
	)
	root := t.TempDir()
	writer, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), root)
	if err != nil {
		t.Fatal(err)
	}
	topic := events.Topic("worker-session/" + sessionID + "/events")
	opening := mustRecord(t, openingAppend(topic, sessionID), 1)
	output := mustRecord(t, workerOutputAppend(topic, sessionID, 1, "message-before-stop"), 2)
	persistWorkerRecoveryPrefix(t, writer, recordingID, sessionID, opening, output)

	reopened, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), root)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := reopened.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("reopened FileWriter does not expose the durable reader")
	}
	first, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(first recovery): %v", err)
	}
	second, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(repeated recovery): %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated recovery changed the snapshot\nfirst: %#v\nsecond: %#v", first, second)
	}
	recoveredSession := requireRecoveredWorkerPrefix(t, first)
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
		Snapshot:        first,
		WorkerSessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording(recovered prefix): %v", err)
	}
	assertRecoveredWorkerReplay(t, replayed, recoveredSession)
}

func persistWorkerRecoveryPrefix(t *testing.T, writer recordings.WorkerRecordingWriter, recordingID, sessionID string, records ...events.Record) {
	t.Helper()
	for _, record := range records {
		if err := writer.PersistWorkerRecord(context.Background(), recordings.WorkerRecordingRecord{
			RecordingID: recordingID, WorkerSessionID: sessionID, Record: record,
		}); err != nil {
			t.Fatalf("PersistWorkerRecord(%d): %v", record.ID.Position, err)
		}
	}
}

func requireRecoveredWorkerPrefix(t *testing.T, snapshot recordings.WorkerRecordingSnapshot) recordings.WorkerSessionRecordingSnapshot {
	t.Helper()
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("recovered snapshot = %#v, want one Worker Session", snapshot)
	}
	session := snapshot.Sessions[0]
	if session.Status != recordings.WorkerRecordingStatusIncomplete || session.LastPosition != 2 ||
		session.InterruptionReason != recordings.WorkerRecordingInterruptionProcessStopped {
		t.Fatalf("recovered session = %#v, want INCOMPLETE at position 2 with process interruption reason", session)
	}
	if len(session.Records) != 2 || session.Records[0].ID.Position != 1 || session.Records[1].ID.Position != 2 {
		t.Fatalf("recovered records = %#v, want the exact positions 1 and 2", session.Records)
	}
	return session
}

func assertRecoveredWorkerReplay(t *testing.T, replayed recordings.WorkerRecordingReplayResult, session recordings.WorkerSessionRecordingSnapshot) {
	t.Helper()
	if replayed.Projection.Status != recordings.WorkerRecordingStatusIncomplete ||
		replayed.Projection.InterruptionReason != recordings.WorkerRecordingInterruptionProcessStopped ||
		replayed.Projection.LastPosition != session.LastPosition {
		t.Fatalf("replayed projection = %#v, want readable INCOMPLETE prefix with stable interruption reason", replayed.Projection)
	}
}

func TestWorkerRecordingRecoveryDerivesCompleteWithoutStoredCompletionMetadata(t *testing.T) {
	const (
		recordingID = "recording-terminal-recovery"
		sessionID   = "worker-terminal-recovery"
	)
	root := t.TempDir()
	storage := platformreplay.NewLocal(runtime.GOOS)
	writer, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	fileWriter, ok := writer.(*FileWriter)
	if !ok {
		t.Fatal("NewFileWriter() did not return FileWriter")
	}
	topic := events.Topic("worker-session/" + sessionID + "/events")
	snapshot := recordings.WorkerRecordingSnapshot{
		RecordingID: recordingID,
		Sessions: []recordings.WorkerSessionRecordingSnapshot{{
			WorkerSessionID: sessionID,
			Topic:           topic,
			Records: []events.Record{
				mustRecord(t, openingAppend(topic, sessionID), 1),
				mustRecord(t, terminalAppend(topic, sessionID), 2),
			},
		}},
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFile(fileWriter.path(recordingID), payload); err != nil {
		t.Fatalf("seed durable terminal snapshot: %v", err)
	}

	reopened, err := NewFileWriter(storage, root)
	if err != nil {
		t.Fatal(err)
	}
	reader, ok := reopened.(recordings.WorkerRecordingReader)
	if !ok {
		t.Fatal("reopened FileWriter does not expose the durable reader")
	}
	recovered, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording(terminal recovery): %v", err)
	}
	if len(recovered.Sessions) != 1 {
		t.Fatalf("recovered snapshot = %#v, want one Worker Session", recovered)
	}
	session := recovered.Sessions[0]
	if session.Status != recordings.WorkerRecordingStatusComplete || session.LastPosition != 2 || session.InterruptionReason != "" {
		t.Fatalf("recovered terminal session = %#v, want COMPLETE at position 2 without interruption reason", session)
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

func TestWorkerCapturePostOpeningPersistenceFailureRetainsTerminalTruth(t *testing.T) {
	eventService := newRecordingEventsService()
	recordingRoot := t.TempDir()
	service := newPostOpeningFailureService(t, eventService, recordingRoot)
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:     "recording-post-opening-loss",
		WorkerSessionID: "worker-post-opening-loss",
		Topic:           events.Topic("worker-session/worker-post-opening-loss/events"),
	}
	handle := startPostOpeningFailureRecording(t, service, eventService, request)
	finalizer := requireWorkerRecordingFinalizer(t, handle)
	closeErr := finalizer.CloseWithTerminal(context.Background(), completedWorkerTerminal())
	if !errors.Is(closeErr, recordings.ErrWorkerRecordingPersistence) {
		t.Fatalf("CloseWithTerminal() error = %v, want persistence failure", closeErr)
	}

	snapshot := loadWorkerRecording(t, recordingRoot, request.RecordingID)
	assertPostOpeningFailureSnapshot(t, snapshot)
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{
		Snapshot:        snapshot,
		WorkerSessionID: request.WorkerSessionID,
	})
	if err != nil {
		t.Fatalf("ReplayWorkerRecording() error = %v", err)
	}
	assertPostOpeningFailureReplay(t, replayed)
}

func newPostOpeningFailureService(t *testing.T, eventService events.Service, root string) recordings.WorkerSessionRecordingService {
	t.Helper()
	baseWriter, err := NewFileWriter(platformreplay.NewLocal(runtime.GOOS), root)
	if err != nil {
		t.Fatal(err)
	}
	failureWriter, ok := baseWriter.(recordings.WorkerRecordingFailureWriter)
	if !ok {
		t.Fatal("FileWriter does not expose the failure writer contract")
	}
	service, err := New(eventService, &postOpeningFailureWriter{
		delegate: baseWriter, failureWriter: failureWriter, failPosition: 2,
	}, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func startPostOpeningFailureRecording(
	t *testing.T,
	service recordings.WorkerSessionRecordingService,
	eventService events.Service,
	request recordings.WorkerSessionRecordingRequest,
) recordings.WorkerSessionRecording {
	t.Helper()
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
	if _, err := eventService.Append(context.Background(), terminalAppend(request.Topic, request.WorkerSessionID)); err != nil {
		t.Fatal(err)
	}
	return handle
}

func requireWorkerRecordingFinalizer(t *testing.T, handle recordings.WorkerSessionRecording) recordings.WorkerSessionRecordingFinalizer {
	t.Helper()
	finalizer, ok := handle.(recordings.WorkerSessionRecordingFinalizer)
	if !ok {
		t.Fatal("capture does not expose terminal-aware finalization")
	}
	return finalizer
}

func completedWorkerTerminal() recordings.WorkerRecordingTerminal {
	return recordings.WorkerRecordingTerminal{Position: 2, Phase: workers.PhaseCompleted, Status: "COMPLETED"}
}

func assertPostOpeningFailureSnapshot(t *testing.T, snapshot recordings.WorkerRecordingSnapshot) {
	t.Helper()
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("durable snapshot = %#v, want one Worker Session", snapshot)
	}
	session := snapshot.Sessions[0]
	if session.Status != recordings.WorkerRecordingStatusDegraded || session.Failure != "PERSISTENCE_FAILED" {
		t.Fatalf("durable session = %#v, want DEGRADED/PERSISTENCE_FAILED", session)
	}
	if session.ExecutionTerminal == nil || session.ExecutionTerminal.Phase != workers.PhaseCompleted || session.ExecutionTerminal.Status != "COMPLETED" {
		t.Fatalf("durable execution terminal = %#v, want completed authoritative truth", session.ExecutionTerminal)
	}
	if len(session.Records) != 1 || session.Records[0].ID.Position != 1 {
		t.Fatalf("durable prefix = %#v, want only the opening record", session.Records)
	}
}

func assertPostOpeningFailureReplay(t *testing.T, replayed recordings.WorkerRecordingReplayResult) {
	t.Helper()
	if replayed.Projection.Status != recordings.WorkerRecordingStatusDegraded || replayed.Projection.Terminal != nil {
		t.Fatalf("replayed projection = %#v, want degraded prefix without fabricated terminal record", replayed.Projection)
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

type postOpeningFailureWriter struct {
	delegate      recordings.WorkerRecordingWriter
	failureWriter recordings.WorkerRecordingFailureWriter
	failPosition  events.AggregateSequence
}

func (writer *postOpeningFailureWriter) PersistWorkerRecord(ctx context.Context, record recordings.WorkerRecordingRecord) error {
	if record.Record.ID.Position == writer.failPosition {
		return errors.New("injected post-opening persistence failure")
	}
	return writer.delegate.PersistWorkerRecord(ctx, record)
}

func (writer *postOpeningFailureWriter) PersistWorkerRecordingFailure(ctx context.Context, failure recordings.WorkerRecordingFailure) error {
	return writer.failureWriter.PersistWorkerRecordingFailure(ctx, failure)
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
	duplicateSessions := snapshot
	duplicateSessions.Sessions = append(append([]recordings.WorkerSessionRecordingSnapshot(nil), snapshot.Sessions...), snapshot.Sessions[0])
	if _, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: duplicateSessions}); !errors.Is(err, recordings.ErrWorkerRecordingDuplicate) {
		t.Fatalf("ReplayWorkerRecording(duplicate session identity) error = %v, want ErrWorkerRecordingDuplicate", err)
	}
	badLastPosition := snapshot
	badLastPosition.Sessions = append([]recordings.WorkerSessionRecordingSnapshot(nil), snapshot.Sessions...)
	badLastPosition.Sessions[0].LastPosition = 9
	if _, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: badLastPosition}); !errors.Is(err, recordings.ErrWorkerRecordingOrder) {
		t.Fatalf("ReplayWorkerRecording(bad last position) error = %v, want ErrWorkerRecordingOrder", err)
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
