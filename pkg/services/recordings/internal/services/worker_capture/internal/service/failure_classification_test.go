package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFileWriterLegacyContinuationTerminalReplays(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"complete", "degraded-recorded", "degraded-authoritative"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			writer := journalWriter(t, local)
			if err := local.WriteFile(writer.path("legacy"), []byte(legacyWorkerOpeningFixture)); err != nil {
				t.Fatal(err)
			}
			baseline, err := writer.LoadWorkerRecording(t.Context(), "legacy")
			if err != nil || baseline.Sessions[0].InterruptionReason != recordings.WorkerRecordingInterruptionProcessStopped {
				t.Fatalf("legacy interruption = %#v, error %v", baseline, err)
			}
			want := persistLegacyContinuationTerminal(t, writer, kind)
			reopened, err := NewFileWriter(local, writer.root)
			if err != nil {
				t.Fatal(err)
			}
			for _, reader := range []recordings.WorkerRecordingReader{writer, reopened.(recordings.WorkerRecordingReader)} {
				assertLegacyTerminalReplay(t, reader, want)
			}
			unchanged, err := local.ReadFile(writer.path("legacy"))
			if err != nil || !bytes.Equal(unchanged, []byte(legacyWorkerOpeningFixture)) {
				t.Fatal("legacy baseline changed")
			}
		})
	}
}

func persistLegacyContinuationTerminal(t *testing.T, writer *FileWriter, kind string) recordings.WorkerRecordingStatus {
	t.Helper()
	record := journalRecord(t, "legacy", "legacy-session")
	want := recordings.WorkerRecordingStatusComplete
	if kind != "complete" {
		want = recordings.WorkerRecordingStatusDegraded
		failure := recordings.WorkerRecordingFailure{RecordingID: record.RecordingID, WorkerSessionID: record.WorkerSessionID, Topic: record.Record.ID.Topic, Code: "PERSISTENCE_FAILED"}
		if kind == "degraded-authoritative" {
			failure.ExecutionTerminal = &recordings.WorkerRecordingTerminal{Position: 2, Phase: workers.PhaseCompleted, Status: "COMPLETED"}
		}
		if err := writer.PersistWorkerRecordingFailure(t.Context(), failure); err != nil {
			t.Fatal(err)
		}
	}
	if kind != "degraded-authoritative" {
		record.Record = mustRecord(t, terminalAppend(record.Record.ID.Topic, record.WorkerSessionID), 2)
		if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
			t.Fatal(err)
		}
	}
	return want
}

func assertLegacyTerminalReplay(t *testing.T, reader recordings.WorkerRecordingReader, want recordings.WorkerRecordingStatus) {
	t.Helper()
	snapshot, err := reader.LoadWorkerRecording(t.Context(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	session := snapshot.Sessions[0]
	if session.Status != want || session.InterruptionReason != "" || session.ExecutionTerminal == nil {
		t.Fatalf("terminal continuation = %#v", session)
	}
	replayed, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot, WorkerSessionID: session.WorkerSessionID})
	if err != nil || replayed.Projection.Status != want || !reflect.DeepEqual(replayed.Projection.Records, session.Records) || !reflect.DeepEqual(replayed.Projection.ExecutionTerminal, session.ExecutionTerminal) {
		t.Fatalf("supported replay = %#v, error %v", replayed, err)
	}
}

type failureClassificationWriter struct {
	failure recordings.WorkerRecordingFailure
}

func (writer *failureClassificationWriter) PersistWorkerRecord(context.Context, recordings.WorkerRecordingRecord) error {
	return nil
}

func (writer *failureClassificationWriter) PersistWorkerRecordingFailure(
	_ context.Context,
	failure recordings.WorkerRecordingFailure,
) error {
	writer.failure = failure
	return nil
}

func TestWorkerRecordingFailurePersistsStableClassification(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "opening", err: recordings.ErrWorkerRecordingOpening, code: "OPENING_INVALID"},
		{name: "persistence", err: recordings.ErrWorkerRecordingPersistence, code: "PERSISTENCE_FAILED"},
		{name: "gap", err: recordings.ErrWorkerRecordingGap, code: "RETENTION_GAP"},
		{name: "closed", err: recordings.ErrWorkerRecordingClosed, code: "SOURCE_CLOSED"},
		{name: "canceled", err: recordings.ErrWorkerRecordingCanceled, code: "CANCELED"},
		{name: "backpressure", err: recordings.ErrWorkerRecordingBackpressure, code: "BACKPRESSURE"},
		{name: "terminal", err: recordings.ErrWorkerRecordingTerminal, code: "TERMINAL_INVALID"},
		{name: "incomplete", err: recordings.ErrWorkerRecordingIncomplete, code: "INCOMPLETE"},
		{name: "duplicate", err: recordings.ErrWorkerRecordingDuplicate, code: "DUPLICATE_CONFLICT"},
		{name: "order", err: recordings.ErrWorkerRecordingOrder, code: "ORDER_INVALID"},
		{name: "delivery", err: errors.New("unexpected delivery"), code: "DELIVERY_FAILED"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			writer := &failureClassificationWriter{}
			capture := &capture{
				request: recordings.WorkerSessionRecordingRequest{
					RecordingID:     "recording-classification",
					WorkerSessionID: "worker-classification",
				},
				writer:  writer,
				logger:  logging.NoopLogger{},
				failure: make(chan struct{}),
				stop:    func() {},
			}

			capture.fail(test.err)

			if !errors.Is(capture.failureError(), test.err) {
				t.Fatalf("failure = %v, want %v", capture.failureError(), test.err)
			}
			if writer.failure.Code != test.code {
				t.Fatalf("persisted failure code = %q, want %q", writer.failure.Code, test.code)
			}
			if writer.failure.RecordingID != "recording-classification" || writer.failure.WorkerSessionID != "worker-classification" {
				t.Fatalf("persisted failure identity = %#v, want recording and Worker Session identity", writer.failure)
			}
		})
	}
}

func TestWorkerRecordingFailureMarkerErrorUsesSafeStructuredDiagnostics(t *testing.T) {
	logger := &captureDiagnosticLogger{}
	capture := &capture{
		request: recordings.WorkerSessionRecordingRequest{
			RecordingID:     "recording-marker-failure",
			WorkerSessionID: "worker-marker-failure",
			Topic:           "worker-session/worker-marker-failure/events",
		},
		writer:  failingWorkerRecordingFailureWriter{},
		logger:  logger,
		failure: make(chan struct{}),
		stop:    func() {},
	}

	capture.fail(fmt.Errorf("%w: secret payload must not be logged", recordings.ErrWorkerRecordingPersistence))

	markerIndex := -1
	for index, message := range logger.messages {
		if message == "Worker recording failure persistence failed" {
			markerIndex = index
			break
		}
	}
	if markerIndex < 0 {
		t.Fatalf("diagnostic messages = %#v, want marker persistence message", logger.messages)
	}
	fields := logger.fields[markerIndex]
	if !containsDiagnosticPair(fields, "stage", "degradation_marker") || !containsDiagnosticPair(fields, "code", "PERSISTENCE_FAILED") {
		t.Fatalf("diagnostic fields = %#v, want safe stage and code", fields)
	}
	if strings.Contains(fmt.Sprint(fields), "secret payload") {
		t.Fatalf("diagnostic fields leaked raw failure detail: %#v", fields)
	}
}

type failingWorkerRecordingFailureWriter struct{}

func (failingWorkerRecordingFailureWriter) PersistWorkerRecord(context.Context, recordings.WorkerRecordingRecord) error {
	return nil
}

func (failingWorkerRecordingFailureWriter) PersistWorkerRecordingFailure(context.Context, recordings.WorkerRecordingFailure) error {
	return errors.New("secret marker-store detail")
}

type captureDiagnosticLogger struct {
	logging.NoopLogger
	messages []string
	fields   [][]any
}

func (logger *captureDiagnosticLogger) Info(message string, fields ...any) {
	logger.messages = append(logger.messages, message)
	logger.fields = append(logger.fields, append([]any(nil), fields...))
}

func containsDiagnosticPair(fields []any, key, want string) bool {
	for index := 0; index+1 < len(fields); index += 2 {
		if fields[index] == key && fields[index+1] == want {
			return true
		}
	}
	return false
}

func (logger *captureDiagnosticLogger) Warn(message string, fields ...any) {
	logger.Info(message, fields...)
}

const legacyWorkerOpeningFixture = `{"recordingId":"legacy","sessions":[{"workerSessionId":"legacy-session","records":[{"ID":{"Topic":"worker-session/legacy-session/events","Position":1},"SourceType":"worker_session_lifecycle","SourceID":"legacy-session","SourceSequence":1,"SourceEventID":"started","SchemaID":"workers.draft.v1","Payload":{"kind":"SESSION","phase":"STARTED","provenance":{"delivery":"SYNTHESIZED","fidelity":"LIFECYCLE_ONLY","nativeEventType":"worker_session_lifecycle","provider":"","representation":"NOTIFICATION"},"payload":{"status":"STARTING","workerSessionId":"legacy-session"}}}]}]}`
