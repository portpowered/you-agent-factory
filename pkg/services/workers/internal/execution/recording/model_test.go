package recording

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type runnerFunc func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error)

func (fn runnerFunc) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	return fn(ctx, request)
}

func TestProviderRecorderPanicDoesNotRewriteRunnerResult(t *testing.T) {
	runner := NewProviderRunner(
		runnerFunc(func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
			return workerexecution.RunnerExecutionResult{Content: "provider-result"}, nil
		}),
		func(workerexecution.InferenceEvent) { panic("recording sink unavailable") },
		func() time.Time { return time.Unix(601, 0).UTC() },
	)

	response, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-provider-sink"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v, want inner runner result", err)
	}
	if response.Content != "provider-result" {
		t.Fatalf("response content = %q, want provider-result", response.Content)
	}
}

func TestProviderRunnerPreservesCanceledAttemptLineage(t *testing.T) {
	start := time.Unix(602, 0).UTC()
	var events []workerexecution.InferenceEvent
	runner := NewProviderRunner(
		runnerFunc(func(ctx context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
			return workerexecution.RunnerExecutionResult{}, ctx.Err()
		}),
		func(event workerexecution.InferenceEvent) { events = append(events, event) },
		func() time.Time { return start },
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	request := workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-canceled",
			Execution:  work.ExecutionMetadata{RequestID: "request-canceled"},
		},
	}

	if _, err := runner.Execute(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if len(events) != 2 || events[0].Request == nil || events[1].Response == nil {
		t.Fatalf("events = %#v, want correlated request and terminal response", events)
	}
	if events[0].Request.InferenceRequestID != "dispatch-canceled/inference-request/1" ||
		events[1].Response.InferenceRequestID != events[0].Request.InferenceRequestID {
		t.Fatalf("inference request lineage = %q/%q, want matching canceled attempt IDs", events[0].Request.InferenceRequestID, events[1].Response.InferenceRequestID)
	}
	if events[0].RequestID != request.Dispatch.Execution.RequestID || events[1].RequestID != request.Dispatch.Execution.RequestID {
		t.Fatalf("request IDs = %q/%q, want %q", events[0].RequestID, events[1].RequestID, request.Dispatch.Execution.RequestID)
	}
	if events[1].Response.Outcome != workerexecution.InferenceOutcomeFailed {
		t.Fatalf("canceled response outcome = %q, want failed terminal observation", events[1].Response.Outcome)
	}
}

func assertRetryResponseIDs(t *testing.T, first, second workerexecution.ModelEvent) {
	t.Helper()
	if first.ID != "factory-event/model-response/dispatch-retry/1" {
		t.Fatalf("first response ID = %q, want dispatch ordinal 1", first.ID)
	}
	if second.ID != "factory-event/model-response/dispatch-retry/2" {
		t.Fatalf("second response ID = %q, want dispatch ordinal 2", second.ID)
	}
	if first.ID == second.ID {
		t.Fatalf("response IDs collided: %q", first.ID)
	}
}

func assertRetryRequestIDs(t *testing.T, first, second workerexecution.ModelEvent, want string) {
	t.Helper()
	wantEventID := "factory-event/model-request/" + want
	if first.ID != wantEventID {
		t.Fatalf("first request ID = %q, want %q", first.ID, wantEventID)
	}
	if second.ID != first.ID {
		t.Fatalf("second request ID = %q, want retry-correlated ID %q", second.ID, first.ID)
	}
}

func assertRetryResponsePayloads(t *testing.T, first, second workerexecution.ModelEvent, wantRequestID string) {
	t.Helper()
	if first.Response == nil {
		t.Fatal("first response payload is nil")
	}
	if second.Response == nil {
		t.Fatal("second response payload is nil")
	}
	if first.Response.ModelRequestID != wantRequestID {
		t.Fatalf("first response request correlation = %q, want %q", first.Response.ModelRequestID, wantRequestID)
	}
	if second.Response.ModelRequestID != wantRequestID {
		t.Fatalf("second response request correlation = %q, want %q", second.Response.ModelRequestID, wantRequestID)
	}
	if first.Response.Outcome != workerexecution.InferenceOutcomeFailed {
		t.Fatalf("first response outcome = %q, want failed", first.Response.Outcome)
	}
	if second.Response.Outcome != workerexecution.InferenceOutcomeSucceeded {
		t.Fatalf("second response outcome = %q, want succeeded", second.Response.Outcome)
	}
}

func TestProviderRunnerRecordsCanonicalEventsAndProviderSession(t *testing.T) {
	start := time.Unix(400, 0).UTC()
	var events []workerexecution.InferenceEvent
	times := []time.Time{start, start.Add(7 * time.Millisecond)}
	now := func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	runner := NewProviderRunner(
		runnerFunc(func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
			continuation := providers.SessionRef{Provider: "agent", Kind: "session-id", ID: "provider-session-1"}.ContinuationRef()
			return workerexecution.RunnerExecutionResult{
				Content:      "provider output",
				Continuation: &continuation,
			}, nil
		}),
		func(event workerexecution.InferenceEvent) { events = append(events, event) },
		now,
	)
	request := workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-provider",
			Execution:  work.ExecutionMetadata{CurrentTick: 8, RequestID: "request-provider"},
		},
		UserMessage: "provider prompt",
	}

	response, err := runner.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Content != "provider output" {
		t.Fatalf("response content = %q, want provider output", response.Content)
	}
	if len(events) != 2 {
		t.Fatalf("recorded events = %d, want request and response", len(events))
	}
	if events[0].Kind != workerexecution.InferenceEventKindRequest || events[1].Kind != workerexecution.InferenceEventKindResponse {
		t.Fatalf("event kinds = %q/%q, want request/response", events[0].Kind, events[1].Kind)
	}
	if events[0].Request == nil || events[1].Response == nil {
		t.Fatalf("events = %#v, want request and response payloads", events)
	}
	if events[0].Request.InferenceRequestID != "dispatch-provider/inference-request/1" ||
		events[1].Response.InferenceRequestID != events[0].Request.InferenceRequestID {
		t.Fatalf("inference request IDs = %q/%q, want matching dispatch correlation", events[0].Request.InferenceRequestID, events[1].Response.InferenceRequestID)
	}
	if events[1].Response.Continuation == nil || events[1].Response.Continuation.ProviderSessionID != "provider-session-1" || events[1].Response.Continuation.Provider != "cursor" {
		t.Fatalf("provider continuation = %#v, want canonical cursor session identity", events[1].Response.Continuation)
	}
	if events[0].DispatchID != request.Dispatch.DispatchID || events[1].RequestID != request.Dispatch.Execution.RequestID {
		t.Fatalf("event correlation = %#v/%#v, want dispatch/request IDs", events[0], events[1])
	}
}
