package recording

import (
	"context"
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
