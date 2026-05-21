package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/scheduler"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestNew_RequiresNet(t *testing.T) {
	_, err := New()
	if err == nil {
		t.Fatal("expected error when Net is not provided")
	}
}

func TestNew_ConfiguresProvidedRuntimeAwareScheduler(t *testing.T) {
	net := buildSimpleNet()
	customScheduler := &runtimeAwareScheduler{}
	runtimeCfg := runtimeSchedulerConfig(&runtimefixtures.RuntimeDefinitionLookupFixture{})

	_, err := New(
		factory.WithNet(net),
		factory.WithScheduler(customScheduler),
		factory.WithRuntimeConfig(runtimeCfg),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if customScheduler.configured != runtimeCfg {
		t.Fatal("expected New to inject runtime config into provided scheduler")
	}

	var _ scheduler.Scheduler = customScheduler
}

func TestNew_InlineDispatchWithNoopExecutorCompletesWorkflow(t *testing.T) {
	n := buildSimpleNet()
	f, err := New(
		factory.WithNet(n),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = submitWorkRequests(ctx, f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-1"}})
	}()

	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
}

func TestNew_InlineDispatchWithoutRegisteredExecutorRecordsMissingExecutorFailure(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNetWithFailureArc()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tickable := tickableFactory(t, f)

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-missing-executor",
		WorkTypeID: "task",
		TraceID:    "trace-missing-executor",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("dispatch history count = %d, want 1", len(snap.DispatchHistory))
	}
	completed := snap.DispatchHistory[0]
	if completed.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("dispatch outcome = %q, want %q", completed.Outcome, interfaces.OutcomeFailed)
	}
	if !strings.Contains(completed.Reason, `no executor registered for worker type "mock"`) {
		t.Fatalf("dispatch reason = %q, want missing executor error", completed.Reason)
	}
}

func TestNew_CompletesWorkflowThroughActiveSubsystems(t *testing.T) {
	f := newPassingInlineRuntime(t)
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-active-path",
		WorkTypeID: "task",
		TraceID:    "trace-active-path",
	}}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want %q", snapshot.FactoryState, interfaces.FactoryStateCompleted)
	}
	if !markingContainsWorkAtPlace(&snapshot.Marking, "work-active-path", "task:done") {
		t.Fatalf("expected work-active-path to reach task:done, marking=%#v", snapshot.Marking.PlaceTokens)
	}

	events := runtimeGeneratedEvents(t, f)
	eventTypes := factoryEventTypes(events)
	if !hasFactoryEventType(events, factoryapi.FactoryEventTypeDispatchRequest) {
		t.Fatalf("expected generated dispatch-created event, got %v", eventTypes)
	}
	if !hasFactoryEventType(events, factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf("expected generated dispatch-completed event, got %v", eventTypes)
	}
}

func TestNew_InitialStructureIncludesRuntimeConfigWorkerMetadata(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithRuntimeConfig(runtimeProjectionConfig{
			Workers: map[string]*interfaces.WorkerConfig{
				"mock": {
					Type:             interfaces.WorkerTypeModel,
					ExecutorProvider: "codex-cli",
					ModelProvider:    "openai",
					Model:            "gpt-5.4",
				},
			},
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	events, err := f.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	if len(events) != 2 || events[0].Type != factoryapi.FactoryEventTypeRunRequest || events[1].Type != factoryapi.FactoryEventTypeInitialStructureRequest {
		t.Fatalf("events = %#v, want run-started then initial structure only", events)
	}
	payload, err := events[1].Payload.AsInitialStructureRequestEventPayload()
	if err != nil {
		t.Fatalf("initial structure payload: %v", err)
	}
	if payload.Factory.Workers == nil || len(*payload.Factory.Workers) != 1 {
		t.Fatalf("Workers = %#v, want one runtime worker", payload.Factory.Workers)
	}
	worker := (*payload.Factory.Workers)[0]
	if worker.Name != "mock" || stringValueForRuntimeTest(worker.ExecutorProvider) != "SCRIPT_WRAP" ||
		stringValueForRuntimeTest(worker.ModelProvider) != "CODEX" ||
		stringValueForRuntimeTest(worker.Model) != "gpt-5.4" {
		t.Fatalf("worker metadata = %#v, want runtime config provider/model metadata", worker)
	}
}

func TestNew_WithMockExecutor(t *testing.T) {
	if _, err := New(factory.WithNet(buildSimpleNet()), factory.WithWorkerExecutor("mock", &passExecutor{})); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestSubmit_AssignsTraceIDWhenMissing(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable := tickableFactory(t, f)
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.Tokens) != 1 {
		t.Fatalf("expected 1 token, got %d", len(snap.Marking.Tokens))
	}
	for _, tok := range snap.Marking.Tokens {
		if tok.Color.TraceID == "" {
			t.Fatal("expected submitted token to have an assigned trace ID")
		}
	}
}

func TestNew_WithClockStampsDispatchesDeterministically(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	clock := replay.NewDeterministicClock(base, time.Second)
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithClock(clock),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tickable := tickableFactory(t, f)
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-clock"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickable.Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.DispatchHistory) != 1 {
		t.Fatalf("expected 1 completed dispatch, got %d", len(snap.DispatchHistory))
	}
	want := base.Add(time.Second)
	completed := snap.DispatchHistory[0]
	if !completed.StartTime.Equal(want) {
		t.Fatalf("dispatch start = %s, want %s", completed.StartTime, want)
	}
	if !completed.EndTime.Equal(want) {
		t.Fatalf("dispatch end = %s, want %s", completed.EndTime, want)
	}
}
