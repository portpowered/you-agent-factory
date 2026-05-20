package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/workers"
)

func TestFactoryEventHistory_SubscribeReplaysHistoryThenStreamsLiveEvents(t *testing.T) {
	f := newPassingInlineRuntime(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := f.SubscribeFactoryEvents(ctx)
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if len(stream.History) != 2 ||
		stream.History[0].Type != factoryapi.FactoryEventTypeRunRequest ||
		stream.History[1].Type != factoryapi.FactoryEventTypeInitialStructureRequest {
		t.Fatalf("replayed history = %#v, want run-started and initial structure events", stream.History)
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-live"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := tickableFactory(t, f).Tick(context.Background()); err != nil {
		t.Fatalf("Tick: %v", err)
	}

	select {
	case event := <-stream.Events:
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			t.Fatalf("live event = %#v, want work request event", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live canonical factory event")
	}

	cancel()
	deadline := time.After(time.Second)
	select {
	case <-deadline:
		t.Fatal("timed out waiting for live event stream closure")
	default:
	}
	for {
		select {
		case _, ok := <-stream.Events:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for live event stream closure")
		}
	}
}

func TestNew_BatchModeWithoutInitialWork_TerminatesWithoutCancellation(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
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

func TestNew_ServiceModeWithoutInitialWork_WaitsForCancellation(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before cancellation: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
	}
}

func TestNew_ServiceModeWithoutInitialWork_AcceptsLateSubmission(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(ctx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before late submission: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	runtimeBeforeSubmit, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot before submit: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	runtimeAfterIdleWait, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot after idle wait: %v", err)
	}

	if runtimeAfterIdleWait.TickCount != runtimeBeforeSubmit.TickCount {
		t.Fatalf("idle service mode should not busy-spin: tick count advanced from %d to %d without new events",
			runtimeBeforeSubmit.TickCount,
			runtimeAfterIdleWait.TickCount,
		)
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-late-submit"}}); err != nil {
		t.Fatalf("SubmitWorkRequest late work: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snap, err := f.GetEngineStateSnapshot(context.Background())
		if err != nil {
			t.Fatalf("GetEngineStateSnapshot: %v", err)
		}
		for _, token := range snap.Marking.Tokens {
			if token.PlaceID == "task:done" {
				cancel()
				select {
				case err := <-errCh:
					if err != nil {
						t.Fatalf("Run after cancellation: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for service-mode runtime to stop after cancellation")
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	<-errCh
	t.Fatal("late-submitted work did not reach task:done before timeout")
}

func TestNew_BatchModeWithoutInitialWork_RejectsLateSubmissionAfterTermination(t *testing.T) {
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithInlineDispatch(),
		factory.WithWorkerExecutor("mock", &workers.NoopExecutor{}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, err = submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-after-stop"}})
	if err == nil {
		t.Fatal("expected late batch submission to fail after runtime termination")
	}
	if !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("expected terminated error, got %v", err)
	}
}

func TestNew_WorkerPoolDispatchResultHookRecordsCompletionAtObservedTick(t *testing.T) {
	var dispatches []interfaces.FactoryDispatchRecord
	var completions []interfaces.FactoryCompletionRecord
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			dispatches = append(dispatches, record)
		}),
		factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-hook"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(dispatches) != 1 {
		t.Fatalf("expected 1 recorded dispatch, got %d", len(dispatches))
	}
	if len(completions) != 1 {
		t.Fatalf("expected 1 recorded completion, got %d", len(completions))
	}
	dispatch := dispatches[0].Dispatch
	if completions[0].DispatchID != dispatch.DispatchID {
		t.Fatalf("completion dispatch ID = %q, want %q", completions[0].DispatchID, dispatch.DispatchID)
	}
	if completions[0].ObservedTick <= dispatch.Execution.DispatchCreatedTick {
		t.Fatalf("completion observed tick = %d, want after dispatch tick %d", completions[0].ObservedTick, dispatch.Execution.DispatchCreatedTick)
	}
}

func TestNew_ReplayDelayedWorkerPoolCompletionWakesAtPlannedTick(t *testing.T) {
	var completions []interfaces.FactoryCompletionRecord
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithCompletionDeliveryPlanner(fixedCompletionDeliveryPlanner{tick: 4}),
		factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-delayed"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].ObservedTick != 4 {
		t.Fatalf("completion observed tick = %d, want 4", completions[0].ObservedTick)
	}

	snap, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if len(snap.Marking.TokensInPlace("task:done")) != 1 {
		t.Fatalf("expected token to reach task:done at planned completion tick, marking = %#v", snap.Marking.Tokens)
	}
}

func TestNew_ReplayPlannerCanReplaceWorkerCompletionResult(t *testing.T) {
	var completions []interfaces.FactoryCompletionRecord
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithWorkerExecutor("mock", &passExecutor{}),
		factory.WithCompletionDeliveryPlanner(fixedCompletionDeliveryPlanner{
			tick: 4,
			plannedResult: interfaces.WorkResult{
				Outcome: interfaces.OutcomeAccepted,
				Output:  "replayed-output",
			},
		}),
		factory.WithCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-replayed-result"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(completions))
	}
	if completions[0].Result.Output != "replayed-output" {
		t.Fatalf("completion output = %q, want replayed-output", completions[0].Result.Output)
	}
}

func TestNew_ServiceModeWorkerPoolResultSignalCompletesLateSubmission(t *testing.T) {
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	f, err := New(
		factory.WithNet(buildSimpleNet()),
		factory.WithServiceMode(),
		factory.WithWorkerExecutor("mock", executor),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(runCtx)
	}()

	select {
	case err := <-errCh:
		t.Fatalf("Run returned before late worker-pool submission: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkID:     "work-late-pool",
		WorkTypeID: "task",
		TraceID:    "trace-late-pool",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest late work: %v", err)
	}

	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before worker result: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for worker-pool executor to start")
	}
	waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	close(executor.release)
	snap := waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount == 0 &&
			len(snapshot.DispatchHistory) == 1 &&
			markingContainsWorkAtPlace(&snapshot.Marking, "work-late-pool", "task:done")
	})
	if !hasFactoryEventType(runtimeGeneratedEvents(t, f), factoryapi.FactoryEventTypeDispatchResponse) {
		t.Fatalf("expected generated dispatch-completed event after result wake-up, snapshot=%#v", snap)
	}

	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func TestGetEngineStateSnapshot_AggregatesRuntimeLifecycleUptimeAndTopology(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	net := buildSimpleNet()
	f, err := New(
		factory.WithNet(net),
		factory.WithServiceMode(),
		factory.WithLogger(logging.NoopLogger{}),
		factory.WithClock(replay.NewDeterministicClock(base, time.Second)),
		factory.WithWorkerExecutor("mock", executor),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Run(runCtx)
	}()

	if _, err := submitWorkRequests(context.Background(), f, []interfaces.SubmitRequest{{
		WorkTypeID: "task",
		TraceID:    "trace-aggregate-snapshot",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	select {
	case <-executor.started:
	case err := <-errCh:
		t.Fatalf("Run returned before in-flight snapshot: %v", err)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking executor to start")
	}

	snap := waitForAggregateSnapshot(t, f, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.RuntimeStatus == interfaces.RuntimeStatusActive && snapshot.InFlightCount > 0
	})

	if snap.FactoryState != string(interfaces.FactoryStateRunning) {
		t.Fatalf("factory state = %q, want %q", snap.FactoryState, interfaces.FactoryStateRunning)
	}
	if snap.Uptime <= 0 {
		t.Fatalf("uptime = %v, want positive duration", snap.Uptime)
	}
	if snap.Topology != net {
		t.Fatal("aggregate snapshot did not include factory topology")
	}
	if snap.TickCount == 0 {
		t.Fatal("expected non-zero tick count in aggregate snapshot")
	}
	if len(snap.Dispatches) == 0 {
		t.Fatal("expected in-flight dispatch details in aggregate snapshot")
	}
	var consumed int
	for _, dispatch := range snap.Dispatches {
		consumed += len(dispatch.ConsumedTokens)
	}
	if consumed == 0 {
		t.Fatal("expected aggregate snapshot dispatches to include consumed tokens")
	}

	close(executor.release)
	cancelRun()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run after cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for factory run to stop")
	}
}
