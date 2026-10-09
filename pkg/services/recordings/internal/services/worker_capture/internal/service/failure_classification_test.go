package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestSelectedWorkerRecordingHealthIsPreparedDetachedAndFresh(t *testing.T) {
	t.Parallel()
	probe := &journalProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	for _, id := range []string{"selected", "sibling", "foreign"} {
		recordingID := "owned"
		if id == "foreign" {
			recordingID = "other"
		}
		if err := writer.PersistWorkerRecord(t.Context(), journalRecord(t, recordingID, id)); err != nil {
			t.Fatal(err)
		}
	}
	store, err := NewFileWriter(probe, probe, probe, &captureTimeProbe{}, writer.root, "historical", nil)
	if err != nil {
		t.Fatal(err)
	}
	restored := store.(*FileWriter)
	if _, err := restored.CurrentWorkerRecordingHealth(t.Context(), "owned", nil); !errors.Is(err, recordings.ErrMissingWorkerRecordingReader) {
		t.Fatalf("unprepared health = %v", err)
	}
	if err := restored.RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	probe.mu.Lock()
	before := probe.reads
	probe.mu.Unlock()
	read := func() recordings.WorkerRecordingSnapshot {
		t.Helper()
		return requireSelectedWorkerHealth(t, restored)
	}
	first := read()
	first.Sessions[0].Records[0].Payload[0] = '!'
	assertConcurrentSelectedHealth(t, restored)
	record := journalRecord(t, "owned", "selected")
	record.Record = mustRecord(t, terminalAppend(record.Record.ID.Topic, "selected"), 2)
	if err := restored.PersistWorkerRecord(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if got := read().Sessions[0].Status; got != recordings.WorkerRecordingStatusComplete {
		t.Fatalf("committed health = %s", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := restored.CurrentWorkerRecordingHealth(ctx, "owned", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled health = %v", err)
	}
	if empty, err := restored.CurrentWorkerRecordingHealth(t.Context(), "owned", nil); err != nil || len(empty.Sessions) != 0 {
		t.Fatalf("empty selected health = %+v, %v", empty, err)
	}
	if _, err := restored.CurrentWorkerRecordingHealth(t.Context(), "absent", nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("absent prepared recording = %v", err)
	}
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if probe.reads != before {
		t.Fatalf("selected health read files: %d -> %d", before, probe.reads)
	}
}

func requireSelectedWorkerHealth(t *testing.T, reader recordings.WorkerRecordingHealthReader) recordings.WorkerRecordingSnapshot {
	t.Helper()
	result, err := reader.CurrentWorkerRecordingHealth(t.Context(), "owned", []string{"selected", "selected", "foreign", "absent"})
	if err != nil || len(result.Sessions) != 1 || result.Sessions[0].WorkerSessionID != "selected" || len(result.Sessions[0].Records) != 1 {
		t.Fatalf("selected health = %+v, %v", result, err)
	}
	return result
}

func assertConcurrentSelectedHealth(t *testing.T, reader recordings.WorkerRecordingHealthReader) {
	t.Helper()
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			result, err := reader.CurrentWorkerRecordingHealth(t.Context(), "owned", []string{"selected"})
			if err != nil || len(result.Sessions) != 1 || result.Sessions[0].Records[0].Payload[0] == '!' {
				t.Errorf("detached concurrent health = %+v, %v", result, err)
			}
		})
	}
	readers.Wait()
}

type selectedHealthStorageFault struct {
	platformreplay.Local
	unavailable string
}

func (fault *selectedHealthStorageFault) ReadFile(path string) ([]byte, error) {
	if path == fault.unavailable {
		return nil, errors.New("private-path-secret-sentinel")
	}
	return fault.Local.ReadFile(path)
}

func TestSelectedWorkerRecordingHealthRetainsPreparationErrors(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"corrupt", "invalid-opening", "unavailable", "torn"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			fault := &selectedHealthStorageFault{Local: platformreplay.NewLocal(runtime.GOOS)}
			writer := journalWriter(t, fault)
			record := journalRecord(t, "damaged", "worker")
			if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			path := writer.path("damaged") + "l"
			switch kind {
			case "corrupt":
				if err := fault.AppendFile(path, []byte("private-path-secret-sentinel\n")); err != nil {
					t.Fatal(err)
				}
			case "invalid-opening":
				if err := fault.WriteFile(path, []byte("private-path-secret-sentinel\n")); err != nil {
					t.Fatal(err)
				}
			case "unavailable":
				fault.unavailable = path
			case "torn":
				if err := fault.AppendFile(path, []byte("uncommitted-tail")); err != nil {
					t.Fatal(err)
				}
			}
			store, err := NewFileWriter(fault, fault, fault, &captureTimeProbe{}, writer.root, "historical", nil)
			if err != nil {
				t.Fatal(err)
			}
			restored := store.(*FileWriter)
			if err := restored.RecoverWorkerOwners(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := restored.CurrentWorkerRecordingHealth(t.Context(), "unrelated", nil); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unrelated recording inherited damage: %v", err)
			}
			assertSelectedPreparationError(t, restored, kind)
			assertSelectedHealthRepair(t, restored, fault, kind)
		})
	}
}

func assertSelectedHealthRepair(t *testing.T, restored *FileWriter, fault *selectedHealthStorageFault, kind string) {
	t.Helper()
	if kind != "unavailable" {
		return
	}
	fault.unavailable = ""
	// A deliberate recording read may repair availability. The ordinary
	// health read itself never retries the failed source.
	if _, err := restored.LoadWorkerRecording(t.Context(), "damaged"); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.CurrentWorkerRecordingHealth(t.Context(), "damaged", []string{"worker"}); err != nil {
		t.Fatal(err)
	}
}

func assertSelectedPreparationError(t *testing.T, reader recordings.WorkerRecordingHealthReader, kind string) {
	t.Helper()
	for _, ids := range [][]string{nil, {"worker"}} {
		result, err := reader.CurrentWorkerRecordingHealth(t.Context(), "damaged", ids)
		if kind == "torn" {
			if err != nil || len(result.Sessions) != len(ids) || (len(ids) > 0 && result.Sessions[0].Status != recordings.WorkerRecordingStatusIncomplete) {
				t.Fatalf("torn committed prefix = %+v, %v", result, err)
			}
			continue
		}
		want := recordings.ErrWorkerRecordingReplay
		if kind == "unavailable" {
			want = recordings.ErrMissingWorkerRecordingReader
		}
		if !errors.Is(err, want) || strings.Contains(err.Error(), "sentinel") || len(result.Sessions) != 0 {
			t.Fatalf("prepared %s health = %+v, %v; want safe %v", kind, result, err, want)
		}
	}
}

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
			reopened, err := newTestFileWriter(local, writer.root)
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

	for _, fields := range logger.fields {
		if strings.Contains(fmt.Sprint(fields), "secret") {
			t.Fatalf("capture diagnostic leaked raw failure detail: %#v", fields)
		}
	}

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

func TestFileWriterRetainsScopedTopicAcrossReload(t *testing.T) {
	t.Parallel()
	const recordingID = "scoped-recording"
	const workerID = "recorded-worker"
	const topic events.Topic = "factory-worker-session/ZmFjdG9yeS1zZXNzaW9u/cmVjb3JkZWQtd29ya2Vy/events"
	root := t.TempDir()
	storage := platformreplay.NewLocal(runtime.GOOS)
	writer, err := NewFileWriter(storage, storage, storage, &captureTimeProbe{}, root, "scoped-owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	opening := mustRecord(t, openingAppend(topic, workerID), 1)
	terminal := mustRecord(t, terminalAppend(topic, workerID), 2)
	persistWorkerRecoveryPrefix(t, writer, recordingID, workerID, opening, terminal)
	reloaded, err := NewFileWriter(storage, storage, storage, &captureTimeProbe{}, root, "reloaded-owner", nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reloaded.(recordings.WorkerRecordingReader).LoadWorkerRecording(context.Background(), recordingID)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := (recordings.WorkerRecordingCodec{}).ReplayWorkerRecording(recordings.WorkerRecordingReplayRequest{Snapshot: snapshot})
	if err != nil || replay.Projection.Topic != topic || replay.Projection.WorkerSessionID != workerID || !replay.Projection.Complete {
		t.Fatalf("scoped replay = %+v, %v", replay, err)
	}
	codec := recordings.WorkerRecordingCodec{}
	portable, err := codec.BuildWorkerPortableRecording(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	portableReplay, err := codec.ReplayWorkerPortableRecording(portable)
	if err != nil || portableReplay.Projection.Topic != topic || portableReplay.Projection.WorkerSessionID != workerID {
		t.Fatalf("portable scoped replay = %+v, %v", portableReplay, err)
	}
	foreign := mustRecord(t, terminalAppend("factory-worker-session/b3RoZXItc2Vzc2lvbg/cmVjb3JkZWQtd29ya2Vy/events", workerID), 3)
	foreign.SourceEventID = "foreign-terminal"
	err = reloaded.PersistWorkerRecord(context.Background(), recordings.WorkerRecordingRecord{RecordingID: recordingID, WorkerSessionID: workerID, Record: foreign})
	if !errors.Is(err, recordings.ErrWorkerRecordingOrder) {
		t.Fatalf("foreign topic append = %v", err)
	}
	after, err := reloaded.(recordings.WorkerRecordingReader).LoadWorkerRecording(context.Background(), recordingID)
	if err != nil || !reflect.DeepEqual(snapshot, after) {
		t.Fatalf("foreign topic changed retained recording: %v", err)
	}
}

func TestFileWriterSelectedSummaryNeverReadsHistory(t *testing.T) {
	t.Parallel()
	probe := &catalogReadProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	opening := journalRecord(t, "summary-denied", "selected")
	if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
		t.Fatal(err)
	}
	usage := opening
	usage.Record = catalogMetadataRecord(t, opening.Record.ID.Topic, workers.KindUsage, `{"inputTokens":0,"totalTokens":12}`, 2)
	if err := writer.PersistWorkerRecord(t.Context(), usage); err != nil {
		t.Fatal(err)
	}
	terminal := opening
	terminal.Record = mustRecord(t, terminalAppend(opening.Record.ID.Topic, "selected"), 3)
	if err := writer.PersistWorkerRecord(t.Context(), terminal); err != nil {
		t.Fatal(err)
	}
	reopened, err := newTestFileWriter(probe, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	reader := reopened.(*FileWriter)
	if _, err := reader.LookupWorkerSessionSummary(t.Context(), "selected"); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
		t.Fatalf("unprepared lookup=%v", err)
	}
	if err := reader.RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	probe.fault = errors.New("recording reads denied")
	for i := 0; i < 3; i++ {
		got, err := reader.LookupWorkerSessionSummary(t.Context(), "selected")
		if err != nil || got.Capture.Catalog.CommittedPosition != 3 || len(got.Capture.MetadataRecords) != 2 || got.Capture.Terminal.Status != "COMPLETED" {
			t.Fatalf("summary=%+v err=%v", got, err)
		}
		got.Capture.Opening.Payload[0] = '!'
		got.Capture.MetadataRecords[0].Payload[0] = '!'
		got.Capture.CapturedAt["1"] = time.Time{}
	}
	if _, err := reader.LookupWorkerSessionSummary(t.Context(), "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing lookup=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := reader.LookupWorkerSessionSummary(ctx, "selected"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled lookup=%v", err)
	}
}

func TestFileWriterSelectedSummaryPreservesTornCommittedPrefix(t *testing.T) {
	t.Parallel()
	probe := &catalogReadProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
	writer := journalWriter(t, probe)
	opening := journalRecord(t, "torn-summary", "selected")
	if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
		t.Fatal(err)
	}
	if err := probe.AppendFile(writer.path(opening.RecordingID)+"l", []byte(`{"uncommitted":`)); err != nil {
		t.Fatal(err)
	}
	reopened, err := newTestFileWriter(probe, writer.root)
	if err != nil {
		t.Fatal(err)
	}
	reader := reopened.(*FileWriter)
	if err := reader.RecoverWorkerOwners(t.Context()); err != nil {
		t.Fatal(err)
	}
	probe.fault = errors.New("recording reads denied")
	got, err := reader.LookupWorkerSessionSummary(t.Context(), "selected")
	if err != nil || got.Capture.Catalog.CommittedPosition != 1 || got.Capture.Health != recordings.WorkerRecordingStatusIncomplete || got.Capture.Terminal != nil || !got.Capture.OwnerLost {
		t.Fatalf("torn committed summary=%+v err=%v", got, err)
	}
}

func TestFileWriterSelectedSummaryRejectsDamagedAndAmbiguous(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"damaged", "ambiguous", "unreadable"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			probe := &catalogReadProbe{Local: platformreplay.NewLocal(runtime.GOOS)}
			writer := journalWriter(t, probe)
			opening := journalRecord(t, "bad-summary", "bad")
			if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
				t.Fatal(err)
			}
			if kind == "damaged" {
				if err := probe.AppendFile(writer.path(opening.RecordingID)+"l", []byte("damaged\n")); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "ambiguous" {
				opening.RecordingID = "collision"
				if err := writer.PersistWorkerRecord(t.Context(), opening); err != nil {
					t.Fatal(err)
				}
			}
			reopened, err := newTestFileWriter(probe, writer.root)
			if err != nil {
				t.Fatal(err)
			}
			reader := reopened.(*FileWriter)
			if kind == "unreadable" {
				probe.fault = errors.New("private path")
			}
			if err := reader.RecoverWorkerOwners(t.Context()); err != nil {
				t.Fatal(err)
			}
			probe.fault = errors.New("recording reads denied")
			if _, err := reader.LookupWorkerSessionSummary(t.Context(), "bad"); !errors.Is(err, recordings.ErrWorkerRecordingReplay) {
				t.Fatalf("unsafe %s summary=%v", kind, err)
			}
		})
	}
}
