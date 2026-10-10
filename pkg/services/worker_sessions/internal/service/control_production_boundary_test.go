package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/recordings"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func sessionRef(id string) providers.SessionRef {
	return providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       id,
	}
}

func providerMetadata(ref providers.SessionRef) *providers.SessionMetadata {
	return &providers.SessionMetadata{
		Provider: string(ref.Provider),
		Kind:     ref.Kind,
		ID:       ref.ID,
	}
}

func newObservationService(
	t *testing.T,
	execution any,
	eventsAppender workersessionservice.EventsAppender,
	clock platformclock.Source,
) workersessions.Service {
	t.Helper()
	service, err := workersessionservice.New(asCanonicalExecution(execution), eventsAppender, logging.NoopLogger{}, clock, testSchedulerForClock(clock), nil, unavailableWorkerControlStore{}, unavailableWorkerControlStore{}, continuationInspectionFake{})
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
				result.Continuation = continuationFromProviderMetadata(providerMetadata(*ref))
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
					DispatchID:   dispatchID,
					Outcome:      workers.OutcomeAccepted,
					Continuation: continuationFromProviderMetadata(providerMetadata(firstRef)),
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
				Continuation:    continuationFromProviderMetadata(providerMetadata(secondRef)),
			},
		}, nil
	}
	registry := newObservationService(t, executionBoundary{execution: execution}, eventsAppender, clock)

	clock.SetTick(1)
	mustInvokeObservationSession(t, registry, startRequest("worker-b", "dispatch-a", "work-1"))
	clock.SetTick(5)
	mustInvokeObservationSession(t, registry, startRequest("worker-a", "dispatch-b", "work-1"))
	clock.SetTick(20)

	result := mustListObservations(t, registry, "work-1")
	assertCorrelatedObservationProjection(t, result, firstRef)
}

func mustInvokeObservationSession(t *testing.T, registry workersessions.Service, request workersessions.InvokeSessionRequest) {
	t.Helper()
	if _, err := registry.InvokeSession(context.Background(), request); err != nil {
		t.Fatalf("InvokeSession() error = %v", err)
	}
}

func mustListObservations(t *testing.T, registry workersessions.Service, workID string) workersessions.ListObservationsResult {
	t.Helper()
	result, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: workID})
	if err != nil {
		t.Fatalf("ListObservations() error = %v", err)
	}
	return result
}

func assertCorrelatedObservationProjection(t *testing.T, result workersessions.ListObservationsResult, firstRef providers.SessionRef) {
	t.Helper()
	if len(result.Observations) != 2 {
		t.Fatalf("ListObservations() returned %d observations, want 2", len(result.Observations))
	}
	if result.Observations[0].WorkerSessionID != "worker-b" || result.Observations[1].WorkerSessionID != "worker-a" {
		t.Fatalf("observation order = (%q, %q), want chronological attempt order (worker-b, worker-a)", result.Observations[0].WorkerSessionID, result.Observations[1].WorkerSessionID)
	}
	assertFirstObservationProjection(t, result.Observations[0], firstRef)
	assertSecondObservationProjection(t, result.Observations[1])

}

func assertFirstObservationProjection(t *testing.T, observation workersessions.Observation, ref providers.SessionRef) {
	t.Helper()
	assertFirstObservationIdentity(t, observation, ref)
	assertFirstObservationTiming(t, observation)
	assertFirstObservationUsage(t, observation)
	assertFirstObservationTranscript(t, observation)
}

func assertFirstObservationIdentity(t *testing.T, observation workersessions.Observation, ref providers.SessionRef) {
	t.Helper()
	if observation.ProviderSession != ref || !observation.ProviderSessionAvailable || observation.TurnID != "turn-worker-b" || observation.AttemptID != "dispatch-a" {
		t.Fatalf("first observation identity/correlation = %#v", observation)
	}
}

func assertFirstObservationTiming(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.State != workersessions.StateCompleted || observation.DurationBasis != workersessions.DurationBasisRecordedTimestamps || observation.Duration == nil || *observation.Duration != 2*time.Second {
		t.Fatalf("first lifecycle timing = %#v, want COMPLETED/recorded/2s", observation)
	}
}

func assertFirstObservationUsage(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.TokenUsage != nil || observation.TurnUsage != nil || observation.Parse.EventCount != 0 || len(observation.Parse.Errors) != 0 {
		t.Fatalf("uncaptured attempt invented usage or parse facts: %#v", observation)
	}
}

func assertFirstObservationTranscript(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.Transcript != workersessions.TranscriptAvailabilityUnavailable || observation.Failure != nil {
		t.Fatalf("first transcript/failure projection = %#v", observation)
	}
}

func assertSecondObservationProjection(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.State != workersessions.StateFailed || observation.Failure == nil || observation.Failure.Kind != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("second failure projection = %#v", observation)
	}
	if observation.Failure.Detail != "family=terminal type=auth_failure" || observation.Duration == nil || *observation.Duration != 4*time.Second {
		t.Fatalf("second failure/timing = %#v", observation)
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
			Result:          workers.WorkResult{DispatchID: request.Execution.Dispatch.DispatchID, Outcome: workers.OutcomeAccepted, Continuation: continuationFromProviderMetadata(providerMetadata(ref))},
		}, nil
	}}

	registry = newObservationService(t, executionBoundary{execution: execution}, newEventsAppender(), platformclock.Real{})
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
	if _, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref}); !errors.Is(err, workersessions.ErrObservationTranscriptProjectionUnavailable) {
		t.Fatalf("ReadTranscript(unavailable) error = %v, want ErrObservationTranscriptUnavailable", err)
	}
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
	registry := newObservationService(t, boundary, newEventsAppender(), clock)
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
	registry := newObservationService(t, boundary, newEventsAppender(), platformclock.Real{})
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

func TestObservationStream_ReplayOnlyTerminalSessionEmitsCompleteRetainedHistory(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newObservationService(t, boundary, newEventsAppender(), platformclock.Real{})
	started := startControlledSession(t, registry, boundary, "replay-terminal-session", "replay-terminal-dispatch")
	ref := sessionRef("replay-terminal-provider-session")
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "replay-terminal-session",
		DispatchID:      "replay-terminal-dispatch",
		Reference:       ref,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}

	boundary.complete(completedDispatchResult("replay-terminal-dispatch"), nil)
	if final := <-started; final.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() final session = %#v, want COMPLETED", final.Session)
	}

	ctx := context.Background()
	replay, err := registry.StreamObservations(ctx, workersessions.StreamObservationsRequest{
		ProviderSession: ref,
		ReplayOnly:      true,
		Limit:           1,
	})
	if err != nil {
		t.Fatalf("StreamObservations(replay-only) error = %v", err)
	}
	defer replay.Close()

	opening := replay.Next(ctx)
	if opening.Kind != workersessions.ObservationDeliveryRecord || opening.Event.SourceSequence != 1 {
		t.Fatalf("replay-only opening = %#v, want RECORD source sequence 1", opening)
	}
	terminal := replay.Next(ctx)
	if terminal.Kind != workersessions.ObservationDeliveryTerminalReplay || terminal.Event.SourceSequence != 2 {
		t.Fatalf("replay-only terminal = %#v, want TERMINAL_REPLAY source sequence 2", terminal)
	}
	summary := replay.Next(ctx)
	if summary.Kind != workersessions.ObservationDeliveryReplaySummary || summary.Summary == nil {
		t.Fatalf("replay-only summary = %#v, want REPLAY_SUMMARY", summary)
	}
	if !summary.Summary.Complete || summary.Summary.Reason != "session-completed" || summary.Summary.EventsEmitted != 2 {
		t.Fatalf("replay-only summary = %#v, want complete completed-session count-two summary", summary.Summary)
	}
	if closed := replay.Next(ctx); closed.Kind != workersessions.ObservationDeliveryClosed {
		t.Fatalf("delivery after replay-only summary = %#v, want CLOSED", closed)
	}
}

func TestObservationStream_CancellationUnregistersSubscription(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newObservationService(t, boundary, newEventsAppender(), platformclock.Real{})
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
	registry := newObservationService(t, executionBoundary{execution: execution}, appender, platformclock.Real{})
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
	registry := newObservationService(t, newControlledBoundary(), newEventsAppender(), platformclock.Real{})
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
	projectedRegistry := newObservationService(t, executionBoundary{execution: execution}, newEventsAppender(), platformclock.Real{})
	if _, err := projectedRegistry.InvokeSession(ctx, startRequest("unavailable-worker", "unavailable-dispatch", "unavailable-work")); err != nil {
		t.Fatalf("Start(unavailable projection) error = %v", err)
	}
	got, err := projectedRegistry.GetObservation(ctx, workersessions.GetObservationRequest{ProviderSession: ref})
	if err != nil || got.WorkerSessionID != "unavailable-worker" || got.State != workersessions.StateCompleted ||
		!got.ProviderSessionAvailable || got.ProviderSession != ref || got.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		t.Fatalf("GetObservation(unavailable projection) = %#v, %v, want retained completed identity", got, err)
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

// observeTerminalReason reads the named reason the Worker Session inspection
// surface reports for id. It uses the Worker-Session-identity observation
// route because that is the projection the CLI show/list commands and the HTTP
// observation handlers all render.
func observeTerminalReason(t *testing.T, registry workersessions.Service, id string) (workersessions.State, *workersessions.FailureCause) {
	t.Helper()
	observed, err := registry.GetObservationByWorkerSessionID(
		context.Background(),
		workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: id},
	)
	if err != nil {
		t.Fatalf("GetObservationByWorkerSessionID(%q) error = %v", id, err)
	}
	if err := observed.Validate(); err != nil {
		t.Fatalf("observation for %q is not a valid projection: %v", id, err)
	}
	return observed.State, observed.Failure
}

func assertNamedTerminalReason(
	t *testing.T,
	registry workersessions.Service,
	id string,
	wantState workersessions.State,
	wantKind workersessions.FailureCauseKind,
	wantDetail string,
) {
	t.Helper()
	state, failure := observeTerminalReason(t, registry, id)
	if state != wantState {
		t.Fatalf("observed state = %q, want %q", state, wantState)
	}
	if failure == nil {
		t.Fatalf("observed reason for a %s session = nil, want the named reason %q", wantState, wantKind)
	}
	if failure.Kind != wantKind {
		t.Fatalf("observed reason kind = %q, want %q", failure.Kind, wantKind)
	}
	if failure.Detail != wantDetail {
		t.Fatalf("observed reason detail = %q, want the fixed safe detail %q", failure.Detail, wantDetail)
	}
}

// TestObservation_OperatorControlReportsANamedTerminalReason covers the reason
// an operator had no way to read before: a session ended by a control carries
// no TerminalResult, so the inspection surface reported "unavailable" — the
// same thing it reports for a cause that was never recorded.
func TestObservation_OperatorControlReportsANamedTerminalReason(t *testing.T) {
	tests := []struct {
		name       string
		control    func(context.Context, workersessions.Service, workersessions.ControlRequest) (workersessions.ControlResult, error)
		wantState  workersessions.State
		wantKind   workersessions.FailureCauseKind
		wantDetail string
	}{
		{
			name: "cancel",
			control: func(ctx context.Context, registry workersessions.Service, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
				return registry.Cancel(ctx, req)
			},
			wantState:  workersessions.StateCanceled,
			wantKind:   workersessions.FailureCauseOperatorCanceled,
			wantDetail: "an operator cancel control ended the Worker Session",
		},
		{
			name: "terminate",
			control: func(ctx context.Context, registry workersessions.Service, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
				return registry.Terminate(ctx, req)
			},
			wantState:  workersessions.StateTerminated,
			wantKind:   workersessions.FailureCauseOperatorTerminated,
			wantDetail: "an operator terminate control ended the Worker Session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			started := startControlledSession(t, registry, boundary, "worker-"+tt.name, "dispatch-"+tt.name)

			result, err := tt.control(context.Background(), registry, workersessions.ControlRequest{ID: "worker-" + tt.name})
			if err != nil || result.Outcome != workersessions.ControlOutcomeApplied {
				t.Fatalf("%s = %#v, %v, want APPLIED", tt.name, result, err)
			}
			if got := <-started; got.Session.State != tt.wantState {
				t.Fatalf("InvokeSession() after %s = %#v, want %q", tt.name, got.Session, tt.wantState)
			}

			assertNamedTerminalReason(t, registry, "worker-"+tt.name, tt.wantState, tt.wantKind, tt.wantDetail)
		})
	}
}

func TestObservation_ProcessGoneReportsANamedTerminalReason(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return processGoneResult(request), workers.ErrWorkstationDispatchProcessGone
		},
	}
	registry := newRegistryWithExecution(execution)

	if _, err := registry.InvokeSession(context.Background(), validStartRequest("worker-gone", "dispatch-gone")); err != nil {
		t.Fatalf("InvokeSession() error = %v", err)
	}

	assertNamedTerminalReason(
		t, registry, "worker-gone",
		workersessions.StateFailed,
		workersessions.FailureCauseProcessGone,
		"the worker process exited before dispatch completion",
	)
}

func TestObservation_ExecutionTimeoutReportsANamedTerminalReason(t *testing.T) {
	base := time.Date(2035, time.March, 4, 5, 6, 7, 0, time.UTC)
	clock := platformclock.NewDeterministic(base, time.Second)
	boundary := newControlledBoundary()
	registry, err := newServiceWithClock(boundary, newEventsAppender(), logging.NoopLogger{}, clock)
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	request := validStartRequest("worker-timeout", "dispatch-timeout")
	request.Execution.Execution.Timeout = 5 * time.Second

	invoked := make(chan error, 1)
	go func() {
		_, invokeErr := registry.InvokeSession(context.Background(), request)
		invoked <- invokeErr
	}()
	select {
	case <-boundary.admitted:
	case <-time.After(controlledBoundaryWaitTimeout):
		t.Fatal("Worker Session did not reach the admitted deadline-watch state")
	}
	clock.SetTick(5)
	select {
	case invokeErr := <-invoked:
		if invokeErr != nil {
			t.Fatalf("InvokeSession() error = %v", invokeErr)
		}
	case <-time.After(controlledBoundaryWaitTimeout):
		t.Fatal("deadline reconciliation did not terminalize the Worker Session")
	}

	assertNamedTerminalReason(
		t, registry, "worker-timeout",
		workersessions.StateFailed,
		workersessions.FailureCauseTimeout,
		"the worker execution exceeded its hard deadline",
	)
}

// TestObservation_LiveSessionReportsNoTerminalReason keeps the projection from
// naming a reason for a session that has not ended.
func TestObservation_LiveSessionReportsNoTerminalReason(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-live", "dispatch-live")
	t.Cleanup(func() {
		_, _ = registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-live"})
		<-started
	})

	state, failure := observeTerminalReason(t, registry, "worker-live")
	if state != workersessions.StateRunning {
		t.Fatalf("observed state = %q, want RUNNING", state)
	}
	if failure != nil {
		t.Fatalf("observed reason for a RUNNING session = %#v, want none", failure)
	}
}

// Existing clock-driven fixtures explicitly select their timer source here.
func testSchedulerForClock(clock platformclock.Source) platformclock.TimerSource {
	if scheduler, ok := clock.(platformclock.TimerSource); ok {
		return scheduler
	}
	return platformclock.Real{}
}

// unavailableWorkerControlStore is a controlled persistence outage for tests
// that do not own durable control behavior. Every operation fails explicitly;
// it must never be used as evidence that an intent was committed or replayed.
type unavailableWorkerControlStore struct{}

func (unavailableWorkerControlStore) BeginWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	return recordings.WorkerControlOperationRecord{}, false, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) AdvanceWorkerControlOperation(context.Context, recordings.WorkerControlOperationRecord, uint64) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) LoadWorkerControlOperation(context.Context, recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	return recordings.WorkerControlOperationRecord{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ListWorkerControlOperations(context.Context, recordings.WorkerControlTarget) ([]recordings.WorkerControlOperationRecord, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) PersistWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, json.RawMessage) (string, error) {
	return "", recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerControlInput(context.Context, recordings.WorkerControlOperationKey, string) (json.RawMessage, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ValidateWorkerRestartRecipe(context.Context, string, workers.WorkstationDispatchRequest) error {
	return recordings.ErrMissingWorkerRestartInputStore
}

func (unavailableWorkerControlStore) SaveWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget, workers.WorkstationDispatchRequest) error {
	return recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerRestartRecipe(context.Context, recordings.WorkerControlTarget) (workers.WorkstationDispatchRequest, error) {
	return workers.WorkstationDispatchRequest{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) LookupPreparedWorkerContinuationSource(context.Context, recordings.WorkerControlTarget) (recordings.WorkerContinuationSource, error) {
	return recordings.WorkerContinuationSource{}, recordings.ErrWorkerRecordingPersistence
}

func (unavailableWorkerControlStore) ReadWorkerContinuationInput(context.Context, recordings.WorkerControlOperationKey) (json.RawMessage, error) {
	return nil, recordings.ErrWorkerRecordingPersistence
}
