package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type observationProjectorFake struct {
	providersessions.Service
	result providersessions.ProjectResult
	err    error
}

func (f observationProjectorFake) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return f.result, f.err
}

type observationEventReaderFake struct {
	subscription events.Subscription
	err          error
}

func (f *observationEventReaderFake) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return f.subscription, f.err
}

func newObservationRegistry(provider providersessions.Service, reader EventsReader) *registry {
	return &registry{
		sessions:         make(map[string]workersessions.Session),
		observations:     make(map[string]*observation),
		publications:     make(map[string]*publication),
		supervisions:     make(map[string]*supervision),
		dispatchOwners:   make(map[string]string),
		providerSessions: provider,
		eventReader:      reader,
		clock:            platformclock.NewDeterministic(time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC), time.Second),
		logger:           logging.NoopLogger{},
	}
}

func observationProviderRef() providers.SessionRef {
	return providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
}

func observationSession(id string, state workersessions.State) workersessions.Session {
	return workersessions.Session{
		ID:    id,
		State: state,
		ProviderSessionAssociation: &workersessions.ProviderSessionAssociation{
			WorkerSessionID: id,
			TurnID:          "turn-1",
			DispatchID:      "attempt-1",
			AttemptID:       "attempt-1",
			Reference:       observationProviderRef(),
		},
	}
}

func observationMetadata() *observation {
	return &observation{
		workIDs:   []string{"work-1"},
		turnID:    "turn-1",
		attemptID: "attempt-1",
		startedAt: time.Date(2026, 8, 8, 11, 59, 58, 0, time.UTC),
	}
}

func TestInvokeObservationHelpersCoverTimingDiagnosticsAndClones(t *testing.T) {
	if observationContextError(nil) != nil || observationContextError(context.Background()) != nil {
		t.Fatal("observationContextError() rejected nil/background context")
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(observationContextError(canceled), workersessions.ErrObservationCanceled) {
		t.Fatal("observationContextError(canceled) did not return ErrObservationCanceled")
	}

	ended := time.Date(2026, 8, 8, 12, 0, 2, 0, time.UTC)
	wantEnded := ended
	metadata := observationMetadata()
	metadata.endedAt = &ended
	clone := cloneObservation(metadata)
	metadata.workIDs[0] = "mutated"
	*metadata.endedAt = ended.Add(time.Hour)
	if clone == nil || clone.workIDs[0] != "work-1" || !clone.endedAt.Equal(wantEnded) {
		t.Fatalf("cloneObservation() retained source state: %#v", clone)
	}
	if cloneObservation(nil) != nil {
		t.Fatal("cloneObservation(nil) returned a value")
	}

	if !containsString([]string{"a", "b"}, "b") || containsString([]string{"a"}, "z") {
		t.Fatal("containsString() did not distinguish present and absent values")
	}
	values := []string{"c", "a", "b"}
	sortStrings(values)
	if strings.Join(values, ",") != "a,b,c" {
		t.Fatalf("sortStrings() = %v, want a,b,c", values)
	}
	orders := []observationOrder{{id: "late", startedAt: time.Unix(2, 0), attemptID: "b"}, {id: "early", startedAt: time.Unix(1, 0), attemptID: "a"}, {id: "tie-b", startedAt: time.Unix(2, 0), attemptID: "a"}}
	sortObservationOrder(orders)
	if orders[0].id != "early" || orders[1].id != "tie-b" {
		t.Fatalf("sortObservationOrder() = %#v", orders)
	}
	tied := []observationOrder{{id: "b", startedAt: time.Unix(3, 0), attemptID: "a"}, {id: "a", startedAt: time.Unix(3, 0), attemptID: "a"}}
	sortObservationOrder(tied)
	if tied[0].id != "a" {
		t.Fatalf("sortObservationOrder() tie = %#v, want id a first", tied)
	}
	observations := []workersessions.Observation{{WorkerSessionID: "without-time", AttemptID: "b"}, {WorkerSessionID: "with-time", AttemptID: "a", StartedAt: timePointer(time.Unix(1, 0))}}
	sortObservationAttempts(observations)
	if observations[0].WorkerSessionID != "with-time" {
		t.Fatalf("sortObservationAttempts() = %#v", observations)
	}
}

func TestInvokeObservationDiagnosticsAndTranscriptHelpers(t *testing.T) {
	if got := nonNegativeDuration(-time.Second); got == nil || *got != 0 {
		t.Fatalf("nonNegativeDuration(-1s) = %v, want zero", got)
	}
	if got := nonNegativeDuration(time.Second); got == nil || *got != time.Second {
		t.Fatalf("nonNegativeDuration(1s) = %v, want 1s", got)
	}
	if observationTokenUsage(nil) != nil {
		t.Fatal("observationTokenUsage(nil) returned a value")
	}
	input := 3
	usage := observationTokenUsage(&providersessions.TokenUsage{InputTokens: &input, TotalTokens: &input})
	input = 8
	if usage == nil || usage.InputTokens == nil || *usage.InputTokens != 3 || *usage.TotalTokens != 3 {
		t.Fatalf("observationTokenUsage() did not detach pointers: %#v", usage)
	}
	parse := observationParseDiagnostics(providersessions.ParseSummary{EventCount: 3, MalformedLineCount: 1, UnknownEventCount: 1, ParseErrors: []providersessions.LineError{
		{LineNumber: 1, Message: "plain diagnostic"},
		{LineNumber: 2, Message: `C:\secret\rollout.json`},
	}})
	if parse.EventCount != 3 || len(parse.Errors) != 2 || parse.Errors[0].Message != "plain diagnostic" || parse.Errors[1].Message == `C:\secret\rollout.json` {
		t.Fatalf("observationParseDiagnostics() = %#v", parse)
	}
}

func TestInvokeSafeDiagnosticMessages(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{"", "provider session parse error"},
		{"password=secret", "provider session parse error"},
		{"a/b", "provider session parse error"},
		{strings.Repeat("x", 300), strings.Repeat("x", 256)},
		{"  ordinary   message ", "ordinary message"},
	} {
		if got := safeDiagnosticMessage(test.input); got != test.want {
			t.Fatalf("safeDiagnosticMessage(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}

func TestInvokeObservationTranscriptHelpers(t *testing.T) {
	boolean, line, text, timestamp, turn := true, 4, "text", time.Unix(10, 0), 2
	entries := transcriptEntries([]providersessions.TranscriptEntry{{
		Arguments: &text, CallID: &text, Encrypted: &boolean, EncryptedContent: &text, LineNumber: &line, Name: &text, Order: 1,
		Output: &text, SourceType: &text, Status: &text, Summary: &text, Text: &text, Timestamp: &timestamp, TurnIndex: &turn,
		Type: providersessions.TranscriptToolOutput,
	}, {Type: providersessions.TranscriptAssistantMessage}})
	if len(entries) != 2 || entries[0].Text == nil || entries[1].Text != nil {
		t.Fatalf("transcriptEntries() = %#v", entries)
	}
	event := projectObservationEvent(events.Record{
		ID: events.RecordID{Topic: "worker-session/worker-1", Position: 2}, SourceType: "worker_session_lifecycle", SourceID: "worker-1", SourceSequence: 2,
		SourceEventID: "terminal", SchemaID: "workers.draft.v1", Payload: []byte(`{"status":"COMPLETED"}`),
	})
	if event.Position != 2 || event.SourceType != "worker_session_lifecycle" || string(event.Payload) == "" {
		t.Fatalf("projectObservationEvent() = %#v", event)
	}
}

func TestInvokeObservationMergeOrdering(t *testing.T) {
	for _, pair := range []struct {
		left  workersessions.Observation
		right workersessions.Observation
		want  workersessions.Observation
	}{
		{left: workersessions.Observation{StartedAt: timePointer(time.Unix(1, 0))}, right: workersessions.Observation{}, want: workersessions.Observation{StartedAt: timePointer(time.Unix(1, 0))}},
		{left: workersessions.Observation{}, right: workersessions.Observation{StartedAt: timePointer(time.Unix(1, 0))}, want: workersessions.Observation{StartedAt: timePointer(time.Unix(1, 0))}},
		{left: workersessions.Observation{AttemptID: "a"}, right: workersessions.Observation{AttemptID: "b"}, want: workersessions.Observation{AttemptID: "a"}},
		{left: workersessions.Observation{WorkerSessionID: "a"}, right: workersessions.Observation{WorkerSessionID: "b"}, want: workersessions.Observation{WorkerSessionID: "a"}},
	} {
		values := []workersessions.Observation{pair.right, pair.left}
		sortObservationAttempts(values)
		if !reflect.DeepEqual(values[0], pair.want) {
			t.Fatalf("sortObservationAttempts(%#v) selected %#v", pair, values[0])
		}
	}
}

func TestInvokeRetryAndObservationBoundaryGuards(t *testing.T) {
	registry := newObservationRegistry(observationProjectorFake{}, nil)
	registry.sessions["worker-1"] = workersessions.Session{ID: "worker-1", State: workersessions.StateCompleted}
	registry.ensureObservation("worker-1", "attempt-1", "turn-1", []string{"work-1"})
	registry.ensureObservation("worker-1", "attempt-2", "turn-2", []string{"work-2"})
	if got := registry.observations["worker-1"].attemptID; got != "attempt-1" {
		t.Fatalf("ensureObservation() overwrote existing attempt = %q", got)
	}

	supervision := newSupervision("dispatch-1", "turn-1")
	if _, prepared := registry.prepareRetryAttempt("missing", supervision); prepared {
		t.Fatal("prepareRetryAttempt(missing) = prepared, want false")
	}
	if _, prepared := registry.prepareRetryAttempt("worker-1", supervision); prepared {
		t.Fatal("prepareRetryAttempt(terminal) = prepared, want false")
	}
	registry.sessions["worker-1"] = workersessions.Session{ID: "worker-1", State: workersessions.StateRunning}
	supervision.controlAction = workersessions.ControlActionCancel
	if _, prepared := registry.prepareRetryAttempt("worker-1", supervision); prepared {
		t.Fatal("prepareRetryAttempt(control-owned) = prepared, want false")
	}

	retryResult := workers.WorkstationDispatchResult{Result: workers.WorkResult{FailureMetadata: &workers.WorkFailureMetadata{Type: workers.WorkFailureTypeTimeout}}}
	retry := newSupervision("dispatch-1", "turn-1")
	retry.retryBudget = 2
	retry.attemptsMade = 1
	if !registry.claimRetryAttempt(retry, "", retryResult, nil) {
		t.Fatal("claimRetryAttempt(retryable) = false, want true")
	}
	retry.retryPending = false
	retry.controlAction = workersessions.ControlActionCancel
	if registry.claimRetryAttempt(retry, "", retryResult, nil) {
		t.Fatal("claimRetryAttempt(control-owned) = true, want false")
	}
	retry.controlAction = ""
	retry.continuing = true
	if registry.claimRetryAttempt(retry, "", retryResult, nil) {
		t.Fatal("claimRetryAttempt(continuing) = true, want false")
	}
	retry.continuing = false
	retry.attemptsMade = retry.retryBudget
	if registry.claimRetryAttempt(retry, "", retryResult, nil) {
		t.Fatal("claimRetryAttempt(exhausted) = true, want false")
	}
}

func TestInvokeObservationProjectionAndTranscriptOutcomes(t *testing.T) {
	ref := observationProviderRef()
	text := "hello"
	providerResult := providersessions.ProjectResult{Detail: providersessions.Detail{
		Transcript: []providersessions.TranscriptEntry{{Order: 0, Type: providersessions.TranscriptAssistantMessage, Text: &text}},
		Parse:      providersessions.ParseSummary{EventCount: 1},
	}}
	provider := observationProjectorFake{result: providerResult}
	registry := newObservationRegistry(provider, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	registry.observations["worker-1"] = observationMetadata()

	listed, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-1"})
	if err != nil || len(listed.Observations) != 1 || listed.Observations[0].DurationBasis != workersessions.DurationBasisActiveClock {
		t.Fatalf("ListObservations() = %#v, %v", listed, err)
	}
	if _, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "missing"}); !errors.Is(err, workersessions.ErrObservationWorkNotFound) {
		t.Fatalf("ListObservations(missing) error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := registry.ListObservations(canceled, workersessions.ListObservationsRequest{WorkID: "work-1"}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("ListObservations(canceled) error = %v", err)
	}

	got, err := registry.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: ref})
	if err != nil || got.WorkerSessionID != "worker-1" || got.Transcript != workersessions.TranscriptAvailabilityAvailable {
		t.Fatalf("GetObservation() = %#v, %v", got, err)
	}
	if _, err := registry.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "missing"}}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("GetObservation(missing) error = %v", err)
	}
	if _, err := registry.GetObservation(canceled, workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("GetObservation(canceled) error = %v", err)
	}
	if _, err := registry.projectObservation(canceled, "worker-1"); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("projectObservation(canceled) error = %v", err)
	}
	if _, err := registry.projectObservation(context.Background(), "missing"); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("projectObservation(missing) error = %v", err)
	}
	noStarted := observationMetadata()
	noStarted.startedAt = time.Time{}
	projected := baseObservation("worker-1", observationSession("worker-1", workersessions.StateRunning), noStarted)
	applyObservationTiming(&projected, observationSession("worker-1", workersessions.StateRunning), noStarted, registry.clock)
	if projected.StartedAt != nil {
		t.Fatalf("applyObservationTiming(zero start) = %#v, want no timing", projected)
	}
}

func TestInvokeObservationProjectionUnavailableOutcomes(t *testing.T) {
	ref := observationProviderRef()
	registry := newObservationRegistry(observationProjectorFake{}, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	registry.observations["worker-1"] = observationMetadata()
	noProvider := newObservationRegistry(nil, nil)
	noProvider.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	noProvider.observations["worker-1"] = observationMetadata()
	if _, err := noProvider.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("GetObservation(without provider service) error = %v", err)
	}
	canceledProvider := newObservationRegistry(observationProjectorFake{err: context.Canceled}, nil)
	canceledProvider.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	canceledProvider.observations["worker-1"] = observationMetadata()
	if _, err := canceledProvider.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("GetObservation(provider canceled) error = %v", err)
	}
	projectionFailure := newObservationRegistry(observationProjectorFake{err: errors.New("projection failed")}, nil)
	projectionFailure.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	projectionFailure.observations["worker-1"] = observationMetadata()
	if _, err := projectionFailure.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-1"}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("ListObservations(projection failure) error = %v", err)
	}
	if _, _, ok := registry.loadObservationState("missing"); ok {
		t.Fatal("loadObservationState(missing) = ok, want false")
	}
	registry.sessions["no-metadata"] = observationSession("no-metadata", workersessions.StateRunning)
	if _, _, ok := registry.loadObservationState("no-metadata"); ok {
		t.Fatal("loadObservationState(missing metadata) = ok, want false")
	}
}

func TestInvokeTranscriptProjectionOutcomes(t *testing.T) {
	ref := observationProviderRef()
	text := "hello"
	providerResult := providersessions.ProjectResult{Detail: providersessions.Detail{
		Transcript: []providersessions.TranscriptEntry{{Order: 0, Type: providersessions.TranscriptAssistantMessage, Text: &text}},
		Parse:      providersessions.ParseSummary{EventCount: 1},
	}}
	provider := observationProjectorFake{result: providerResult}
	terminalRegistry := newObservationRegistry(provider, nil)
	terminalRegistry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	terminalRegistry.observations["worker-1"] = observationMetadata()
	read, err := terminalRegistry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref})
	if err != nil || len(read.Entries) != 1 || read.Entries[0].Text == nil || *read.Entries[0].Text != "hello" {
		t.Fatalf("ReadTranscript() = %#v, %v", read, err)
	}
	active := newObservationRegistry(provider, nil)
	active.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	active.observations["worker-1"] = observationMetadata()
	if _, err := active.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationTranscriptActive) {
		t.Fatalf("ReadTranscript(active) error = %v", err)
	}
	withoutProvider := newObservationRegistry(nil, nil)
	withoutProvider.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	withoutProvider.observations["worker-1"] = observationMetadata()
	if _, err := withoutProvider.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable) {
		t.Fatalf("ReadTranscript(without provider service) error = %v", err)
	}
	missingMetadata := newObservationRegistry(provider, nil)
	missingMetadata.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	if _, err := missingMetadata.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("ReadTranscript(missing metadata) error = %v", err)
	}
}

func TestObservationSubscriptionMapsCanonicalOutcomesAndClosesIdempotently(t *testing.T) {
	record := events.Record{ID: events.RecordID{Topic: "worker-session/worker-1", Position: 1}, SourceType: "worker", SourceID: "source", SourceSequence: 1, SourceEventID: "event", SchemaID: "schema", Payload: []byte(`{}`)}
	terminal := record
	terminal.SourceType = "worker_session_lifecycle"
	terminal.SourceSequence = 2
	terminal.SourceEventID = "terminal"
	cases := []struct {
		name string
		got  events.Delivery
		want workersessions.ObservationDeliveryKind
		err  error
	}{
		{"record", events.Delivery{Kind: events.DeliveryRecord, Record: record}, workersessions.ObservationDeliveryRecord, nil},
		{"terminal", events.Delivery{Kind: events.DeliveryRecord, Record: terminal}, workersessions.ObservationDeliveryTerminal, nil},
		{"canceled", events.Delivery{Kind: events.DeliveryCanceled}, workersessions.ObservationDeliveryCanceled, workersessions.ErrObservationCanceled},
		{"gap", events.Delivery{Kind: events.DeliveryGap, Gap: &events.GapFacts{Topic: record.ID.Topic, Requested: 1, EarliestRetained: 2, Head: 3}}, workersessions.ObservationDeliverySourceFailure, workersessions.ErrObservationSourceGap},
		{"backpressure", events.Delivery{Kind: events.DeliveryBackpressure}, workersessions.ObservationDeliverySourceFailure, workersessions.ErrObservationSourceUnavailable},
		{"closed", events.Delivery{Kind: events.DeliveryClosed}, workersessions.ObservationDeliverySourceFailure, workersessions.ErrObservationSourceClosed},
		{"unknown", events.Delivery{}, workersessions.ObservationDeliverySourceFailure, workersessions.ErrObservationSourceUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			subscription := &observationSubscription{source: events.Subscription(func(context.Context) events.Delivery { return test.got })}
			got := subscription.Next(nil)
			if got.Kind != test.want || !errors.Is(got.Err, test.err) {
				t.Fatalf("Next() = %#v, want kind %q error %v", got, test.want, test.err)
			}
			subscription.Close()
			subscription.Close()
			if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
				t.Fatalf("Next() after Close = %#v, want CLOSED", got)
			}
		})
	}

	replay := &observationSubscription{source: events.Subscription(func(context.Context) events.Delivery {
		return events.Delivery{Kind: events.DeliveryRecord, Record: terminal}
	}), terminalReplay: true}
	if got := replay.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryTerminalReplay {
		t.Fatalf("terminal replay Next() = %#v, want TERMINAL_REPLAY", got)
	}
	closedAfterRecord := &observationSubscription{source: events.Subscription(func(context.Context) events.Delivery {
		return events.Delivery{Kind: events.DeliveryRecord, Record: record}
	})}
	closedAfterRecord.closed = true
	if got := closedAfterRecord.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("closed subscription record Next() = %#v, want CLOSED", got)
	}
	var closesDuringNext *observationSubscription
	closesDuringNext = &observationSubscription{source: events.Subscription(func(context.Context) events.Delivery {
		closesDuringNext.closed = true
		return events.Delivery{Kind: events.DeliveryRecord, Record: record}
	})}
	if got := closesDuringNext.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("subscription closed during Next() = %#v, want CLOSED", got)
	}
	activeCancelCalled := false
	activeCancel := &observationSubscription{activeCancel: func() { activeCancelCalled = true }}
	activeCancel.Close()
	if !activeCancelCalled {
		t.Fatal("Close() did not cancel an active Next")
	}
}

func TestStreamObservationsMapsLookupAndSubscribeErrors(t *testing.T) {
	ref := observationProviderRef()
	withoutReader := newObservationRegistry(observationProjectorFake{}, nil)
	if _, err := withoutReader.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: providers.SessionRef{}}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("StreamObservations(invalid request) error = %v", err)
	}
	withoutReader.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	if _, err := withoutReader.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("StreamObservations(without reader) error = %v", err)
	}
	missing := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{subscription: events.Subscription(func(context.Context) events.Delivery { return events.Delivery{Kind: events.DeliveryClosed} })})
	if _, err := missing.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("StreamObservations(missing session) error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := missing.StreamObservations(canceled, workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("StreamObservations(canceled) error = %v", err)
	}

	active := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{err: errors.New("subscribe failed")})
	active.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	if _, err := active.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("StreamObservations(subscribe failure) error = %v", err)
	}
	canceledReader := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{err: context.Canceled})
	canceledReader.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	if _, err := canceledReader.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("StreamObservations(canceled subscribe) error = %v", err)
	}
	terminal := newObservationRegistry(observationProjectorFake{}, &observationEventReaderFake{subscription: events.Subscription(func(context.Context) events.Delivery { return events.Delivery{Kind: events.DeliveryClosed} })})
	terminal.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	subscription, err := terminal.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref, Limit: 2})
	if err != nil {
		t.Fatalf("StreamObservations(terminal) error = %v", err)
	}
	subscription.Close()
}

func TestReadTranscriptMapsProviderProjectionErrors(t *testing.T) {
	ref := observationProviderRef()
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"canceled", context.Canceled, workersessions.ErrObservationCanceled},
		{"provider canceled", providersessions.ErrOperationCanceled, workersessions.ErrObservationCanceled},
		{"source unavailable", providersessions.ErrSessionNotFound, workersessions.ErrObservationTranscriptUnavailable},
		{"projection failure", errors.New("projection failed"), workersessions.ErrObservationTranscriptProjectionUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry := newObservationRegistry(observationProjectorFake{err: test.err}, nil)
			registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
			registry.observations["worker-1"] = observationMetadata()
			_, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref})
			if !errors.Is(err, test.want) {
				t.Fatalf("ReadTranscript() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadTranscriptRejectsInvalidRequestAndProjection(t *testing.T) {
	ref := observationProviderRef()
	registry := newObservationRegistry(observationProjectorFake{result: providersessions.ProjectResult{Detail: providersessions.Detail{Transcript: []providersessions.TranscriptEntry{{Order: 0}}}}}, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateCompleted)
	registry.observations["worker-1"] = observationMetadata()
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("ReadTranscript(invalid request) error = %v", err)
	}
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); err == nil {
		t.Fatal("ReadTranscript(invalid projected entry) error = nil, want validation error")
	}
}

func timePointer(value time.Time) *time.Time { return &value }
