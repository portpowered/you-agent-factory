package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type observationProjectionStub struct {
	providersessions.Service
	results    map[providers.SessionRef]providersessions.ProjectResult
	projectErr error
	requested  []providers.SessionRef
}

func (s *observationProjectionStub) Project(req providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	s.requested = append(s.requested, req.Session)
	if s.projectErr != nil {
		return providersessions.ProjectResult{}, s.projectErr
	}
	return s.results[req.Session], nil
}

func sessionRef(id string) providers.SessionRef {
	return providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       id,
	}
}

func providerMetadata(ref providers.SessionRef) *workers.ProviderSessionMetadata {
	return &workers.ProviderSessionMetadata{
		Provider: string(ref.Provider),
		Kind:     ref.Kind,
		ID:       ref.ID,
	}
}

func projectedDetail(ref providers.SessionRef) providersessions.ProjectResult {
	inputTokens := 11
	outputTokens := 7
	totalTokens := 18
	text := "normalized assistant response"
	return providersessions.ProjectResult{
		Session: ref,
		Detail: providersessions.Detail{
			ProviderSession: providersessions.Ref{
				Provider: providersessions.ProviderCodex,
				Kind:     ref.Kind,
				ID:       ref.ID,
			},
			Parse: providersessions.ParseSummary{
				EventCount:         4,
				MalformedLineCount: 1,
				UnknownEventCount:  2,
				ParseErrors: []providersessions.LineError{{
					LineNumber: 9,
					Message:    "malformed provider event",
				}},
				TokenUsage: &providersessions.TokenUsage{
					InputTokens:  &inputTokens,
					OutputTokens: &outputTokens,
					TotalTokens:  &totalTokens,
				},
			},
			Transcript: []providersessions.TranscriptEntry{{
				Order: 0,
				Text:  &text,
				Type:  providersessions.TranscriptAssistantMessage,
			}},
		},
	}
}

func newObservationService(
	t *testing.T,
	boundary workers.WorkstationPoolBoundary,
	eventsAppender workersessionservice.EventsAppender,
	clock platformclock.Source,
	projection providersessions.Service,
) workersessions.Service {
	t.Helper()
	if projection == nil {
		projection = unavailableProviderSessions{}
	}
	service, err := workersessionservice.New(boundary, eventsAppender, logging.NoopLogger{}, clock, projection)
	if err != nil {
		t.Fatalf("worker session service construction: %v", err)
	}
	return service
}

func startRequest(id, dispatchID, workID string) workersessions.InvokeSessionRequest {
	request := validStartRequest(id, dispatchID)
	request.Execution.Execution.Dispatch.Execution.RequestID = "turn-" + id
	request.Execution.Execution.Dispatch.Execution.WorkIDs = []string{workID}
	return request
}

func executionFor(
	ref *providers.SessionRef,
	outcome workers.WorkOutcome,
	metadata *workers.WorkFailureMetadata,
	beforeReturn func(string),
) *fakeExecution {
	return &fakeExecution{
		dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			dispatchID := request.Execution.Dispatch.DispatchID
			if beforeReturn != nil {
				beforeReturn(dispatchID)
			}
			terminalOutcome := workers.WorkstationDispatchTerminalOutcomeCompleted
			if outcome != workers.OutcomeAccepted && outcome != workers.OutcomeContinue {
				terminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
			}
			result := workers.WorkResult{
				DispatchID:      dispatchID,
				Outcome:         outcome,
				FailureMetadata: metadata,
			}
			if ref != nil {
				result.ProviderSession = providerMetadata(*ref)
			}
			return workers.WorkstationDispatchResult{
				DispatchID:      dispatchID,
				WorkstationName: request.WorkstationName,
				TerminalOutcome: terminalOutcome,
				Result:          result,
			}, nil
		},
	}
}

func TestObservationProjection_ListsCorrelatedAttemptsAndNormalizedFacts(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	eventsAppender := newEventsAppender()
	firstRef := sessionRef("provider-session-a")
	secondRef := sessionRef("provider-session-b")
	projection := &observationProjectionStub{results: map[providers.SessionRef]providersessions.ProjectResult{
		firstRef:  projectedDetail(firstRef),
		secondRef: projectedDetail(secondRef),
	}}
	failureMetadata := &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureTypeAuthFailure,
	}
	execution := executionFor(nil, workers.OutcomeAccepted, nil, func(dispatchID string) {
		if dispatchID == "dispatch-a" {
			clock.SetTick(3)
		}
	})
	execution.dispatch = func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
		dispatchID := request.Execution.Dispatch.DispatchID
		if dispatchID == "dispatch-a" {
			clock.SetTick(3)
			return workers.WorkstationDispatchResult{
				DispatchID:      dispatchID,
				WorkstationName: request.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID:      dispatchID,
					Outcome:         workers.OutcomeAccepted,
					ProviderSession: providerMetadata(firstRef),
				},
			}, nil
		}
		clock.SetTick(9)
		return workers.WorkstationDispatchResult{
			DispatchID:      dispatchID,
			WorkstationName: request.WorkstationName,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
			Result: workers.WorkResult{
				DispatchID:      dispatchID,
				Outcome:         workers.OutcomeFailed,
				FailureMetadata: failureMetadata,
				ProviderSession: providerMetadata(secondRef),
			},
		}, nil
	}
	registry := newObservationService(t, executionBoundary{execution: execution}, eventsAppender, clock, projection)

	clock.SetTick(1)
	if _, err := registry.InvokeSession(context.Background(), startRequest("worker-b", "dispatch-a", "work-1")); err != nil {
		t.Fatalf("Start(first) error = %v", err)
	}
	clock.SetTick(5)
	if _, err := registry.InvokeSession(context.Background(), startRequest("worker-a", "dispatch-b", "work-1")); err != nil {
		t.Fatalf("Start(second) error = %v", err)
	}
	clock.SetTick(20)

	result, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-1"})
	if err != nil {
		t.Fatalf("ListObservations() error = %v", err)
	}
	if len(result.Observations) != 2 {
		t.Fatalf("ListObservations() returned %d observations, want 2", len(result.Observations))
	}
	if result.Observations[0].WorkerSessionID != "worker-b" || result.Observations[1].WorkerSessionID != "worker-a" {
		t.Fatalf("observation order = (%q, %q), want chronological attempt order (worker-b, worker-a)", result.Observations[0].WorkerSessionID, result.Observations[1].WorkerSessionID)
	}

	first := result.Observations[0]
	if first.ProviderSession != firstRef || first.ProviderSessionAvailable == false || first.TurnID != "turn-worker-b" || first.AttemptID != "dispatch-a" {
		t.Fatalf("first observation identity/correlation = %#v", first)
	}
	if first.State != workersessions.StateCompleted || first.DurationBasis != workersessions.DurationBasisRecordedTimestamps || first.Duration == nil || *first.Duration != 2*time.Second {
		t.Fatalf("first lifecycle timing = %#v, want COMPLETED/recorded/2s", first)
	}
	if first.TokenUsage == nil || first.TokenUsage.InputTokens == nil || *first.TokenUsage.InputTokens != 11 || first.Parse.EventCount != 4 || first.Parse.MalformedLineCount != 1 || len(first.Parse.Errors) != 1 {
		t.Fatalf("first normalized projection = %#v", first)
	}
	if first.Transcript != workersessions.TranscriptAvailabilityAvailable || first.Failure != nil {
		t.Fatalf("first transcript/failure projection = %#v", first)
	}

	second := result.Observations[1]
	if second.State != workersessions.StateFailed || second.Failure == nil || second.Failure.Kind != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("second failure projection = %#v", second)
	}
	if second.Failure.Detail != "family=terminal type=auth_failure" || second.Duration == nil || *second.Duration != 4*time.Second {
		t.Fatalf("second failure/timing = %#v", second)
	}
	if len(projection.requested) != 2 || projection.requested[0] != firstRef || projection.requested[1] != secondRef {
		t.Fatalf("Provider Sessions projection requests = %#v, want chronological attempts", projection.requested)
	}
}

func TestReadTranscript_ReturnsFinishedNormalizedEntriesAndCorrelation(t *testing.T) {
	ref := sessionRef("provider-session-transcript")
	text := "assistant answer"
	toolName := "search"
	arguments := `{"query":"factory"}`
	output := "tool result"
	encrypted := true
	encryptedContent := "encrypted reasoning"
	projection := &observationProjectionStub{results: map[providers.SessionRef]providersessions.ProjectResult{
		ref: {
			Session: ref,
			Detail: providersessions.Detail{Transcript: []providersessions.TranscriptEntry{
				{Order: 1, Type: providersessions.TranscriptUserMessage, Text: stringPtr("operator request")},
				{Order: 2, Type: providersessions.TranscriptToolCall, Name: &toolName, Arguments: &arguments},
				{Order: 3, Type: providersessions.TranscriptToolOutput, Output: &output},
				{Order: 4, Type: providersessions.TranscriptReasoning, Encrypted: &encrypted, EncryptedContent: &encryptedContent},
				{Order: 5, Type: providersessions.TranscriptAssistantMessage, Text: &text},
			}},
		},
	}}
	registry := newObservationService(t, executionBoundary{execution: executionFor(&ref, workers.OutcomeAccepted, nil, nil)}, newEventsAppender(), platformclock.Real{}, projection)
	if _, err := registry.InvokeSession(context.Background(), startRequest("worker-transcript", "dispatch-transcript", "work-transcript")); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	result, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	if result.WorkerSessionID != "worker-transcript" || result.AttemptID != "dispatch-transcript" || result.TurnID != "turn-worker-transcript" || result.State != workersessions.StateCompleted {
		t.Fatalf("correlation = %#v, want terminal Worker Session envelope", result)
	}
	if len(result.WorkIDs) != 1 || result.WorkIDs[0] != "work-transcript" || len(result.Entries) != 5 {
		t.Fatalf("work/entries = %#v/%d, want work-transcript and five entries", result.WorkIDs, len(result.Entries))
	}
	if result.Entries[1].Type != workersessions.TranscriptToolCall || result.Entries[1].Name == nil || *result.Entries[1].Name != toolName || result.Entries[1].Arguments == nil || *result.Entries[1].Arguments != arguments {
		t.Fatalf("tool-call entry = %#v, want normalized tool fields", result.Entries[1])
	}
	if result.Entries[3].Type != workersessions.TranscriptReasoning || result.Entries[3].Encrypted == nil || !*result.Entries[3].Encrypted || result.Entries[3].EncryptedContent == nil {
		t.Fatalf("encrypted reasoning entry = %#v, want explicit encrypted fields", result.Entries[3])
	}
	if len(projection.requested) != 1 || projection.requested[0] != ref {
		t.Fatalf("projection requests = %#v, want one exact Provider Session request", projection.requested)
	}
	result.Entries[0].Text = stringPtr("mutated")
	again, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("second ReadTranscript() error = %v", err)
	}
	if again.Entries[0].Text == nil || *again.Entries[0].Text != "operator request" {
		t.Fatalf("detached transcript entry = %#v, want provider projection unchanged", again.Entries[0])
	}
}

func TestReadTranscript_DistinguishesActiveMissingUnavailableAndCanceled(t *testing.T) {
	ref := sessionRef("provider-session-active")
	started := make(chan struct{})
	release := make(chan struct{})
	var registry workersessions.Service
	execution := &fakeExecution{dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
		if _, err := registry.ObserveProviderSession(context.Background(), workersessions.ProviderSessionObservationRequest{DispatchID: request.Execution.Dispatch.DispatchID, Reference: ref}); err != nil {
			return workers.WorkstationDispatchResult{}, err
		}
		close(started)
		<-release
		return workers.WorkstationDispatchResult{
			DispatchID:      request.Execution.Dispatch.DispatchID,
			TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
			Result:          workers.WorkResult{DispatchID: request.Execution.Dispatch.DispatchID, Outcome: workers.OutcomeAccepted, ProviderSession: providerMetadata(ref)},
		}, nil
	}}
	projection := &observationProjectionStub{projectErr: providersessions.ErrSessionStorageUnavailable}
	registry = newObservationService(t, executionBoundary{execution: execution}, newEventsAppender(), platformclock.Real{}, projection)
	done := make(chan error, 1)
	go func() {
		_, err := registry.InvokeSession(context.Background(), startRequest("worker-active", "dispatch-active", "work-active"))
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for active Worker Session")
	}
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationTranscriptActive) {
		t.Fatalf("ReadTranscript(active) error = %v, want ErrObservationTranscriptActive", err)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start(active) error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal Worker Session")
	}
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationTranscriptUnavailable) {
		t.Fatalf("ReadTranscript(unavailable) error = %v, want ErrObservationTranscriptUnavailable", err)
	}
	projection.projectErr = errors.New("normalized transcript parser failed")
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable) {
		t.Fatalf("ReadTranscript(projection failure) error = %v, want ErrObservationTranscriptProjectionUnavailable", err)
	}
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: sessionRef("missing")}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("ReadTranscript(missing) error = %v, want ErrObservationSessionNotFound", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := registry.ReadTranscript(canceled, workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("ReadTranscript(canceled) error = %v, want ErrObservationCanceled", err)
	}
}

func TestObservationProjection_UsesInjectedClockForActiveDurationAndFreezesTerminalDuration(t *testing.T) {
	base := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	boundary := newControlledBoundary()
	registry := newObservationService(t, boundary, newEventsAppender(), clock, nil)
	started := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), startRequest("active-session", "active-dispatch", "active-work"))
		if err != nil {
			t.Errorf("Start(active) error = %v", err)
		}
		started <- result
	}()
	<-boundary.started

	clock.SetTick(2)
	active, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "active-work"})
	if err != nil {
		t.Fatalf("ListObservations(active) error = %v", err)
	}
	if len(active.Observations) != 1 || active.Observations[0].State != workersessions.StateRunning || active.Observations[0].DurationBasis != workersessions.DurationBasisActiveClock || active.Observations[0].Duration == nil || *active.Observations[0].Duration != 2*time.Second {
		t.Fatalf("active observation = %#v, want RUNNING/active-clock/2s", active.Observations)
	}

	clock.SetTick(5)
	boundary.complete(completedDispatchResult("active-dispatch"), nil)
	<-started
	clock.SetTick(20)
	terminal, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "active-work"})
	if err != nil {
		t.Fatalf("ListObservations(terminal) error = %v", err)
	}
	observation := terminal.Observations[0]
	if observation.State != workersessions.StateCompleted || observation.DurationBasis != workersessions.DurationBasisRecordedTimestamps || observation.Duration == nil || *observation.Duration != 5*time.Second {
		t.Fatalf("terminal observation = %#v, want COMPLETED/recorded/5s", observation)
	}
	if observation.EndedAt == nil || !observation.EndedAt.Equal(base.Add(5*time.Second)) {
		t.Fatalf("terminal end time = %v, want %v", observation.EndedAt, base.Add(5*time.Second))
	}
}

func TestObservationStream_ReplaysRetainedEventsThenCompletesAndReplaysTerminalSessions(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newObservationService(t, boundary, newEventsAppender(), platformclock.Real{}, nil)
	started := startControlledSession(t, registry, boundary, "stream-session", "stream-dispatch")
	ref := sessionRef("stream-provider-session")
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "stream-session",
		DispatchID:      "stream-dispatch",
		Reference:       ref,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}

	ctx := context.Background()
	subscription, err := registry.StreamObservations(ctx, workersessions.StreamObservationsRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	opening := subscription.Next(ctx)
	if opening.Kind != workersessions.ObservationDeliveryRecord || opening.Event.SourceSequence != 1 {
		t.Fatalf("opening delivery = %#v, want RECORD source sequence 1", opening)
	}

	terminalDelivery := make(chan workersessions.ObservationDelivery, 1)
	go func() { terminalDelivery <- subscription.Next(ctx) }()
	boundary.complete(completedDispatchResult("stream-dispatch"), nil)
	terminal := awaitObservationDelivery(t, terminalDelivery)
	if terminal.Kind != workersessions.ObservationDeliveryTerminal || terminal.Event.SourceSequence != 2 {
		t.Fatalf("terminal delivery = %#v, want TERMINAL source sequence 2", terminal)
	}
	if final := <-started; final.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() final session = %#v, want COMPLETED", final.Session)
	}
	if closed := subscription.Next(ctx); closed.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after terminal = %#v, want CLOSED", closed)
	}

	replay, err := registry.StreamObservations(ctx, workersessions.StreamObservationsRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("StreamObservations(already terminal) error = %v", err)
	}
	if got := replay.Next(ctx); got.Kind != workersessions.ObservationDeliveryRecord || got.Event.SourceSequence != 1 {
		t.Fatalf("already-terminal first replay = %#v, want opening RECORD", got)
	}
	if got := replay.Next(ctx); got.Kind != workersessions.ObservationDeliveryTerminalReplay || got.Event.SourceSequence != 2 {
		t.Fatalf("already-terminal second replay = %#v, want TERMINAL_REPLAY", got)
	}
}

func TestObservationStream_CancellationUnregistersSubscription(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newObservationService(t, boundary, newEventsAppender(), platformclock.Real{}, nil)
	started := startControlledSession(t, registry, boundary, "cancel-session", "cancel-dispatch")
	ref := sessionRef("cancel-provider-session")
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "cancel-session",
		DispatchID:      "cancel-dispatch",
		Reference:       ref,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	subscription, err := registry.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	if opening := subscription.Next(context.Background()); opening.Kind != workersessions.ObservationDeliveryRecord {
		t.Fatalf("opening delivery = %#v, want RECORD", opening)
	}

	ctx, cancel := context.WithCancel(context.Background())
	delivery := make(chan workersessions.ObservationDelivery, 1)
	go func() { delivery <- subscription.Next(ctx) }()
	cancel()
	got := awaitObservationDelivery(t, delivery)
	if got.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(got.Err, context.Canceled) {
		t.Fatalf("canceled delivery = %#v, want CANCELED wrapping context.Canceled", got)
	}
	boundary.complete(completedDispatchResult("cancel-dispatch"), nil)
	<-started
}

func TestObservationStream_MapsRetainedSourceFailureToTypedOutcome(t *testing.T) {
	source := events.Subscription(func(context.Context) events.Delivery {
		return events.Delivery{Kind: events.DeliveryGap, Gap: &events.GapFacts{}}
	})
	appender := &observationEventsAppender{subscription: source}
	ref := sessionRef("source-failure-session")
	execution := executionFor(&ref, workers.OutcomeAccepted, nil, nil)
	registry := newObservationService(t, executionBoundary{execution: execution}, appender, platformclock.Real{}, nil)
	if _, err := registry.InvokeSession(context.Background(), startRequest("source-failure-worker", "source-failure-dispatch", "source-failure-work")); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	subscription, err := registry.StreamObservations(context.Background(), workersessions.StreamObservationsRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("StreamObservations() error = %v", err)
	}
	got := subscription.Next(context.Background())
	if got.Kind != workersessions.ObservationDeliverySourceFailure || !errors.Is(got.Err, workersessions.ErrObservationSourceGap) {
		t.Fatalf("source failure delivery = %#v, want SOURCE_FAILURE wrapping ErrObservationSourceGap", got)
	}
}

func TestObservationContract_MissingAndUnavailableIdentitiesAreTyped(t *testing.T) {
	registry := newObservationService(t, newControlledBoundary(), newEventsAppender(), platformclock.Real{}, nil)
	ctx := context.Background()
	if _, err := registry.ListObservations(ctx, workersessions.ListObservationsRequest{}); !errors.Is(err, workersessions.ErrInvalidObservationWorkID) {
		t.Fatalf("ListObservations(blank) error = %v, want ErrInvalidObservationWorkID", err)
	}
	if _, err := registry.ListObservations(ctx, workersessions.ListObservationsRequest{WorkID: "missing-work"}); !errors.Is(err, workersessions.ErrObservationWorkNotFound) {
		t.Fatalf("ListObservations(missing) error = %v, want ErrObservationWorkNotFound", err)
	}
	if _, err := registry.GetObservation(ctx, workersessions.GetObservationRequest{}); !errors.Is(err, workersessions.ErrInvalidObservationIdentity) {
		t.Fatalf("GetObservation(blank) error = %v, want ErrInvalidObservationIdentity", err)
	}
	ref := sessionRef("missing-session")
	if _, err := registry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("GetObservation(missing) error = %v, want ErrObservationSessionNotFound", err)
	}
	if _, err := registry.StreamObservations(ctx, workersessions.StreamObservationsRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationSessionNotFound) {
		t.Fatalf("StreamObservations(missing) error = %v, want ErrObservationSessionNotFound", err)
	}

	execution := executionFor(&ref, workers.OutcomeAccepted, nil, nil)
	projectedRegistry := newObservationService(t, executionBoundary{execution: execution}, newEventsAppender(), platformclock.Real{}, nil)
	if _, err := projectedRegistry.InvokeSession(ctx, startRequest("unavailable-worker", "unavailable-dispatch", "unavailable-work")); err != nil {
		t.Fatalf("Start(unavailable projection) error = %v", err)
	}
	if _, err := projectedRegistry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationProjectionUnavailable) {
		t.Fatalf("GetObservation(unavailable projection) error = %v, want ErrObservationProjectionUnavailable", err)
	}
}

type observationEventsAppender struct {
	subscription events.Subscription
}

func (a *observationEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	return events.AppendResult{}, nil
}

func (a *observationEventsAppender) Subscribe(context.Context, events.SubscribeRequest) (events.Subscription, error) {
	return a.subscription, nil
}

func awaitObservationDelivery(t *testing.T, deliveries <-chan workersessions.ObservationDelivery) workersessions.ObservationDelivery {
	t.Helper()
	select {
	case delivery := <-deliveries:
		return delivery
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Worker Session observation delivery")
		return workersessions.ObservationDelivery{}
	}
}

func stringPtr(value string) *string { return &value }
