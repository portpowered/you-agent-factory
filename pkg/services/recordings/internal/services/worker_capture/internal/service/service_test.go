package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkerCapturePersistsOpeningBeforeBarrierRelease(t *testing.T) {
	t.Parallel()
	eventService := newRecordingEventsService()

	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var writeOnce sync.Once
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseWrite) }) })
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
	service, err := New(eventService, writer, logging.NoopLogger{})
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID:         "recording-1",
		OriginatingArtifact: "selected-factory.jsonl",
		FactorySessionID:    "factory-session-1",
		WorkerSessionID:     "worker-1",
		Topic:               events.Topic("worker-session/worker-1/events"),
	}
	handle, err := service.StartWorkerSessionRecording(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	registerCaptureCleanup(t, handle)

	if _, err := eventService.Append(context.Background(), openingAppend(request.Topic, request.WorkerSessionID)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-writeEntered:
	case <-time.After(time.Second):
		t.Fatal("opening was not offered to the durable writer")
	}
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelWait()
	observer := &captureOpeningObserver{Context: waitCtx, ready: make(chan struct{})}
	waitResult := make(chan error, 1)
	go func() { waitResult <- handle.AwaitOpening(observer) }()
	select {
	case <-observer.ready:
	case <-waitCtx.Done():
		t.Fatal("opening observer did not enter its wait")
	}
	select {
	case err := <-waitResult:
		t.Fatalf("opening returned before durable acceptance: %v", err)
	default:
	}

	releaseOnce.Do(func() { close(releaseWrite) })
	if err := <-waitResult; err != nil {
		t.Fatal(err)
	}
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
	assertPersistedWorkerHistory(t, persisted, request.FactorySessionID)
	if persisted[0].OriginatingArtifact != request.OriginatingArtifact {
		t.Fatalf("opening capture lost originating artifact: %q", persisted[0].OriginatingArtifact)
	}
}

// Done signals that AwaitOpening evaluated its wait while persistence is
// blocked. The context deadline is a safety ceiling, not synchronization.
type captureOpeningObserver struct {
	context.Context
	ready chan struct{}
	once  sync.Once
}

func (observer *captureOpeningObserver) Done() <-chan struct{} {
	observer.once.Do(func() { close(observer.ready) })
	return observer.Context.Done()
}

func assertPersistedWorkerHistory(t *testing.T, persisted []recordings.WorkerRecordingRecord, factorySessionID string) {
	t.Helper()

	if len(persisted) != 2 || persisted[0].Record.ID.Position != 1 || persisted[1].Record.ID.Position != 2 {
		t.Fatalf("persisted Worker history = %#v, want positions 1 and 2", persisted)
	}
	for _, record := range persisted {
		if record.FactorySessionID != factorySessionID {
			t.Fatalf("persisted Factory Session ID = %q, want %q", record.FactorySessionID, factorySessionID)
		}
	}
}

func TestWorkerCaptureRejectsNonOpeningPositionBeforeBarrier(t *testing.T) {
	eventService := newRecordingEventsService()
	var writes int
	service, err := New(eventService, recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error {
		writes++
		return nil
	}), logging.NoopLogger{})
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
	}), logging.NoopLogger{})
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
	serviceValue, err := New(newRecordingEventsService(), writer, logging.NoopLogger{})
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
	writerOnly, err := New(newRecordingEventsService(), recordings.WorkerRecordingWriterFunc(func(context.Context, recordings.WorkerRecordingRecord) error { return nil }), logging.NoopLogger{})
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

// These witnesses isolate capture from Events and durable storage. Read is
// always at head; delivery and readiness are controlled independently.
type selectedCaptureSource struct {
	events.Service
	deliveries chan events.Delivery
	ready      chan struct{}
}

func (source *selectedCaptureSource) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return events.Subscription(func(ctx context.Context) events.Delivery {
		source.ready <- struct{}{}
		select {
		case delivery := <-source.deliveries:
			return delivery
		case <-ctx.Done():
			return events.Delivery{Kind: events.DeliveryCanceled}
		}
	}), nil
}

type selectedCaptureWriter struct {
	mu           sync.Mutex
	failPosition events.AggregateSequence
	markerError  bool
	records      []recordings.WorkerRecordingRecord
	failures     []recordings.WorkerRecordingFailure
}

func (writer *selectedCaptureWriter) PersistWorkerRecord(_ context.Context, record recordings.WorkerRecordingRecord) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if record.Record.ID.Position == writer.failPosition {
		return errors.New("private writer sentinel")
	}
	record.Record = record.Record.Detached()
	writer.records = append(writer.records, record)
	return nil
}

func (writer *selectedCaptureWriter) PersistWorkerRecordingFailure(_ context.Context, failure recordings.WorkerRecordingFailure) error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	failure.ExecutionTerminal = cloneWorkerRecordingTerminal(failure.ExecutionTerminal)
	writer.failures = append(writer.failures, failure)
	if writer.markerError {
		return errors.New("private marker sentinel")
	}
	return nil
}

type selectedCaptureFixture struct {
	source  *selectedCaptureSource
	writer  *selectedCaptureWriter
	handle  recordings.WorkerSessionRecording
	request recordings.WorkerSessionRecordingRequest
	ctx     context.Context
}

func newSelectedCaptureFixture(t *testing.T, name string, writer *selectedCaptureWriter, logger logging.Logger) *selectedCaptureFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	source := &selectedCaptureSource{Service: newRecordingEventsService(), deliveries: make(chan events.Delivery), ready: make(chan struct{}, 16)}
	service, err := New(source, writer, logger)
	if err != nil {
		t.Fatal(err)
	}
	request := recordings.WorkerSessionRecordingRequest{
		RecordingID: "recording-" + name, FactorySessionID: "factory-" + name,
		WorkerSessionID: "worker-selected", Topic: workersessions.Topic("worker-selected", "factory-"+name),
	}
	handle, err := service.StartWorkerSessionRecording(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	registerCaptureCleanup(t, handle)
	return &selectedCaptureFixture{source: source, writer: writer, handle: handle, request: request, ctx: ctx}
}

func registerCaptureCleanup(t *testing.T, handle recordings.WorkerSessionRecording) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = handle.Abort(ctx, recordings.ErrWorkerRecordingOpening)
		select {
		case <-handle.(*capture).done:
		default:
			t.Error("capture cleanup did not join owned consumer")
		}
	})
}

func (fixture *selectedCaptureFixture) deliver(t *testing.T, record events.Record) {
	t.Helper()
	select {
	case <-fixture.source.ready:
	case <-fixture.ctx.Done():
		t.Fatal("subscription did not become ready")
	}
	select {
	case fixture.source.deliveries <- events.Delivery{Kind: events.DeliveryRecord, Record: record, Cursor: events.Cursor{Topic: record.ID.Topic, Position: record.ID.Position}}:
	case <-fixture.ctx.Done():
		t.Fatal("subscription did not receive record")
	}
}

func (fixture *selectedCaptureFixture) open(t *testing.T) {
	t.Helper()
	fixture.deliver(t, mustRecord(t, openingAppend(fixture.request.Topic, fixture.request.WorkerSessionID), 1))
	if err := fixture.handle.AwaitOpening(fixture.ctx); err != nil {
		t.Fatal(err)
	}
}

func (fixture *selectedCaptureFixture) joinFailure(t *testing.T) {
	t.Helper()
	select {
	case <-fixture.handle.(*capture).done:
	case <-fixture.ctx.Done():
		t.Fatal("failed capture did not join")
	}
}

type selectedCaptureLogger struct {
	logging.NoopLogger
	mu       sync.Mutex
	messages []string
	fields   [][]any
}

func (logger *selectedCaptureLogger) Warn(message string, fields ...any) {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	logger.messages = append(logger.messages, message)
	logger.fields = append(logger.fields, append([]any(nil), fields...))
}

func assertSelectedCaptureDiagnostics(t *testing.T, logger *selectedCaptureLogger, request recordings.WorkerSessionRecordingRequest, code string, marker bool) {
	t.Helper()
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if len(logger.messages) == 0 {
		t.Fatal("selected logger received no diagnostics")
	}
	sawCapture, sawTerminalMarker := false, false
	for i, message := range logger.messages {
		fields := logger.fields[i]
		if strings.Contains(message+fmt.Sprint(fields), "private") {
			t.Fatalf("diagnostic disclosed private data: %s %v", message, fields)
		}
		for key, want := range map[string]string{"workerSessionID": request.WorkerSessionID, "topic": string(request.Topic), "outcome": "failed", "code": code} {
			if !containsSelectedDiagnosticPair(fields, key, want) {
				t.Fatalf("missing %s=%s: %v", key, want, fields)
			}
		}
		switch message {
		case "Worker recording capture failed":
			sawCapture = true
		case "Worker recording failure persistence failed":
			if !containsDiagnosticPair(fields, "stage", "degradation_marker") {
				t.Fatalf("missing marker stage: %v", fields)
			}
			sawTerminalMarker = sawTerminalMarker || containsDiagnosticPair(fields, "executionOutcome", "COMPLETED")
		default:
			t.Fatalf("unexpected diagnostic: %s", message)
		}
	}
	if !sawCapture || (marker && !sawTerminalMarker) {
		t.Fatalf("missing capture/terminal-marker diagnostic: %v", logger.messages)
	}
}

func containsSelectedDiagnosticPair(fields []any, key, want string) bool {
	for i := 0; i+1 < len(fields); i += 2 {
		if fields[i] == key && fmt.Sprint(fields[i+1]) == want {
			return true
		}
	}
	return false
}

func runSelectedPostOpeningFailure(t *testing.T, logger logging.Logger, markerError bool) *selectedCaptureFixture {
	t.Helper()
	fixture := newSelectedCaptureFixture(t, "post-opening", &selectedCaptureWriter{failPosition: 2, markerError: markerError}, logger)
	fixture.open(t)
	// This source payload must never appear in diagnostics.
	fixture.deliver(t, selectedPrivateOutput(t, fixture.request))
	fixture.joinFailure(t)
	err := requireWorkerRecordingFinalizer(t, fixture.handle).CloseWithTerminal(fixture.ctx, completedWorkerTerminal())
	if !errors.Is(err, recordings.ErrWorkerRecordingPersistence) {
		t.Fatalf("close = %v", err)
	}
	if !strings.Contains(err.Error(), "private writer sentinel") || strings.Contains(err.Error(), "private marker sentinel") {
		t.Fatalf("returned error policy changed: %v", err)
	}
	assertSelectedFailureTruth(t, fixture, recordings.WorkerRecordingStatusDegraded, "PERSISTENCE_FAILED")
	return fixture
}

func selectedPrivateOutput(t *testing.T, request recordings.WorkerSessionRecordingRequest) events.Record {
	t.Helper()
	appendRequest := workerOutputAppend(request.Topic, request.WorkerSessionID, 1, "private-output")
	appendRequest.Payload = []byte(strings.ReplaceAll(string(appendRequest.Payload), "captured", "private transcript sentinel"))
	return mustRecord(t, appendRequest, 2)
}

func assertSelectedFailureTruth(t *testing.T, fixture *selectedCaptureFixture, status recordings.WorkerRecordingStatus, code string) {
	t.Helper()
	projection, err := fixture.handle.(recordings.WorkerRecordingProjectionReader).WorkerRecordingProjection()
	if err != nil || projection.Status != status || projection.Degradation != code || projection.Complete {
		t.Fatalf("failure projection = %#v, error %v", projection, err)
	}
	if len(fixture.writer.records) != 1 || len(projection.Records) != 1 || projection.Terminal != nil {
		t.Fatalf("failed record accepted or terminal fabricated: %#v", projection)
	}
	failures := fixture.writer.failures
	if len(failures) == 0 || failures[len(failures)-1].Code != code {
		t.Fatalf("failure markers = %#v", failures)
	}
	last := failures[len(failures)-1]
	if last.Topic != fixture.request.Topic || last.WorkerSessionID != fixture.request.WorkerSessionID {
		t.Fatalf("marker attribution = %#v", last)
	}
	assertSelectedTerminalTruth(t, status, last.ExecutionTerminal, projection.ExecutionTerminal)
}

func assertSelectedTerminalTruth(t *testing.T, status recordings.WorkerRecordingStatus, marker, projection *recordings.WorkerRecordingTerminal) {
	t.Helper()
	if status == recordings.WorkerRecordingStatusDegraded && (marker == nil || projection == nil || marker.Status != "COMPLETED") {
		t.Fatalf("missing authoritative terminal: %#v, %#v", marker, projection)
	}
	if status == recordings.WorkerRecordingStatusIncomplete && (marker != nil || projection != nil) {
		t.Fatal("fabricated execution terminal")
	}
}

func TestWorkerCaptureSelectedLoggerReportsPostOpeningPersistenceFailure(t *testing.T) {
	t.Parallel()
	logger := &selectedCaptureLogger{}
	fixture := runSelectedPostOpeningFailure(t, logger, false)
	assertSelectedCaptureDiagnostics(t, logger, fixture.request, "PERSISTENCE_FAILED", false)
	if len(fixture.writer.failures) != 2 || fixture.writer.failures[0].ExecutionTerminal != nil {
		t.Fatalf("marker upgrade = %#v", fixture.writer.failures)
	}
}

func TestWorkerCaptureSelectedLoggerReportsFailureMarkerErrorSafely(t *testing.T) {
	t.Parallel()
	logger := &selectedCaptureLogger{}
	fixture := runSelectedPostOpeningFailure(t, logger, true)
	assertSelectedCaptureDiagnostics(t, logger, fixture.request, "PERSISTENCE_FAILED", true)
	if len(logger.messages) != 3 {
		t.Fatalf("diagnostics = %v", logger.messages)
	}
}

func TestWorkerCaptureExplicitNoopPreservesFailureOutcome(t *testing.T) {
	t.Parallel()
	diagnostic := runSelectedPostOpeningFailure(t, &selectedCaptureLogger{}, false)
	quiet := runSelectedPostOpeningFailure(t, logging.NoopLogger{}, false)
	if !reflect.DeepEqual(diagnostic.writer.records, quiet.writer.records) || !reflect.DeepEqual(diagnostic.writer.failures, quiet.writer.failures) {
		t.Fatal("Noop changed durable outcome")
	}
}

func TestWorkerCaptureSelectedLoggerReportsOpeningFailure(t *testing.T) {
	t.Parallel()
	for _, persistence := range []bool{false, true} {
		t.Run(fmt.Sprint(persistence), func(t *testing.T) {
			t.Parallel()
			logger := &selectedCaptureLogger{}
			writer := &selectedCaptureWriter{}
			wantErr, code := recordings.ErrWorkerRecordingOpening, "OPENING_INVALID"
			if persistence {
				writer.failPosition = 1
				wantErr = recordings.ErrWorkerRecordingPersistence
				code = "PERSISTENCE_FAILED"
			}
			fixture := newSelectedCaptureFixture(t, "opening-"+code, writer, logger)
			opening := openingAppend(fixture.request.Topic, fixture.request.WorkerSessionID)
			if !persistence {
				opening.SourceType = "invalid-opening"
			}
			fixture.deliver(t, mustRecord(t, opening, 1))
			if err := fixture.handle.AwaitOpening(fixture.ctx); !errors.Is(err, wantErr) {
				t.Fatalf("opening = %v", err)
			}
			fixture.joinFailure(t)
			if err := fixture.handle.Close(fixture.ctx); !errors.Is(err, wantErr) {
				t.Fatalf("close = %v", err)
			}
			if len(writer.records) != 0 {
				t.Fatal("failed opening was accepted")
			}
			assertSelectedCaptureDiagnostics(t, logger, fixture.request, code, false)
		})
	}
}

func TestWorkerCaptureSelectedLoggerReportsDuplicateConflict(t *testing.T) {
	t.Parallel()
	logger := &selectedCaptureLogger{}
	fixture := newSelectedCaptureFixture(t, "duplicate", &selectedCaptureWriter{}, logger)
	fixture.open(t)
	opening := mustRecord(t, openingAppend(fixture.request.Topic, fixture.request.WorkerSessionID), 1)
	fixture.deliver(t, opening)
	conflicting := opening.Detached()
	conflicting.Payload = []byte(`{"private":"conflicting payload sentinel"}`)
	fixture.deliver(t, conflicting)
	fixture.joinFailure(t)
	if err := fixture.handle.Close(fixture.ctx); !errors.Is(err, recordings.ErrWorkerRecordingDuplicate) {
		t.Fatalf("duplicate = %v", err)
	}
	assertSelectedFailureTruth(t, fixture, recordings.WorkerRecordingStatusIncomplete, "DUPLICATE_CONFLICT")
	assertSelectedCaptureDiagnostics(t, logger, fixture.request, "DUPLICATE_CONFLICT", false)
}

func TestWorkerCaptureSelectedLoggerCancellationJoinsOwnedCapture(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"opening", "close", "abort"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			logger := &selectedCaptureLogger{}
			fixture := newSelectedCaptureFixture(t, operation, &selectedCaptureWriter{}, logger)
			ctx, cancel := context.WithCancel(fixture.ctx)
			cancel()
			var err error
			want, code := recordings.ErrWorkerRecordingCanceled, "CANCELED"
			switch operation {
			case "opening":
				err = fixture.handle.AwaitOpening(ctx)
			case "close":
				fixture.open(t)
				err = fixture.handle.Close(ctx)
			case "abort":
				want = recordings.ErrWorkerRecordingOpening
				code = "OPENING_INVALID"
				err = fixture.handle.Abort(fixture.ctx, want)
			}
			if !errors.Is(err, want) || (operation != "abort" && !errors.Is(err, context.Canceled)) {
				t.Fatalf("%s = %v", operation, err)
			}
			if err = fixture.handle.Close(fixture.ctx); !errors.Is(err, want) {
				t.Fatalf("live join = %v", err)
			}
			fixture.joinFailure(t)
			assertSelectedCaptureDiagnostics(t, logger, fixture.request, code, false)
		})
	}
}

func TestWorkerCaptureParallelSelectedLoggersKeepTopicAttribution(t *testing.T) {
	t.Parallel()
	firstLog, peerLog := &selectedCaptureLogger{}, &selectedCaptureLogger{}
	first := newSelectedCaptureFixture(t, "first", &selectedCaptureWriter{failPosition: 2}, firstLog)
	peer := newSelectedCaptureFixture(t, "peer", &selectedCaptureWriter{}, peerLog)
	first.open(t)
	peer.open(t)
	first.deliver(t, mustRecord(t, terminalAppend(first.request.Topic, first.request.WorkerSessionID), 2))
	first.joinFailure(t)
	if err := first.handle.Close(first.ctx); !errors.Is(err, recordings.ErrWorkerRecordingPersistence) {
		t.Fatalf("first close = %v", err)
	}
	assertSelectedFailureTruth(t, first, recordings.WorkerRecordingStatusIncomplete, "PERSISTENCE_FAILED")
	// First failure must leave the peer subscribed and able to accept work.
	peer.deliver(t, selectedPrivateOutput(t, peer.request))
	select {
	case <-peer.source.ready:
	case <-peer.ctx.Done():
		t.Fatal("peer stopped after first capture failure")
	}
	if err := peer.handle.Close(peer.ctx); !errors.Is(err, recordings.ErrWorkerRecordingIncomplete) {
		t.Fatalf("peer close = %v", err)
	}
	peer.joinFailure(t)
	projection, err := peer.handle.(recordings.WorkerRecordingProjectionReader).WorkerRecordingProjection()
	if err != nil || projection.Status != recordings.WorkerRecordingStatusIncomplete || len(peer.writer.records) != 2 || projection.ExecutionTerminal != nil {
		t.Fatalf("peer projection = %#v, %v", projection, err)
	}
	assertSelectedCaptureDiagnostics(t, firstLog, first.request, "PERSISTENCE_FAILED", false)
	assertSelectedCaptureDiagnostics(t, peerLog, peer.request, "INCOMPLETE", false)
}

func TestWorkerWorkAttributionOriginatingArtifactSurvivesCatalogRebuild(t *testing.T) {
	t.Parallel()
	for _, artifact := range []string{"", "selected/custom-recording.json"} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			local := platformreplay.NewLocal(runtime.GOOS)
			writer := journalWriter(t, local)
			record := journalRecord(t, "origin-recording", "origin-worker")
			record.OriginatingArtifact = artifact
			if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			// The opening establishes provenance; later records cannot replace it.
			record.OriginatingArtifact = "later-substitution"
			record.Record = mustRecord(t, terminalAppend(record.Record.ID.Topic, record.WorkerSessionID), 2)
			if err := writer.PersistWorkerRecord(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			reopened, err := NewFileWriter(local, local, local, &captureTimeProbe{}, writer.root, "restarted", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, store := range []recordings.WorkerRecordingStore{writer, reopened} {
				page, err := store.ReadWorkerCapturedActivity(t.Context(), recordings.WorkerCapturedActivityRequest{WorkerSessionID: record.WorkerSessionID, Limit: 1})
				if err != nil || page.Catalog.OriginatingArtifact != artifact || page.Health != recordings.WorkerRecordingStatusComplete {
					t.Fatalf("origin after committed/reopened read = %+v, %v; want %q", page.Catalog, err, artifact)
				}
			}
		})
	}
}
