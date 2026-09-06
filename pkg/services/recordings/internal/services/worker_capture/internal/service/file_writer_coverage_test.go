package service

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestFileWriterConstructionRejectsMissingInputs(t *testing.T) {
	storage := platformreplay.NewLocal(runtime.GOOS)
	if _, err := NewFileWriterWithDirectoryReader(nil, t.TempDir(), nil); err == nil {
		t.Fatal("NewFileWriterWithDirectoryReader(nil) succeeded")
	}
	if _, err := NewFileWriterWithDirectoryReader(storage, "", nil); err == nil {
		t.Fatal("NewFileWriterWithDirectoryReader(empty root) succeeded")
	}
	if _, err := New(nil, recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error { return nil }), nil); err == nil {
		t.Fatal("New(nil Events service) succeeded")
	}
	if _, err := New(newRecordingEventsService(), nil, nil); err == nil {
		t.Fatal("New(nil writer) succeeded")
	}
}

func TestServiceForwardsWorkerRecordingHistoryCapabilities(t *testing.T) {
	root := t.TempDir()
	writer, _ := newDirectoryReaderFileWriter(t, root)
	const (
		recordingID = "recording-service-history"
		sessionID   = "worker-service-history"
	)
	topic := events.Topic("worker-session/" + sessionID + "/events")
	persistWorkerRecordingRecords(t, writer, recordings.WorkerRecordingRecord{
		RecordingID:      recordingID,
		WorkerSessionID:  sessionID,
		FactorySessionID: "factory-service",
		WorkIDs:          []string{"work-service"},
		AttemptID:        "attempt-service",
	}, []events.Record{
		mustRecord(t, openingAppend(topic, sessionID), 1),
		mustRecord(t, terminalAppend(topic, sessionID), 2),
	})

	serviceValue, err := New(newRecordingEventsService(), writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := serviceValue.(*Service)
	if snapshot, err := service.LoadWorkerRecording(nil, recordingID); err != nil || len(snapshot.Sessions) != 1 {
		t.Fatalf("LoadWorkerRecording() = %#v, %v", snapshot, err)
	}
	if page, err := service.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 1}); err != nil || len(page.Projections) != 1 {
		t.Fatalf("ListWorkerRecordingProjections() = %#v, %v", page, err)
	}
	if snapshot, err := service.LoadWorkerRecordingByWorkerSessionID(context.Background(), sessionID); err != nil || len(snapshot.Sessions) != 1 {
		t.Fatalf("LoadWorkerRecordingByWorkerSessionID() = %#v, %v", snapshot, err)
	}
}

func TestFileWriterV2RecordWithoutWorkerIDUsesTopicIdentity(t *testing.T) {
	const (
		recordingID = "recording-topic-identity"
		sessionID   = "worker-topic-identity"
	)
	root := t.TempDir()
	writer, storage := newDirectoryReaderFileWriter(t, root)
	topic := events.Topic("worker-session/" + sessionID + "/events")
	header, err := json.Marshal(workerRecordingV2Header{
		SchemaVersion:   workerRecordingV2SchemaVersion,
		Kind:            "header",
		RecordingID:     recordingID,
		WorkerSessionID: sessionID,
		Topic:           topic,
		Status:          recordings.WorkerRecordingStatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(workerRecordingV2Record{
		SchemaVersion: workerRecordingV2SchemaVersion,
		Kind:          "record",
		Record:        mustRecord(t, openingAppend(topic, sessionID), 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	data := append(header, '\n')
	data = append(data, body...)
	data = append(data, '\n')
	if err := storage.WriteFile(writer.v2Path(recordingID), data); err != nil {
		t.Fatal(err)
	}

	reopened := reopenDirectoryReaderFileWriter(t, storage, root)
	snapshot, err := reopened.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("LoadWorkerRecording() error = %v", err)
	}
	if len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Records) != 1 || snapshot.Sessions[0].WorkerSessionID != sessionID {
		t.Fatalf("topic-derived Worker Session = %#v, want one record under %q", snapshot, sessionID)
	}
}

func TestFileWriterCatalogReportsUnsupportedAndUnreadableArtifacts(t *testing.T) {
	root := t.TempDir()
	writer, storage := newDirectoryReaderFileWriter(t, root)
	if err := storage.WriteFile(writer.v2Path("unsupported"), []byte(`{"schemaVersion":"worker-session-recording.future","kind":"header"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	if err := storage.WriteFile(writer.path("unreadable"), []byte("{not-json\n")); err != nil {
		t.Fatal(err)
	}
	reader := reopenDirectoryReaderFileWriter(t, storage, root)
	page, err := reader.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 10})
	if err != nil {
		t.Fatalf("ListWorkerRecordingProjections() error = %v", err)
	}
	if len(page.Projections) != 0 || len(page.Diagnostics) != 2 {
		t.Fatalf("catalog page = %#v, want two item diagnostics", page)
	}
	seenUnsupported, seenUnreadable := false, false
	for _, diagnostic := range page.Diagnostics {
		seenUnsupported = seenUnsupported || diagnostic.Code == recordings.WorkerRecordingCatalogUnsupported && diagnostic.Message != ""
		seenUnreadable = seenUnreadable || diagnostic.Code == recordings.WorkerRecordingCatalogUnreadable && diagnostic.Message != ""
	}
	if !seenUnsupported || !seenUnreadable {
		t.Fatalf("catalog diagnostics = %#v, want unsupported and unreadable", page.Diagnostics)
	}
}

func TestFileWriterLegacyHealthAndPendingErrorHelpers(t *testing.T) {
	if got := healthReasonFromSession(recordings.WorkerSessionRecordingSnapshot{Status: recordings.WorkerRecordingStatusDegraded, Failure: "PERSISTENCE_FAILED"}); got != "PERSISTENCE_FAILED" {
		t.Fatalf("healthReasonFromSession(degraded) = %q", got)
	}
	if got := healthReasonFromSession(recordings.WorkerSessionRecordingSnapshot{Status: recordings.WorkerRecordingStatusIncomplete, InterruptionReason: "PROCESS_INTERRUPTED"}); got != "PROCESS_INTERRUPTED" {
		t.Fatalf("healthReasonFromSession(incomplete) = %q", got)
	}
	topic := events.Topic("worker-session/helper/events")
	sessions := []recordings.WorkerSessionRecordingSnapshot{{WorkerSessionID: "preferred", Topic: topic}}
	if got := findSessionByTopic(sessions, topic, "preferred"); got != "preferred" {
		t.Fatalf("findSessionByTopic(preferred) = %q", got)
	}
	if got := findSessionByTopic(sessions, topic, ""); got != "preferred" {
		t.Fatalf("findSessionByTopic(topic) = %q", got)
	}

	capture := &capture{failure: make(chan struct{}), stop: func() {}, logger: logging.NoopLogger{}}
	capture.classifyPendingRecordsError(context.Background(), errors.New("catalog unavailable"))
	if !errors.Is(capture.failureError(), recordings.ErrWorkerRecordingDelivery) {
		t.Fatalf("classifyPendingRecordsError() = %v, want delivery error", capture.failureError())
	}
}
