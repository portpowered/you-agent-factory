package service_test

import (
	"context"
	"errors"
	"sync"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/events"
	eventswire "github.com/portpowered/infinite-you/pkg/services/events/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// fakeExecution is a controlled test double for
// workers.WorkstationExecutionService. dispatch is called for every
// DispatchWorkstation invocation; a nil dispatch reports an unconfigured
// double as an error rather than silently succeeding.
type fakeExecution struct {
	dispatch func(context.Context, workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error)
	cancel   func(context.Context, workers.WorkstationDispatchCancelRequest) (workers.WorkstationDispatchCancelResult, error)

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

// validStartRequest returns a minimally well-formed StartRequest for id,
// naming attemptID as its resolved attempt (dispatch) identity.
func validStartRequest(id, attemptID string) workersessions.StartRequest {
	return workersessions.StartRequest{
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
