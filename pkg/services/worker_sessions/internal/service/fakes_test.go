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
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func continuationFromProviderMetadata(metadata *providers.SessionMetadata) *providers.ContinuationRef {
	return (metadata).ContinuationRef()
}

func continuationFromProviderReference(reference providers.SessionRef) *providers.ContinuationRef {
	continuation := reference.ContinuationRef()
	return &continuation
}

func newService(
	execution workers.WorkstationExecutionService,
	eventsAppender workersessionservice.EventsAppender,
	logger logging.Logger,
) (workersessions.Service, error) {
	return newServiceWithClock(execution, eventsAppender, logger, platformclock.Real{})
}

func newServiceWithClock(
	execution workers.WorkstationExecutionService,
	eventsAppender workersessionservice.EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
) (workersessions.Service, error) {
	return workersessionservice.New(execution, eventsAppender, logger, clock, unavailableProviderSessions{}, nil)
}

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

// fakeExecution is a controlled test double for
// workers.WorkstationExecutionService. dispatch is called for every
// DispatchWorkstation invocation; a nil dispatch reports an unconfigured
// double as an error rather than silently succeeding.
type fakeExecution struct {
	dispatch func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	cancel   func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)

	admissionStarted chan struct{}
	releaseAdmission chan struct{}
	admissionOnce    sync.Once

	mu          sync.Mutex
	calls       []workers.WorkstationDispatchRequest
	cancelCalls []workers.WorkstationDispatchCancelRequest
}

var _ workers.WorkstationExecutionService = (*fakeExecution)(nil)

func (f *fakeExecution) StartWorkstationPool(
	context.Context,
	workers.WorkstationPoolStartRequest,
) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{}, nil
}

func (f *fakeExecution) StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{}, nil
}

func (f *fakeExecution) DispatchWorkstation(
	ctx context.Context,
	req workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	dispatch := f.dispatch
	f.mu.Unlock()
	if dispatch == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrWorkstationPoolUnavailable
	}
	return dispatch(ctx, req)
}

func (f *fakeExecution) DispatchWorkstationWithAdmission(
	ctx context.Context,
	req workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	if f.admissionStarted != nil {
		f.admissionOnce.Do(func() { close(f.admissionStarted) })
		<-f.releaseAdmission
	}
	if admitted != nil {
		admitted()
	}
	return f.DispatchWorkstation(ctx, req)
}

func (f *fakeExecution) CancelWorkstationDispatch(
	ctx context.Context,
	req workers.WorkstationDispatchCancelRequest,
) (workers.WorkstationDispatchCancelResult, error) {
	f.mu.Lock()
	f.cancelCalls = append(f.cancelCalls, req)
	cancel := f.cancel
	f.mu.Unlock()
	if cancel != nil {
		return cancel(ctx, req)
	}
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (f *fakeExecution) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeExecution) requests() []workers.WorkstationDispatchRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]workers.WorkstationDispatchRequest(nil), f.calls...)
}

func (f *fakeExecution) cancellationRequests() []workers.WorkstationDispatchCancelRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]workers.WorkstationDispatchCancelRequest(nil), f.cancelCalls...)
}

// executionBoundary is the direct Workers execution double used by the
// Worker Sessions tests. It deliberately exposes only the supported Workers
// execution contract; admission and terminal behavior are supplied by the
// Workers-shaped methods below.
type executionBoundary struct {
	execution workers.WorkstationExecutionService
}

var _ workers.WorkstationExecutionService = executionBoundary{}

func (b executionBoundary) StartWorkstationPool(ctx context.Context, request workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error) {
	return b.execution.StartWorkstationPool(ctx, request)
}

func (b executionBoundary) StopWorkstationPool(ctx context.Context) (workers.WorkstationPoolStopResult, error) {
	return b.execution.StopWorkstationPool(ctx)
}

func (b executionBoundary) DispatchWorkstation(ctx context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
	return b.execution.DispatchWorkstation(ctx, request)
}

func (b executionBoundary) DispatchWorkstationWithAdmission(ctx context.Context, request workers.WorkstationDispatchRequest, admitted workers.WorkstationDispatchAdmissionFunc) (workers.WorkstationDispatchResult, error) {
	return b.execution.DispatchWorkstationWithAdmission(ctx, request, admitted)
}

func (b executionBoundary) CancelWorkstationDispatch(ctx context.Context, request workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	return b.execution.CancelWorkstationDispatch(ctx, request)
}

// appendOnlyEvents deliberately exposes no Read or Subscribe method. It
// proves Start refuses to claim acceptance when the opening append succeeds
// but the readiness barrier cannot verify the session topic.
type appendOnlyEvents struct {
	inner events.Service
}

func (a appendOnlyEvents) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	return a.inner.Append(ctx, req)
}

// gatedEvents pauses the live-subscription half of Start's readiness barrier
// so a test can issue a control while the session is STARTING but before
// supervision can hand anything to Workers.
type gatedEvents struct {
	events.Service
	subscribeStarted chan struct{}
	releaseSubscribe chan struct{}
	startedOnce      sync.Once
}

func newGatedEvents(inner events.Service) *gatedEvents {
	return &gatedEvents{
		Service:          inner,
		subscribeStarted: make(chan struct{}),
		releaseSubscribe: make(chan struct{}),
	}
}

func (e *gatedEvents) Subscribe(ctx context.Context, req events.SubscribeRequest) (events.Subscription, error) {
	e.startedOnce.Do(func() { close(e.subscribeStarted) })
	select {
	case <-e.releaseSubscribe:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return e.Service.Subscribe(ctx, req)
}

// noAdmissionBoundary reports a terminal dispatch failure without invoking
// the admission callback. It models a Workers handoff that fails before the
// cancellable admission point and lets Start prove that it never returns
// success for that path.
type noAdmissionBoundary struct {
	err error
}

var _ workers.WorkstationExecutionService = noAdmissionBoundary{}

func (b noAdmissionBoundary) StartWorkstationPool(context.Context, workers.WorkstationPoolStartRequest) (workers.WorkstationPoolStartResult, error) {
	return workers.WorkstationPoolStartResult{}, nil
}

func (b noAdmissionBoundary) StopWorkstationPool(context.Context) (workers.WorkstationPoolStopResult, error) {
	return workers.WorkstationPoolStopResult{}, nil
}

func (b noAdmissionBoundary) DispatchWorkstation(
	ctx context.Context,
	req workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	return b.DispatchWorkstationWithAdmission(ctx, req, nil)
}

func (b noAdmissionBoundary) DispatchWorkstationWithAdmission(
	_ context.Context,
	req workers.WorkstationDispatchRequest,
	_ workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	result := workers.WorkstationDispatchResult{
		DispatchID:      req.Execution.Dispatch.DispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: req.Execution.Dispatch.DispatchID,
			Outcome:    workers.OutcomeFailed,
		},
	}
	return result, b.err
}

func (b noAdmissionBoundary) CancelWorkstationDispatch(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}

// validStartRequest returns a minimally well-formed StartRequest for id,
// naming attemptID as its resolved attempt (dispatch) identity.
func validStartRequest(id, attemptID string) workersessions.InvokeSessionRequest {
	return workersessions.InvokeSessionRequest{
		ID: id,
		Execution: workers.WorkstationDispatchRequest{
			WorkstationName: "review",
			Execution: workers.WorkstationExecutionRequest{
				Dispatch: work.WorkDispatch{
					DispatchID:      attemptID,
					WorkstationName: "review",
				},
			},
		},
	}
}

func validAsyncStartRequest(id, attemptID string) workersessions.StartRequest {
	request := validStartRequest(id, attemptID)
	return workersessions.StartRequest{
		RequestID: "request-" + id + "-" + attemptID,
		ID:        request.ID,
		Execution: request.Execution,
		Retry:     request.Retry,
	}
}

// succeedingExecution returns a fakeExecution whose DispatchWorkstation
// reports an ordinary successful WorkResult.
func succeedingExecution() *fakeExecution {
	return &fakeExecution{
		dispatch: func(_ context.Context, req workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return workers.WorkstationDispatchResult{
				DispatchID:      req.Execution.Dispatch.DispatchID,
				WorkstationName: req.WorkstationName,
				TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
				Result: workers.WorkResult{
					DispatchID: req.Execution.Dispatch.DispatchID,
					Outcome:    workers.OutcomeAccepted,
				},
			}, nil
		},
	}
}

// newEventsAppender returns the real in-memory Events service, used by
// default across this package's tests per the acp-worker-events proposal's
// "use the real in-memory Events implementation" testing rule rather than a
// hand-rolled fake of Events' own append/dedup/ordering behavior.
func newEventsAppender() events.Service {
	svc, err := eventswire.NewService(logging.NoopLogger{})
	if err != nil {
		panic(err)
	}
	return svc
}

// brokenEventsAppender is a controlled EventsAppender test double whose
// Append always fails, used to prove Start's before-handoff publication
// barrier explicitly fails the attempt and never reaches Workers.
type brokenEventsAppender struct {
	err error
}

func (b *brokenEventsAppender) Append(context.Context, events.AppendRequest) (events.AppendResult, error) {
	if b.err != nil {
		return events.AppendResult{}, b.err
	}
	return events.AppendResult{}, errors.New("broken events appender: append always fails")
}

// countingEventsAppender wraps a real events.Service, counting Append calls
// so a test can prove a rejected Start published no Events record at all,
// not just that it skipped the Workers call.
type countingEventsAppender struct {
	events.Service

	mu    sync.Mutex
	calls int
}

func (c *countingEventsAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return c.Service.Append(ctx, req)
}

func (c *countingEventsAppender) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// failOnNthAppendEventsAppender wraps the real in-memory Events service,
// failing exactly its nth (1-indexed) Append call and delegating to the real
// implementation on every other call. This lets a test simulate a terminal
// (or opening) record publication failure in isolation, without a
// permanently broken Events dependency masking whether the surrounding calls
// still succeed.
type failOnNthAppendEventsAppender struct {
	events.Service
	n int

	mu    sync.Mutex
	calls int
}

func (f *failOnNthAppendEventsAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	f.mu.Lock()
	f.calls++
	call := f.calls
	f.mu.Unlock()
	if call == f.n {
		return events.AppendResult{}, errors.New("failOnNthAppendEventsAppender: simulated append failure")
	}
	return f.Service.Append(ctx, req)
}

// blockingTerminalAppendEventsAppender pauses the terminal append after the
// Worker Session state has committed. It lets replay tests observe the exact
// interleaving in which a terminal state exists without a terminal lifecycle
// record in the retained Events snapshot.
type blockingTerminalAppendEventsAppender struct {
	events.Service
	terminalAppendStarted chan struct{}
	releaseTerminalAppend chan struct{}
	once                  sync.Once
	releaseOnce           sync.Once

	mu    sync.Mutex
	calls int
}

func (b *blockingTerminalAppendEventsAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	b.mu.Lock()
	b.calls++
	call := b.calls
	b.mu.Unlock()
	if call != 2 {
		return b.Service.Append(ctx, req)
	}
	b.once.Do(func() { close(b.terminalAppendStarted) })
	<-b.releaseTerminalAppend
	return events.AppendResult{}, errors.New("blockingTerminalAppendEventsAppender: terminal append failed")
}

func (b *blockingTerminalAppendEventsAppender) release() {
	b.releaseOnce.Do(func() { close(b.releaseTerminalAppend) })
}

func TestContinue_CreatesDistinctSuccessorWithExactReferenceAndLineage(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-exact",
	}

	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	source := <-sourceResult
	if source.Session.State != workersessions.StateCompleted {
		t.Fatalf("source session = %#v, want COMPLETED", source.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "the exact follow-up input",
	}
	continued, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil", err)
	}
	assertContinuationAdmission(t, continued, request)

	handoff := boundary.currentRequest()
	assertContinuationHandoff(t, handoff, request, reference, source)

	sourceAfter, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SourceWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(source) error = %v", err)
	}
	assertSourceLineage(t, sourceAfter, request, reference)

	successorAfter, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(successor) error = %v", err)
	}
	assertSuccessorLineage(t, successorAfter, request, reference)

	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
	finalSuccessor, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(final successor) error = %v", err)
	}
	if finalSuccessor.State != workersessions.StateCompleted {
		t.Fatalf("final successor = %#v, want COMPLETED", finalSuccessor)
	}
}

func assertContinuationAdmission(
	t *testing.T,
	result workersessions.ContinueResult,
	request workersessions.ContinueRequest,
) {
	t.Helper()
	if result.SourceWorkerSessionID != request.SourceWorkerSessionID ||
		result.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID {
		t.Fatalf("Continue() lineage result = %#v, want source/successor identities", result)
	}
	if result.Session.State != workersessions.StateRunning {
		t.Fatalf("Continue() successor = %#v, want RUNNING at admission", result.Session)
	}
}

func assertContinuationHandoff(
	t *testing.T,
	handoff workers.WorkstationDispatchRequest,
	request workersessions.ContinueRequest,
	reference providers.SessionRef,
	source workersessions.InvokeSessionResult,
) {
	t.Helper()
	if handoff.Execution.UserMessage != request.FollowUpInput {
		t.Fatalf("continuation input = %q, want %q", handoff.Execution.UserMessage, request.FollowUpInput)
	}
	wantContinuation := reference.ContinuationRef()
	if handoff.Execution.Continuation == nil || *handoff.Execution.Continuation != wantContinuation {
		t.Fatalf("continuation Continuation = %#v, want exact %#v", handoff.Execution.Continuation, wantContinuation)
	}
	if source.Session.ProviderSessionAssociation == nil {
		t.Fatal("source session has no provider association")
	}
	source.Session.ProviderSessionAssociation.Reference.ID = "source-mutated"
	if handoff.Execution.Continuation.ProviderSessionID != reference.ID {
		t.Fatal("continuation shares source association storage")
	}
	if handoff.Execution.Dispatch.DispatchID == "dispatch-source" || handoff.Execution.Dispatch.DispatchID == "" {
		t.Fatalf("continuation dispatch ID = %q, want a distinct non-empty identity", handoff.Execution.Dispatch.DispatchID)
	}
}

func assertSourceLineage(
	t *testing.T,
	session workersessions.Session,
	request workersessions.ContinueRequest,
	reference providers.SessionRef,
) {
	t.Helper()
	if session.State != workersessions.StateCompleted || session.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID {
		t.Fatalf("source after continuation = %#v, want terminal source with successor", session)
	}
	if session.ProviderSessionAssociation == nil || session.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("source association after continuation = %#v, want unchanged reference", session.ProviderSessionAssociation)
	}
}

func assertSuccessorLineage(
	t *testing.T,
	session workersessions.Session,
	request workersessions.ContinueRequest,
	reference providers.SessionRef,
) {
	t.Helper()
	if session.PredecessorWorkerSessionID != request.SourceWorkerSessionID {
		t.Fatalf("successor predecessor = %q, want %q", session.PredecessorWorkerSessionID, request.SourceWorkerSessionID)
	}
	if session.ProviderSessionAssociation == nil || session.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("successor association = %#v, want exact reference", session.ProviderSessionAssociation)
	}
}

func TestContinue_IdempotencyAndLineageConflictsAvoidDuplicateAdmission(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	<-sourceResult

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "first follow-up",
	}
	first, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("first Continue() error = %v", err)
	}
	duplicate, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("duplicate Continue() error = %v", err)
	}
	if duplicate.SuccessorWorkerSessionID != first.SuccessorWorkerSessionID || duplicate.Session.ID != first.Session.ID {
		t.Fatalf("duplicate result = %#v, want the original successor", duplicate)
	}

	conflict := request
	conflict.FollowUpInput = "different follow-up"
	if _, err := registry.Continue(context.Background(), conflict); !errors.Is(err, workersessions.ErrContinuationRequestIDConflict) {
		t.Fatalf("request-ID reuse error = %v, want ErrContinuationRequestIDConflict", err)
	}

	competing := request
	competing.RequestID = "continue-request-2"
	competing.SuccessorWorkerSessionID = "another-successor"
	if _, err := registry.Continue(context.Background(), competing); !errors.Is(err, workersessions.ErrContinuationSourceConflict) {
		t.Fatalf("competing continuation error = %v, want ErrContinuationSourceConflict", err)
	}

	if boundary.publishCount() != 2 {
		t.Fatalf("boundary publish count = %d, want source plus one successor", boundary.publishCount())
	}
	handoff := boundary.currentRequest()
	boundary.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
}

func TestContinue_ConcurrentIdenticalRequestsShareOneAdmission(t *testing.T) {
	base := newControlledBoundary()
	continuationGate := make(chan struct{})
	boundary := &continuationAdmissionBoundary{
		controlledBoundary: base,
		continuationGate:   continuationGate,
		continuationReady:  make(chan struct{}),
	}
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-concurrent"}
	sourceResult := startControlledSession(t, registry, base, "source-session", "dispatch-source")
	base.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	if result := <-sourceResult; result.Session.State != workersessions.StateCompleted {
		t.Fatalf("source result = %#v, want COMPLETED", result.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-concurrent",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "concurrent follow-up",
	}
	const callers = 8
	start := make(chan struct{})
	results := make(chan struct {
		result workersessions.ContinueResult
		err    error
	}, callers)
	var group sync.WaitGroup
	group.Add(callers)
	for range callers {
		go func() {
			defer group.Done()
			<-start
			result, err := registry.Continue(context.Background(), request)
			results <- struct {
				result workersessions.ContinueResult
				err    error
			}{result: result, err: err}
		}()
	}
	close(start)

	select {
	case <-boundary.continuationReady:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for one continuation admission")
	}
	if got := base.publishCount(); got != 2 {
		t.Fatalf("Worker Sessions publish count before release = %d, want source plus one continuation", got)
	}
	close(continuationGate)
	group.Wait()
	close(results)

	for outcome := range results {
		if outcome.err != nil {
			t.Fatalf("concurrent Continue() error = %v, want nil", outcome.err)
		}
		if outcome.result.SuccessorWorkerSessionID != request.SuccessorWorkerSessionID ||
			outcome.result.Session.ID != request.SuccessorWorkerSessionID {
			t.Fatalf("concurrent Continue() result = %#v, want shared successor", outcome.result)
		}
	}
	if got := base.publishCount(); got != 2 {
		t.Fatalf("Worker Sessions publish count = %d, want source plus one continuation", got)
	}
	handoff := base.currentRequest()
	base.complete(completedDispatchWithProviderSession(handoff.Execution.Dispatch.DispatchID, reference), nil)
}

type continuationAdmissionBoundary struct {
	*controlledBoundary
	continuationGate  <-chan struct{}
	continuationReady chan struct{}
	readyOnce         sync.Once
}

type continuationFailureBoundary struct {
	*controlledBoundary
	err error
}

func (b *continuationFailureBoundary) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	if request.Execution.Continuation == nil {
		return b.controlledBoundary.DispatchWorkstationWithAdmission(ctx, request, admitted)
	}
	_, err := b.controlledBoundary.prepare(request)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	return workers.WorkstationDispatchResult{
		DispatchID:      request.Execution.Dispatch.DispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: request.Execution.Dispatch.DispatchID,
			Outcome:    workers.OutcomeFailed,
		},
	}, b.err
}

func TestContinue_PreAdmissionFailureReturnsTerminalSuccessorAndTypedError(t *testing.T) {
	base := newControlledBoundary()
	boundary := &continuationFailureBoundary{controlledBoundary: base, err: errors.New("continuation admission rejected")}
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-failure"}
	sourceResult := startControlledSession(t, registry, base, "source-session", "dispatch-source")
	base.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	if source := <-sourceResult; source.Session.State != workersessions.StateCompleted {
		t.Fatalf("source result = %#v, want COMPLETED", source.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-failure",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "follow-up",
	}
	result, err := registry.Continue(context.Background(), request)
	if !errors.Is(err, workersessions.ErrContinuationNotAccepted) {
		t.Fatalf("Continue() error = %v, want ErrContinuationNotAccepted", err)
	}
	if result.Session.ID != request.SuccessorWorkerSessionID || result.Session.State != workersessions.StateFailed {
		t.Fatalf("Continue() result = %#v, want failed successor snapshot", result)
	}
	if result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("Continue() result = %#v, want terminal failure cause", result.Session)
	}
	source, getErr := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SourceWorkerSessionID})
	if getErr != nil {
		t.Fatalf("Get(source after refused continuation) error = %v", getErr)
	}
	successor, getErr := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if getErr != nil {
		t.Fatalf("Get(successor after refused continuation) error = %v", getErr)
	}
	if source.SuccessorWorkerSessionID != "" || successor.PredecessorWorkerSessionID != "" {
		t.Fatalf("refused continuation mutated lineage: source=%#v successor=%#v", source, successor)
	}
}

func TestContinue_CanceledCallerDoesNotCancelReservedContinuation(t *testing.T) {
	base := newControlledBoundary()
	continuationGate := make(chan struct{})
	boundary := &continuationAdmissionBoundary{
		controlledBoundary: base,
		continuationGate:   continuationGate,
		continuationReady:  make(chan struct{}),
	}
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-cancel"}
	sourceResult := startControlledSession(t, registry, base, "source-session", "dispatch-source")
	base.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	if source := <-sourceResult; source.Session.State != workersessions.StateCompleted {
		t.Fatalf("source result = %#v, want COMPLETED", source.Session)
	}

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-cancel",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "follow up",
	}
	ctx, cancel := context.WithCancel(context.Background())
	outcomes := make(chan struct {
		result workersessions.ContinueResult
		err    error
	}, 1)
	go func() {
		result, err := registry.Continue(ctx, request)
		outcomes <- struct {
			result workersessions.ContinueResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-boundary.continuationReady:
	case <-time.After(time.Second):
		t.Fatal("continuation did not reach admission wait")
	}
	cancel()
	select {
	case outcome := <-outcomes:
		if !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("Continue(canceled caller) error = %v, want context.Canceled", outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Continue(canceled caller) did not return")
	}
	close(continuationGate)
	base.complete(completedDispatchWithProviderSession(base.currentRequest().Execution.Dispatch.DispatchID, reference), nil)
	final, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
	if err != nil || final.State != workersessions.StateCompleted {
		t.Fatalf("successor after canceled caller = %#v, %v, want server-owned completion", final, err)
	}
}

func (b *continuationAdmissionBoundary) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	completed, err := b.controlledBoundary.prepare(request)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	if request.Execution.Continuation != nil && admitted != nil {
		b.readyOnce.Do(func() { close(b.continuationReady) })
		select {
		case <-b.continuationGate:
		case <-ctx.Done():
			return workers.WorkstationDispatchResult{}, ctx.Err()
		}
	}
	if admitted != nil {
		admitted()
		b.controlledBoundary.admittedOnce.Do(func() { close(b.controlledBoundary.admitted) })
	}
	return b.controlledBoundary.await(completed)
}

func TestContinue_ProviderFailureLeavesSuccessorFailedWithExactAssociation(t *testing.T) {
	boundary := newControlledBoundary()
	registry := newControlledRegistry(t, boundary)
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "provider-session-1"}
	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	<-sourceResult

	request := workersessions.ContinueRequest{
		RequestID:                "continue-request-1",
		SourceWorkerSessionID:    "source-session",
		SuccessorWorkerSessionID: "successor-session",
		FollowUpInput:            "follow-up",
	}
	continued, err := registry.Continue(context.Background(), request)
	if err != nil {
		t.Fatalf("Continue() error = %v, want nil at admission", err)
	}
	handoff := boundary.currentRequest()
	boundary.complete(failedContinuationDispatch(handoff.Execution.Dispatch.DispatchID, reference), nil)

	var successor workersessions.Session
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		successor, err = registry.Get(context.Background(), workersessions.GetRequest{ID: request.SuccessorWorkerSessionID})
		if err != nil {
			t.Fatalf("Get(successor) error = %v", err)
		}
		if successor.State == workersessions.StateFailed && successor.Result != nil && successor.Result.Cause != nil {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("successor after provider failure = %#v, want FAILED with cause", successor)
		case <-ticker.C:
		}
	}
	if successor.State != workersessions.StateFailed || successor.Result == nil || successor.Result.Cause == nil {
		t.Fatalf("successor after provider failure = %#v, want FAILED with cause", successor)
	}
	if successor.Result.Cause.ProviderContinuationFailureKind != providers.ContinuationFailureKindStale {
		t.Fatalf("successor failure cause = %#v, want stale continuation classification", successor.Result.Cause)
	}
	if successor.ProviderSessionAssociation == nil || successor.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("successor association = %#v, want exact retained reference", successor.ProviderSessionAssociation)
	}
	source, err := registry.Get(context.Background(), workersessions.GetRequest{ID: request.SourceWorkerSessionID})
	if err != nil {
		t.Fatalf("Get(source) error = %v", err)
	}
	if source.State != workersessions.StateCompleted || source.ProviderSessionAssociation == nil || source.ProviderSessionAssociation.Reference != reference {
		t.Fatalf("source after provider failure = %#v, want unchanged terminal source/reference", source)
	}
	if continued.Session.ID != request.SuccessorWorkerSessionID {
		t.Fatalf("admitted result = %#v, want successor identity", continued)
	}
}

func TestContinue_RejectsUnknownActiveAndUnassociatedSourcesWithoutSuccessor(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, registry workersessions.Service, boundary *controlledBoundary)
		wantErr error
	}{
		{
			name:    "unknown",
			prepare: func(*testing.T, workersessions.Service, *controlledBoundary) {},
			wantErr: workersessions.ErrContinuationSourceNotFound,
		},
		{
			name: "active",
			prepare: func(t *testing.T, registry workersessions.Service, _ *controlledBoundary) {
				t.Helper()
				if _, err := registry.Reserve(context.Background(), workersessions.ReserveRequest{ID: "source-session"}); err != nil {
					t.Fatalf("Reserve(source) error = %v", err)
				}
			},
			wantErr: workersessions.ErrContinuationSourceActive,
		},
		{
			name: "unassociated terminal",
			prepare: func(t *testing.T, registry workersessions.Service, boundary *controlledBoundary) {
				t.Helper()
				sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
				boundary.complete(completedDispatch("dispatch-source"), nil)
				if result := <-sourceResult; result.Session.State != workersessions.StateCompleted {
					t.Fatalf("source result = %#v, want COMPLETED", result.Session)
				}
			},
			wantErr: workersessions.ErrContinuationProviderSessionMissing,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledBoundary()
			registry := newControlledRegistry(t, boundary)
			test.prepare(t, registry, boundary)
			publishCountBefore := boundary.publishCount()
			_, err := registry.Continue(context.Background(), workersessions.ContinueRequest{
				RequestID:                "continue-request-" + test.name,
				SourceWorkerSessionID:    "source-session",
				SuccessorWorkerSessionID: "successor-session",
				FollowUpInput:            "follow-up",
			})
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Continue() error = %v, want %v", err, test.wantErr)
			}
			if _, getErr := registry.Get(context.Background(), workersessions.GetRequest{ID: "successor-session"}); !errors.Is(getErr, workersessions.ErrSessionNotFound) {
				t.Fatalf("Get(successor) error = %v, want ErrSessionNotFound", getErr)
			}
			if boundary.publishCount() != publishCountBefore {
				t.Fatalf("boundary publish count = %d, want unchanged %d and zero continuation effects", boundary.publishCount(), publishCountBefore)
			}
		})
	}
}

func completedDispatchWithProviderSession(dispatchID string, reference providers.SessionRef) workers.WorkstationDispatchResult {
	result := completedDispatch(dispatchID)
	result.Result.Continuation = continuationFromProviderMetadata(&providers.SessionMetadata{
		Provider: reference.Provider.String(),
		Kind:     reference.Kind,
		ID:       reference.ID,
	})
	return result
}

func completedDispatch(dispatchID string) workers.WorkstationDispatchResult {
	return workers.WorkstationDispatchResult{
		DispatchID:      dispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeCompleted,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeAccepted,
		},
	}
}

func failedContinuationDispatch(dispatchID string, reference providers.SessionRef) workers.WorkstationDispatchResult {
	result := completedDispatchWithProviderSession(dispatchID, reference)
	result.TerminalOutcome = workers.WorkstationDispatchTerminalOutcomeFailed
	result.Result.Outcome = workers.OutcomeFailed
	result.Result.FailureMetadata = &workers.WorkFailureMetadata{
		Family: workers.WorkFailureFamilyTerminal,
		Type:   workers.WorkFailureTypePermanentBadRequest,
	}
	result.Result.ProviderContinuationFailureKind = providers.ContinuationFailureKindStale
	return result
}
