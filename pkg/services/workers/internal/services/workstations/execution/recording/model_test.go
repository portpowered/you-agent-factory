package recording

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type runnerFunc func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error)

func (fn runnerFunc) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	return fn(ctx, request)
}

func TestRunnerRequiresInjectedClock(t *testing.T) {
	runner := NewRunner(
		runnerFunc(func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
			return workerexecution.RunnerExecutionResult{}, nil
		}),
		nil,
		&workerconfig.FactoryWorkerConfig{Name: "worker"},
		func(workerexecution.ModelEvent) {},
		nil,
	)
	if _, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{}); err == nil {
		t.Fatal("Execute() error = nil, want missing-clock failure")
	}
}

func TestRunnerAndHooksRecordOneCanonicalTrace(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	hooks := Hooks()
	inner := runnerFunc(func(ctx context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
		hooks.MarkResourceWaitStarted(ctx, start)
		hooks.MarkResourceWaitFinished(ctx, start.Add(25*time.Millisecond), true)
		hooks.MarkLoadRequested(ctx, start)
		hooks.MarkLoadFinished(ctx, start.Add(40*time.Millisecond))
		hooks.MarkLoadReused(ctx)
		return workerexecution.RunnerExecutionResult{Content: "done"}, nil
	})

	var events []workerexecution.ModelEvent
	times := []time.Time{start, start.Add(100 * time.Millisecond)}
	now := func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	runner := NewRunner(inner, nil, &workerconfig.FactoryWorkerConfig{Name: "worker", Model: "model"}, func(event workerexecution.ModelEvent) {
		events = append(events, event)
	}, now)

	_, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch", Execution: work.ExecutionMetadata{CurrentTick: 3}},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(events) != 2 || events[1].Response == nil {
		t.Fatalf("events = %#v, want request and response", events)
	}
	response := events[1].Response
	if response.ResourceWaitMillis == nil || *response.ResourceWaitMillis != 25 || response.ResourceAcquired == nil || !*response.ResourceAcquired {
		t.Fatalf("resource trace = %#v, want 25ms acquired", response)
	}
	if response.LoadDurationMillis == nil || *response.LoadDurationMillis != 40 || response.LoadRequested == nil || !*response.LoadRequested || response.LoadReused == nil || !*response.LoadReused {
		t.Fatalf("load trace = %#v, want 40ms requested/reused", response)
	}

	// Nil contexts are an explicit no-op boundary for model hooks.
	hooks.MarkResourceWaitStarted(nil, start)
	hooks.MarkResourceWaitFinished(nil, start, false)
	hooks.MarkLoadRequested(nil, start)
	hooks.MarkLoadFinished(nil, start)
	hooks.MarkLoadReused(nil)
}

func TestRunnerRecordsDistinctResponseIDsAcrossRetryOutcomes(t *testing.T) {
	start := time.Unix(200, 0).UTC()
	calls := 0
	inner := runnerFunc(func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
		calls++
		if calls == 1 {
			return workerexecution.RunnerExecutionResult{}, errors.New("transient model failure")
		}
		return workerexecution.RunnerExecutionResult{Content: "recovered"}, nil
	})

	var events []workerexecution.ModelEvent
	times := []time.Time{start, start.Add(time.Millisecond), start.Add(2 * time.Millisecond), start.Add(3 * time.Millisecond)}
	now := func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	runner := NewRunner(inner, nil, &workerconfig.FactoryWorkerConfig{Name: "worker", Model: "model"}, func(event workerexecution.ModelEvent) {
		events = append(events, event)
	}, now)
	request := workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-retry", Execution: work.ExecutionMetadata{CurrentTick: 4}},
	}

	executeRetryPair(t, runner, request)
	assertRetryRecording(t, events)
}

func executeRetryPair(t *testing.T, runner workerexecution.Runner, request workerexecution.RunnerExecutionRequest) {
	t.Helper()
	if _, err := runner.Execute(context.Background(), request); err == nil {
		t.Fatal("first Execute() error = nil, want transient failure")
	}
	if _, err := runner.Execute(context.Background(), request); err != nil {
		t.Fatalf("second Execute() error = %v, want success", err)
	}
}

func assertRetryRecording(t *testing.T, events []workerexecution.ModelEvent) {
	t.Helper()
	if len(events) != 4 {
		t.Fatalf("recorded %d events, want request/response pairs for both executions", len(events))
	}

	firstRequest, firstResponse := events[0], events[1]
	secondRequest, secondResponse := events[2], events[3]
	assertRetryResponseIDs(t, firstResponse, secondResponse)
	wantRequestID := "dispatch-retry/model-request/1"
	assertRetryRequestIDs(t, firstRequest, secondRequest, wantRequestID)
	assertRetryResponsePayloads(t, firstResponse, secondResponse, wantRequestID)
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

func TestRunnerResponseOrdinalIsMonotonicAcrossDispatches(t *testing.T) {
	start := time.Unix(300, 0).UTC()
	var events []workerexecution.ModelEvent
	times := []time.Time{
		start, start.Add(time.Millisecond),
		start.Add(2 * time.Millisecond), start.Add(3 * time.Millisecond),
		start.Add(4 * time.Millisecond), start.Add(5 * time.Millisecond),
	}
	now := func() time.Time {
		value := times[0]
		times = times[1:]
		return value
	}
	runner := NewRunner(
		runnerFunc(func(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
			return workerexecution.RunnerExecutionResult{Content: "ok"}, nil
		}),
		nil,
		&workerconfig.FactoryWorkerConfig{Name: "worker", Model: "model"},
		func(event workerexecution.ModelEvent) { events = append(events, event) },
		now,
	)
	for _, dispatchID := range []string{"dispatch-a", "dispatch-b", "dispatch-a"} {
		if _, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
			Dispatch: work.WorkDispatch{DispatchID: dispatchID},
		}); err != nil {
			t.Fatalf("Execute(%q): %v", dispatchID, err)
		}
	}
	if len(events) != 6 {
		t.Fatalf("recorded %d events, want three request/response pairs", len(events))
	}
	got := []string{events[1].ID, events[3].ID, events[5].ID}
	want := []string{
		"factory-event/model-response/dispatch-a/1",
		"factory-event/model-response/dispatch-b/2",
		"factory-event/model-response/dispatch-a/3",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response IDs = %#v, want monotonic runner ordinals %#v", got, want)
	}
}

func TestModelResponseEventIDIsDeterministicForAnOrdinal(t *testing.T) {
	first := modelResponseEventID("dispatch", 2)
	second := modelResponseEventID("dispatch", 2)
	if first != second {
		t.Fatalf("same response ordinal produced IDs %q and %q", first, second)
	}
	if first == modelResponseEventID("dispatch", 1) {
		t.Fatalf("different response ordinals produced the same ID %q", first)
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
			return workerexecution.RunnerExecutionResult{
				Content: "provider output",
				ProviderSession: &workerexecution.ProviderSessionMetadata{
					Provider: "agent",
					Kind:     "session-id",
					ID:       "provider-session-1",
				},
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
	if events[1].Response.ProviderSession == nil || events[1].Response.ProviderSession.ID != "provider-session-1" || events[1].Response.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider session = %#v, want canonical cursor session identity", events[1].Response.ProviderSession)
	}
	if events[0].DispatchID != request.Dispatch.DispatchID || events[1].RequestID != request.Dispatch.Execution.RequestID {
		t.Fatalf("event correlation = %#v/%#v, want dispatch/request IDs", events[0], events[1])
	}
}
