// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/events"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNew_RejectsNilExecution(t *testing.T) {
	if _, err := newService(nil, newEventsAppender(), nil); !errors.Is(err, service.ErrMissingExecution) {
		t.Fatalf("New(nil, events, nil) error = %v, want ErrMissingExecution", err)
	}
}

func TestNew_RejectsNilEventsAppender(t *testing.T) {
	if _, err := newService(executionBoundary{execution: succeedingExecution()}, nil, nil); !errors.Is(err, service.ErrMissingEventsAppender) {
		t.Fatalf("New(execution, nil, nil) error = %v, want ErrMissingEventsAppender", err)
	}
}

func TestStart_InvalidRequest_ReturnsTypedErrorAndMakesNoWorkersCall(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	req := validStartRequest("worker-1", "dispatch-1")
	req.ID = "   "
	if _, err := registry.InvokeSession(ctx, req); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Start() error = %v, want ErrInvalidSessionID", err)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Start() with invalid request called Workers %d times, want 0", execution.callCount())
	}

	if _, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after invalid Start() = %v, want ErrSessionNotFound (no registry mutation)", err)
	}
}

func TestStart_InvalidRequestHasNoRegistryEventsOrWorkersSideEffects(t *testing.T) {
	eventsSvc := newEventsAppender()
	execution := succeedingExecution()
	registry, err := newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	req := validStartRequest("worker-1", "dispatch-1")
	req.ID = "   "

	if _, err := registry.Start(context.Background(), req); !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Start() error = %v, want ErrInvalidSessionID", err)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Workers call count = %d, want 0", execution.callCount())
	}
	if _, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after invalid Start() = %v, want ErrSessionNotFound", err)
	}
	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{
		Topic: workersessions.Topic("worker-1"),
		From:  events.Cursor{Topic: workersessions.Topic("worker-1")},
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("Read() after invalid Start() error = %v, want nil", err)
	}
	if read.Outcome != events.ReadOutcomeAtHead || len(read.Records) != 0 {
		t.Fatalf("Read() after invalid Start() = %+v, want an empty topic", read)
	}
}

type asyncStartOutcome struct {
	result workersessions.StartResult
	err    error
}

func waitForStartSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func waitForAsyncStart(t *testing.T, outcomes <-chan asyncStartOutcome) asyncStartOutcome {
	t.Helper()
	select {
	case outcome := <-outcomes:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("Start did not return at the Workers admission barrier")
		return asyncStartOutcome{}
	}
}

func assertOpeningRecordReady(t *testing.T, eventsSvc events.Service, topic events.Topic) {
	t.Helper()
	opening, err := eventsSvc.Read(context.Background(), events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 1,
	})
	if err != nil || opening.Outcome != events.ReadOutcomeProgress || len(opening.Records) != 1 {
		t.Fatalf("opening Read() = %+v, %v, want one retained record before Start returned", opening, err)
	}
}

func subscribeForTerminal(t *testing.T, eventsSvc events.Service, topic events.Topic) events.Subscription {
	t.Helper()
	subscription, err := eventsSvc.Subscribe(context.Background(), events.SubscribeRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic, Position: 1},
		Limit: 1,
	})
	if err != nil {
		t.Fatalf("terminal Subscribe() error = %v, want nil", err)
	}
	return subscription
}

func assertTerminalDelivery(t *testing.T, subscription events.Subscription) {
	t.Helper()
	waitContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	delivery := subscription.Next(waitContext)
	if delivery.Kind != events.DeliveryRecord || delivery.Record.ID.Position != 2 {
		t.Fatalf("terminal delivery = %+v, want record at position 2", delivery)
	}
}

func TestStart_ReturnsAfterEventReadyAndWorkersAdmission(t *testing.T) {
	eventsSvc := newEventsAppender()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseDispatch := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseDispatch()
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(started)
			<-release
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	registry, err := newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	outcomes := make(chan asyncStartOutcome, 1)
	go func() {
		result, startErr := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		outcomes <- asyncStartOutcome{result: result, err: startErr}
	}()
	waitForStartSignal(t, started, "Workers dispatch did not begin")
	outcome := waitForAsyncStart(t, outcomes)
	if outcome.err != nil {
		t.Fatalf("Start() error = %v, want nil after admission", outcome.err)
	}
	if outcome.result.Session.ID != "worker-1" || outcome.result.Session.State != workersessions.StateRunning {
		t.Fatalf("Start() result session = %+v, want worker-1 RUNNING while dispatch is in flight", outcome.result.Session)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers call count = %d, want 1", execution.callCount())
	}

	topic := workersessions.Topic("worker-1")
	assertOpeningRecordReady(t, eventsSvc, topic)
	subscription := subscribeForTerminal(t, eventsSvc, topic)
	releaseDispatch()
	assertTerminalDelivery(t, subscription)
	final, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() after release error = %v, want nil", err)
	}
	if final.State != workersessions.StateCompleted {
		t.Fatalf("final session state = %q, want COMPLETED", final.State)
	}
}

func TestStart_RequiresReadableAndSubscribableEventsTopic(t *testing.T) {
	eventsSvc := newEventsAppender()
	execution := succeedingExecution()
	registry, err := newService(executionBoundary{execution: execution}, appendOnlyEvents{inner: eventsSvc}, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1"))
	if !errors.Is(err, workersessions.ErrStartNotAccepted) || !errors.Is(err, workersessions.ErrEventTopicUnavailable) {
		t.Fatalf("Start() error = %v, want ErrStartNotAccepted and ErrEventTopicUnavailable", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() session state = %q, want FAILED", result.Session.State)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Workers call count = %d, want 0 when topic readiness fails", execution.callCount())
	}
}

func TestStart_PreAdmissionFailureDoesNotReturnAccepted(t *testing.T) {
	registry, err := newService(
		noAdmissionBoundary{err: errors.New("admission rejected")},
		newEventsAppender(),
		nil,
	)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1"))
	if !errors.Is(err, workersessions.ErrStartNotAccepted) || !errors.Is(err, workersessions.ErrStartAdmissionFailed) {
		t.Fatalf("Start() error = %v, want ErrStartNotAccepted and ErrStartAdmissionFailed", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() session state = %q, want FAILED", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("Start() result = %+v, want a terminal failure cause", result.Session.Result)
	}
}

func TestStart_CancelBeforeAdmissionCannotReturnAcceptedOrDuplicateTerminal(t *testing.T) {
	innerEvents := newEventsAppender()
	eventsSvc := newGatedEvents(innerEvents)
	execution := succeedingExecution()
	registry, err := newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	results := make(chan struct {
		result workersessions.StartResult
		err    error
	}, 1)
	go func() {
		result, startErr := registry.Start(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		results <- struct {
			result workersessions.StartResult
			err    error
		}{result: result, err: startErr}
	}()
	select {
	case <-eventsSvc.subscribeStarted:
	case <-time.After(time.Second):
		t.Fatal("Start did not reach the live topic readiness barrier")
	}

	control, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || control.Outcome != workersessions.ControlOutcomeApplied || control.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() = %#v, %v, want applied CANCELED before admission", control, err)
	}
	close(eventsSvc.releaseSubscribe)

	var outcome struct {
		result workersessions.StartResult
		err    error
	}
	select {
	case outcome = <-results:
	case <-time.After(time.Second):
		t.Fatal("Start did not resolve after pre-admission cancellation")
	}
	if !errors.Is(outcome.err, workersessions.ErrStartNotAccepted) {
		t.Fatalf("Start() error = %v, want ErrStartNotAccepted", outcome.err)
	}
	if outcome.result.Session.State != workersessions.StateCanceled {
		t.Fatalf("Start() session = %+v, want CANCELED", outcome.result.Session)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Workers call count = %d, want 0 after pre-admission cancellation", execution.callCount())
	}

	topic := workersessions.Topic("worker-1")
	read, err := innerEvents.Read(context.Background(), events.ReadRequest{
		Topic: topic,
		From:  events.Cursor{Topic: topic},
		Limit: 10,
	})
	if err != nil || read.Outcome != events.ReadOutcomeProgress || len(read.Records) != 2 {
		t.Fatalf("session topic after cancellation = %+v, %v, want one opening and one terminal record", read, err)
	}
}

// TestStart_BlankNestedDispatchWorkstationName_RejectedBeforeEffects proves a
// malformed resolved dispatch request (missing nested route) is rejected by
// Start's own validation before any registry mutation or Workers call,
// mirroring the identity invariant the Workers boundary itself enforces.
func TestStart_BlankNestedDispatchWorkstationName_RejectedBeforeEffects(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	req := validStartRequest("worker-1", "dispatch-1")
	req.Execution.Execution.Dispatch.WorkstationName = "   "

	if _, err := registry.InvokeSession(ctx, req); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Fatalf("Start() error = %v, want ErrInvalidExecutionRequest", err)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Start() with blank nested workstation name called Workers %d times, want 0", execution.callCount())
	}
	if _, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after rejected Start() = %v, want ErrSessionNotFound (no registry mutation)", err)
	}
}

// TestStart_MismatchedNestedDispatchWorkstationName_RejectedBeforeEffects
// proves a resolved dispatch request whose nested route disagrees with the
// top-level route is rejected before any registry mutation or Workers call,
// instead of reaching Workers and being rejected there after effects.
func TestStart_MismatchedNestedDispatchWorkstationName_RejectedBeforeEffects(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	req := validStartRequest("worker-1", "dispatch-1")
	req.Execution.Execution.Dispatch.WorkstationName = "other-route"

	if _, err := registry.InvokeSession(ctx, req); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Fatalf("Start() error = %v, want ErrInvalidExecutionRequest", err)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Start() with mismatched nested workstation name called Workers %d times, want 0", execution.callCount())
	}
	if _, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after rejected Start() = %v, want ErrSessionNotFound (no registry mutation)", err)
	}
}

// TestStart_WhitespacePaddedNestedDispatchWorkstationName_RejectedBeforeEffects
// proves the exact review-flagged shape is rejected: a nested dispatch route
// that only matches the top-level route after trimming (but not as raw
// values) is exactly what the real Workers boundary's validDispatch rejects
// (it compares the untrimmed nested value against the trimmed top-level
// name), so Start must reject it too, before any registry mutation or
// Workers call, instead of accepting it and letting Workers reject it later.
func TestStart_WhitespacePaddedNestedDispatchWorkstationName_RejectedBeforeEffects(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	req := validStartRequest("worker-1", "dispatch-1")
	req.Execution.Execution.Dispatch.WorkstationName = " review "

	if _, err := registry.InvokeSession(ctx, req); !errors.Is(err, workersessions.ErrInvalidExecutionRequest) {
		t.Fatalf("Start() error = %v, want ErrInvalidExecutionRequest", err)
	}
	if execution.callCount() != 0 {
		t.Fatalf("Start() with whitespace-padded nested workstation name called Workers %d times, want 0", execution.callCount())
	}
	if _, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() after rejected Start() = %v, want ErrSessionNotFound (no registry mutation)", err)
	}
}

func TestStart_ValidNewIdentity_ObservesRunningDuringAdmittedInFlightHandoff(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	execution := &fakeExecution{
		dispatch: func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(started)
			<-release
			return workers.WorkstationDispatchResult{
				Result: workers.WorkResult{Outcome: workers.OutcomeAccepted},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
			t.Errorf("Start() error = %v, want nil", err)
		}
	}()

	<-started
	session, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() during in-flight Start() error = %v, want nil", err)
	}
	if session.State != workersessions.StateRunning {
		t.Fatalf("Get() during admitted in-flight Start() state = %q, want RUNNING", session.State)
	}

	close(release)
	wg.Wait()
}

// TestStart_RetainsExactProviderSessionAssociationFromWorkerResult proves the
// completion bridge commits the exact Providers-owned reference while the
// attempt is still supervised, before terminal publication closes the session.
func TestStart_RetainsExactProviderSessionAssociationFromWorkerResult(t *testing.T) {
	returnedReference := &workers.ProviderSessionMetadata{
		Provider: "codex",
		Kind:     "provider-native-kind",
		ID:       "opaque-provider-session-1",
	}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				WorkstationName: req.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID:      req.Execution.Dispatch.DispatchID,
					Outcome:         workers.OutcomeAccepted,
					ProviderSession: returnedReference,
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	req := validStartRequest("worker-1", "dispatch-1")
	req.Execution.Execution.Dispatch.Execution.RequestID = "turn-1"

	started, err := registry.InvokeSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	association := started.Session.ProviderSessionAssociation
	if association == nil {
		t.Fatal("Start() ProviderSessionAssociation = nil, want the returned exact reference")
	}
	if association.WorkerSessionID != "worker-1" || association.TurnID != "turn-1" ||
		association.DispatchID != "dispatch-1" || association.AttemptID != "dispatch-1" {
		t.Fatalf("association correlation = %#v, want worker/turn/dispatch/attempt identity", association)
	}
	if association.Reference.Provider != "codex" || association.Reference.Kind != "provider-native-kind" ||
		association.Reference.ID != "opaque-provider-session-1" {
		t.Fatalf("association reference = %#v, want the exact provider/kind/id returned by Workers", association.Reference)
	}
	if err := association.Validate(); err != nil {
		t.Fatalf("association.Validate() = %v, want nil", err)
	}

	returnedReference.ID = "worker-mutated"
	association.Reference.ID = "caller-mutated"
	after, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if after.ProviderSessionAssociation == nil || after.ProviderSessionAssociation.Reference.ID != "opaque-provider-session-1" {
		t.Fatalf("stored association after caller mutation = %#v, want original detached reference", after.ProviderSessionAssociation)
	}
}

// TestStart_ProviderProgressCommitsAssociationBeforeOutputAndEnablesResume
// proves the production progress bridge, rather than a test-side direct
// AssociateProviderSession call, commits the exact typed reference while the
// attempt is live. The downstream response publisher observes the committed
// association first, a foreign dispatch cannot publish or replace it, and the
// subsequent pause/resume carries that same reference into the new attempt.
//
// It drives that through the dispatch terminal signal, which is what still
// reaches the downstream publisher: Worker-authored output is committed to the
// Worker Session topic instead (see TestPublish_RoutesWorkerOutputToTheTopic),
// so it is no longer observable at this seam.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestStart_ProviderProgressCommitsAssociationBeforeOutputAndEnablesResume(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-live-1",
	}

	var forwarded []workers.ProgressFragment
	publisher := workersessions.NewProviderSessionObservationPublisher(func(fragment workers.ProgressFragment) {
		current, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
		if err != nil || current.ProviderSessionAssociation == nil {
			t.Fatalf("Get() before forwarding provider output = %#v, %v, want committed association", current, err)
		}
		if got := current.ProviderSessionAssociation.Reference; got != reference {
			t.Fatalf("association before forwarding = %#v, want %#v", got, reference)
		}
		forwarded = append(forwarded, workers.ProgressFragment{
			DispatchID:         fragment.DispatchID,
			Kind:               fragment.Kind,
			ProviderSessionRef: workers.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		})
	})
	publisher.Bind(registry)

	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")
	publisher.Publish(workers.ProgressFragment{
		DispatchID:               "dispatch-1",
		Kind:                     workers.CompletedFragmentKind,
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(),
			Kind:     reference.Kind,
			ID:       reference.ID,
		},
	})
	if len(forwarded) != 1 || forwarded[0].DispatchID != "dispatch-1" ||
		forwarded[0].ProviderSessionRef == nil || forwarded[0].ProviderSessionRef.ID != reference.ID {
		t.Fatalf("forwarded provider output = %#v, want one associated dispatch-1 fragment", forwarded)
	}
	// The typed source reference and the response-stream metadata must agree;
	// otherwise output would advertise a different Provider Session than the
	// one Worker Sessions retained.
	publisher.Publish(workers.ProgressFragment{
		DispatchID:               "dispatch-1",
		Kind:                     workers.CompletedFragmentKind,
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(), Kind: reference.Kind, ID: "mismatched-metadata-session",
		},
	})
	if len(forwarded) != 1 {
		t.Fatalf("forwarded output after mismatched metadata = %#v, want no inconsistent output", forwarded)
	}

	// A reference-bearing fragment from a sibling/foreign dispatch is rejected
	// by Worker Sessions and never reaches the response publisher.
	publisher.Publish(workers.ProgressFragment{
		DispatchID:               "foreign-dispatch",
		Kind:                     workers.CompletedFragmentKind,
		ProviderSessionReference: workers.CloneProviderSessionReference(&reference),
		ProviderSessionRef: &workers.ProviderSessionMetadata{
			Provider: reference.Provider.String(), Kind: reference.Kind, ID: "foreign-session",
		},
	})
	if len(forwarded) != 1 {
		t.Fatalf("forwarded provider output after foreign dispatch = %#v, want no cross-session output", forwarded)
	}

	boundary.setCancel(func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
		boundary.complete(canceledDispatchResult("dispatch-1"), workers.ErrWorkstationDispatchCanceled)
		return workers.WorkstationDispatchCancelResult{DispatchID: "dispatch-1", Outcome: workers.WorkstationDispatchCancelOutcomeCanceled}, nil
	})
	paused, err := registry.Pause(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || paused.Outcome != workersessions.ControlOutcomeApplied || paused.Session.State != workersessions.StatePaused {
		t.Fatalf("Pause() = %#v, %v, want associated PAUSED session", paused, err)
	}

	resumed, err := registry.Resume(context.Background(), workersessions.ControlRequest{ID: "worker-1"})
	if err != nil || resumed.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Resume() = %#v, %v, want applied exact continuation", resumed, err)
	}
	continuation := boundary.currentRequest()
	if continuation.Execution.ResumeSession == nil || *continuation.Execution.ResumeSession != reference {
		t.Fatalf("continuation ResumeSession = %#v, want exact %#v", continuation.Execution.ResumeSession, reference)
	}

	continued := completedDispatchResult(resumed.DispatchID)
	continued.Result.ProviderSession = &workers.ProviderSessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	}
	boundary.complete(continued, nil)
	if result := <-started; result.Session.State != workersessions.StateCompleted ||
		result.Session.ProviderSessionAssociation == nil || result.Session.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("Start() terminal result = %#v, want completed session retaining %#v", result, reference)
	}
}

// TestObserveProviderSession_RejectsUntrustedAndConflictingObservations
// exercises the trust boundary used by the live progress bridge: malformed
// source facts, an unknown dispatch, and a reference rewrite are all rejected
// without changing the first trusted association.
func TestObserveProviderSession_RejectsUntrustedAndConflictingObservations(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	started := startControlledSession(t, registry, boundary, "worker-1", "dispatch-1")

	trusted := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-trusted-1",
	}
	if _, err := registry.ObserveProviderSession(context.Background(), workersessions.ProviderSessionObservationRequest{
		DispatchID: "dispatch-1",
		Reference: providers.SessionRef{
			Kind: trusted.Kind,
			ID:   trusted.ID,
		},
	}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ObserveProviderSession() with invalid reference error = %v, want Providers ErrInvalidID", err)
	}
	if _, err := registry.ObserveProviderSession(context.Background(), workersessions.ProviderSessionObservationRequest{
		DispatchID: "foreign-dispatch",
		Reference:  trusted,
	}); !errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch) {
		t.Fatalf("ObserveProviderSession() for foreign dispatch error = %v, want ErrProviderSessionAssociationAttemptMismatch", err)
	}

	accepted, err := registry.ObserveProviderSession(context.Background(), workersessions.ProviderSessionObservationRequest{
		DispatchID: "dispatch-1",
		Reference:  trusted,
	})
	if err != nil || accepted.Outcome != workersessions.ProviderSessionAssociationOutcomeAccepted {
		t.Fatalf("ObserveProviderSession() = %#v, %v, want accepted", accepted, err)
	}
	duplicate, err := registry.ObserveProviderSession(context.Background(), workersessions.ProviderSessionObservationRequest{
		DispatchID: "dispatch-1",
		Reference:  trusted,
	})
	if err != nil || duplicate.Outcome != workersessions.ProviderSessionAssociationOutcomeDuplicate {
		t.Fatalf("duplicate ObserveProviderSession() = %#v, %v, want duplicate", duplicate, err)
	}
	conflicting := trusted
	conflicting.ID = "provider-session-conflict"
	if _, err := registry.ObserveProviderSession(context.Background(), workersessions.ProviderSessionObservationRequest{
		DispatchID: "dispatch-1",
		Reference:  conflicting,
	}); !errors.Is(err, workersessions.ErrProviderSessionAssociationConflict) {
		t.Fatalf("conflicting ObserveProviderSession() error = %v, want ErrProviderSessionAssociationConflict", err)
	}

	snapshot, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil || snapshot.ProviderSessionAssociation == nil || snapshot.ProviderSessionAssociation.Reference != trusted {
		t.Fatalf("Get() association after rejected observations = %#v, %v, want %#v", snapshot.ProviderSessionAssociation, err, trusted)
	}
	boundary.complete(completedDispatchResult("dispatch-1"), nil)
	<-started
}

// TestAssociateProviderSession_CommitsBeforeDependentWorkerRecord proves a
// Worker attempt can record its exact reference before it emits a source
// record whose ProviderSessionRef relies on that association. The resulting
// automatic completion observation is an idempotent duplicate, not a rewrite.
func TestAssociateProviderSession_CommitsBeforeDependentWorkerRecord(t *testing.T) {
	eventsSvc := newEventsAppender()
	var registry workersessions.Service
	var association workersessions.ProviderSessionAssociationResult
	var published workersessions.PublishRecordResult
	execution := &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			var err error
			association, err = registry.AssociateProviderSession(ctx, workersessions.ProviderSessionAssociationRequest{
				WorkerSessionID: "worker-1",
				DispatchID:      req.Execution.Dispatch.DispatchID,
				Reference: providers.SessionRef{
					Provider: "claude",
					Kind:     "conversation-token",
					ID:       "opaque-provider-session-2",
				},
			})
			if err != nil {
				t.Fatalf("AssociateProviderSession() error = %v, want nil", err)
			}
			stored, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
			if err != nil || stored.ProviderSessionAssociation == nil {
				t.Fatalf("Get() before dependent record = %#v, %v, want committed association", stored, err)
			}
			published, err = registry.PublishRecord(ctx, workersessions.PublishRecordRequest{
				SessionID: "worker-1",
				Draft: workers.Draft{
					Kind:               workers.KindTool,
					Phase:              workers.PhaseStarted,
					Payload:            []byte(`{"toolCallId":"tool-1","toolName":"edit"}`),
					DispatchID:         req.Execution.Dispatch.DispatchID,
					ProviderSessionRef: "opaque-provider-session-2",
				},
				SourceType:     "worker_provider",
				SourceID:       "provider-attempt-1",
				SourceSequence: 1,
				SourceEventID:  "provider-event-1",
				SchemaID:       "workers.draft.v1",
			})
			if err != nil {
				t.Fatalf("PublishRecord() error = %v, want nil", err)
			}
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				WorkstationName: req.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
					ProviderSession: &workers.ProviderSessionMetadata{
						Provider: "claude", Kind: "conversation-token", ID: "opaque-provider-session-2",
					},
				},
			}, nil
		},
	}
	var err error
	registry, err = newService(executionBoundary{execution: execution}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	if _, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if association.Outcome != workersessions.ProviderSessionAssociationOutcomeAccepted {
		t.Fatalf("AssociateProviderSession() outcome = %q, want ACCEPTED", association.Outcome)
	}
	if published.Outcome != workersessions.PublishOutcomeAccepted {
		t.Fatalf("PublishRecord() outcome = %v, want accepted", published.Outcome)
	}

	read, err := eventsSvc.Read(context.Background(), events.ReadRequest{
		Topic: workersessions.Topic("worker-1"), From: events.Cursor{Topic: workersessions.Topic("worker-1")}, Limit: 10,
	})
	if err != nil {
		t.Fatalf("Read() error = %v, want nil", err)
	}
	if len(read.Records) != 3 {
		t.Fatalf("Read() records = %d, want opening, associated tool, terminal", len(read.Records))
	}
	tool := decodeDraft(t, read.Records[1])
	if tool.ProviderSessionRef != "opaque-provider-session-2" || read.Records[1].ID.Position != published.AggregateSequence {
		t.Fatalf("dependent Worker record = %#v at %d, want committed associated tool record at %d", tool, read.Records[1].ID.Position, published.AggregateSequence)
	}
}

// TestAssociateProviderSession_IsIdempotentAndRejectsConflictsWithoutCrossSessionMutation
// proves exact retries retain their original association, while a conflicting
// reference cannot replace it or affect a sibling Worker Session.
func TestAssociateProviderSession_IsIdempotentAndRejectsConflictsWithoutCrossSessionMutation(t *testing.T) {
	started := make(chan string, 2)
	release := map[string]chan struct{}{
		"dispatch-a": make(chan struct{}),
		"dispatch-b": make(chan struct{}),
	}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			started <- req.Execution.Dispatch.DispatchID
			<-release[req.Execution.Dispatch.DispatchID]
			return workers.WorkstationDispatchResult{Result: workers.WorkResult{Outcome: workers.OutcomeAccepted}}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	var starts sync.WaitGroup
	starts.Add(2)
	for _, identity := range []struct{ sessionID, dispatchID string }{
		{sessionID: "worker-a", dispatchID: "dispatch-a"},
		{sessionID: "worker-b", dispatchID: "dispatch-b"},
	} {
		go func(sessionID, dispatchID string) {
			defer starts.Done()
			if _, err := registry.InvokeSession(context.Background(), validStartRequest(sessionID, dispatchID)); err != nil {
				t.Errorf("Start(%q) error = %v, want nil", sessionID, err)
			}
		}(identity.sessionID, identity.dispatchID)
	}
	<-started
	<-started

	first := workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-a", DispatchID: "dispatch-a",
		Reference: providers.SessionRef{Provider: "codex", Kind: "thread", ID: "opaque-a"},
	}
	accepted, err := registry.AssociateProviderSession(context.Background(), first)
	if err != nil || accepted.Outcome != workersessions.ProviderSessionAssociationOutcomeAccepted {
		t.Fatalf("first AssociateProviderSession() = %#v, %v, want accepted", accepted, err)
	}
	duplicate, err := registry.AssociateProviderSession(context.Background(), first)
	if err != nil || duplicate.Outcome != workersessions.ProviderSessionAssociationOutcomeDuplicate {
		t.Fatalf("duplicate AssociateProviderSession() = %#v, %v, want duplicate", duplicate, err)
	}
	conflict := first
	conflict.Reference.ID = "opaque-conflict"
	if _, err := registry.AssociateProviderSession(context.Background(), conflict); !errors.Is(err, workersessions.ErrProviderSessionAssociationConflict) {
		t.Fatalf("conflicting AssociateProviderSession() error = %v, want ErrProviderSessionAssociationConflict", err)
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-b", DispatchID: "dispatch-b",
		Reference: providers.SessionRef{Provider: "claude", Kind: "thread", ID: "opaque-b"},
	}); err != nil {
		t.Fatalf("sibling AssociateProviderSession() error = %v, want nil", err)
	}

	for _, want := range []struct{ sessionID, referenceID string }{
		{sessionID: "worker-a", referenceID: "opaque-a"},
		{sessionID: "worker-b", referenceID: "opaque-b"},
	} {
		snapshot, err := registry.Get(context.Background(), workersessions.GetRequest{ID: want.sessionID})
		if err != nil || snapshot.ProviderSessionAssociation == nil || snapshot.ProviderSessionAssociation.Reference.ID != want.referenceID {
			t.Fatalf("Get(%q) association = %#v, %v, want independent reference %q", want.sessionID, snapshot.ProviderSessionAssociation, err, want.referenceID)
		}
	}
	close(release["dispatch-a"])
	close(release["dispatch-b"])
	starts.Wait()
}

// TestAssociateProviderSession_RejectsMalformedOrForeignAttemptWithoutWriting
// proves malformed provider identity and a dispatch from another attempt are
// explicit typed failures that leave the live session unassociated.
func TestAssociateProviderSession_RejectsMalformedOrForeignAttemptWithoutWriting(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(started)
			<-release
			return workers.WorkstationDispatchResult{Result: workers.WorkResult{Outcome: workers.OutcomeAccepted}}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	done := make(chan error, 1)
	go func() {
		_, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
		done <- err
	}()
	<-started

	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1", DispatchID: "dispatch-1",
		Reference: providers.SessionRef{Kind: "thread", ID: "opaque-invalid"},
	}); !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("malformed AssociateProviderSession() error = %v, want Providers ErrInvalidID", err)
	}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1", DispatchID: "foreign-dispatch",
		Reference: providers.SessionRef{Provider: "codex", Kind: "thread", ID: "opaque-foreign"},
	}); !errors.Is(err, workersessions.ErrProviderSessionAssociationAttemptMismatch) {
		t.Fatalf("foreign-attempt AssociateProviderSession() error = %v, want ErrProviderSessionAssociationAttemptMismatch", err)
	}
	snapshot, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if snapshot.ProviderSessionAssociation != nil {
		t.Fatalf("Get() association after rejections = %#v, want nil", snapshot.ProviderSessionAssociation)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
}

// TestAssociateProviderSession_RejectsMissingOrTerminalSessionWithoutWriting
// proves an association can only be committed to its still-live Worker
// Session. Missing and terminal sessions return typed failures without
// synthesizing or mutating an association.
func TestAssociateProviderSession_RejectsMissingOrTerminalSessionWithoutWriting(t *testing.T) {
	ctx := context.Background()
	registry := newRegistryWithExecution(succeedingExecution())
	reference := providers.SessionRef{Provider: "codex", Kind: "thread", ID: "opaque-session"}

	if _, err := registry.AssociateProviderSession(ctx, workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "missing-worker", DispatchID: "dispatch-1", Reference: reference,
	}); !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("AssociateProviderSession() for missing session error = %v, want ErrSessionNotFound", err)
	}

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if _, err := registry.AssociateProviderSession(ctx, workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "worker-1", DispatchID: "dispatch-1", Reference: reference,
	}); !errors.Is(err, workersessions.ErrProviderSessionAssociationNotAvailable) {
		t.Fatalf("AssociateProviderSession() for terminal session error = %v, want ErrProviderSessionAssociationNotAvailable", err)
	}
	snapshot, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if snapshot.ProviderSessionAssociation != nil {
		t.Fatalf("Get() association after terminal rejection = %#v, want nil", snapshot.ProviderSessionAssociation)
	}
}

// TestStart_AbsentProviderSessionDoesNotSynthesizeAssociation proves a
// successful Worker result remains explicitly unassociated when the provider
// did not supply a complete reference; runner, model, and dispatch identity
// are never used as substitutes.
func TestStart_AbsentProviderSessionDoesNotSynthesizeAssociation(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	req := validStartRequest("worker-1", "dispatch-1")
	req.Execution.Execution.RunnerID = "codex"
	req.Execution.Execution.Model = "gpt-5"

	started, err := registry.InvokeSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if started.Session.ProviderSessionAssociation != nil {
		t.Fatalf("Start() ProviderSessionAssociation = %#v, want nil without a provider-supplied reference", started.Session.ProviderSessionAssociation)
	}
}

// TestStart_InvalidProviderSessionResultRemainsExplicitlyUnassociated proves a
// malformed reference returned by Workers cannot create a false resumable
// association. The completion path reports its rejection with safe
// correlation fields and keeps the terminal Worker Session unassociated.
func TestStart_InvalidProviderSessionResultRemainsExplicitlyUnassociated(t *testing.T) {
	logger := &recordingLogger{}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				WorkstationName: req.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
					ProviderSession: &workers.ProviderSessionMetadata{
						Kind: "thread", ID: "opaque-invalid",
					},
				},
			}, nil
		},
	}
	registry, err := newService(executionBoundary{execution: execution}, newEventsAppender(), logger)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	started, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if started.Session.ProviderSessionAssociation != nil {
		t.Fatalf("Start() ProviderSessionAssociation = %#v, want nil after invalid Provider result", started.Session.ProviderSessionAssociation)
	}
	rejected := logger.entriesFor("worker session provider session association from result rejected")
	if len(rejected) != 1 {
		t.Fatalf("rejected result association log entries = %d, want 1", len(rejected))
	}
	if rejected[0].fields["sessionID"] != "worker-1" || rejected[0].fields["attemptID"] != "dispatch-1" ||
		rejected[0].fields["outcome"] != "rejected" {
		t.Fatalf("rejected result association log fields = %#v, want safe session/attempt/rejected fields", rejected[0].fields)
	}
}

// gatingLogger is a recordingLogger whose Info call blocks after recording
// any call whose message equals gateMessage, until the test signals proceed.
// This turns the production "worker session start accepted" operation log --
// which Start emits after reserveIfAbsent and before transitionToStarting --
// into a deterministic synchronization point, without adding a test-only
// hook to production code.
type gatingLogger struct {
	recordingLogger
	gateMessage string
	reached     chan struct{}
	proceed     chan struct{}
	once        sync.Once
}

func (l *gatingLogger) Info(message string, keysAndValues ...any) {
	l.recordingLogger.Info(message, keysAndValues...)
	if message == l.gateMessage {
		l.once.Do(func() { close(l.reached) })
		<-l.proceed
	}
}

// TestStart_ReservationIsObservableBeforeWorkersHandoff proves, through the
// public Start API and a controlled Workers fake (never calling
// reserveIfAbsent/transitionToStarting directly), that a brand-new identity
// is committed and observable as RESERVED strictly before the injected
// workers.WorkstationExecutionService receives DispatchWorkstation. The
// production "worker session start accepted" log -- emitted right after the
// RESERVED write and before the STARTING transition -- gates the Start
// goroutine so a concurrent Get() can observe RESERVED, and the fake's own
// call count proves Workers has not yet been invoked at that point.
func TestStart_ReservationIsObservableBeforeWorkersHandoff(t *testing.T) {
	execution := succeedingExecution()
	logger := &gatingLogger{
		gateMessage: "worker session start accepted",
		reached:     make(chan struct{}),
		proceed:     make(chan struct{}),
	}
	registry, err := newService(executionBoundary{execution: execution}, newEventsAppender(), logger)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
			t.Errorf("Start() error = %v, want nil", err)
		}
	}()

	<-logger.reached
	session, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() while Start() is paused at the reservation log error = %v, want nil", err)
	}
	if session.State != workersessions.StateReserved {
		t.Fatalf("Get() while Start() is paused at the reservation log state = %q, want RESERVED", session.State)
	}
	if got := execution.callCount(); got != 0 {
		t.Fatalf("Workers received %d dispatch calls before the reservation was observed, want 0", got)
	}

	close(logger.proceed)
	wg.Wait()

	if got := execution.callCount(); got != 1 {
		t.Fatalf("Workers received %d dispatch calls after Start() completed, want 1", got)
	}
}

func TestStart_ReuseAlreadyReservedIdentity_DoesNotCreateAReplacementSession(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.ID != "worker-1" || result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() session = %+v, want ID=worker-1 State=COMPLETED", result.Session)
	}
}

func TestStart_ExactlyOneDetachedRequestReachesWorkersWithExpectedAttemptIdentity(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	requests := execution.requests()
	if len(requests) != 1 {
		t.Fatalf("Workers received %d dispatch calls, want 1", len(requests))
	}
	if got := requests[0].Execution.Dispatch.DispatchID; got != "dispatch-1" {
		t.Fatalf("dispatch request attempt id = %q, want dispatch-1", got)
	}
}

// TestStart_ClonesExecutionRequestBeforeHandoff proves Start hands Workers a
// detached clone of req.Execution, not a shallow alias: an injected
// implementation mutating a reference-backed field (EnvVars) on the request
// it receives must never be able to mutate the caller's original
// StartRequest.
func TestStart_ClonesExecutionRequestBeforeHandoff(t *testing.T) {
	req := validStartRequest("worker-1", "dispatch-1")
	req.Execution.Execution.EnvVars = map[string]string{"SAFE": "value"}
	req.Execution.Execution.ProcessEnvironment = []string{"PATH=/usr/bin"}
	req.Execution.Execution.InputTokens = []any{"token-a"}

	execution := &fakeExecution{
		dispatch: func(_ context.Context, received workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			// An injected implementation is entitled to treat the request it
			// receives as its own: mutate the reference-backed fields it was
			// handed, exactly as a real executor implementation could.
			received.Execution.EnvVars["INJECTED"] = "mutated-by-executor"
			received.Execution.ProcessEnvironment[0] = "mutated"
			received.Execution.InputTokens[0] = "mutated"
			return workers.WorkstationDispatchResult{
				DispatchID: received.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: received.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, req); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	if _, mutated := req.Execution.Execution.EnvVars["INJECTED"]; mutated {
		t.Fatalf("original request EnvVars mutated by injected execution: %v", req.Execution.Execution.EnvVars)
	}
	if got := req.Execution.Execution.EnvVars["SAFE"]; got != "value" {
		t.Fatalf("original request EnvVars[SAFE] = %q, want unchanged %q", got, "value")
	}
	if got := req.Execution.Execution.ProcessEnvironment[0]; got != "PATH=/usr/bin" {
		t.Fatalf("original request ProcessEnvironment[0] = %q, want unchanged", got)
	}
	if got := req.Execution.Execution.InputTokens[0]; got != "token-a" {
		t.Fatalf("original request InputTokens[0] = %v, want unchanged", got)
	}
}

func TestStart_SuccessfulWorkersResult_TerminalizesCompleted(t *testing.T) {
	registry := newRegistryWithExecution(succeedingExecution())
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("Start() state = %q, want COMPLETED", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Outcome != workersessions.TerminalOutcomeCompleted || result.Session.Result.Cause != nil {
		t.Fatalf("Start() result = %+v, want COMPLETED with nil Cause", result.Session.Result)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateCompleted {
		t.Fatalf("Get() after Start() state = %q, want COMPLETED", got.State)
	}
}

func TestStart_DispatchAdmissionFailure_TerminalizesFailedWithStartFailureCauseAndNoRunningObservation(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("Start() result = %+v, want a non-nil Cause", result.Session.Result)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseStartFailure {
		t.Fatalf("Start() cause kind = %q, want START_FAILURE", got)
	}
}

func TestStart_OrdinaryFailedWorkResult_TerminalizesFailedWithWorkersExecutionFailureCause(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "the business rule rejected this attempt",
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseWorkersExecutionFailure {
		t.Fatalf("Start() cause kind = %q, want WORKERS_EXECUTION_FAILURE", got)
	}
}

func TestStart_ResultAndErrorDisagreement_TrustsWorkResultOutcomeOverAdapterError(t *testing.T) {
	// TerminalOutcome (adapter-error-derived) says COMPLETED, but the nested
	// WorkResult says FAILED. Worker Sessions must classify FAILED: the
	// Workers result is authoritative over the adapter's own summary field.
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "silently disagreeing failure",
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED despite a COMPLETED-looking dispatch TerminalOutcome", result.Session.State)
	}
}

func TestStart_ResultSuccessDisagreesWithNonNilAdapterError_TerminalizesFailedWithCause(t *testing.T) {
	// A successful WorkResult cannot erase a non-nil adapter error. The
	// contradictory evidence must remain visible as a failed terminal result.
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, errors.New("cosmetic non-nil error alongside an accepted result")
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED for contradictory completion evidence", result.Session.State)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil ||
		strings.TrimSpace(result.Session.Result.Cause.Detail) == "" {
		t.Fatalf("Start() result = %#v, want a non-empty failure cause", result.Session.Result)
	}
}

func TestStart_ExecutorPanicEvidenceWithNilAdapterError_MapsToExecutorPanicCause(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "executor panic: boom",
				},
			}, nil // adapter error is nil despite the panic evidence in the result
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateFailed {
		t.Fatalf("Start() state = %q, want FAILED", result.Session.State)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("Start() cause kind = %q, want EXECUTOR_PANIC despite a nil adapter error", got)
	}
}

func TestStart_ExecutorPanicTypedAdapterError_MapsToExecutorPanicCause(t *testing.T) {
	panicErr := &workers.WorkerExecutorPanicError{Cause: "boom"}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      panicErr.Error(),
				},
			}, panicErr
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseExecutorPanic {
		t.Fatalf("Start() cause kind = %q, want EXECUTOR_PANIC", got)
	}
}

func TestStart_FailedResultWithNonPanicAdapterError_MapsToAdapterFailureCause(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      "transport reset",
				},
			}, errors.New("transport reset")
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("Start() cause kind = %q, want ADAPTER_FAILURE", got)
	}
}

// TestStart_FailedResultWithBlankWorkResultError_UsesGenericAdapterFailureDetail
// proves Detail never falls back to a raw adapter error message: with no
// WorkResult.FailureMetadata available, Detail is the fixed generic
// placeholder for the classified Kind, not the adapter error's own text.
func TestStart_FailedResultWithBlankWorkResultError_UsesGenericAdapterFailureDetail(t *testing.T) {
	adapterErr := errors.New("transport reset")
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
				},
			}, adapterErr
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Kind; got != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("Start() cause kind = %q, want ADAPTER_FAILURE", got)
	}
	if got := result.Session.Result.Cause.Detail; got == adapterErr.Error() || got == "" {
		t.Fatalf("Start() cause detail = %q, want a fixed generic placeholder, not the raw adapter error text", got)
	}
}

// TestStart_FailedResultWithSensitivePromptOrCommandText_NeverExposesItInDetail
// proves the exact review concern directly: raw WorkResult.Error/adapter
// error text that looks like an ordinary sentence (no secret/token/path/URL
// marker a blacklist could catch) containing a prompt or raw provider
// command is still never attached to the public FailureCause.Detail, because
// Detail is never built from that free-form text at all.
func TestStart_FailedResultWithSensitivePromptOrCommandText_NeverExposesItInDetail(t *testing.T) {
	sensitive := "codex exec summarize confidential acquisition memo for board review"
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeFailed,
					Error:      sensitive,
				},
			}, errors.New(sensitive)
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	result, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if got := result.Session.Result.Cause.Detail; got == sensitive || strings.Contains(got, "confidential acquisition") {
		t.Fatalf("Start() cause detail = %q, leaked raw prompt/command text", got)
	}
}

func TestStart_DuplicateConflictingStart_ReturnsTypedErrorAndMakesNoWorkersCall(t *testing.T) {
	execution := succeedingExecution()
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	first, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("first Start() error = %v, want nil", err)
	}
	if first.Session.State != workersessions.StateCompleted {
		t.Fatalf("first Start() state = %q, want COMPLETED", first.Session.State)
	}

	callsBefore := execution.callCount()
	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-2")); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("second Start() on a terminal session error = %v, want ErrSessionNotStartable", err)
	}
	if execution.callCount() != callsBefore {
		t.Fatalf("conflicting Start() called Workers, want no additional calls")
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if !sessionsEqual(got, first.Session) {
		t.Fatalf("Get() after conflicting Start() = %+v, want unchanged %+v", got, first.Session)
	}
}

// sessionsEqual compares two detached Session snapshots by value, following
// the Result pointer, since Get/Start return freshly cloned pointers even
// when the underlying committed outcome is unchanged.
func sessionsEqual(a, b workersessions.Session) bool {
	if a.ID != b.ID || a.State != b.State {
		return false
	}
	if (a.Result == nil) != (b.Result == nil) {
		return false
	}
	if a.Result == nil {
		return true
	}
	if a.Result.Outcome != b.Result.Outcome {
		return false
	}
	if (a.Result.Cause == nil) != (b.Result.Cause == nil) {
		return false
	}
	if a.Result.Cause == nil {
		return true
	}
	return *a.Result.Cause == *b.Result.Cause
}

func TestStart_MissingReservedIdentityCollidingWithInFlightStart_ReturnsTypedErrorAndMakesNoWorkersCall(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	execution := &fakeExecution{
		dispatch: func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(started)
			<-release
			return workers.WorkstationDispatchResult{
				Result: workers.WorkResult{Outcome: workers.OutcomeAccepted},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
			t.Errorf("first Start() error = %v, want nil", err)
		}
	}()
	<-started

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-2")); !errors.Is(err, workersessions.ErrSessionNotStartable) {
		t.Fatalf("concurrent Start() on an in-flight session error = %v, want ErrSessionNotStartable", err)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers received %d dispatch calls while in-flight, want 1", execution.callCount())
	}

	close(release)
	wg.Wait()
}

func TestStart_TerminalStateIsAbsorbingUnderConcurrentGetAndList(t *testing.T) {
	registry := newRegistryWithExecution(succeedingExecution())
	ctx := context.Background()

	if _, err := registry.InvokeSession(ctx, validStartRequest("worker-1", "dispatch-1")); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	var wg sync.WaitGroup
	const readers = 50
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			session, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
			if err != nil {
				t.Errorf("Get() error = %v, want nil", err)
				return
			}
			if err := session.Validate(); err != nil {
				t.Errorf("Get() returned an invalid terminal snapshot: %v", err)
			}
			if session.State != workersessions.StateCompleted {
				t.Errorf("Get() state = %q, want COMPLETED (absorbing)", session.State)
			}
			result, err := registry.List(ctx, workersessions.ListRequest{})
			if err != nil {
				t.Errorf("List() error = %v, want nil", err)
				return
			}
			for _, s := range result.Sessions {
				if err := s.Validate(); err != nil {
					t.Errorf("List() returned an invalid snapshot: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentStart_DistinctSessions_TerminalizeIndependentlyWithoutCrossAssignment(t *testing.T) {
	ctx := context.Background()
	const count = 100

	dispatches := make(chan workers.WorkstationDispatchRequest, count)
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			dispatches <- req
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)

	var wg sync.WaitGroup
	wg.Add(count)
	for i := range count {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i)
			attemptID := fmt.Sprintf("dispatch-%d", i)
			result, err := registry.InvokeSession(ctx, validStartRequest(id, attemptID))
			if err != nil {
				t.Errorf("Start(%q) error = %v, want nil", id, err)
				return
			}
			if result.Session.ID != id {
				t.Errorf("Start(%q) returned session ID %q, cross-assigned identity", id, result.Session.ID)
			}
			if result.Session.State != workersessions.StateCompleted {
				t.Errorf("Start(%q) state = %q, want COMPLETED", id, result.Session.State)
			}
		}(i)
	}
	wg.Wait()
	close(dispatches)

	seen := make(map[string]bool, count)
	for req := range dispatches {
		if seen[req.Execution.Dispatch.DispatchID] {
			t.Fatalf("attempt id %q dispatched more than once", req.Execution.Dispatch.DispatchID)
		}
		seen[req.Execution.Dispatch.DispatchID] = true
	}
	if len(seen) != count {
		t.Fatalf("saw %d distinct dispatched attempt ids, want %d", len(seen), count)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != count {
		t.Fatalf("List() returned %d sessions, want %d", len(result.Sessions), count)
	}
	for _, session := range result.Sessions {
		if err := session.Validate(); err != nil {
			t.Errorf("List() returned an invalid session %q: %v", session.ID, err)
		}
		if session.State != workersessions.StateCompleted {
			t.Errorf("session %q state = %q, want COMPLETED", session.ID, session.State)
		}
	}
}

// TestInvokeSession_RetryableFailureUsesOneSessionAcrossAttempts proves the
// retry loop keeps the Worker Session identity and publication window stable
// while minting a distinct attempt dispatch ID. The first timeout is a
// retryable Workers result; the second attempt completes authoritatively.
func TestInvokeSession_RetryableFailureUsesOneSessionAcrossAttempts(t *testing.T) {
	var calls int
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			calls++
			if calls == 1 {
				return workers.WorkstationDispatchResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Result: workers.WorkResult{
						DispatchID: req.Execution.Dispatch.DispatchID,
						Outcome:    workers.OutcomeFailed,
						FailureMetadata: &workers.WorkFailureMetadata{
							Family: workers.WorkFailureFamilyRetryable,
							Type:   workers.WorkFailureTypeTimeout,
						},
					},
				}, nil
			}
			return workers.WorkstationDispatchResult{
				DispatchID: req.Execution.Dispatch.DispatchID,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
	registry := newRegistryWithExecution(execution)
	request := validStartRequest("worker-retry", "dispatch-1")
	request.Retry = workersessions.RetryPolicy{MaxAttempts: 2}

	result, err := registry.InvokeSession(context.Background(), request)
	if err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("InvokeSession() state = %q, want COMPLETED", result.Session.State)
	}
	if result.Attempts != 2 {
		t.Fatalf("InvokeSession() attempts = %d, want 2", result.Attempts)
	}
	requests := execution.requests()
	if len(requests) != 2 {
		t.Fatalf("Workers received %d dispatches, want 2", len(requests))
	}
	if got := requests[0].Execution.Dispatch.DispatchID; got != "dispatch-1" {
		t.Fatalf("first attempt dispatch ID = %q, want dispatch-1", got)
	}
	if got := requests[1].Execution.Dispatch.DispatchID; got != "dispatch-1/attempt/2" {
		t.Fatalf("retry attempt dispatch ID = %q, want dispatch-1/attempt/2", got)
	}
	if result.Session.ID != request.ID {
		t.Fatalf("retry session ID = %q, want %q", result.Session.ID, request.ID)
	}
}
