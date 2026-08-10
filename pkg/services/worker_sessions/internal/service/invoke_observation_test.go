package service

import (
	"context"
	"errors"
	"fmt"
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
	subscription   events.Subscription
	err            error
	readResults    []events.ReadResult
	readErr        error
	readFunc       func(context.Context, events.ReadRequest) (events.ReadResult, error)
	subscribeCalls int
	readCalls      int
}

func (f *observationEventReaderFake) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	f.subscribeCalls++
	return f.subscription, f.err
}

func (f *observationEventReaderFake) Read(ctx context.Context, req events.ReadRequest) (events.ReadResult, error) {
	f.readCalls++
	if f.readFunc != nil {
		return f.readFunc(ctx, req)
	}
	if f.readErr != nil {
		return events.ReadResult{}, f.readErr
	}
	if len(f.readResults) == 0 {
		return events.ReadResult{}, errors.New("observation reader fake: no read result")
	}
	result := f.readResults[0]
	f.readResults = f.readResults[1:]
	return result, nil
}

func newObservationRegistry(provider providersessions.Service, reader EventsReader) *registry {
	registry := &registry{
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
	if retainedReader, ok := reader.(EventsRetainedReader); ok {
		registry.retainedReader = retainedReader
	}
	return registry
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
	assertWorkerObservationLookups(t, registry, got, canceled)
	if _, err := registry.GetObservation(context.Background(), workersessions.GetObservationRequest{ProviderSession: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "missing"}}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("GetObservation(missing) error = %v", err)
	}
	if _, err := registry.GetObservation(canceled, workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("GetObservation(canceled) error = %v", err)
	}
	assertObservationProjectionEdges(t, registry, canceled)
	noStarted := observationMetadata()
	noStarted.startedAt = time.Time{}
	projected := baseObservation("worker-1", observationSession("worker-1", workersessions.StateRunning), noStarted)
	applyObservationTiming(&projected, observationSession("worker-1", workersessions.StateRunning), noStarted, registry.clock)
	if projected.StartedAt != nil {
		t.Fatalf("applyObservationTiming(zero start) = %#v, want no timing", projected)
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

func TestStreamObservationsReplayOnlyUsesReadSnapshotWithoutSubscriber(t *testing.T) {
	ref := observationProviderRef()
	topic := workersessions.Topic("worker-1")
	reader := &observationEventReaderFake{
		readResults: []events.ReadResult{
			{
				Outcome: events.ReadOutcomeProgress,
				Records: []events.Record{
					replayObservationRecord(topic, 1, "event-1"),
					replayObservationRecord(topic, 2, "event-2"),
				},
				Next:     events.Cursor{Topic: topic, Position: 2},
				Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 3},
			},
			{
				Outcome: events.ReadOutcomeProgress,
				// Position 4 was appended after the captured head. It is
				// deliberately returned by the fake to prove the replay
				// subscription truncates the page at position 3.
				Records: []events.Record{
					replayObservationRecord(topic, 3, "event-3"),
					replayObservationRecord(topic, 4, "event-4"),
				},
				Next:     events.Cursor{Topic: topic, Position: 4},
				Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 4},
			},
		},
		subscription: events.Subscription(func(context.Context) events.Delivery {
			t.Fatal("replay-only registered a live Events subscriber")
			return events.Delivery{}
		}),
	}
	registry := newObservationRegistry(observationProjectorFake{}, reader)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)

	subscription, err := registry.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{
		ProviderSession: ref,
		ReplayOnly:      true,
		Limit:           2,
	})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	defer subscription.Close()

	var positions []uint64
	for range 3 {
		delivery := subscription.Next(context.Background())
		if delivery.Kind != workersessions.ObservationDeliveryRecord {
			t.Fatalf("replay delivery = %#v, want RECORD", delivery)
		}
		positions = append(positions, delivery.Event.Position)
	}
	if got, want := fmt.Sprint(positions), "[1 2 3]"; got != want {
		t.Fatalf("replayed positions = %s, want %s", got, want)
	}
	summary := subscription.Next(context.Background())
	if summary.Kind != workersessions.ObservationDeliveryReplaySummary || summary.Summary == nil {
		t.Fatalf("summary delivery = %#v, want REPLAY_SUMMARY", summary)
	}
	if summary.Summary.Complete || summary.Summary.Reason != "session-active" || summary.Summary.EventsEmitted != 3 {
		t.Fatalf("summary = %#v, want incomplete active replay with count 3", summary.Summary)
	}
	if reader.subscribeCalls != 0 {
		t.Fatalf("Subscribe() calls = %d, want 0 for replay-only", reader.subscribeCalls)
	}
	if reader.readCalls != 2 {
		t.Fatalf("Read() calls = %d, want initial snapshot plus one page", reader.readCalls)
	}
}

func TestStreamObservationsReplayOnlyEmitsSummaryForEmptyActiveTopic(t *testing.T) {
	ref := observationProviderRef()
	topic := workersessions.Topic("worker-1")
	reader := &observationEventReaderFake{readResults: []events.ReadResult{{
		Outcome:  events.ReadOutcomeAtHead,
		Next:     events.Cursor{Topic: topic},
		Retained: events.RetainedRange{Topic: topic},
	}}}
	registry := newObservationRegistry(observationProjectorFake{}, reader)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)

	subscription, err := registry.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{
		ProviderSession: ref,
		ReplayOnly:      true,
	})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	defer subscription.Close()
	summary := subscription.Next(context.Background())
	if summary.Kind != workersessions.ObservationDeliveryReplaySummary || summary.Summary == nil {
		t.Fatalf("summary delivery = %#v, want empty replay summary", summary)
	}
	if summary.Summary.Complete || summary.Summary.Reason != "session-active" || summary.Summary.EventsEmitted != 0 {
		t.Fatalf("summary = %#v, want zero-event incomplete active replay", summary.Summary)
	}
	if reader.subscribeCalls != 0 {
		t.Fatalf("Subscribe() calls = %d, want 0 for replay-only", reader.subscribeCalls)
	}
}

func TestStreamObservationsByWorkerSessionIDUsesCanonicalTopic(t *testing.T) {
	topic := workersessions.Topic("worker-no-reference")
	opening := replayObservationRecord(topic, 1, "opening")
	terminal := replayObservationRecord(topic, 2, "terminal")
	terminal.SourceType = "worker_session_lifecycle"
	terminal.SourceSequence = 2
	terminal.SourceEventID = "terminal"
	terminal.Payload = []byte(`{"kind":"SESSION","phase":"COMPLETED","payload":{"status":"COMPLETED"}}`)
	reader := &observationEventReaderFake{readResults: []events.ReadResult{{
		Outcome:  events.ReadOutcomeProgress,
		Records:  []events.Record{opening},
		Next:     events.Cursor{Topic: topic, Position: 1},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 2},
	}, {
		Outcome:  events.ReadOutcomeProgress,
		Records:  []events.Record{terminal},
		Next:     events.Cursor{Topic: topic, Position: 2},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 2},
	}}}
	registry := newObservationRegistry(nil, reader)
	registry.sessions["worker-no-reference"] = workersessions.Session{ID: "worker-no-reference", State: workersessions.StateCompleted}

	subscription, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "worker-no-reference",
		ReplayOnly:      true,
	})
	if err != nil {
		t.Fatalf("StreamObservationsByWorkerSessionID() error = %v", err)
	}
	defer subscription.Close()
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.Position != 1 {
		t.Fatalf("Worker Session identity stream opening delivery = %#v, want record position 1", delivery)
	}
	if delivery := subscription.Next(context.Background()); delivery.Kind != workersessions.ObservationDeliveryTerminalReplay || delivery.Event.Position != 2 {
		t.Fatalf("Worker Session identity stream terminal delivery = %#v, want terminal replay position 2", delivery)
	}
	if summary := subscription.Next(context.Background()); summary.Kind != workersessions.ObservationDeliveryReplaySummary || summary.Summary == nil || !summary.Summary.Complete {
		t.Fatalf("Worker Session identity stream summary = %#v, want complete replay summary", summary)
	}
}

func TestReplayObservationSubscriptionRejectsInitialReadFailures(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	valid := replayProgressResult(topic, 1, 1)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name   string
		ctx    context.Context
		reader EventsRetainedReader
		want   error
	}{
		{
			name: "nil context uses background",
			ctx:  nil,
			reader: &observationEventReaderFake{
				readResults: []events.ReadResult{valid},
			},
			want: nil,
		},
		{
			name: "canceled context",
			ctx:  canceled,
			reader: &observationEventReaderFake{
				readResults: []events.ReadResult{valid},
			},
			want: context.Canceled,
		},
		{
			name: "missing reader",
			ctx:  context.Background(),
			want: workersessions.ErrObservationSourceUnavailable,
		},
		{
			name: "read canceled",
			ctx:  context.Background(),
			reader: &observationEventReaderFake{
				readErr: context.Canceled,
			},
			want: workersessions.ErrObservationCanceled,
		},
		{
			name: "read gap",
			ctx:  context.Background(),
			reader: &observationEventReaderFake{
				readErr: workersessions.ErrObservationSourceGap,
			},
			want: workersessions.ErrObservationSourceGap,
		},
		{
			name: "read failure",
			ctx:  context.Background(),
			reader: &observationEventReaderFake{
				readErr: errors.New("read failed"),
			},
			want: workersessions.ErrObservationSourceUnavailable,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assertReplayInitialization(t, test.ctx, test.reader, topic, test.want)
		})
	}
}

func TestReplayObservationSubscriptionRejectsInitialReadResults(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	otherTopic := workersessions.Topic("worker-2")
	validGap := events.ReadResult{
		Outcome: events.ReadOutcomeGap,
		Gap: &events.GapFacts{
			Topic:            topic,
			Requested:        0,
			EarliestRetained: 1,
			Head:             1,
		},
	}
	cases := []struct {
		name   string
		result events.ReadResult
		want   error
	}{
		{name: "invalid read result", result: events.ReadResult{}, want: workersessions.ErrObservationSourceUnavailable},
		{name: "retention gap result", result: validGap, want: workersessions.ErrObservationSourceGap},
		{name: "result topic mismatch", result: replayProgressResult(otherTopic, 1, 1), want: workersessions.ErrObservationSourceUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reader := &observationEventReaderFake{readResults: []events.ReadResult{test.result}}
			assertReplayInitialization(t, context.Background(), reader, topic, test.want)
		})
	}
}

func assertReplayInitialization(t *testing.T, ctx context.Context, reader EventsRetainedReader, topic events.Topic, want error) {
	t.Helper()
	got, err := newReplayObservationSubscription(ctx, reader, topic, workersessions.StateRunning, 1)
	if want == nil {
		if err != nil || got == nil {
			t.Fatalf("newReplayObservationSubscription() = %v, %v, want success", got, err)
		}
		return
	}
	if got != nil || !errors.Is(err, want) {
		t.Fatalf("newReplayObservationSubscription() = %v, %v, want error %v", got, err, want)
	}
}

func TestStreamObservationsReplayOnlyMapsInitialReadFailures(t *testing.T) {
	ref := observationProviderRef()
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "canceled", err: context.Canceled, want: workersessions.ErrObservationCanceled},
		{name: "source failure", err: errors.New("read failed"), want: workersessions.ErrObservationSourceUnavailable},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reader := &observationEventReaderFake{readErr: test.err}
			registry := newObservationRegistry(observationProjectorFake{}, reader)
			registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
			_, err := registry.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{
				ProviderSession: ref,
				ReplayOnly:      true,
				Limit:           1,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("StreamObservations(replay-only) error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReplayObservationSubscriptionEmitsTypedReadFailures(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	cases := []struct {
		name      string
		second    events.ReadResult
		secondErr error
		wantKind  workersessions.ObservationDeliveryKind
		wantErr   error
	}{
		{
			name:      "read canceled",
			secondErr: context.Canceled,
			wantKind:  workersessions.ObservationDeliveryCanceled,
			wantErr:   workersessions.ErrObservationCanceled,
		},
		{
			name:      "read failure",
			secondErr: errors.New("read failed"),
			wantKind:  workersessions.ObservationDeliverySourceFailure,
			wantErr:   workersessions.ErrObservationSourceUnavailable,
		},
		{
			name:     "invalid cursor",
			second:   events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor},
			wantKind: workersessions.ObservationDeliverySourceFailure,
			wantErr:  workersessions.ErrObservationSourceUnavailable,
		},
		{
			name: "gap",
			second: events.ReadResult{
				Outcome: events.ReadOutcomeGap,
				Gap: &events.GapFacts{
					Topic:            topic,
					Requested:        0,
					EarliestRetained: 1,
					Head:             2,
				},
			},
			wantKind: workersessions.ObservationDeliverySourceFailure,
			wantErr:  workersessions.ErrObservationSourceGap,
		},
		{
			name:     "invalid progress",
			second:   events.ReadResult{Outcome: events.ReadOutcomeProgress},
			wantKind: workersessions.ObservationDeliverySourceFailure,
			wantErr:  workersessions.ErrObservationSourceUnavailable,
		},
		{
			name:     "non contiguous progress",
			second:   replayProgressResult(topic, 3, 3),
			wantKind: workersessions.ObservationDeliverySourceFailure,
			wantErr:  workersessions.ErrObservationSourceUnavailable,
		},
		{
			name: "at head before snapshot",
			second: events.ReadResult{
				Outcome:  events.ReadOutcomeAtHead,
				Next:     events.Cursor{Topic: topic, Position: 1},
				Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 1},
			},
			wantKind: workersessions.ObservationDeliverySourceFailure,
			wantErr:  workersessions.ErrObservationSourceUnavailable,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reader := &observationEventReaderFake{}
			reads := 0
			reader.readFunc = func(context.Context, events.ReadRequest) (events.ReadResult, error) {
				reads++
				if reads == 1 {
					return initial, nil
				}
				return test.second, test.secondErr
			}
			subscription, err := newReplayObservationSubscription(context.Background(), reader, topic, workersessions.StateRunning, 1)
			if err != nil {
				t.Fatalf("newReplayObservationSubscription() error = %v", err)
			}
			if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryRecord {
				t.Fatalf("initial delivery = %#v, want RECORD", got)
			}
			got := subscription.Next(context.Background())
			if got.Kind != test.wantKind || !errors.Is(got.Err, test.wantErr) {
				t.Fatalf("failure delivery = %#v, want kind %q error %v", got, test.wantKind, test.wantErr)
			}
		})
	}
}

func TestReplayObservationSubscriptionCompletesAndCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	terminalPage := replayProgressResult(topic, 2, 2)
	terminalPage.Records[0].SourceType = lifecycleSourceType
	terminalPage.Records[0].SourceSequence = terminalSourceSequence
	terminalPage.Records[0].SourceEventID = terminalSourceEventID
	reader := &observationEventReaderFake{readResults: []events.ReadResult{initial, terminalPage}}
	subscription, err := newReplayObservationSubscription(nil, reader, topic, workersessions.StateCompleted, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription() error = %v", err)
	}
	if got := subscription.Next(nil); got.Kind != workersessions.ObservationDeliveryRecord {
		t.Fatalf("initial delivery = %#v, want RECORD", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryTerminalReplay {
		t.Fatalf("terminal delivery = %#v, want TERMINAL_REPLAY", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryReplaySummary || got.Summary == nil || !got.Summary.Complete {
		t.Fatalf("completion delivery = %#v, want complete summary", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after summary = %#v, want CLOSED", got)
	}
	subscription.Close()
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after Close() = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionCancellationCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	subscription, err := newReplayObservationSubscription(context.Background(), &observationEventReaderFake{readResults: []events.ReadResult{initial}}, topic, workersessions.StateRunning, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription(canceled) error = %v", err)
	}
	if got := subscription.Next(canceled); got.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(got.Err, workersessions.ErrObservationCanceled) {
		t.Fatalf("canceled delivery = %#v, want CANCELED", got)
	}
	if got := subscription.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after cancellation = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionCloseDuringReadCloses(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	initial := replayProgressResult(topic, 2, 1)
	var racing *replayObservationSubscription
	reads := 0
	racingReader := &observationEventReaderFake{}
	racingReader.readFunc = func(context.Context, events.ReadRequest) (events.ReadResult, error) {
		reads++
		if reads == 1 {
			return initial, nil
		}
		racing.Close()
		return replayProgressResult(topic, 2, 2), nil
	}
	var err error
	racing, err = newReplayObservationSubscription(context.Background(), racingReader, topic, workersessions.StateRunning, 1)
	if err != nil {
		t.Fatalf("newReplayObservationSubscription(racing) error = %v", err)
	}
	if got := racing.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryRecord {
		t.Fatalf("racing initial delivery = %#v, want RECORD", got)
	}
	if got := racing.Next(context.Background()); got.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("racing delivery = %#v, want CLOSED", got)
	}
}

func TestReplayObservationSubscriptionRejectsSnapshotInvariantViolations(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	subscription := &replayObservationSubscription{
		topic:        topic,
		snapshotHead: 1,
		next:         events.Cursor{Topic: topic, Position: 1},
	}
	result := replayProgressResult(topic, 2, 2)
	if err := subscription.appendPage(result); !errors.Is(err, workersessions.ErrObservationSourceUnavailable) {
		t.Fatalf("appendPage(after snapshot) error = %v, want source unavailable", err)
	}
}

func replayProgressResult(topic events.Topic, head uint64, positions ...uint64) events.ReadResult {
	records := make([]events.Record, 0, len(positions))
	for _, position := range positions {
		records = append(records, replayObservationRecord(topic, position, fmt.Sprintf("event-%d", position)))
	}
	return events.ReadResult{
		Outcome:  events.ReadOutcomeProgress,
		Records:  records,
		Next:     events.Cursor{Topic: topic, Position: events.AggregateSequence(positions[len(positions)-1])},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: events.AggregateSequence(head)},
	}
}

func replayObservationRecord(topic events.Topic, position uint64, eventID string) events.Record {
	return events.Record{
		ID:             events.RecordID{Topic: topic, Position: events.AggregateSequence(position)},
		SourceType:     "worker_observation",
		SourceID:       "provider-session-1",
		SourceSequence: events.SourceSequence(position),
		SourceEventID:  events.SourceEventID(eventID),
		SchemaID:       "worker_session.observation",
		Payload:        []byte(`{"position":1}`),
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
