package service_test

import (
	"context"
	"sync/atomic"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestInvokeSession_ProcessGoneTerminalizesExactlyOnceWithSafeClassification(t *testing.T) {
	logger := &recordingLogger{}
	execution := &fakeExecution{
		dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			return processGoneResult(request), workers.ErrWorkstationDispatchProcessGone
		},
	}
	registry, err := newService(executionBoundary{execution: execution}, newEventsAppender(), logger)
	if err != nil {
		t.Fatalf("service.New() error = %v", err)
	}

	result, err := registry.InvokeSession(context.Background(), validStartRequest("worker-gone", "dispatch-gone"))
	if err != nil {
		t.Fatalf("InvokeSession() error = %v", err)
	}
	if result.Session.State != workersessions.StateFailed || result.Session.Result == nil {
		t.Fatalf("ProcessGone session = %#v, want one FAILED terminal result", result.Session)
	}
	if result.Session.Result.Cause == nil || result.Session.Result.Cause.Kind != workersessions.FailureCauseProcessGone {
		t.Fatalf("ProcessGone cause = %#v, want PROCESS_GONE", result.Session.Result.Cause)
	}
	if result.Session.Result.Cause.Detail != "the worker process exited before dispatch completion" {
		t.Fatalf("ProcessGone cause detail = %q, want fixed safe detail", result.Session.Result.Cause.Detail)
	}
	if result.Attempts != 1 || execution.callCount() != 1 {
		t.Fatalf("ProcessGone attempts = %d, dispatch calls = %d, want one each", result.Attempts, execution.callCount())
	}

	reconciliation := logger.entriesFor("worker session reconciliation")
	if len(reconciliation) != 1 {
		t.Fatalf("reconciliation log entries = %d, want exactly one", len(reconciliation))
	}
	fields := reconciliation[0].fields
	if fields["sessionID"] != "worker-gone" || fields["attemptID"] != "dispatch-gone" ||
		fields["dispatchID"] != "dispatch-gone" || fields["reason"] != "PROCESS_GONE" ||
		fields["prior_state"] != string(workersessions.StateRunning) ||
		fields["resulting_state"] != string(workersessions.StateFailed) ||
		fields["result"] != string(workers.WorkstationDispatchTerminalOutcomeFailed) {
		t.Fatalf("reconciliation log fields = %#v, want bounded identity and state facts", fields)
	}
	if elapsed, ok := fields["elapsed_ms"].(int64); !ok || elapsed < 0 {
		t.Fatalf("reconciliation elapsed_ms = %#v, want non-negative int64", fields["elapsed_ms"])
	}
	if fields["deadline"] != "not_applicable" {
		t.Fatalf("reconciliation deadline = %#v, want not_applicable", fields["deadline"])
	}
}

func TestInvokeSession_ProcessGoneRemainsRetryEligible(t *testing.T) {
	var attempts atomic.Int32
	execution := &fakeExecution{
		dispatch: func(_ context.Context, request workers.WorkstationDispatchRequest) (workers.WorkstationDispatchResult, error) {
			if attempts.Add(1) == 1 {
				return processGoneResult(request), workers.ErrWorkstationDispatchProcessGone
			}
			return acceptedResult(request), nil
		},
	}
	registry := newRegistryWithExecution(execution)
	request := validStartRequest("worker-retry-gone", "dispatch-gone")
	request.Retry = workersessions.RetryPolicy{MaxAttempts: 2}

	result, err := registry.InvokeSession(context.Background(), request)
	if err != nil {
		t.Fatalf("InvokeSession() error = %v", err)
	}
	if result.Session.State != workersessions.StateCompleted || result.Attempts != 2 {
		t.Fatalf("retry after ProcessGone = %#v, want COMPLETED after two attempts", result)
	}
	if execution.callCount() != 2 {
		t.Fatalf("dispatch calls after ProcessGone = %d, want 2", execution.callCount())
	}
	requests := execution.requests()
	if len(requests) != 2 || requests[1].Execution.Dispatch.DispatchID != "dispatch-gone/attempt/2" {
		t.Fatalf("retry requests = %#v, want stable identity with /attempt/2 dispatch", requests)
	}
}

func processGoneResult(request workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult {
	dispatchID := request.Execution.Dispatch.DispatchID
	return workers.WorkstationDispatchResult{
		DispatchID:           dispatchID,
		WorkstationName:      request.WorkstationName,
		TerminalOutcome:      workers.WorkstationDispatchTerminalOutcomeFailed,
		ReconciliationReason: workers.WorkstationDispatchReconciliationReasonProcessGone,
		Result: workers.WorkResult{
			DispatchID: dispatchID,
			Outcome:    workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyRetryable,
			},
		},
	}
}
