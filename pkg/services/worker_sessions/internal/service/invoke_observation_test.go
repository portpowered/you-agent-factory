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

type trackingObservationProjector struct {
	providersessions.Service
	result  providersessions.ProjectResult
	request providersessions.ProjectRequest
}

func (f *trackingObservationProjector) Project(request providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	f.request = request
	return f.result, nil
}

type observationEventReaderFake struct {
	subscription   events.Subscription
	err            error
	readResults    []events.ReadResult
	readErr        error
	readFunc       func(context.Context, events.ReadRequest) (events.ReadResult, error)
	subscribeCalls int
	readCalls      int
	lastSubscribe  events.SubscribeRequest
	readRequests   []events.ReadRequest
}

func (f *observationEventReaderFake) Subscribe(_ context.Context, request events.SubscribeRequest) (events.Subscription, error) {
	f.subscribeCalls++
	f.lastSubscribe = request
	return f.subscription, f.err
}

func (f *observationEventReaderFake) Read(ctx context.Context, req events.ReadRequest) (events.ReadResult, error) {
	f.readCalls++
	f.readRequests = append(f.readRequests, req)
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
	registry.sessions["worker-factory"] = workersessions.Session{ID: "worker-factory", State: workersessions.StateRunning}
	registry.ensureObservationWithFactorySession("worker-factory", "attempt-factory", "turn-factory", []string{"work-factory"}, false, " session-factory ")
	projectedFactory := baseObservation("worker-factory", registry.sessions["worker-factory"], registry.observations["worker-factory"])
	if projectedFactory.FactorySessionID != "session-factory" {
		t.Fatalf("Factory Session attribution = %q, want trimmed session-factory", projectedFactory.FactorySessionID)
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
		Parse:      providersessions.ParseSummary{EventCount: 1, CumulativeInputTokens: []int{100, 250, 700}},
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
	assertObservationTurnUsage(t, got)
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

func TestListWorkerSessionObservationsUsesFleetDefaultFiltersAndCursor(t *testing.T) {
	registry := newObservationRegistry(nil, nil)
	registry.sessions["direct-a"] = workersessions.Session{ID: "direct-a", State: workersessions.StateCompleted}
	metadataA := observationMetadata()
	metadataA.direct = true
	registry.observations["direct-a"] = metadataA
	registry.sessions["direct-b"] = workersessions.Session{ID: "direct-b", State: workersessions.StateRunning}
	metadataB := observationMetadata()
	metadataB.direct = true
	registry.observations["direct-b"] = metadataB
	registry.sessions["factory-a"] = workersessions.Session{ID: "factory-a", State: workersessions.StateCompleted}
	registry.observations["factory-a"] = observationMetadata()

	first, err := registry.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 1})
	assertObservationListPage(t, "fleet first", first, err, "direct-a", true, true)
	second, err := registry.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 1, NextToken: first.NextToken})
	assertObservationListPage(t, "fleet second", second, err, "direct-b", true, true)
	third, err := registry.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{MaxResults: 1, NextToken: second.NextToken})
	assertObservationListPage(t, "fleet third", third, err, "factory-a", false, false)
	factory, err := registry.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{Scope: workersessions.ObservationScopeFactory, States: []workersessions.State{workersessions.StateCompleted}})
	assertObservationListPage(t, "factory filtered", factory, err, "factory-a", false, false)
	filtered, err := registry.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{
		States:     []workersessions.State{workersessions.StateRunning, workersessions.StateCompleted},
		MaxResults: 2,
	})
	if err != nil || len(filtered.Observations) != 2 || filtered.Observations[0].WorkerSessionID != "direct-a" || filtered.Observations[1].WorkerSessionID != "direct-b" || filtered.NextToken == "" {
		t.Fatalf("OR-filtered fleet page = %#v, %v, want first two matching IDs and a cursor", filtered, err)
	}
	if _, err := registry.ListWorkerSessionObservations(context.Background(), workersessions.ListWorkerSessionObservationsRequest{NextToken: "not-base64"}); !errors.Is(err, workersessions.ErrInvalidObservationPagination) {
		t.Fatalf("invalid cursor error = %v, want ErrInvalidObservationPagination", err)
	}
}

func TestListWorkerSessionObservationsKeepsBaseFactsWhenProviderProjectionIsUnavailable(t *testing.T) {
	registry := newObservationRegistry(nil, nil)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	registry.observations["worker-1"] = observationMetadata()

	result, err := registry.ListWorkerSessionObservations(
		context.Background(),
		workersessions.ListWorkerSessionObservationsRequest{},
	)
	if !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("top-level list error = %v, want optional projection error", err)
	}
	if len(result.Observations) != 1 || result.Observations[0].WorkerSessionID != "worker-1" {
		t.Fatalf("top-level list result = %#v, want preserved base observation", result)
	}
	if result.Observations[0].State != workersessions.StateRunning || result.Observations[0].WorkIDs[0] != "work-1" {
		t.Fatalf("preserved base observation = %#v, want lifecycle and Work facts", result.Observations[0])
	}
}

func assertObservationListPage(t *testing.T, label string, result workersessions.ListWorkerSessionObservationsResult, err error, wantID string, wantDirect, wantCursor bool) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s error = %v", label, err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("%s observations = %#v, want one", label, result.Observations)
	}
	observation := result.Observations[0]
	if observation.WorkerSessionID != wantID {
		t.Fatalf("%s worker session = %q, want %q", label, observation.WorkerSessionID, wantID)
	}
	if observation.Direct != wantDirect {
		t.Fatalf("%s direct = %t, want %t", label, observation.Direct, wantDirect)
	}
	if (result.NextToken != "") != wantCursor {
		t.Fatalf("%s next token = %q, want cursor=%t", label, result.NextToken, wantCursor)
	}
}

func TestReadTranscriptByWorkerSessionIDResolvesRecordedAssociationAndLifecycle(t *testing.T) {
	text := "continued"
	projector := &trackingObservationProjector{result: providersessions.ProjectResult{Detail: providersessions.Detail{
		Transcript: []providersessions.TranscriptEntry{{Order: 1, Type: providersessions.TranscriptAssistantMessage, Text: &text}},
	}}}
	registry := newObservationRegistry(projector, nil)
	registry.sessions["direct-1"] = observationSession("direct-1", workersessions.StateCompleted)
	registry.observations["direct-1"] = observationMetadata()
	result, err := registry.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "direct-1"})
	if err != nil || result.WorkerSessionID != "direct-1" || len(result.Entries) != 1 || result.Entries[0].Text == nil || *result.Entries[0].Text != text {
		t.Fatalf("identity transcript = %#v, %v, want normalized entry", result, err)
	}
	if projector.request.Session != observationProviderRef() {
		t.Fatalf("projector reference = %#v, want recorded association %v", projector.request.Session, observationProviderRef())
	}
	active := newObservationRegistry(projector, nil)
	active.sessions["direct-active"] = workersessions.Session{ID: "direct-active", State: workersessions.StateRunning}
	active.observations["direct-active"] = observationMetadata()
	if _, err := active.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "direct-active"}); !errors.Is(err, workersessions.ErrObservationTranscriptActive) {
		t.Fatalf("active identity transcript error = %v, want ErrObservationTranscriptActive", err)
	}
	if _, err := registry.ReadTranscriptByWorkerSessionID(context.Background(), workersessions.ReadTranscriptByWorkerSessionIDRequest{WorkerSessionID: "missing"}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("missing identity transcript error = %v, want ErrObservationSessionNotFound", err)
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

func TestStreamObservationsByWorkerSessionIDResumesWithScopedCursor(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	record := replayObservationRecord(topic, 2, "event-2")
	reader := &observationEventReaderFake{readResults: []events.ReadResult{{
		Outcome:  events.ReadOutcomeProgress,
		Records:  []events.Record{record},
		Next:     events.Cursor{Topic: topic, Position: 2},
		Retained: events.RetainedRange{Topic: topic, Earliest: 1, Head: 2},
	}}}
	registry := newObservationRegistry(nil, reader)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)

	subscription, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "worker-1",
		ReplayOnly:      true,
		Cursor:          &workersessions.ObservationCursor{WorkerSessionID: "worker-1", Position: 1},
	})
	if err != nil {
		t.Fatalf("cursor stream = %v", err)
	}
	defer subscription.Close()
	if len(reader.readRequests) != 1 || reader.readRequests[0].From.Position != 1 {
		t.Fatalf("read request = %#v, want exclusive position 1", reader.readRequests)
	}
	delivery := subscription.Next(context.Background())
	if delivery.Kind != workersessions.ObservationDeliveryRecord || delivery.Event.Position != 2 {
		t.Fatalf("resumed delivery = %#v, want record position 2", delivery)
	}
	if delivery.Event.Cursor.WorkerSessionID != "worker-1" || delivery.Event.Cursor.Position != 2 {
		t.Fatalf("resumed cursor = %#v, want worker-1/2", delivery.Event.Cursor)
	}
}

func TestStreamObservationsByWorkerSessionIDClassifiesCursorFailures(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	gap := events.ReadResult{
		Outcome: events.ReadOutcomeGap,
		Gap: &events.GapFacts{
			Topic: topic, Requested: 1, EarliestRetained: 2, Head: 3,
		},
	}
	for _, test := range []struct {
		name   string
		cursor workersessions.ObservationCursor
		result events.ReadResult
		want   error
	}{
		{name: "foreign", cursor: workersessions.ObservationCursor{WorkerSessionID: "worker-2", Position: 1}, want: workersessions.ErrObservationCursorForeign},
		{name: "future", cursor: workersessions.ObservationCursor{WorkerSessionID: "worker-1", Position: 3}, result: events.ReadResult{Outcome: events.ReadOutcomeInvalidCursor}, want: workersessions.ErrObservationCursorFuture},
		{name: "stale", cursor: workersessions.ObservationCursor{WorkerSessionID: "worker-1", Position: 1}, result: gap, want: workersessions.ErrObservationCursorStale},
		{name: "generation", cursor: workersessions.ObservationCursor{WorkerSessionID: "worker-1", StreamGenerationID: "generation-1", Position: 1}, want: workersessions.ErrObservationCursorUnavailable},
		{name: "malformed", cursor: workersessions.ObservationCursor{WorkerSessionID: "worker-1"}, want: workersessions.ErrInvalidObservationCursor},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &observationEventReaderFake{}
			if test.result.Outcome != events.ReadOutcomeUnspecified {
				reader.readResults = []events.ReadResult{test.result}
			}
			registry := newObservationRegistry(nil, reader)
			registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
			_, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
				WorkerSessionID: "worker-1", ReplayOnly: true, Cursor: &test.cursor,
			})
			if !errors.Is(err, test.want) {
				t.Fatalf("cursor error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestStreamObservationsByWorkerSessionIDPassesCursorToLiveSubscribe(t *testing.T) {
	topic := workersessions.Topic("worker-1")
	reader := &observationEventReaderFake{
		subscription: events.Subscription(func(context.Context) events.Delivery {
			return events.Delivery{Kind: events.DeliveryClosed}
		}),
	}
	registry := newObservationRegistry(nil, reader)
	registry.sessions["worker-1"] = observationSession("worker-1", workersessions.StateRunning)
	_, err := registry.StreamObservationsByWorkerSessionID(context.Background(), workersessions.StreamObservationsByWorkerSessionIDRequest{
		WorkerSessionID: "worker-1",
		Cursor:          &workersessions.ObservationCursor{WorkerSessionID: "worker-1", Position: 4},
	})
	if err != nil {
		t.Fatalf("live cursor stream = %v", err)
	}
	if reader.lastSubscribe.Topic != topic || reader.lastSubscribe.From.Position != 4 {
		t.Fatalf("subscribe request = %#v, want worker-1 after position 4", reader.lastSubscribe)
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
