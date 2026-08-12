package service_test

import (
	"context"
	"errors"
	"sync"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workersessionservice "github.com/portpowered/infinite-you/pkg/services/worker_sessions/internal/service"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newService(
	boundary workers.WorkstationPoolBoundary,
	eventsAppender workersessionservice.EventsAppender,
	logger logging.Logger,
) (workersessions.Service, error) {
	return newServiceWithClock(boundary, eventsAppender, logger, platformclock.Real{})
}

func newServiceWithClock(
	boundary workers.WorkstationPoolBoundary,
	eventsAppender workersessionservice.EventsAppender,
	logger logging.Logger,
	clock platformclock.Source,
) (workersessions.Service, error) {
	return workersessionservice.New(boundary, eventsAppender, logger, clock, unavailableProviderSessions{}, nil)
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

// executionBoundary adapts the existing controlled Workers execution fake to
// the exact pool boundary Worker Sessions receives in production. It keeps
// tests focused on observable Worker Sessions behavior while preserving the
// production rule that control enters through Boundary.Cancel.
type executionBoundary struct {
	execution workers.WorkstationExecutionService
}

var _ workers.WorkstationPoolBoundary = executionBoundary{}

func (b executionBoundary) Start(ctx context.Context) error {
	_, err := b.execution.StartWorkstationPool(ctx, workers.WorkstationPoolStartRequest{})
	return err
}

func (b executionBoundary) Publish(ctx context.Context, req workers.WorkstationDispatchRequest, accept workers.WorkstationDispatchAcceptFunc) error {
	return b.PublishWithAdmission(ctx, req, nil, accept)
}

func (b executionBoundary) PublishWithAdmission(ctx context.Context, req workers.WorkstationDispatchRequest, admitted workers.WorkstationDispatchAdmissionFunc, accept workers.WorkstationDispatchAcceptFunc) error {
	if err := b.Start(ctx); err != nil {
		return err
	}
	result, err := b.execution.DispatchWorkstationWithAdmission(ctx, req, admitted)
	accept(context.Background(), req, result, err)
	return nil
}

func (b executionBoundary) Cancel(ctx context.Context, req workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	return b.execution.CancelWorkstationDispatch(ctx, req)
}

func (b executionBoundary) Stop(ctx context.Context) error {
	_, err := b.execution.StopWorkstationPool(ctx)
	return err
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

var _ workers.WorkstationPoolBoundary = noAdmissionBoundary{}

func (b noAdmissionBoundary) Start(context.Context) error { return nil }

func (b noAdmissionBoundary) Publish(
	ctx context.Context,
	req workers.WorkstationDispatchRequest,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	return b.PublishWithAdmission(ctx, req, nil, accept)
}

func (b noAdmissionBoundary) PublishWithAdmission(
	_ context.Context,
	req workers.WorkstationDispatchRequest,
	_ workers.WorkstationDispatchAdmissionFunc,
	accept workers.WorkstationDispatchAcceptFunc,
) error {
	result := workers.WorkstationDispatchResult{
		DispatchID:      req.Execution.Dispatch.DispatchID,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: req.Execution.Dispatch.DispatchID,
			Outcome:    workers.OutcomeFailed,
		},
	}
	accept(context.Background(), req, result, b.err)
	return nil
}

func (b noAdmissionBoundary) Cancel(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error) {
	return workers.WorkstationDispatchCancelResult{}, nil
}

func (b noAdmissionBoundary) Stop(context.Context) error { return nil }

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
