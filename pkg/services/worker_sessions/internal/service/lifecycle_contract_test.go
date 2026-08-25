package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type usagePublishingExecution struct {
	publish func(context.Context, workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error)
	draft   workers.Draft
}

func (e usagePublishingExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	draft := e.draft
	if draft.Kind == "" {
		payload, err := json.Marshal(workers.UsagePayload{InputTokens: 11, Model: "model-usage"})
		if err != nil {
			return workers.ExecuteResult{}, err
		}
		draft = workers.Draft{
			Kind:    workers.KindUsage,
			Phase:   workers.PhaseUpdated,
			Payload: payload,
		}
	}
	publication, err := e.publish(ctx, validPublishRecordRequest("worker-usage", 1, draft))
	if err != nil {
		return workers.ExecuteResult{}, err
	}
	if publication.Outcome != workersessions.PublishOutcomeAccepted {
		return workers.ExecuteResult{}, errors.New("usage publication was not accepted")
	}
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
	}, nil
}

func (usagePublishingExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func TestInvokeSession_UsagePublicationProjectsDetachedTokenFacts(t *testing.T) {
	var registry workersessions.Service
	execution := usagePublishingExecution{
		publish: func(ctx context.Context, request workersessions.PublishRecordRequest) (workersessions.PublishRecordResult, error) {
			return registry.PublishRecord(ctx, request)
		},
	}
	var err error
	registry, err = newService(execution, newEventsAppender(), nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	request := validStartRequest("worker-usage", "dispatch-usage")
	request.Execution.Execution.Dispatch.Execution.WorkIDs = []string{"work-usage"}
	if _, err := registry.InvokeSession(context.Background(), request); err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil", err)
	}

	listed, err := registry.ListObservations(context.Background(), workersessions.ListObservationsRequest{WorkID: "work-usage"})
	if err != nil {
		t.Fatalf("ListObservations() error = %v, want nil", err)
	}
	if len(listed.Observations) != 1 {
		t.Fatalf("ListObservations() returned %d observations, want 1", len(listed.Observations))
	}
	observation := listed.Observations[0]
	if observation.Model == nil || *observation.Model != "model-usage" {
		t.Fatalf("observation.Model = %v, want model-usage", observation.Model)
	}
	if observation.TokenUsage == nil || observation.TokenUsage.InputTokens == nil || *observation.TokenUsage.InputTokens != 11 {
		t.Fatalf("observation.TokenUsage = %#v, want detached input token count 11", observation.TokenUsage)
	}
	if observation.TokenUsage.OutputTokens != nil || observation.TokenUsage.TotalTokens != nil {
		t.Fatalf("observation.TokenUsage = %#v, want omitted token classes to remain nil", observation.TokenUsage)
	}
}

type processGoneExecution struct{}

func (processGoneExecution) Execute(_ context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	if request.Input.ProcessLifecycleObserver == nil {
		return workers.ExecuteResult{}, errors.New("missing process lifecycle observer")
	}
	request.Input.ProcessLifecycleObserver.ProcessExited(platformprocess.ProcessInfo{})
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
	}, nil
}

func (processGoneExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func TestInvokeSession_ProcessExitPublishesProcessGoneFailure(t *testing.T) {
	registry, err := newService(processGoneExecution{}, newEventsAppender(), nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-process-gone", "dispatch-process-gone"))
	if err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil with a terminal session result", err)
	}
	if result.Session.State != workersessions.StateFailed || result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("InvokeSession() session = %#v, want FAILED with a cause", result.Session)
	}
	if result.Session.Result.Cause.Kind != workersessions.FailureCauseProcessGone {
		t.Fatalf("InvokeSession() failure cause = %#v, want PROCESS_GONE", result.Session.Result.Cause)
	}
	if result.Dispatch.ReconciliationReason != workers.WorkstationDispatchReconciliationReasonProcessGone {
		t.Fatalf("InvokeSession() reconciliation reason = %q, want PROCESS_GONE", result.Dispatch.ReconciliationReason)
	}
}

type contradictoryResultExecution struct{}

func (contradictoryResultExecution) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	return workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, errors.New("adapter failed after success")
}

func (contradictoryResultExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func TestInvokeSession_ContradictorySuccessAndErrorPublishesAdapterFailure(t *testing.T) {
	registry, err := newService(contradictoryResultExecution{}, newEventsAppender(), nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-contradictory", "dispatch-contradictory"))
	if err != nil {
		t.Fatalf("InvokeSession() error = %v, want nil with a terminal session result", err)
	}
	if result.Session.State != workersessions.StateFailed || result.Session.Result == nil || result.Session.Result.Cause == nil {
		t.Fatalf("InvokeSession() session = %#v, want FAILED with a cause", result.Session)
	}
	if result.Session.Result.Cause.Kind != workersessions.FailureCauseAdapterFailure {
		t.Fatalf("InvokeSession() failure cause = %#v, want ADAPTER_FAILURE", result.Session.Result.Cause)
	}
}

type cancellationResultExecution struct {
	returnError bool
}

func (e cancellationResultExecution) Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error) {
	if e.returnError {
		return workers.ExecuteResult{Outcome: workers.ExecutionOutcomeAccepted}, workers.ErrWorkstationDispatchCanceled
	}
	return workers.ExecuteResult{
		Outcome: workers.ExecutionOutcomeAccepted,
		Cancellation: &workers.DispatchCancellation{
			Reason: workers.DispatchCancellationReasonCanceled,
		},
	}, nil
}

func (cancellationResultExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func TestInvokeSession_CancellationSignalsNormalizeToCanceledSession(t *testing.T) {
	for _, returnError := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancellation result", true: "cancellation error"}[returnError], func(t *testing.T) {
			registry, err := newService(cancellationResultExecution{returnError: returnError}, newEventsAppender(), nil)
			if err != nil {
				t.Fatalf("service.New() error = %v, want nil", err)
			}
			result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-cancel-signal", "dispatch-cancel-signal"))
			if err != nil {
				t.Fatalf("InvokeSession() error = %v, want nil with a terminal session result", err)
			}
			if result.Session.State != workersessions.StateCanceled {
				t.Fatalf("InvokeSession() session = %#v, want CANCELED", result.Session)
			}
		})
	}
}

type cancellationObserverExecution struct {
	started     chan struct{}
	startedOnce sync.Once
}

func (e *cancellationObserverExecution) Execute(ctx context.Context, request workers.ExecuteRequest) (workers.ExecuteResult, error) {
	e.startedOnce.Do(func() { close(e.started) })
	<-ctx.Done()
	request.Input.ProcessLifecycleObserver.ProcessExited(platformprocess.ProcessInfo{})
	return workers.ExecuteResult{Correlation: request.Correlation, Outcome: workers.ExecutionOutcomeCanceled}, ctx.Err()
}

func (*cancellationObserverExecution) InvokeModel(context.Context, string, modelinference.Request) (modelinference.Result, error) {
	return modelinference.Result{}, workers.ErrExecuteUnavailable
}

func TestCancel_CancellationWinningProcessExitDoesNotReclassifySessionAsProcessGone(t *testing.T) {
	execution := &cancellationObserverExecution{started: make(chan struct{})}
	registry, err := newService(execution, newEventsAppender(), nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	resultCh := make(chan workersessions.InvokeSessionResult, 1)
	go func() {
		result, invokeErr := registry.InvokeSession(context.Background(), validStartRequest("worker-canceled-exit", "dispatch-canceled-exit"))
		if invokeErr != nil {
			return
		}
		resultCh <- result
	}()
	waitForStartSignal(t, execution.started, "execution did not reach the running boundary")
	control, err := registry.Cancel(context.Background(), workersessions.ControlRequest{ID: "worker-canceled-exit"})
	if err != nil || control.Outcome != workersessions.ControlOutcomeApplied || control.Session.State != workersessions.StateCanceled {
		t.Fatalf("Cancel() = %#v, %v, want applied CANCELED", control, err)
	}
	select {
	case result := <-resultCh:
		if result.Session.State != workersessions.StateCanceled {
			t.Fatalf("InvokeSession() session = %#v, want CANCELED", result.Session)
		}
	case <-time.After(time.Second):
		t.Fatal("InvokeSession() did not join after cancellation")
	}
}

func TestStart_CallerCancellationDoesNotCancelServerOwnedAdmission(t *testing.T) {
	dispatchDone := make(chan struct{})
	execution := &fakeExecution{
		admissionStarted: make(chan struct{}),
		releaseAdmission: make(chan struct{}),
		dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			close(dispatchDone)
			return acceptedResult(request), nil
		},
	}
	registry, err := newService(executionBoundary{execution: execution}, newEventsAppender(), nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan error, 1)
	go func() {
		_, startErr := registry.Start(ctx, validAsyncStartRequest("worker-start-canceled", "dispatch-start-canceled"))
		results <- startErr
	}()
	waitForStartSignal(t, execution.admissionStarted, "Start did not reach the controlled admission barrier")
	cancel()
	select {
	case startErr := <-results:
		if !errors.Is(startErr, context.Canceled) {
			t.Fatalf("Start() error = %v, want context.Canceled", startErr)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after caller cancellation")
	}

	close(execution.releaseAdmission)
	select {
	case <-dispatchDone:
	case <-time.After(time.Second):
		t.Fatal("server-owned admission did not finish after caller cancellation")
	}
}

func TestStart_ReplayCallerCancellationReturnsBeforeOriginalAdmission(t *testing.T) {
	innerEvents := newEventsAppender()
	eventsSvc := newGatedEvents(innerEvents)
	registry, err := newService(executionBoundary{execution: succeedingExecution()}, eventsSvc, nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	request := validAsyncStartRequest("worker-start-replay", "dispatch-start-replay")
	originalDone := make(chan error, 1)
	go func() {
		_, startErr := registry.Start(context.Background(), request)
		originalDone <- startErr
	}()
	waitForStartSignal(t, eventsSvc.subscribeStarted, "original Start did not reach the live topic readiness barrier")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := registry.Start(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("replayed Start() error = %v, want context.Canceled", err)
	}

	close(eventsSvc.releaseSubscribe)
	select {
	case err := <-originalDone:
		if err != nil {
			t.Fatalf("original Start() error = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("original Start() did not finish after admission release")
	}
}

func TestListWorkerSessionObservations_CanceledContextIsTyped(t *testing.T) {
	registry, err := newService(succeedingExecution(), newEventsAppender(), nil)
	if err != nil {
		t.Fatalf("service.New() error = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = registry.ListWorkerSessionObservations(ctx, workersessions.ListWorkerSessionObservationsRequest{})
	if !errors.Is(err, workersessions.ErrObservationCanceled) {
		t.Fatalf("ListWorkerSessionObservations() error = %v, want ErrObservationCanceled", err)
	}
}
