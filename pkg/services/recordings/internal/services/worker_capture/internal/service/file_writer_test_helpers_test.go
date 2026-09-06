package service

// Shared helpers for the owner-local Worker recording tests.

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"strings"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func newDirectoryReaderFileWriter(t *testing.T, root string) (*FileWriter, platformreplay.Storage) {
	t.Helper()
	storage := platformreplay.NewLocal(runtime.GOOS)
	return newDirectoryReaderFileWriterWithStorage(t, storage, root)
}

func newDirectoryReaderFileWriterWithStorage(t *testing.T, storage platformreplay.Storage, root string) (*FileWriter, platformreplay.Storage) {
	t.Helper()
	writer, err := NewFileWriterWithDirectoryReader(storage, root, os.ReadDir)
	if err != nil {
		t.Fatal(err)
	}
	return writer.(*FileWriter), storage
}

func reopenDirectoryReaderFileWriter(t *testing.T, storage platformreplay.Storage, root string) *FileWriter {
	t.Helper()
	writer, _ := newDirectoryReaderFileWriterWithStorage(t, storage, root)
	return writer
}

func persistWorkerRecordingRecords(
	t *testing.T,
	writer recordings.WorkerRecordingWriter,
	metadata recordings.WorkerRecordingRecord,
	records []events.Record,
) {
	t.Helper()
	for _, record := range records {
		metadata.Record = record
		if err := writer.PersistWorkerRecord(context.Background(), metadata); err != nil {
			t.Fatalf("PersistWorkerRecord(%d): %v", record.ID.Position, err)
		}
	}
}

func assertV2Artifact(t *testing.T, storage platformreplay.Storage, writer *FileWriter, recordingID string) {
	t.Helper()
	data, err := storage.ReadFile(writer.v2Path(recordingID))
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
	if _, err := storage.ReadFile(writer.path(recordingID)); err == nil {
		t.Fatal("new Worker recording unexpectedly wrote a v1 snapshot")
	}
}

func assertCompleteWorkerRecordingCatalog(t *testing.T, reader recordings.WorkerRecordingHistoryReader, sessionID string) {
	t.Helper()
	snapshot, err := reader.LoadWorkerRecordingByWorkerSessionID(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("LoadWorkerRecordingByWorkerSessionID(): %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].WorkerSessionID != sessionID ||
		snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusComplete ||
		len(snapshot.Sessions[0].Records) != 3 {
		t.Fatalf("Worker-ID snapshot = %#v, want complete three-record history", snapshot)
	}
	page, err := reader.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 1})
	if err != nil {
		t.Fatalf("ListWorkerRecordingProjections(): %v", err)
	}
	if len(page.Projections) != 1 || page.NextToken != "" || len(page.Diagnostics) != 0 ||
		page.Projections[0].WorkerSessionID != sessionID || page.Projections[0].FactorySessionID != "factory-v2" ||
		!containsString(page.Projections[0].WorkIDs, "work-v2") {
		t.Fatalf("catalog page = %#v, want one complete Worker-ID projection without diagnostics", page)
	}
}

func assertMalformedTailRecording(t *testing.T, reader recordings.WorkerRecordingReader, recordingID string) {
	t.Helper()
	snapshot, err := reader.LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatalf("read valid prefix: %v", err)
	}
	if len(snapshot.Sessions) != 1 || snapshot.Sessions[0].Status != recordings.WorkerRecordingStatusDegraded ||
		snapshot.Sessions[0].Failure != "MALFORMED_TAIL" || len(snapshot.Sessions[0].Records) != 2 {
		t.Fatalf("malformed-tail snapshot = %#v, want degraded valid prefix", snapshot)
	}
}

func assertMalformedTailCatalog(t *testing.T, reader recordings.WorkerRecordingHistoryReader, recordingID string) {
	t.Helper()
	page, err := reader.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 1})
	if err != nil {
		t.Fatalf("catalog malformed tail: %v", err)
	}
	if len(page.Projections) != 1 || page.Projections[0].Status != recordings.WorkerRecordingStatusDegraded || len(page.Diagnostics) != 1 ||
		page.Diagnostics[0].Code != recordings.WorkerRecordingCatalogMalformedTail || page.Diagnostics[0].RecordingID != recordingID {
		t.Fatalf("malformed-tail catalog = %#v, want degraded projection plus MALFORMED_TAIL diagnostic", page)
	}
}
