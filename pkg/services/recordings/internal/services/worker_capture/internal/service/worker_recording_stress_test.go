package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const workerRecordingV2StressRecordCount = 10_000

type workerRecordingV2StressCase struct {
	recordingID      string
	workerSessionID  string
	factorySessionID string
	workID           string
	attemptID        string
	schemaVersion    string
	topic            events.Topic
	root             string
	storage          platformreplay.Storage
}

type workerRecordingV2StressHeader struct {
	SchemaVersion    string                           `json:"schemaVersion"`
	Kind             string                           `json:"kind"`
	RecordingID      string                           `json:"recordingId"`
	WorkerSessionID  string                           `json:"workerSessionId"`
	FactorySessionID string                           `json:"factorySessionId"`
	Topic            events.Topic                     `json:"topic"`
	WorkIDs          []string                         `json:"workIds"`
	AttemptID        string                           `json:"attemptId"`
	Status           recordings.WorkerRecordingStatus `json:"status"`
}

type workerRecordingV2StressRecord struct {
	SchemaVersion   string        `json:"schemaVersion"`
	Kind            string        `json:"kind"`
	WorkerSessionID string        `json:"workerSessionId"`
	Record          events.Record `json:"record"`
}

type workerRecordingV2StressHealth struct {
	SchemaVersion   string                           `json:"schemaVersion"`
	Kind            string                           `json:"kind"`
	WorkerSessionID string                           `json:"workerSessionId"`
	Status          recordings.WorkerRecordingStatus `json:"status"`
	LastPosition    events.AggregateSequence         `json:"lastPosition"`
}

// TestWorkerRecordingV2TenThousandRecords exercises the bounded v2 reader and
// Worker-ID catalog over a realistic retained history without starting a
// factory or provider process. The dedicated Make target runs this test
// outside the short stress suite.
func TestWorkerRecordingV2TenThousandRecords(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10k Worker recording stress test in short mode")
	}

	fixture := workerRecordingV2StressCase{
		recordingID:      "recording-v2-10k",
		workerSessionID:  "worker-v2-10k",
		factorySessionID: "factory-v2-10k",
		workID:           "work-v2-10k",
		attemptID:        "attempt-v2-10k",
		schemaVersion:    workerRecordingV2SchemaVersion,
		topic:            events.Topic("worker-session/worker-v2-10k/events"),
		root:             t.TempDir(),
		storage:          platformreplay.NewLocal(runtime.GOOS),
	}
	writeWorkerRecordingV2StressArtifact(t, fixture)
	historyReader := openWorkerRecordingV2StressHistory(t, fixture)
	assertWorkerRecordingV2StressHistory(t, historyReader, fixture)
}

func writeWorkerRecordingV2StressArtifact(t *testing.T, fixture workerRecordingV2StressCase) {
	t.Helper()
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	if err := encoder.Encode(workerRecordingV2StressHeader{
		SchemaVersion:    fixture.schemaVersion,
		Kind:             "header",
		RecordingID:      fixture.recordingID,
		WorkerSessionID:  fixture.workerSessionID,
		FactorySessionID: fixture.factorySessionID,
		Topic:            fixture.topic,
		WorkIDs:          []string{fixture.workID},
		AttemptID:        fixture.attemptID,
		Status:           recordings.WorkerRecordingStatusActive,
	}); err != nil {
		t.Fatalf("encode v2 header: %v", err)
	}
	for position := 1; position <= workerRecordingV2StressRecordCount; position++ {
		record := workerRecordingV2StressRecord{
			SchemaVersion:   fixture.schemaVersion,
			Kind:            "record",
			WorkerSessionID: fixture.workerSessionID,
			Record:          workerRecordingV2StressRecordAt(fixture.topic, fixture.workerSessionID, position),
		}
		if err := encoder.Encode(record); err != nil {
			t.Fatalf("encode v2 record %d: %v", position, err)
		}
	}
	if err := encoder.Encode(workerRecordingV2StressHealth{
		SchemaVersion:   fixture.schemaVersion,
		Kind:            "health",
		WorkerSessionID: fixture.workerSessionID,
		Status:          recordings.WorkerRecordingStatusComplete,
		LastPosition:    workerRecordingV2StressRecordCount,
	}); err != nil {
		t.Fatalf("encode v2 health: %v", err)
	}
	artifact := filepath.Join(fixture.root, workerRecordingV2StressPath(fixture.recordingID))
	if err := fixture.storage.WriteFile(artifact, data.Bytes()); err != nil {
		t.Fatalf("write v2 artifact: %v", err)
	}
}

func openWorkerRecordingV2StressHistory(t *testing.T, fixture workerRecordingV2StressCase) recordings.WorkerRecordingHistoryReader {
	t.Helper()
	writer, err := NewFileWriterWithDirectoryReader(fixture.storage, fixture.root, os.ReadDir)
	if err != nil {
		t.Fatalf("construct Worker recording writer: %v", err)
	}
	historyReader, ok := writer.(recordings.WorkerRecordingHistoryReader)
	if !ok {
		t.Fatal("Worker recording writer does not expose history reads")
	}
	return historyReader
}

func assertWorkerRecordingV2StressHistory(
	t *testing.T,
	historyReader recordings.WorkerRecordingHistoryReader,
	fixture workerRecordingV2StressCase,
) {
	t.Helper()
	snapshot, err := historyReader.LoadWorkerRecordingByWorkerSessionID(context.Background(), fixture.workerSessionID)
	if err != nil {
		t.Fatalf("load 10k Worker-ID history: %v", err)
	}
	if len(snapshot.Sessions) != 1 {
		t.Fatalf("loaded sessions = %d, want one", len(snapshot.Sessions))
	}
	session := snapshot.Sessions[0]
	if session.WorkerSessionID != fixture.workerSessionID || session.Status != recordings.WorkerRecordingStatusComplete ||
		len(session.Records) != workerRecordingV2StressRecordCount || session.LastPosition != workerRecordingV2StressRecordCount {
		t.Fatalf("loaded 10k session = id=%q status=%q records=%d last=%d", session.WorkerSessionID, session.Status, len(session.Records), session.LastPosition)
	}
	assertWorkerRecordingV2StressPositions(t, session.Records)

	page, err := historyReader.ListWorkerRecordingProjections(context.Background(), recordings.WorkerRecordingListRequest{MaxResults: 1})
	if err != nil {
		t.Fatalf("catalog 10k Worker-ID history: %v", err)
	}
	if len(page.Projections) != 1 || page.Projections[0].WorkerSessionID != fixture.workerSessionID ||
		page.Projections[0].Status != recordings.WorkerRecordingStatusComplete || len(page.Diagnostics) != 0 {
		t.Fatalf("catalog projection = %#v, want one complete projection without diagnostics", page)
	}
}

func assertWorkerRecordingV2StressPositions(t *testing.T, records []events.Record) {
	t.Helper()
	for index, record := range records {
		wantPosition := events.AggregateSequence(index + 1)
		if record.ID.Position != wantPosition {
			t.Fatalf("record %d position = %d, want %d", index+1, record.ID.Position, wantPosition)
		}
	}
}

func workerRecordingV2StressPath(recordingID string) string {
	digest := sha256.Sum256([]byte(recordingID))
	return filepath.Join(hex.EncodeToString(digest[:]) + ".worker.jsonl")
}

func workerRecordingV2StressRecordAt(topic events.Topic, workerSessionID string, position int) events.Record {
	var draft workers.Draft
	sourceType := events.SourceType("worker_session_lifecycle")
	sourceID := events.SourceID(workerSessionID)
	sourceSequence := events.SourceSequence(position)
	eventID := events.SourceEventID(fmt.Sprintf("event-%05d", position))
	if position == 1 {
		eventID = "started"
		payload, _ := json.Marshal(workers.SessionPayload{
			Status:           "STARTING",
			WorkerSessionID:  workerSessionID,
			FactorySessionID: "factory-v2-10k",
			WorkIDs:          []string{"work-v2-10k"},
			AttemptID:        "attempt-v2-10k",
		})
		draft = workers.Draft{
			Kind:       workers.KindSession,
			Phase:      workers.PhaseStarted,
			Provenance: workerRecordingV2StressProvenance("worker_session_lifecycle"),
			Payload:    payload,
		}
	} else if position == workerRecordingV2StressRecordCount {
		sourceType = "worker_session_lifecycle"
		sourceID = events.SourceID(workerSessionID)
		sourceSequence = 2
		eventID = "terminal"
		payload, _ := json.Marshal(map[string]string{"status": "COMPLETED"})
		draft = workers.Draft{
			Kind:       workers.KindSession,
			Phase:      workers.PhaseCompleted,
			Provenance: workerRecordingV2StressProvenance("worker_session_lifecycle"),
			Payload:    payload,
		}
	} else {
		payload, _ := json.Marshal(workers.MessagePayload{
			Role: "assistant",
			ContentBlocks: []workers.ContentBlock{{
				Kind: workers.ContentBlockText,
				Text: fmt.Sprintf("event-%05d", position),
			}},
		})
		sourceType = "worker_provider"
		sourceID = events.SourceID(workerSessionID + "/provider")
		draft = workers.Draft{
			Kind:       workers.KindMessage,
			Phase:      workers.PhaseCompleted,
			Provenance: workerRecordingV2StressProvenance("message.completed"),
			Payload:    payload,
			DispatchID: workerSessionID,
		}
	}
	payload, err := json.Marshal(draft)
	if err != nil {
		panic(fmt.Sprintf("encode Worker draft %d: %v", position, err))
	}
	return events.Record{
		ID:             events.RecordID{Topic: topic, Position: events.AggregateSequence(position)},
		SourceType:     sourceType,
		SourceID:       sourceID,
		SourceSequence: sourceSequence,
		SourceEventID:  eventID,
		SchemaID:       "workers.draft.v1",
		Payload:        payload,
	}
}

func workerRecordingV2StressProvenance(nativeEventType string) workers.Provenance {
	return workers.Provenance{
		Delivery:        workers.DeliverySynthesized,
		Fidelity:        workers.FidelityLifecycleOnly,
		NativeEventType: nativeEventType,
		Representation:  workers.RepresentationNotification,
	}
}
