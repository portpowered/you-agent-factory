package recording

import (
	"context"
	"errors"
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

	if _, err := runner.Execute(context.Background(), request); err == nil {
		t.Fatal("first Execute() error = nil, want transient failure")
	}
	if _, err := runner.Execute(context.Background(), request); err != nil {
		t.Fatalf("second Execute() error = %v, want success", err)
	}
	if len(events) != 4 {
		t.Fatalf("recorded %d events, want request/response pairs for both executions", len(events))
	}

	firstRequest, firstResponse := events[0], events[1]
	secondRequest, secondResponse := events[2], events[3]
	if firstResponse.ID != "factory-event/model-response/dispatch-retry/1" || secondResponse.ID != "factory-event/model-response/dispatch-retry/2" {
		t.Fatalf("response IDs = %q, %q, want stable dispatch ordinals 1 and 2", firstResponse.ID, secondResponse.ID)
	}
	if firstResponse.ID == secondResponse.ID {
		t.Fatalf("response IDs collided: %q", firstResponse.ID)
	}
	wantRequestID := "dispatch-retry/model-request/1"
	if firstRequest.ID != "factory-event/model-request/"+wantRequestID || secondRequest.ID != firstRequest.ID {
		t.Fatalf("request IDs = %q, %q, want the existing retry-correlated ID %q", firstRequest.ID, secondRequest.ID, wantRequestID)
	}
	if firstResponse.Response == nil || secondResponse.Response == nil {
		t.Fatal("response payloads are nil")
	}
	if firstResponse.Response.ModelRequestID != wantRequestID || secondResponse.Response.ModelRequestID != wantRequestID {
		t.Fatalf("response request correlations = %q, %q, want %q", firstResponse.Response.ModelRequestID, secondResponse.Response.ModelRequestID, wantRequestID)
	}
	if firstResponse.Response.Outcome != workerexecution.InferenceOutcomeFailed || secondResponse.Response.Outcome != workerexecution.InferenceOutcomeSucceeded {
		t.Fatalf("response outcomes = %q, %q, want failed then succeeded", firstResponse.Response.Outcome, secondResponse.Response.Outcome)
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
