package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	events "github.com/portpowered/infinite-you/pkg/services/events"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/cursors"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseeventstore"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	responsestreamservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream"
	internalservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/response_stream/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/testing/eventsstub"
)

func newTestEventsService(t *testing.T) events.Service {
	t.Helper()
	return eventsstub.New()
}

type memoryCursorStore struct {
	checkpoint cursors.Checkpoint
	found      bool
}

func (s *memoryCursorStore) Load(context.Context, cursors.StorageIdentity) (cursors.Checkpoint, bool, error) {
	return s.checkpoint, s.found, nil
}

func (s *memoryCursorStore) Save(_ context.Context, _ cursors.StorageIdentity, checkpoint cursors.Checkpoint) error {
	s.checkpoint, s.found = checkpoint, true
	return nil
}

func (*memoryCursorStore) Close() error { return nil }

type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time { return c.now }

func newService(t *testing.T) responsestreamservice.Service {
	t.Helper()
	var next atomic.Uint64
	service, err := internalservice.New(func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	}, nil, newTestEventsService(t), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func serviceWithEvents(t *testing.T, eventsService events.Service) (responsestreamservice.Service, error) {
	t.Helper()
	var next atomic.Uint64
	return internalservice.New(func() string {
		return fmt.Sprintf("response-event-%d", next.Add(1))
	}, nil, eventsService, logging.NoopLogger{})
}

func newStore(t *testing.T, service responsestreamservice.Service) *responseeventstore.SessionResponseEventStore {
	t.Helper()
	store, err := service.NewEventStore("session-1", &fixedClock{now: time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	return store
}

// TestService_SubscribeReconnectsAfterKnownCursorWithOrderedFilteredEvents
// publishes through publishThroughService (service.Publish), not the store's
// own Publish directly: this store is bound to the injected Events root (see
// newStore/NewEventStore), so a test that published straight to the store
// would produce a session with retained local content Events never
// observed, which now correctly surfaces as a gap under
// substituteFromEvents' no-fallback delegation instead of silently masking
// the mismatch. See mirror_test.go's own doc comment on this exact pitfall.
func TestService_SubscribeReconnectsAfterKnownCursorWithOrderedFilteredEvents(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	first := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-2")
	third := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")

	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{
		AfterSequence: first.Sequence,
		DispatchID:    "dispatch-1",
		Kinds:         []responseevents.Kind{responseevents.KindMessage},
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 1 || events[0].Sequence != third.Sequence {
		t.Fatalf("events = %#v, want only sequence %d", events, third.Sequence)
	}
}

func TestService_StaleCursorSignalsGapAndPreservesFirstAvailableEvent(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	if err := store.SetRetentionLimits(responseeventstore.RetentionLimits{MaxEvents: 1, MaxBytes: 1 << 20}); err != nil {
		t.Fatalf("SetRetentionLimits: %v", err)
	}
	publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	retained := publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if len(events) != 2 || events[0].Kind != responseevents.KindStreamGap || events[1].Sequence != retained.Sequence {
		t.Fatalf("events = %#v, want gap followed by sequence %d", events, retained.Sequence)
	}
}

func TestService_CancellationAndSlowSubscribersStayBounded(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := newStore(t, service)
	cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	for range 100 {
		publishThroughService(t, service, store, responseevents.KindMessage, "dispatch-1")
	}
	if got := store.SubscriberCount(); got != 1 {
		t.Fatalf("SubscriberCount = %d, want 1", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancellation races with retained delivery by contract; drain retained
	// data, then verify cancellation without a publisher-owned queue or goroutine.
	if _, err := cursor.Next(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Next after cancellation: %v", err)
	}
	cursor.Detach()
	if got := store.SubscriberCount(); got != 0 {
		t.Fatalf("SubscriberCount after detach = %d, want 0", got)
	}
	if accounting := store.RetentionAccounting(); accounting.EventCount > store.RetentionLimits().MaxEvents {
		t.Fatalf("retention accounting = %#v, exceeds limits %#v", accounting, store.RetentionLimits())
	}
}

func TestService_NewEventStoreAppliesConfiguredRetentionLimits(t *testing.T) {
	t.Parallel()

	wantLimits := &factorysessions.ResponseEventRetentionLimits{
		MaxEvents:                42,
		MaxBytes:                 8192,
		CompletedRetentionWindow: time.Minute,
	}
	service, err := internalservice.New(func() string { return "response-event-1" }, wantLimits, newTestEventsService(t), logging.NoopLogger{})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	store, err := service.NewEventStore("session-1", &fixedClock{now: time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewEventStore: %v", err)
	}
	got := store.RetentionLimits()
	if got.MaxEvents != wantLimits.MaxEvents ||
		got.MaxBytes != wantLimits.MaxBytes ||
		got.CompletedRetentionWindow != wantLimits.CompletedRetentionWindow {
		t.Fatalf("RetentionLimits = %#v, want max events %d max bytes %d completed window %s",
			got, wantLimits.MaxEvents, wantLimits.MaxBytes, wantLimits.CompletedRetentionWindow)
	}
}

func TestService_ConstructionIsInertAndRejectsMissingEffects(t *testing.T) {
	t.Parallel()
	if service, err := internalservice.New(nil, nil, newTestEventsService(t), logging.NoopLogger{}); err == nil || service != nil {
		t.Fatalf("NewService(nil) = %#v, %v; want deterministic dependency error", service, err)
	}
	if service, err := internalservice.New(func() string { return "response-event-1" }, nil, nil, logging.NoopLogger{}); err == nil || service != nil {
		t.Fatalf("NewService(eventsService=nil) = %#v, %v; want deterministic dependency error", service, err)
	}
	service := newService(t)
	if store, err := service.NewEventStore("session-1", nil); err == nil || store != nil {
		t.Fatalf("NewEventStore without clock = %#v, %v; want clock error", store, err)
	}
}

func TestService_CursorPersistenceAndPublicationDiagnosticsUseInjectedEffects(t *testing.T) {
	t.Parallel()
	service := newService(t)
	store := &memoryCursorStore{}
	tracker, err := service.NewCursorTracker(store, cursors.StorageIdentity{
		BackendScopeID: "backend-1", FactorySessionID: "session-1",
		StreamGenerationID: "stream-1", ConsumerID: "consumer-1",
	})
	if err != nil {
		t.Fatalf("NewCursorTracker: %v", err)
	}
	sequence := 7
	checkpoint := cursors.Checkpoint{AfterSequence: &sequence}
	if err := tracker.Advance(context.Background(), checkpoint); err != nil {
		t.Fatalf("Advance: %v", err)
	}
	restored, found, err := tracker.Restore(context.Background())
	if err != nil || !found || restored.AfterSequence == nil || *restored.AfterSequence != sequence {
		t.Fatalf("Restore = %#v, %t, %v; want sequence %d", restored, found, err, sequence)
	}

	registry, err := service.NewStreamRegistry(&fixedClock{now: time.Now().UTC()})
	if err != nil {
		t.Fatalf("NewStreamRegistry: %v", err)
	}
	stream := registry.Streams("session-1").Stream("dispatch-1")
	var diagnosticCount atomic.Int64
	publisher := service.NewPublisher(stream, func(responsestream.CompactionSummary) {
		diagnosticCount.Add(1)
	})
	publisher.ReportCompaction(responsestream.CompactionSummary{Reason: responsestream.CompactionReasonTruncated})
	if got := publisher.Diagnostics().CompactionCount; got != 1 || diagnosticCount.Load() != 1 {
		t.Fatalf("diagnostics = %#v, observer count = %d; want one compaction", publisher.Diagnostics(), diagnosticCount.Load())
	}
}

type publicationLog struct {
	level   string
	message string
	fields  []any
}

type publicationCapture struct {
	mu      sync.Mutex
	records []publicationLog
}

func (c *publicationCapture) record(level, message string, fields ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, publicationLog{level, message, append([]any(nil), fields...)})
}

func (c *publicationCapture) Debug(message string, fields ...any) {
	c.record("debug", message, fields...)
}
func (c *publicationCapture) Info(message string, fields ...any) {
	c.record("info", message, fields...)
}
func (c *publicationCapture) Warn(message string, fields ...any) {
	c.record("warn", message, fields...)
}
func (c *publicationCapture) Error(message string, fields ...any) {
	c.record("error", message, fields...)
}
func (c *publicationCapture) Verbose(message string, fields ...any) {
	c.record("verbose", message, fields...)
}

func (c *publicationCapture) assertPublication(t *testing.T, session, eventID, outcome, class string, sequence int64) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	want := []publicationLog{
		{"debug", "Factory Session response publication", []any{"operation", "publish", "outcome", "started", "session_id", session}},
		{"debug", "Factory Session response publication", []any{"operation", "publish", "outcome", outcome, "class", class,
			"session_id", session, "event_id", eventID, "sequence", sequence}},
	}
	if !reflect.DeepEqual(c.records, want) {
		t.Fatalf("diagnostics = %#v, want %#v", c.records, want)
	}
}

func diagnosticInput() responseevents.FactoryResponseEvent {
	return responseevents.FactoryResponseEvent{
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta,
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"SECRET_PAYLOAD /private/path token=credential"}`),
	}
}

func diagnosticService(t *testing.T, authority events.Service, logger logging.Logger, id string) *internalservice.ResponseStream {
	t.Helper()
	service, err := internalservice.New(func() string { return id }, nil, authority, logger)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestService_PublishDiagnosticsPreserveNoopResults(t *testing.T) {
	t.Parallel()
	var baseline []responseevents.FactoryResponseEvent
	for _, captureEnabled := range []bool{false, true} {
		capture := &publicationCapture{}
		var logger logging.Logger = logging.NoopLogger{}
		if captureEnabled {
			logger = capture
		}
		authority := newTestEventsService(t)
		var next atomic.Uint64
		service, err := internalservice.New(func() string { return fmt.Sprintf("event-safe-%d", next.Add(1)) }, nil, authority, logger)
		if err != nil {
			t.Fatal(err)
		}
		store := newStore(t, service)
		t.Cleanup(store.Close)
		cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cursor.Detach)
		var results []responseevents.FactoryResponseEvent
		for sequence := int64(1); sequence <= 2; sequence++ {
			capture.records = nil
			result, err := service.Publish(store, diagnosticInput())
			if err != nil {
				t.Fatal(err)
			}
			if result.Sequence != sequence || result.EventID != fmt.Sprintf("event-safe-%d", sequence) || !result.RecordedAt.Equal((&fixedClock{now: time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)}).Now()) {
				t.Fatalf("accepted identity/time: %#v", result)
			}
			if captureEnabled {
				capture.assertPublication(t, "session-1", result.EventID, "succeeded", "none", sequence)
			}
			results = append(results, result)
		}
		assertDiagnosticDelivery(t, store, cursor, results)
		assertDiagnosticAuthority(t, authority, results)
		if captureEnabled && !reflect.DeepEqual(results, baseline) {
			t.Fatal("capture changed publication results")
		}
		baseline = results
	}
}

func assertDiagnosticDelivery(t *testing.T, store *responseeventstore.SessionResponseEventStore, cursor *responsestreamservice.Cursor, results []responseevents.FactoryResponseEvent) {
	t.Helper()
	delivered, err := cursor.Drain()
	if err != nil || !reflect.DeepEqual(delivered, results) || !reflect.DeepEqual(store.Events(), results) {
		t.Fatalf("delivery/store differ: %#v, %v", delivered, err)
	}
}

func assertDiagnosticAuthority(t *testing.T, authority events.Service, results []responseevents.FactoryResponseEvent) {
	t.Helper()
	topic := responseEventTopicForTest("session-1")
	read, err := authority.Read(context.Background(), events.ReadRequest{Topic: topic, From: events.Cursor{Topic: topic}, Limit: 3})
	if err != nil || len(read.Records) != 2 {
		t.Fatalf("authority: %#v, %v", read, err)
	}
	for i, record := range read.Records {
		var decoded responseevents.FactoryResponseEvent
		if err := json.Unmarshal(record.Payload, &decoded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded, results[i]) || int64(record.ID.Position) != results[i].Sequence {
			t.Fatalf("authority content differs: %#v", decoded)
		}
	}
}

type diagnosticAuthority struct {
	events.Service
	calls     int
	rejection error
}

func (a *diagnosticAuthority) Append(ctx context.Context, request events.AppendRequest) (events.AppendResult, error) {
	a.calls++
	if a.rejection != nil {
		return events.AppendResult{}, a.rejection
	}
	return a.Service.Append(ctx, request)
}

func TestService_PublishRejectedDiagnosticsPreserveStateAndPrivacy(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("SECRET_ERROR /private/path token=credential")
	for _, tc := range []struct {
		name, class string
	}{
		{"events", "events_rejected"}, {"completed", "store_completed"},
		{"closed", "store_closed"}, {"malformed", "invalid_event"},
		{"empty_id", "empty_identity"}, {"nil_store", "missing_store"},
		{"session_mismatch", "invalid_event"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var baselineError string
			for _, enabled := range []bool{false, true} {
				capture := &publicationCapture{}
				var logger logging.Logger = logging.NoopLogger{}
				if enabled {
					logger = capture
				}
				authority := &diagnosticAuthority{Service: newTestEventsService(t)}
				id := "event-safe"
				if tc.name == "empty_id" {
					id = " \t "
				}
				service := diagnosticService(t, authority, logger, id)
				store := newStore(t, service)
				t.Cleanup(store.Close)
				cursor, err := service.Subscribe(context.Background(), store, responsestreamservice.SubscriptionRequest{})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(cursor.Detach)
				prepareDiagnosticTerminal(t, tc.name, service, store, cursor)
				before, sequence, calls := store.Events(), store.LatestSequence(), authority.calls
				capture.records = nil
				selectedStore, input := diagnosticRejectionInput(tc.name, store, authority, sentinel)
				result, err := service.Publish(selectedStore, input)
				if err == nil || !reflect.DeepEqual(result, responseevents.FactoryResponseEvent{}) {
					t.Fatalf("rejection = %#v, %v", result, err)
				}
				if enabled && err.Error() != baselineError {
					t.Fatalf("logger changed error: %v vs %s", err, baselineError)
				}
				baselineError = err.Error()
				assertDiagnosticRejection(t, tc.name, err, sentinel)
				assertDiagnosticState(t, tc.name, store, cursor, authority, before, sequence, calls)
				if enabled {
					session, eventID := "session-1", strings.TrimSpace(id)
					if tc.name == "nil_store" {
						session, eventID = "", ""
					}
					capture.assertPublication(t, session, eventID, "rejected", tc.class, 0)
				}
			}
		})
	}
}

func diagnosticRejectionInput(name string, store *responseeventstore.SessionResponseEventStore, authority *diagnosticAuthority, sentinel error) (*responseeventstore.SessionResponseEventStore, responseevents.FactoryResponseEvent) {
	input := diagnosticInput()
	switch name {
	case "events":
		authority.rejection = sentinel
	case "malformed":
		input.Payload = json.RawMessage(`{"secret":"SECRET_MALFORMED"`)
	case "session_mismatch":
		input.FactorySessionID = "SECRET_CALLER_SESSION"
	case "nil_store":
		store = nil
	}
	return store, input
}

func TestService_PublishDiagnosticsIsolateIndependentOwners(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for _, session := range []string{"session-left", "session-right"} {
		capture := &publicationCapture{}
		service := diagnosticService(t, newTestEventsService(t), capture, "event-"+session)
		store, err := service.NewEventStore(session, &fixedClock{now: time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := service.Publish(store, diagnosticInput())
			if err != nil || result.FactorySessionID != session || result.Sequence != 1 {
				t.Errorf("isolated publication: %#v, %v", result, err)
			}
			capture.assertPublication(t, session, "event-"+session, "succeeded", "none", 1)
		}()
	}
	wg.Wait()
}

func TestService_ConstructionDoesNotActivateEffects(t *testing.T) {
	t.Parallel()
	capture := &publicationCapture{}
	authority := &diagnosticAuthority{}
	for _, logger := range []logging.Logger{capture, logging.NoopLogger{}} {
		_, err := internalservice.New(func() string { t.Error("construction used ID effect"); return "" }, nil, authority, logger)
		if err != nil {
			t.Fatal(err)
		}
	}
	if authority.calls != 0 || len(capture.records) != 0 {
		t.Fatal("construction activated operation effects")
	}
}

func assertDiagnosticRejection(t *testing.T, name string, err, sentinel error) {
	t.Helper()

	switch name {
	case "events":
		if !errors.Is(err, sentinel) || !strings.HasPrefix(err.Error(), "factory session response event rejected by Events: ") {
			t.Fatalf("lost cause/wrapper: %v", err)
		}
	case "completed":
		if !errors.Is(err, responseeventstore.ErrStoreCompleted) {
			t.Fatal(err)
		}
	case "closed":
		if !errors.Is(err, responseeventstore.ErrStoreClosed) {
			t.Fatal(err)
		}
	case "malformed":
		var validation *responseevents.ValidationError
		if !errors.As(err, &validation) || validation.Field != "payload" {
			t.Fatalf("lost validation category: %v", err)
		}
	case "session_mismatch":
		if !errors.Is(err, responseeventstore.ErrFactorySessionMismatch) {
			t.Fatal(err)
		}
	}
}

func prepareDiagnosticTerminal(t *testing.T, name string, service *internalservice.ResponseStream, store *responseeventstore.SessionResponseEventStore, cursor *responsestreamservice.Cursor) {
	t.Helper()

	if name == "completed" || name == "closed" {
		if _, err := service.Publish(store, diagnosticInput()); err != nil {
			t.Fatal(err)
		}
		if _, err := cursor.Drain(); err != nil {
			t.Fatal(err)
		}
		if name == "closed" {
			service.Close(store)
		} else {
			service.Complete(store)
			if !store.CompletedAt().Equal(time.Date(2026, 7, 22, 20, 0, 0, 0, time.UTC)) {
				t.Fatal("completion clock changed")
			}
		}
	}
}

func assertDiagnosticState(t *testing.T, name string, store *responseeventstore.SessionResponseEventStore, cursor *responsestreamservice.Cursor, authority *diagnosticAuthority, before []responseevents.FactoryResponseEvent, sequence int64, calls int) {
	t.Helper()

	if !reflect.DeepEqual(store.Events(), before) || store.LatestSequence() != sequence {
		t.Fatal("partial store mutation")
	}
	wantCalls := calls
	if name == "events" {
		wantCalls++
	}
	if authority.calls != wantCalls {
		t.Fatalf("unauthorized Append: %d, want %d", authority.calls, wantCalls)
	}
	delivered, drainErr := cursor.Drain()
	if len(delivered) != 0 {
		t.Fatalf("partial subscriber delivery: %#v", delivered)
	}
	if name != "completed" && name != "closed" && drainErr != nil {
		t.Fatal(drainErr)
	}
}
