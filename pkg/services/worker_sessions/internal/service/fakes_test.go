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
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
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

func newService(
	execution any,
	eventsAppender workersessionservice.EventsAppender,
	logger logging.Logger,
) (workersessions.Service, error) {
	return newServiceWithClock(execution, eventsAppender, logger, platformclock.Real{})
}

func newServiceWithClock(
	execution any,
	eventsAppender workersessionservice.EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
) (workersessions.Service, error) {
	return workersessionservice.New(asCanonicalExecution(execution), eventsAppender, logger, clock, unavailableProviderSessions{}, nil)
}

type unavailableProviderSessions struct {
	providersessions.Service
}

func (unavailableProviderSessions) Project(providersessions.ProjectRequest) (providersessions.ProjectResult, error) {
	return providersessions.ProjectResult{}, providersessions.ErrSessionStorageUnavailable
}

// fakeExecution is a controlled legacy-shaped test double. The
// executionBoundary adapter below presents it to Worker Sessions as the
// canonical request-scoped workers.Service.
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

func (f *fakeExecution) DispatchWorkstation(
	ctx context.Context,
	req workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	dispatch := f.dispatch
	f.mu.Unlock()
	if dispatch == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrExecuteUnavailable
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

type legacyExecution interface {
	DispatchWorkstationWithAdmission(context.Context, workers.WorkstationDispatchRequest, workers.WorkstationDispatchAdmissionFunc) (workers.WorkstationDispatchResult, error)
}

// executionBoundary is a test-only adapter from the historical dispatch
// fixture shape to the supported request-scoped Workers service. Production
// code injects workers.Service directly; this adapter keeps the older session
// assertions focused on observable results while they migrate incrementally.
type executionBoundary struct {
	execution legacyExecution
}

var _ workers.Service = executionBoundary{}

func (b executionBoundary) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	if b.execution == nil {
		return workers.ExecuteResult{}, workers.ErrExecuteUnavailable
	}
	legacy := legacyDispatchRequest(request)
	result, err := b.execution.DispatchWorkstationWithAdmission(ctx, legacy, func() {})
	return executeResultFromLegacy(request, result, err), err
}

func (executionBoundary) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
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

func asCanonicalExecution(execution any) workers.Service {
	if execution == nil {
		return nil
	}
	if service, ok := execution.(workers.Service); ok {
		return service
	}
	if legacy, ok := execution.(legacyExecution); ok {
		return executionBoundary{execution: legacy}
	}
	return nil
}

func legacyDispatchRequest(request workers.ExecuteRequest) workers.WorkstationDispatchRequest {
	target := request.Target
	return workers.WorkstationDispatchRequest{
		WorkstationName: target.WorkstationName,
		Execution: workers.WorkstationExecutionRequest{
			Dispatch:                 work.CloneWorkDispatch(request.Input.Dispatch),
			WorkerName:               target.WorkerName,
			WorkerType:               target.WorkerType,
			WorkstationType:          target.WorkstationName,
			RunnerID:                 target.RunnerID,
			ExecutorProvider:         target.ExecutorProvider,
			Model:                    target.Model.Name,
			ModelProvider:            target.Model.Provider,
			ReasoningEffort:          target.Model.ReasoningEffort,
			SystemPrompt:             target.Prompt.SystemPrompt,
			UserMessage:              target.Prompt.UserMessage,
			OutputSchema:             target.Prompt.OutputSchema,
			Timeout:                  target.Timeout,
			EnvVars:                  cloneStringMapForSessionTest(target.Environment.Vars),
			ProcessEnvironment:       append([]string(nil), target.Environment.ProcessEnvironment...),
			Worktree:                 target.Workspace.Worktree,
			WorkingDirectory:         target.Environment.WorkingDirectory,
			WorkingDirectoryAuthored: target.Environment.WorkingDirectorySet,
			SkipPermissions:          target.Permissions.SkipPermissions,
			Continuation:             (request.Input.Resume).ClonePtr(),
		},
	}
}

func executeResultFromLegacy(request workers.ExecuteRequest, result workers.WorkstationDispatchResult, err error) workers.ExecuteResult {
	execution := result.Result
	executeResult := workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
		Diagnostics: execution.Diagnostics.ToSafeDiagnostics(),
		Output: workers.ProposedOutput{Primary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: execution.Output,
		}}},
		Continuation: (execution.Continuation).ClonePtr(),
	}
	if execution.Output == "" {
		executeResult.Output.Primary = nil
	}
	if err != nil || result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeFailed || execution.Outcome == workers.OutcomeFailed || execution.Outcome == workers.OutcomeRejected {
		executeResult.Outcome = workers.ExecutionOutcomeFailed
		message := execution.Error
		if message == "" && err != nil {
			message = err.Error()
		}
		executeResult.Failure = &workers.ExecutionFailure{
			Message:                         message,
			Type:                            workers.WorkFailureTypeUnknown,
			Family:                          workers.WorkFailureFamilyTerminal,
			ProviderFailureKind:             execution.ProviderFailureKind,
			ProviderContinuationFailureKind: execution.ProviderContinuationFailureKind,
			ProviderContinuationOutcome:     execution.ProviderContinuationOutcome,
		}
		if execution.FailureMetadata != nil {
			executeResult.Failure.Family = execution.FailureMetadata.Family
			executeResult.Failure.Type = execution.FailureMetadata.Type
		}
	}
	switch execution.Outcome {
	case workers.OutcomeContinue:
		executeResult.Outcome = workers.ExecutionOutcomeContinue
	case workers.OutcomeRejected:
		executeResult.Outcome = workers.ExecutionOutcomeRejected
	case workers.OutcomeFailed:
		executeResult.Outcome = workers.ExecutionOutcomeFailed
	}
	if err != nil && errors.Is(err, context.Canceled) || result.TerminalOutcome == workers.WorkstationDispatchTerminalOutcomeCanceled {
		executeResult.Outcome = workers.ExecutionOutcomeCanceled
	}
	return executeResult
}

func cloneStringMapForSessionTest(value map[string]string) map[string]string {
	if len(value) == 0 {
		return nil
	}
	clone := make(map[string]string, len(value))
	for key, item := range value {
		clone[key] = item
	}
	return clone
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

// terminalAppendObserver preserves the real in-memory Events implementation
// while exposing a signal after a targeted Worker Session terminal record has
// been committed. The controlled Workers boundary returns before Worker
// Sessions commits that record, so waiting for the boundary alone is not a
// sufficient observation point for a terminal Get assertion.
type terminalAppendObserver struct {
	events.Service

	mu      sync.Mutex
	signals map[events.Topic]chan struct{}
	once    map[events.Topic]*sync.Once
}

func newTerminalAppendObserver(inner events.Service, topics ...events.Topic) *terminalAppendObserver {
	observer := &terminalAppendObserver{
		Service: inner,
		signals: make(map[events.Topic]chan struct{}, len(topics)),
		once:    make(map[events.Topic]*sync.Once, len(topics)),
	}
	for _, topic := range topics {
		observer.signals[topic] = make(chan struct{})
		observer.once[topic] = &sync.Once{}
	}
	return observer
}

func (o *terminalAppendObserver) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	result, err := o.Service.Append(ctx, req)
	if err != nil || !isWorkerSessionTerminalAppend(req) {
		return result, err
	}

	o.mu.Lock()
	signal := o.signals[req.Topic]
	once := o.once[req.Topic]
	o.mu.Unlock()
	if signal != nil && once != nil {
		once.Do(func() { close(signal) })
	}
	return result, nil
}

func (o *terminalAppendObserver) waitForTerminalAppend(t *testing.T, topic events.Topic) {
	t.Helper()
	o.mu.Lock()
	signal, registered := o.signals[topic]
	o.mu.Unlock()
	if !registered {
		t.Fatalf("terminal append signal for topic %q is not registered", topic)
	}
	if err := waitControlledSignal(signal, controlledBoundaryWaitTimeout); err != nil {
		t.Fatalf("terminal append for topic %q: %v", topic, err)
	}
}

func isWorkerSessionTerminalAppend(req events.AppendRequest) bool {
	return req.SourceType == "worker_session_lifecycle" &&
		req.SourceSequence == 2 &&
		req.SourceEventID == "terminal" &&
		req.SchemaID == "workers.draft.v1"
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

// failOnTopicAppendEventsAppender rejects publication for one session topic
// while preserving the real Events implementation for every other session.
// It makes before-handoff publication failures deterministic without relying
// on unrelated lifecycle records sharing one global append count.
type failOnTopicAppendEventsAppender struct {
	events.Service
	topic events.Topic
}

func (f *failOnTopicAppendEventsAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	if req.Topic == f.topic {
		return events.AppendResult{}, errors.New("failOnTopicAppendEventsAppender: simulated append failure")
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
	sourceTopic := workersessions.Topic("source-session")
	successorTopic := workersessions.Topic("successor-session")
	eventsSvc := newTerminalAppendObserver(newEventsAppender(), sourceTopic, successorTopic)
	registry, err := newService(boundary, eventsSvc, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}
	reference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "provider-session-exact",
	}

	sourceResult := startControlledSession(t, registry, boundary, "source-session", "dispatch-source")
	boundary.complete(completedDispatchWithProviderSession("dispatch-source", reference), nil)
	eventsSvc.waitForTerminalAppend(t, sourceTopic)
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

	handoff := boundary.requestFor(t, continued.Session.ProviderSessionAssociation.DispatchID)
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
	eventsSvc.waitForTerminalAppend(t, successorTopic)
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

func (b *continuationAdmissionBoundary) DispatchWorkstationWithAdmission(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
	admitted workers.WorkstationDispatchAdmissionFunc,
) (workers.WorkstationDispatchResult, error) {
	dispatch, err := b.controlledBoundary.prepare(request)
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
	return b.controlledBoundary.await(ctx, dispatch, request.Execution.Dispatch.DispatchID)
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

	successor := waitForFailedContinuation(t, registry, request.SuccessorWorkerSessionID)
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

func waitForFailedContinuation(t *testing.T, registry workersessions.Service, id string) workersessions.Session {
	t.Helper()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		session, err := registry.Get(context.Background(), workersessions.GetRequest{ID: id})
		if err != nil {
			t.Fatalf("Get(successor) error = %v", err)
		}
		if session.State == workersessions.StateFailed && session.Result != nil && session.Result.Cause != nil {
			return session
		}
		select {
		case <-deadline.C:
			t.Fatalf("successor after provider failure = %#v, want FAILED with cause", session)
		case <-ticker.C:
		}
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
