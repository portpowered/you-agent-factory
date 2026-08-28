package service_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newRegistry() workersessions.Service {
	return newRegistryWithExecution(succeedingExecution())
}

func newRegistryWithExecution(execution any) workersessions.Service {
	registry, err := newService(execution, newEventsAppender(), logging.NoopLogger{})
	if err != nil {
		panic(fmt.Sprintf("service.New() error = %v, want nil", err))
	}
	return registry
}

func TestReserve_ValidIdentity_StoresSessionInReservedState(t *testing.T) {
	registry := newRegistry()

	session, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if session.ID != "worker-1" || session.State != workersessions.StateReserved {
		t.Fatalf("Reserve() = %+v, want ID=worker-1 State=RESERVED", session)
	}
}

func TestReserve_InvalidIdentity_ReturnsTypedValidationError(t *testing.T) {
	registry := newRegistry()

	_, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "   "})
	if !errors.Is(err, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Reserve() error = %v, want ErrInvalidSessionID", err)
	}
}

func TestReserve_DuplicateIdentity_ReturnsTypedErrorAndLeavesExistingSessionUnchanged(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	first, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("first Reserve() error = %v, want nil", err)
	}

	_, err = registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"})
	if !errors.Is(err, workersessions.ErrSessionAlreadyExists) {
		t.Fatalf("duplicate Reserve() error = %v, want ErrSessionAlreadyExists", err)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != first {
		t.Fatalf("Get() after duplicate Reserve() = %+v, want unchanged %+v", got, first)
	}
}

func TestGet_UnknownIdentity_ReturnsTypedNotFoundDistinctFromValidationFailure(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	_, notFoundErr := registry.Get(ctx, workersessions.GetRequest{ID: "missing"})
	if !errors.Is(notFoundErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("Get() error = %v, want ErrSessionNotFound", notFoundErr)
	}

	_, invalidErr := registry.Get(ctx, workersessions.GetRequest{ID: ""})
	if !errors.Is(invalidErr, workersessions.ErrInvalidSessionID) {
		t.Fatalf("Get() error = %v, want ErrInvalidSessionID", invalidErr)
	}

	if errors.Is(notFoundErr, workersessions.ErrInvalidSessionID) || errors.Is(invalidErr, workersessions.ErrSessionNotFound) {
		t.Fatalf("not-found and validation errors must be distinguishable")
	}
}

func TestGet_RepeatedCalls_ReturnSameIdentityAndCurrentState(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	first, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("first Get() error = %v, want nil", err)
	}
	second, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("second Get() error = %v, want nil", err)
	}
	if first != second {
		t.Fatalf("repeated Get() = %+v then %+v, want identical snapshots", first, second)
	}
}

func TestGet_MutatingReturnedSnapshot_DoesNotAffectLaterGet(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	snapshot, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	snapshot.State = workersessions.StateTerminated
	snapshot.ID = "mutated"
	t.Logf("mutated local snapshot: %+v", snapshot)

	after, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if after.State != workersessions.StateReserved || after.ID != "worker-1" {
		t.Fatalf("mutating a returned snapshot leaked into the registry: got %+v", after)
	}
}

func TestList_EmptyRegistry_ReturnsSuccessfulEmptyResult(t *testing.T) {
	registry := newRegistry()

	result, err := registry.List(context.Background(), workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("List() sessions = %v, want empty", result.Sessions)
	}
}

func TestList_MultipleInsertionOrders_ReturnSameDeterministicOrdering(t *testing.T) {
	ctx := context.Background()
	ids := []string{"worker-c", "worker-a", "worker-b"}
	reverseIDs := []string{"worker-b", "worker-a", "worker-c"}

	forward := newRegistry()
	for _, id := range ids {
		if _, err := forward.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
			t.Fatalf("Reserve(%q) error = %v, want nil", id, err)
		}
	}
	reversed := newRegistry()
	for _, id := range reverseIDs {
		if _, err := reversed.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
			t.Fatalf("Reserve(%q) error = %v, want nil", id, err)
		}
	}

	forwardResult, err := forward.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	reversedResult, err := reversed.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	wantOrder := []string{"worker-a", "worker-b", "worker-c"}
	if got := idsOf(forwardResult.Sessions); !equalStrings(got, wantOrder) {
		t.Fatalf("List() order = %v, want %v", got, wantOrder)
	}
	if got := idsOf(reversedResult.Sessions); !equalStrings(got, wantOrder) {
		t.Fatalf("List() order = %v, want %v", got, wantOrder)
	}
}

func TestList_FilterByState_ReturnsExactlyMatchingSessions(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()

	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}
	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-2"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{workersessions.StateRunning}}})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("List() filtered by RUNNING = %v, want empty (all sessions are RESERVED)", result.Sessions)
	}

	result, err = registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{workersessions.StateReserved}}})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if got := idsOf(result.Sessions); !equalStrings(got, []string{"worker-1", "worker-2"}) {
		t.Fatalf("List() filtered by RESERVED = %v, want [worker-1 worker-2]", got)
	}
}

func TestList_InvalidFilter_ReturnsTypedValidationErrorAndNoPartialResult(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{Filter: workersessions.Filter{States: []workersessions.State{"INTERRUPTED"}}})
	if !errors.Is(err, workersessions.ErrInvalidState) {
		t.Fatalf("List() error = %v, want ErrInvalidState", err)
	}
	if len(result.Sessions) != 0 {
		t.Fatalf("List() with invalid filter returned a partial result: %v", result.Sessions)
	}
}

func TestList_MutatingReturnedResult_DoesNotAffectLaterListOrGet(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Reserve() error = %v, want nil", err)
	}

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	result.Sessions[0].State = workersessions.StateTerminated
	result.Sessions = append(result.Sessions, workersessions.Session{ID: "injected", State: workersessions.StateRunning})

	after, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(after.Sessions) != 1 || after.Sessions[0].State != workersessions.StateReserved {
		t.Fatalf("mutating a returned List() result leaked into the registry: got %+v", after.Sessions)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "worker-1"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.State != workersessions.StateReserved {
		t.Fatalf("Get() after mutating a List() result = %+v, want State=RESERVED", got)
	}
}

func TestConcurrentReserve_DistinctIdentities_RetainsEveryUniqueSession(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	const count = 200

	var wg sync.WaitGroup
	wg.Add(count)
	for i := range count {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i)
			if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
				t.Errorf("Reserve(%q) error = %v, want nil", id, err)
			}
		}(i)
	}
	wg.Wait()

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != count {
		t.Fatalf("List() returned %d sessions, want %d", len(result.Sessions), count)
	}
	for _, session := range result.Sessions {
		if session.State != workersessions.StateReserved {
			t.Errorf("session %q state = %q, want RESERVED", session.ID, session.State)
		}
	}
}

func TestConcurrentReserve_SameIdentity_ExactlyOneSucceeds(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	const attempts = 50

	var wg sync.WaitGroup
	results := make([]error, attempts)
	wg.Add(attempts)
	for i := range attempts {
		go func(i int) {
			defer wg.Done()
			_, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: "shared"})
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes, duplicates := 0, 0
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, workersessions.ErrSessionAlreadyExists):
			duplicates++
		default:
			t.Fatalf("Reserve() error = %v, want nil or ErrSessionAlreadyExists", err)
		}
	}
	if successes != 1 || duplicates != attempts-1 {
		t.Fatalf("got %d successes and %d duplicates, want exactly 1 success and %d duplicates", successes, duplicates, attempts-1)
	}

	got, err := registry.Get(ctx, workersessions.GetRequest{ID: "shared"})
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ID != "shared" || got.State != workersessions.StateReserved {
		t.Fatalf("Get() after concurrent duplicate Reserve() = %+v, want ID=shared State=RESERVED", got)
	}
}

func TestConcurrentGetAndList_DuringReservation_ReturnInternallyConsistentSnapshots(t *testing.T) {
	registry := newRegistry()
	ctx := context.Background()
	const writers = 50
	const readers = 50

	var wg sync.WaitGroup
	wg.Add(writers + readers)
	for i := range writers {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i)
			if _, err := registry.Reserve(ctx, workersessions.ReserveRequest{ID: id}); err != nil {
				t.Errorf("Reserve(%q) error = %v, want nil", id, err)
			}
		}(i)
	}
	for i := range readers {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("worker-%d", i%writers)
			if session, err := registry.Get(ctx, workersessions.GetRequest{ID: id}); err == nil {
				if err := session.Validate(); err != nil {
					t.Errorf("Get(%q) returned an invalid snapshot: %v", id, err)
				}
			} else if !errors.Is(err, workersessions.ErrSessionNotFound) {
				t.Errorf("Get(%q) error = %v, want nil or ErrSessionNotFound", id, err)
			}
			if result, err := registry.List(ctx, workersessions.ListRequest{}); err != nil {
				t.Errorf("List() error = %v, want nil", err)
			} else {
				for _, session := range result.Sessions {
					if err := session.Validate(); err != nil {
						t.Errorf("List() returned an invalid snapshot: %v", err)
					}
				}
			}
		}(i)
	}
	wg.Wait()

	result, err := registry.List(ctx, workersessions.ListRequest{})
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(result.Sessions) != writers {
		t.Fatalf("List() returned %d sessions, want %d", len(result.Sessions), writers)
	}
}

func idsOf(sessions []workersessions.Session) []string {
	ids := make([]string, len(sessions))
	for i, session := range sessions {
		ids[i] = session.ID
	}
	return ids
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInvokeSession_RetriesARetryableFailureUnderOneWorkerIdentity(t *testing.T) {
	var attempts atomic.Int32
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			if attempts.Add(1) == 1 {
				return retryableFailureResult(req), nil
			}
			return acceptedResult(req), nil
		},
	}
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 2}
	result, err := registry.InvokeSession(context.Background(), req)
	if err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if result.Session.State != workersessions.StateCompleted {
		t.Fatalf("session state = %q, want COMPLETED after the retry succeeded", result.Session.State)
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}

	requests := execution.requests()
	if len(requests) != 2 {
		t.Fatalf("Workers dispatches = %d, want 2", len(requests))
	}
	if got := requests[0].Execution.Dispatch.DispatchID; got != "dispatch-1" {
		t.Fatalf("first attempt dispatch ID = %q, want the caller's own", got)
	}
	if got := requests[1].Execution.Dispatch.DispatchID; got != "dispatch-1/attempt/2" {
		t.Fatalf("second attempt dispatch ID = %q, want dispatch-1/attempt/2", got)
	}
	if strings.TrimSpace(result.Session.ID) != "worker-1" {
		t.Fatalf("session ID = %q, want the one Worker identity to survive the retry", result.Session.ID)
	}
}

// TestInvokeSession_StopsAtTheAttemptBudget proves the budget is a ceiling on
// attempts, not on retries after the first: a Worker allowed two attempts that
// fails both is terminal, and Workers is not asked a third time.
func TestInvokeSession_StopsAtTheAttemptBudget(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return retryableFailureResult(req), nil
		},
	}
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 2}
	result, err := registry.InvokeSession(context.Background(), req)
	if err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if !result.Session.Terminal() {
		t.Fatalf("session state = %q, want a terminal state once the budget is spent", result.Session.State)
	}
	if execution.callCount() != 2 {
		t.Fatalf("Workers dispatches = %d, want exactly the 2-attempt budget", execution.callCount())
	}
	if result.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", result.Attempts)
	}
}

// TestInvokeSession_DefaultPolicyIsOneAttempt pins the property that lets one
// operation serve both orchestrators. A Petri dispatch has always been one
// attempt with retryability classified and handed outward for the graph to act
// on; converging JavaScript children onto this call must not quietly give
// every Petri Worker attempt-level retry it never had.
func TestInvokeSession_DefaultPolicyIsOneAttempt(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return retryableFailureResult(req), nil
		},
	}
	registry := newRegistryWithExecution(execution)

	result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-1", "dispatch-1"))
	if err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers dispatches = %d, want 1 for the zero-value retry policy", execution.callCount())
	}
	if result.Attempts != 1 {
		t.Fatalf("attempts = %d, want 1", result.Attempts)
	}
}

// TestInvokeSession_DoesNotRetryATerminalFailure keeps the retry decision
// Workers' own: a failure Workers classifies as terminal is terminal here too,
// because a second opinion would let two orchestrators disagree about the
// identical provider failure.
func TestInvokeSession_DoesNotRetryATerminalFailure(t *testing.T) {
	execution := &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			result := retryableFailureResult(req)
			result.Result.FailureMetadata = &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypePermanentBadRequest,
			}
			return result, nil
		},
	}
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 5}
	if _, err := registry.InvokeSession(context.Background(), req); err != nil {
		t.Fatalf("InvokeSession: %v", err)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers dispatches = %d, want 1; a terminal classification is not retried", execution.callCount())
	}
}

func retryableFailureResult(req workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	dispatchID := req.Execution.Dispatch.DispatchID
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: req.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyRetryable,
				Type:   workers.WorkFailureTypeInternalServerError,
			},
		},
	}
}

func acceptedResult(req workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	dispatchID := req.Execution.Dispatch.DispatchID
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		WorkstationName: req.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
		},
	}
}

// TestInvokeSession_ControlDuringAnAttemptSpendsNoRetryBudget closes the race
// between the attempt loop and a control.
//
// Every disqualifying condition is checked before the budget, so a Worker a
// control already owns can never consume an attempt: publishing another would
// run provider work for a session that has moved on.
func TestInvokeSession_ControlDuringAnAttemptSpendsNoRetryBudget(t *testing.T) {
	execution := newGatedRetryExecution()
	registry := newRegistryWithExecution(execution)

	req := validStartRequest("worker-1", "dispatch-1")
	req.Retry = workersessions.RetryPolicy{MaxAttempts: 3}
	done := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, err := registry.InvokeSession(context.Background(), req)
		if err != nil {
			t.Errorf("InvokeSession: %v", err)
		}
		done <- result
	}()

	<-execution.entered
	if _, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-1"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	result := <-done
	if !result.Session.Terminal() {
		t.Fatalf("session state = %q, want terminal after the control won", result.Session.State)
	}
	if execution.callCount() != 1 {
		t.Fatalf("Workers dispatches = %d, want 1; a canceled Worker must not consume its retry budget",
			execution.callCount())
	}
}

// gatedRetryExecution holds its first attempt open until a control cancels it,
// so the retry decision is made for a Worker a control already owns.
type gatedRetryExecution struct {
	*fakeExecution

	entered     chan struct{}
	enteredOnce sync.Once
	release     chan struct{}
	releaseOnce sync.Once
}

func newGatedRetryExecution() *gatedRetryExecution {
	gated := &gatedRetryExecution{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	gated.fakeExecution = &fakeExecution{
		dispatch: func(ctx context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			gated.enteredOnce.Do(func() { close(gated.entered) })
			select {
			case <-gated.release:
			case <-ctx.Done():
			}
			return retryableFailureResult(req), nil
		},
		cancel: func(_ context.Context, req workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
			gated.releaseOnce.Do(func() { close(gated.release) })
			return workers.WorkstationDispatchCancelResult{
				DispatchID: req.DispatchID,
				Outcome:    workers.WorkstationDispatchCancelOutcomeCanceled,
			}, nil
		},
	}
	return gated
}

func TestInterrupt_CancelsExactSourceBeforeAdmittingExactSessionSuccessor(t *testing.T) {
	boundary := newControlledBoundary()
	sourceTopic := workersessions.Topic("source-session")
	successorTopic := workersessions.Topic("successor-session")
	eventsSvc := newTerminalAppendObserver(newEventsAppender(), sourceTopic, successorTopic)
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	sourceResult, reference := prepareInterruptSource(t, registry, boundary)

	request := workersessions.InterruptRequest{
		RequestID:                "interrupt-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		ReplacementMessage:       " replacement message ",
	}
	result, err := registry.Interrupt(context.Background(), request)
	if err != nil {
		t.Fatalf("Interrupt() error = %v", err)
	}
	eventsSvc.waitForTerminalAppend(t, sourceTopic)
	assertInterruptAcceptedResult(t, result, reference)
	cancellations := boundary.cancellations()
	if len(cancellations) != 1 || cancellations[0].DispatchID != "dispatch-source" {
		t.Fatalf("cancellations = %#v, want one exact source dispatch", cancellations)
	}

	handoff := boundary.requestFor(t, result.Successor.ProviderSessionAssociation.DispatchID)
	assertInterruptHandoff(t, handoff, request, reference)
	if sourceResult := <-sourceResult; sourceResult.Session.State != workersessions.StateCanceled {
		t.Fatalf("source InvokeSession() = %#v, want CANCELED", sourceResult.Session)
	}

	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	eventsSvc.waitForTerminalAppend(t, successorTopic)
	finalSuccessor, err := registry.Get(context.Background(), workersessions.GetRequest{ID: "successor-session"})
	if err != nil {
		t.Fatalf("Get(successor) error = %v", err)
	}
	if finalSuccessor.State != workersessions.StateCompleted {
		t.Fatalf("final successor = %#v, want COMPLETED", finalSuccessor)
	}
}

func prepareInterruptSource(
	t *testing.T,
	registry workersessions.Service,
	boundary *controlledBoundary,
) (<-chan workersessions.InvokeSessionResult, providers.SessionRef) {
	t.Helper()
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-interrupt"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	return sourceResult, reference
}

func assertInterruptAcceptedResult(t *testing.T, result workersessions.InterruptResult, reference providers.SessionRef) {
	t.Helper()
	if !result.Accepted || result.Phase != workersessions.InterruptPhaseSuccessorAdmission {
		t.Fatalf("Interrupt() result = %#v, want accepted successor admission", result)
	}
	if result.Source.State != workersessions.StateCanceled || result.Source.SuccessorWorkerSessionID != "successor-session" {
		t.Fatalf("interrupt source = %#v, want CANCELED with successor lineage", result.Source)
	}
	if result.Successor.State != workersessions.StateRunning || result.Successor.PredecessorWorkerSessionID != "source-session" {
		t.Fatalf("interrupt successor = %#v, want RUNNING with predecessor lineage", result.Successor)
	}
	if result.Source.ProviderSessionAssociation == nil || result.Source.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("interrupt source reference = %#v, want exact %v", result.Source.ProviderSessionAssociation, reference)
	}
	if result.Successor.ProviderSessionAssociation == nil || result.Successor.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("interrupt successor reference = %#v, want exact %v", result.Successor.ProviderSessionAssociation, reference)
	}
}

func assertInterruptHandoff(
	t *testing.T,
	handoff workers.WorkstationDispatchRequest,
	request workersessions.InterruptRequest,
	reference providers.SessionRef,
) {
	t.Helper()
	if handoff.Execution.UserMessage != request.ReplacementMessage {
		t.Fatalf("replacement message = %q, want byte-equivalent %q", handoff.Execution.UserMessage, request.ReplacementMessage)
	}
	wantContinuation := reference.ContinuationRef()
	if handoff.Execution.Continuation == nil || *handoff.Execution.Continuation != wantContinuation {
		t.Fatalf("successor Continuation = %#v, want exact %v", handoff.Execution.Continuation, wantContinuation)
	}
	if handoff.Execution.Dispatch.DispatchID == "dispatch-source" || handoff.Execution.Dispatch.DispatchID == "" {
		t.Fatalf("successor dispatch ID = %q, want distinct non-empty ID", handoff.Execution.Dispatch.DispatchID)
	}
}

func TestInterrupt_ValidationRejectsBeforeDispatchEffects(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	for name, request := range map[string]workersessions.InterruptRequest{
		"missing request ID": {
			SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
		},
		"same source and successor": {
			RequestID: "interrupt-invalid", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "source-session", ReplacementMessage: "replacement",
		},
		"blank replacement": {
			RequestID: "interrupt-invalid", SourceWorkerSessionID: "source-session", SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "  \t",
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := registry.Interrupt(context.Background(), request)
			var interruptErr *workersessions.InterruptError
			if !errors.As(err, &interruptErr) || interruptErr.Phase != workersessions.InterruptPhaseValidation ||
				!errors.Is(err, workersessions.ErrInterruptValidation) || result.Phase != workersessions.InterruptPhaseValidation {
				t.Fatalf("validation Interrupt() = %#v, %v, want VALIDATION", result, err)
			}
		})
	}
	if boundary.publishCount() != 0 || len(boundary.cancellations()) != 0 {
		t.Fatalf("validation effects = publishes %d, cancels %d, want 0/0", boundary.publishCount(), len(boundary.cancellations()))
	}
}

func TestInterrupt_IdempotencyAndRequestConflictAvoidDuplicateEffects(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-idempotent"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	request := workersessions.InterruptRequest{
		RequestID: "interrupt-request-idempotent", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "first replacement",
	}
	first, err := registry.Interrupt(context.Background(), request)
	if err != nil || !first.Accepted {
		t.Fatalf("first Interrupt() = %#v, %v, want accepted", first, err)
	}
	firstPublishCount := boundary.publishCount()
	firstCancellationCount := len(boundary.cancellations())
	second, err := registry.Interrupt(context.Background(), request)
	if err != nil || !second.Accepted {
		t.Fatalf("replayed Interrupt() = %#v, %v, want accepted replay", second, err)
	}
	if boundary.publishCount() != firstPublishCount || len(boundary.cancellations()) != firstCancellationCount {
		t.Fatalf("replay effects changed: publishes=%d cancels=%d, want %d/%d", boundary.publishCount(), len(boundary.cancellations()), firstPublishCount, firstCancellationCount)
	}
	conflict := request
	conflict.ReplacementMessage = "different replacement"
	conflictResult, err := registry.Interrupt(context.Background(), conflict)
	var interruptErr *workersessions.InterruptError
	if !errors.As(err, &interruptErr) || !errors.Is(err, workersessions.ErrInterruptValidation) ||
		!errors.Is(err, workersessions.ErrInterruptRequestIDConflict) || conflictResult.Phase != workersessions.InterruptPhaseValidation {
		t.Fatalf("conflicting Interrupt() = %#v, %v, want validation/idempotency conflict", conflictResult, err)
	}
	if boundary.publishCount() != firstPublishCount || len(boundary.cancellations()) != firstCancellationCount {
		t.Fatalf("conflict effects changed: publishes=%d cancels=%d", boundary.publishCount(), len(boundary.cancellations()))
	}
	handoff := boundary.requestFor(t, first.Successor.ProviderSessionAssociation.DispatchID)
	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	if got := <-sourceResult; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("source InvokeSession() = %#v, want CANCELED", got.Session)
	}
}

func TestInterrupt_ReportsSuccessorAdmissionPhaseAfterSourceCancellation(t *testing.T) {
	boundary := newControlledBoundary()
	registry, err := newService(
		asCanonicalExecution(boundary),
		&failOnTopicAppendEventsAppender{Service: newEventsAppender(), topic: workersessions.Topic("successor-session")},
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-admission-failure"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	result, err := registry.Interrupt(context.Background(), workersessions.InterruptRequest{
		RequestID: "interrupt-admission-failure", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
	})
	var interruptErr *workersessions.InterruptError
	if !errors.As(err, &interruptErr) || interruptErr.Phase != workersessions.InterruptPhaseSuccessorAdmission ||
		!errors.Is(err, workersessions.ErrInterruptSuccessorAdmission) || result.Phase != workersessions.InterruptPhaseSuccessorAdmission ||
		result.Source.State != workersessions.StateCanceled || result.Successor.State != workersessions.StateFailed {
		t.Fatalf("successor admission failure = %#v, %v, want SUCCESSOR_ADMISSION with canceled source and failed successor", result, err)
	}
	if boundary.publishCount() != 1 || len(boundary.cancellations()) != 1 {
		t.Fatalf("successor admission failure effects = publishes %d, cancels %d, want 1/1", boundary.publishCount(), len(boundary.cancellations()))
	}
	if got := <-sourceResult; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("source cleanup InvokeSession() = %#v, want CANCELED", got.Session)
	}
}

func TestInterrupt_ConcurrentIdenticalRequestsReplayOneOperation(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-concurrent"}
	if _, err := registry.AssociateProviderSession(context.Background(), workersessions.ProviderSessionAssociationRequest{
		WorkerSessionID: "source-session", DispatchID: "dispatch-source", Reference: reference,
	}); err != nil {
		t.Fatalf("AssociateProviderSession() error = %v", err)
	}
	request := workersessions.InterruptRequest{
		RequestID: "interrupt-concurrent", SourceWorkerSessionID: "source-session",
		SuccessorWorkerSessionID: "successor-session", ReplacementMessage: "replacement",
	}
	type outcome struct {
		result workersessions.InterruptResult
		err    error
	}
	outcomes := make(chan outcome, 2)
	var group sync.WaitGroup
	group.Add(2)
	for range 2 {
		go func() {
			defer group.Done()
			result, err := registry.Interrupt(context.Background(), request)
			outcomes <- outcome{result: result, err: err}
		}()
	}
	group.Wait()
	close(outcomes)
	var successorDispatchID string
	for result := range outcomes {
		if result.err != nil || !result.result.Accepted {
			t.Fatalf("concurrent Interrupt() = %#v, %v, want accepted replay", result.result, result.err)
		}
		if result.result.Successor.ProviderSessionAssociation != nil {
			successorDispatchID = result.result.Successor.ProviderSessionAssociation.DispatchID
		}
	}
	if got := len(boundary.cancellations()); got != 1 {
		t.Fatalf("concurrent interrupt cancellations = %d, want 1", got)
	}
	if got := boundary.publishCount(); got != 2 {
		t.Fatalf("concurrent interrupt publishes = %d, want source plus one successor", got)
	}
	handoff := boundary.requestFor(t, successorDispatchID)
	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	if got := <-sourceResult; got.Session.State != workersessions.StateCanceled {
		t.Fatalf("source cleanup InvokeSession() = %#v, want CANCELED", got.Session)
	}
}
