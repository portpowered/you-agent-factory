package service_test

import (
	"context"
	"errors"
	"sync"
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

// admissionControlledExecution is a controlled Workers execution service used
// only behind the real asynchronous WorkstationPoolBoundary. Its channels make
// the production boundary's admission barrier and terminal/control race
// observable without sleeps or polling.
type admissionControlledExecution struct {
	mu sync.Mutex

	dispatchEntered     chan struct{}
	dispatchEnteredOnce sync.Once
	allowAdmission      chan struct{}
	admitted            chan struct{}
	admittedOnce        sync.Once
	cancelCalls         chan workers.WorkstationDispatchCancelRequest
	complete            chan struct{}
	completionCommitted chan struct{}
	completionOnce      sync.Once
	completionWins      bool
	alreadyCanceled     bool
	cancellations       int
	completeOnce        sync.Once
}

var _ workers.WorkstationExecutionService = (*admissionControlledExecution)(nil)

func newAdmissionControlledExecution(completionWins bool) *admissionControlledExecution {
	return &admissionControlledExecution{
		dispatchEntered:     make(chan struct{}),
		allowAdmission:      make(chan struct{}),
		admitted:            make(chan struct{}),
		cancelCalls:         make(chan workers.WorkstationDispatchCancelRequest, 1),
		complete:            make(chan struct{}),
		completionCommitted: make(chan struct{}),
		completionWins:      completionWins,
	}
}

func (*admissionControlledExecution) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStarted}, nil
}

func (*admissionControlledExecution) StopWorkstationPool(
	context.Context,
) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{Outcome: workers.WorkstationPoolLifecycleOutcomeStopped}, nil
}

func (e *admissionControlledExecution) DispatchWorkstation(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return e.DispatchWorkstationWithAdmission(ctx, request, nil)
}

func (e *admissionControlledExecution) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	e.dispatchEnteredOnce.Do(func() { close(e.dispatchEntered) })
	select {
	case <-e.allowAdmission:
	case <-ctx.Done():
		return workers.WorkstationDispatchResult{}, ctx.Err()
	}
	if admitted != nil {
		admitted()
	}
	e.admittedOnce.Do(func() { close(e.admitted) })
	select {
	case <-e.complete:
	case <-ctx.Done():
		return workers.WorkstationDispatchResult{}, ctx.Err()
	}
	e.completionOnce.Do(func() { close(e.completionCommitted) })
	result := completedDispatchResult(request.Execution.Dispatch.DispatchID)
	if e.completionWins {
		return result, nil
	}
	return canceledDispatchResult(request.Execution.Dispatch.DispatchID), workers.ErrWorkstationDispatchCanceled
}

func (e *admissionControlledExecution) CancelWorkstationDispatch(
	_ context.Context,
	request workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	e.mu.Lock()
	e.cancellations++
	alreadyCanceled := e.alreadyCanceled && e.cancellations > 1
	firstCancellation := e.alreadyCanceled && e.cancellations == 1
	e.mu.Unlock()
	e.cancelCalls <- request
	if alreadyCanceled {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeAlreadyCanceled,
		}, nil
	}
	if firstCancellation {
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
		}, nil
	}
	e.completeOnce.Do(func() { close(e.complete) })
	if e.completionWins {
		<-e.completionCommitted
		return workers.WorkstationDispatchCancelResult{
			DispatchID: request.DispatchID,
			Outcome:    workers.WorkstationDispatchCancelOutcomeAlreadyTerminal,
		}, workers.ErrWorkstationDispatchAlreadyTerminal
	}
	return workers.WorkstationDispatchCancelResult{
		DispatchID: request.DispatchID,
		Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
	}, nil
}

func newProductionBoundaryRegistry(t *testing.T, execution workers.WorkstationExecutionService) workersessions.Service {
	t.Helper()
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      true,
	})
	return newControlledRegistry(t, boundary)
}

func newSynchronousProductionBoundaryRegistry(t *testing.T, execution workers.WorkstationExecutionService) workersessions.Service {
	t.Helper()
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      false,
	})
	return newControlledRegistry(t, boundary)
}

func TestCancel_WaitsForProductionAsyncBoundaryAdmissionBeforeExactCancellation(t *testing.T) {
	execution := newAdmissionControlledExecution(false)
	registry := newProductionBoundaryRegistry(t, execution)
	started := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		started <- result
	}()
	<-execution.dispatchEntered

	cancelled := make(chan workersessions.ControlResult, 1)
	cancelErr := make(chan error, 1)
	go func() {
		result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		cancelled <- result
		cancelErr <- err
	}()
	select {
	case call := <-execution.cancelCalls:
		t.Fatalf("Cancel reached dispatch %q before Workers admission", call.DispatchID)
	default:
	}

	close(execution.allowAdmission)
	result := <-cancelled
	if err := <-cancelErr; err != nil || result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Cancel() = %#v, %v, want applied after admission", result, err)
	}
	call := <-execution.cancelCalls
	if call.DispatchID != "dispatch-1" {
		t.Fatalf("Cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	if final := <-started; final.Session.State != workersessions.StateCanceled ||
		final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled {
		t.Fatalf("Start() after admitted cancellation = %#v, want canceled terminal result", final)
	}
}

func TestCancel_ProductionBoundaryCompletionWinReturnsNoopAndPreservesCompletedSession(t *testing.T) {
	execution := newAdmissionControlledExecution(true)
	registry := newProductionBoundaryRegistry(t, execution)
	started := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
		started <- result
	}()
	<-execution.dispatchEntered
	close(execution.allowAdmission)

	result, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || result.Outcome != workersessions.ControlOutcomeNoop {
		t.Fatalf("Cancel() = %#v, %v, want completion-wins NOOP", result, err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Cancel() session state = %q, want COMPLETED", result.Session.State)
	}
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("Cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	final := <-started
	if final.Session.State != workersessions.StateCompleted ||
		final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("Start() after completion win = %#v, want completed terminal result", final)
	}
	if final.DispatchErr != nil {
		t.Fatalf("Start() dispatch error = %v, want nil", final.DispatchErr)
	}
}

func TestTerminate_ProductionSynchronousBoundaryCancelsAdmittedDispatchBeforePublishReturns(t *testing.T) {
	execution := newAdmissionControlledExecution(false)
	registry := newSynchronousProductionBoundaryRegistry(t, execution)
	started := make(chan workersessions.InvokeSessionResult, 1)
	startErr := make(chan error, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		started <- result
		startErr <- err
	}()
	<-execution.dispatchEntered
	close(execution.allowAdmission)
	<-execution.admitted

	select {
	case <-execution.complete:
		t.Fatal("synchronous dispatch completed before explicit control")
	default:
	}

	terminated := make(chan workersessions.ControlResult, 1)
	terminateErr := make(chan error, 1)
	go func() {
		result, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		terminated <- result
		terminateErr <- err
	}()
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("Terminate dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	result := <-terminated
	if err := <-terminateErr; err != nil || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != workersessions.StateTerminated {
		t.Fatalf("Terminate() = %#v, %v, want applied TERMINATED result", result, err)
	}
	final := <-started
	if err := <-startErr; err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if final.Session.State != workersessions.StateTerminated || final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled || !errors.Is(final.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("Start() after synchronous cancellation = %#v, want one terminated canceled result", final)
	}
}

func TestTerminate_ProductionBoundaryAlreadyCanceledJoinsHeldCallback(t *testing.T) {
	execution := newAdmissionControlledExecution(false)
	execution.alreadyCanceled = true
	boundary := workers.NewWorkstationPoolBoundary(workers.WorkstationPoolBoundaryConfig{
		Service:    execution,
		RouteNames: []string{"review"},
		Async:      true,
	})
	registry := newControlledRegistry(t, boundary)
	started := make(chan workersessions.InvokeSessionResult, 1)
	startErr := make(chan error, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		started <- result
		startErr <- err
	}()
	<-execution.dispatchEntered
	close(execution.allowAdmission)
	<-execution.admitted

	first, err := boundary.Cancel(context.Background(), workers.WorkstationDispatchCancelRequest{DispatchID: "dispatch-1"})
	if err != nil || first.Outcome != workers.WorkstationDispatchCancelOutcomeCanceled {
		t.Fatalf("first boundary Cancel() = %#v, %v, want committed cancellation", first, err)
	}
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("first boundary cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}

	terminated := make(chan workersessions.ControlResult, 1)
	terminateErr := make(chan error, 1)
	go func() {
		result, err := registry.Terminate(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
		terminated <- result
		terminateErr <- err
	}()
	if call := <-execution.cancelCalls; call.DispatchID != "dispatch-1" {
		t.Fatalf("Terminate boundary cancel dispatch ID = %q, want dispatch-1", call.DispatchID)
	}
	select {
	case result := <-terminated:
		t.Fatalf("Terminate() returned before canceled callback joined: %#v", result)
	default:
	}

	close(execution.complete)
	result := <-terminated
	if err := <-terminateErr; err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Terminate() = %#v, %v, want joined canceled NOOP", result, err)
	}
	final := <-started
	if err := <-startErr; err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if final.Session.State != workersessions.StateCanceled || final.Dispatch.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCanceled || !errors.Is(final.DispatchErr, workers.ErrWorkstationDispatchCanceled) {
		t.Fatalf("Start() after held cancellation = %#v, want established canceled result", final)
	}
}

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

func providerMetadata(ref providers.SessionRef) *providers.SessionMetadata {
	return &providers.SessionMetadata{
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
	service, err := workersessionservice.New(boundary, eventsAppender, logging.NoopLogger{}, clock, projection, nil)
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
					Continuation:    continuationFromProviderMetadata(providerMetadata(firstRef)),
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
	registry := newObservationService(t, executionBoundary{execution: execution}, eventsAppender, clock, projection)

	clock.SetTick(1)
	mustInvokeObservationSession(t, registry, startRequest("worker-b", "dispatch-a", "work-1"))
	clock.SetTick(5)
	mustInvokeObservationSession(t, registry, startRequest("worker-a", "dispatch-b", "work-1"))
	clock.SetTick(20)

	result := mustListObservations(t, registry, "work-1")
	assertCorrelatedObservationProjection(t, result, projection, firstRef, secondRef)
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
	mustInvokeObservationSession(t, registry, startRequest("worker-transcript", "dispatch-transcript", "work-transcript"))

	result := mustReadTranscript(t, registry, ref)
	assertTranscriptProjection(t, result, projection, ref, toolName, arguments)
	byWorker, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{WorkerSessionID: "worker-transcript"})
	if err != nil {
		t.Fatalf("ReadTranscript(WorkerSessionID) error = %v", err)
	}
	if byWorker.WorkerSessionID != result.WorkerSessionID || byWorker.ProviderSession != ref || len(byWorker.Entries) != len(result.Entries) || byWorker.Entries[4].Text == nil || *byWorker.Entries[4].Text != text {
		t.Fatalf("ReadTranscript(WorkerSessionID) = %#v, want same normalized terminal transcript", byWorker)
	}
	result.Entries[0].Text = stringPtr("mutated")
	again := mustReadTranscript(t, registry, ref)
	if again.Entries[0].Text == nil || *again.Entries[0].Text != "operator request" {
		t.Fatalf("detached transcript entry = %#v, want provider projection unchanged", again.Entries[0])
	}
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

func mustReadTranscript(t *testing.T, registry workersessions.Service, ref providers.SessionRef) workersessions.ReadTranscriptResult {
	t.Helper()
	result, err := registry.ReadTranscript(context.Background(), workersessions.ReadTranscriptRequest{ProviderSession: ref})
	if err != nil {
		t.Fatalf("ReadTranscript() error = %v", err)
	}
	return result
}

func assertCorrelatedObservationProjection(t *testing.T, result workersessions.ListObservationsResult, projection *observationProjectionStub, firstRef, secondRef providers.SessionRef) {
	t.Helper()
	if len(result.Observations) != 2 {
		t.Fatalf("ListObservations() returned %d observations, want 2", len(result.Observations))
	}
	if result.Observations[0].WorkerSessionID != "worker-b" || result.Observations[1].WorkerSessionID != "worker-a" {
		t.Fatalf("observation order = (%q, %q), want chronological attempt order (worker-b, worker-a)", result.Observations[0].WorkerSessionID, result.Observations[1].WorkerSessionID)
	}
	assertFirstObservationProjection(t, result.Observations[0], firstRef)
	assertSecondObservationProjection(t, result.Observations[1])
	if len(projection.requested) != 2 || projection.requested[0] != firstRef || projection.requested[1] != secondRef {
		t.Fatalf("Provider Sessions projection requests = %#v, want chronological attempts", projection.requested)
	}
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
	if observation.TokenUsage == nil || observation.TokenUsage.InputTokens == nil || *observation.TokenUsage.InputTokens != 11 || observation.Parse.EventCount != 4 || observation.Parse.MalformedLineCount != 1 || len(observation.Parse.Errors) != 1 {
		t.Fatalf("first normalized projection = %#v", observation)
	}
}

func assertFirstObservationTranscript(t *testing.T, observation workersessions.Observation) {
	t.Helper()
	if observation.Transcript != workersessions.TranscriptAvailabilityAvailable || observation.Failure != nil {
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

func assertTranscriptProjection(t *testing.T, result workersessions.ReadTranscriptResult, projection *observationProjectionStub, ref providers.SessionRef, toolName, arguments string) {
	t.Helper()
	if result.WorkerSessionID != "worker-transcript" || result.AttemptID != "dispatch-transcript" || result.TurnID != "turn-worker-transcript" || result.State != workersessions.StateCompleted {
		t.Fatalf("correlation = %#v, want terminal Worker Session envelope", result)
	}
	if len(result.WorkIDs) != 1 || result.WorkIDs[0] != "work-transcript" || len(result.Entries) != 5 {
		t.Fatalf("work/entries = %#v/%d, want work-transcript and five entries", result.WorkIDs, len(result.Entries))
	}
	assertTranscriptToolCall(t, result.Entries[1], toolName, arguments)
	assertTranscriptReasoning(t, result.Entries[3])
	if len(projection.requested) != 1 || projection.requested[0] != ref {
		t.Fatalf("projection requests = %#v, want one exact Provider Session request", projection.requested)
	}
}

func assertTranscriptToolCall(t *testing.T, entry workersessions.TranscriptEntry, toolName, arguments string) {
	t.Helper()
	if entry.Type != workersessions.TranscriptToolCall || entry.Name == nil || *entry.Name != toolName || entry.Arguments == nil || *entry.Arguments != arguments {
		t.Fatalf("tool-call entry = %#v, want normalized tool fields", entry)
	}
}

func assertTranscriptReasoning(t *testing.T, entry workersessions.TranscriptEntry) {
	t.Helper()
	if entry.Type != workersessions.TranscriptReasoning || entry.Encrypted == nil || !*entry.Encrypted || entry.EncryptedContent == nil {
		t.Fatalf("encrypted reasoning entry = %#v, want explicit encrypted fields", entry)
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

func TestObservationStream_ReplayOnlyTerminalSessionEmitsCompleteRetainedHistory(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newObservationService(t, boundary, newEventsAppender(), platformclock.Real{}, nil)
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
