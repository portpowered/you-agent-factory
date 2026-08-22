package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type orderedWorkerSessionsLifecycle struct {
	workersessions.Service
	order *[]string
}

func (s *orderedWorkerSessionsLifecycle) Stop(context.Context) error {
	*s.order = append(*s.order, "worker-sessions")
	return nil
}

func TestFactoryRuntimeShutdownStopsWorkerSessionsBeforeWorkers(t *testing.T) {
	order := make([]string, 0, 2)
	f := &factoryImpl{
		cfg: &runtimeConfig{
			workerSessions: &orderedWorkerSessionsLifecycle{
				Service: &fakeWorkerSessionsService{},
				order:   &order,
			},
		},
	}

	if err := f.stopDispatchRuntimeLocked(context.Background(), dispatchplanning.RuntimeStopReasonCancelled); err != nil {
		t.Fatalf("stopDispatchRuntimeLocked() error = %v, want nil", err)
	}
	if got, want := strings.Join(order, ","), "worker-sessions"; got != want {
		t.Fatalf("shutdown order = %q, want %q", got, want)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this subscription contract test keeps replay ordering and live-stream assertions together at the runtime seam.
func TestFactoryEventHistory_SubscribeReplaysHistoryThenStreamsLiveEvents(t *testing.T) {
	live := make(chan interfaces.FactoryEvent, 1)
	historyEvent := interfaces.FactoryEvent{Id: "history", Type: interfaces.FactoryEventTypeRunRequest}
	liveEvent := interfaces.FactoryEvent{Id: "live", Type: interfaces.FactoryEventTypeWorkRequest}
	live <- liveEvent
	close(live)
	ledger := &recordingfixtures.ScriptedRuntimeLedger{
		Events: []interfaces.FactoryEvent{historyEvent},
		SubscribeResult: interfaces.FactoryEventStream{
			Events: live,
		},
	}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withFactoryEventHistory(ledger),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := f.SubscribeFactoryEvents(ctx, nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if len(stream.History) != 1 || stream.History[0].Id != historyEvent.Id {
		t.Fatalf("replayed history = %#v, want scripted root event", stream.History)
	}
	if ledger.CallCount("Subscribe") != 1 {
		t.Fatalf("Subscribe calls = %d, want 1", ledger.CallCount("Subscribe"))
	}

	select {
	case event := <-stream.Events:
		if event.Id != liveEvent.Id || event.Type != liveEvent.Type {
			t.Fatalf("live event = %#v, want scripted root event %#v", event, liveEvent)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for injected live factory event")
	}
	// Recordings owns replay-before-live selection and publication ordering.
}

func TestFactoryEventHistory_StreamGenerationIDRemainsStableWithinOneLiveHistory(t *testing.T) {
	f := newPassingInlineRuntime(t)

	firstSnapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot(first): %v", err)
	}
	if strings.TrimSpace(firstSnapshot.StreamGenerationID) == "" {
		t.Fatal("first snapshot stream generation id is empty")
	}

	stream, err := f.SubscribeFactoryEvents(context.Background(), nil, interfaces.FactoryEventReconnectScope{})
	if err != nil {
		t.Fatalf("SubscribeFactoryEvents: %v", err)
	}
	if stream.StreamGenerationID != firstSnapshot.StreamGenerationID {
		t.Fatalf("stream generation id = %q, want snapshot id %q", stream.StreamGenerationID, firstSnapshot.StreamGenerationID)
	}

	secondSnapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot(second): %v", err)
	}
	if secondSnapshot.StreamGenerationID != firstSnapshot.StreamGenerationID {
		t.Fatalf("second snapshot stream generation id = %q, want %q", secondSnapshot.StreamGenerationID, firstSnapshot.StreamGenerationID)
	}
}

func TestNew_BatchModeWithoutInitialWork_TerminatesWithoutCancellation(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
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

func TestPetriMutationRecorderFailureStopsRuntimeWithDispatchContext(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
		withPetriMutationRecorder(func(string, []interfaces.TokenMutationRecord) error {
			return errors.New("persistence unavailable")
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-recording-failure"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	err = f.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "record completed dispatch") || !strings.Contains(err.Error(), "persistence unavailable") {
		t.Fatalf("Run error = %v, want contextual Petri recording failure", err)
	}
}

func TestPetriMutationRecorderSizeRejectionKeepsRuntimeLoopAvailable(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
		withPetriMutationRecorder(func(string, []interfaces.TokenMutationRecord) error {
			return nonFatalPetriMutationPersistenceError{}
		}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-recording-size-rejection"}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	if err := f.Run(context.Background()); err != nil {
		t.Fatalf("Run = %v, want size rejection logged without fatal engine error", err)
	}
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if snapshot.FactoryState != string(interfaces.FactoryStateCompleted) {
		t.Fatalf("factory state = %q, want completed after non-fatal size rejection", snapshot.FactoryState)
	}
}

type nonFatalPetriMutationPersistenceError struct{}

func (nonFatalPetriMutationPersistenceError) Error() string {
	return "durable session snapshot exceeds configured byte bound"
}

func (nonFatalPetriMutationPersistenceError) NonFatalPetriMutationPersistenceError() {}

func TestNew_ServiceModeWithoutInitialWork_WaitsForCancellation(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
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
	default:
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
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
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
	default:
	}

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-late-submit"}}); err != nil {
		t.Fatalf("SubmitWorkRequest late work: %v", err)
	}

	waitForWorkAtPlace(t, f, "task:done", time.Second)
	cancel()
	waitForRunStop(t, errCh)
}

func TestNew_BatchModeWithoutInitialWork_RejectsLateSubmissionAfterTermination(t *testing.T) {
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", &acceptedNoOutputExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := f.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	_, err = submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-after-stop"}})
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
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withWorkerExecutor("mock", &passExecutor{}),
		withDispatchRecorder(func(record interfaces.FactoryDispatchRecord) {
			dispatches = append(dispatches, record)
		}),
		withCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-hook"}}); err != nil {
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
	// On the async worker-pool path the engine deliberately drains the result
	// buffer mid-tick (see FactoryEngine.drainPendingResults and
	// forwardDispatches) so async results stay visible to TerminationCheck and
	// do not trip false deadlock detection. A worker that finishes before the
	// dispatching tick ends is therefore legitimately observed on that same
	// tick, so requiring a strictly later tick here is a race, not a contract.
	// The strictly-after ordering that IS a contract is covered deterministically
	// by TestNew_ReplayDelayedWorkerPoolCompletionWakesAtPlannedTick, which pins
	// delivery to a planned tick. What this test owns is that the async hook
	// records exactly one completion, correlated to its dispatch, stamped with a
	// tick that is never earlier than the dispatch tick.
	if completions[0].ObservedTick < dispatch.Execution.DispatchCreatedTick {
		t.Fatalf("completion observed tick = %d, want at or after dispatch tick %d", completions[0].ObservedTick, dispatch.Execution.DispatchCreatedTick)
	}
}

func TestNew_ReplayDelayedWorkerPoolCompletionWakesAtPlannedTick(t *testing.T) {
	var completions []interfaces.FactoryCompletionRecord
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withWorkerExecutor("mock", &passExecutor{}),
		withCompletionDeliveryPlanner(fixedCompletionDeliveryPlanner{tick: 4}),
		withCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-delayed"}}); err != nil {
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
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withWorkerExecutor("mock", &passExecutor{}),
		withCompletionDeliveryPlanner(fixedCompletionDeliveryPlanner{
			tick: 4,
			plannedResult: workerexecution.WorkResult{
				Outcome: workerexecution.OutcomeAccepted,
				Output:  "replayed-output",
			},
		}),
		withCompletionRecorder(func(record interfaces.FactoryCompletionRecord) {
			completions = append(completions, record)
		}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{WorkTypeID: "task", TraceID: "trace-replayed-result"}}); err != nil {
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
	history := &recordingfixtures.ScriptedRuntimeLedger{}
	f, err := newTestFactory(
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
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

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
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
	if history.CallCount("RecordWorkstationResponse") != 1 {
		t.Fatalf("RecordWorkstationResponse calls = %d, want 1 after result wake-up; snapshot=%#v", history.CallCount("RecordWorkstationResponse"), snap)
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

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime snapshot test keeps lifecycle, topology, and observability assertions in one readable flow.
func TestGetEngineStateSnapshot_AggregatesRuntimeLifecycleUptimeAndTopology(t *testing.T) {
	base := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	net := buildSimpleNet()
	f, err := newTestFactory(
		withNet(net),
		withServiceMode(),
		withLogger(logging.NoopLogger{}),
		withClock(platformclock.NewDeterministic(base, time.Second)),
		withWorkerExecutor("mock", executor),
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

	if _, err := submitWorkRequests(context.Background(), f, []work.SubmitRequest{{
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

func TestRuntimeVisitCountRoutesSharedTraceSiblingsIndependentlyAtThreshold(t *testing.T) {
	const (
		maxReviews  = 3
		sharedTrace = "trace-shared-siblings"
	)
	f, err := newTestFactory(
		withNet(buildVisitCountSiblingIsolationNet(maxReviews)),
		withServiceMode(),
		withInlineDispatch(),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, sharedTraceSiblingSubmissions(sharedTrace)); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	tickable := tickableFactory(t, f)
	for completedReviews := 1; completedReviews <= maxReviews; completedReviews++ {
		if err := tickable.Tick(ctx); err != nil {
			t.Fatalf("Tick review cycle %d: %v", completedReviews, err)
		}
		snapshot := runtimeSnapshot(t, f)
		assertWorkVisitState(t, snapshot, "work-repeated", "task:review", completedReviews)
		assertWorkVisitState(t, snapshot, "work-unaffected", "task:held", 0)
		assertReviewDispatch(t, snapshot, completedReviews-1, "work-repeated")
	}

	if err := tickable.Tick(ctx); err != nil {
		t.Fatalf("Tick loop breaker: %v", err)
	}
	exhausted := runtimeSnapshot(t, f)
	assertWorkVisitState(t, exhausted, "work-repeated", "task:failed", maxReviews)
	assertWorkVisitState(t, exhausted, "work-unaffected", "task:held", 0)
	if len(exhausted.DispatchHistory) != maxReviews {
		t.Fatalf("dispatch count after exhaustion = %d, want %d", len(exhausted.DispatchHistory), maxReviews)
	}

	if _, err := f.MoveWork(ctx, "work-unaffected", "review", work.WorkStateChangeSourceCLI, ""); err != nil {
		t.Fatalf("MoveWork unaffected sibling to review: %v", err)
	}
	if err := tickable.Tick(ctx); err != nil {
		t.Fatalf("Tick unaffected sibling first review: %v", err)
	}
	independent := runtimeSnapshot(t, f)
	assertWorkVisitState(t, independent, "work-unaffected", "task:review", 1)
	assertReviewDispatch(t, independent, maxReviews, "work-unaffected")
}

func TestRuntimeVisitCountReplayPreservesSharedTraceSiblingRouting(t *testing.T) {
	const maxReviews = 3
	live, history := runVisitCountSiblingIsolationScenario(t, maxReviews)
	liveSnapshot := runtimeSnapshot(t, live)
	assertWorkVisitState(t, liveSnapshot, "work-repeated", "task:failed", maxReviews)
	assertWorkVisitState(t, liveSnapshot, "work-unaffected", "task:review", 1)
	if len(history.WorkRequests) != 1 || len(history.WorkStateChanges) != 1 {
		t.Fatalf("replay input facts = requests %#v moves %#v, want one batch and one operator move", history.WorkRequests, history.WorkStateChanges)
	}
	if history.CallCount("RecordWorkstationResponse") != maxReviews+1 {
		t.Fatalf("completion facts = %d, want %d", history.CallCount("RecordWorkstationResponse"), maxReviews+1)
	}
	// Recordings owns encoding and replay-hook reconstruction of these injected
	// root facts; Runtime owns producing the complete, sibling-specific inputs.
}

func runVisitCountSiblingIsolationScenario(
	t *testing.T,
	maxReviews int,
) (factoryhost.Engine, *recordingfixtures.ScriptedRuntimeLedger) {
	t.Helper()
	f, history, err := newTestFactoryWithScriptedLedger(
		withNet(buildVisitCountSiblingIsolationNet(maxReviews)),
		withServiceMode(),
		withWorkerExecutor("mock", &passExecutor{}),
		withCompletionDeliveryPlanner(nextTickCompletionPlanner{}),
		withLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New live factory: %v", err)
	}

	ctx := context.Background()
	if _, err := submitWorkRequests(ctx, f, sharedTraceSiblingSubmissions("trace-shared-siblings-replay")); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	tickable := tickableFactory(t, f)
	for completedReviews := 1; completedReviews <= maxReviews; completedReviews++ {
		for phase := 0; phase < 2; phase++ {
			if err := tickable.Tick(ctx); err != nil {
				t.Fatalf("Tick live review cycle %d phase %d: %v", completedReviews, phase+1, err)
			}
		}
		snapshot := runtimeSnapshot(t, f)
		assertWorkVisitState(t, snapshot, "work-repeated", "task:review", completedReviews)
		assertWorkVisitState(t, snapshot, "work-unaffected", "task:held", 0)
	}
	if err := tickable.Tick(ctx); err != nil {
		t.Fatalf("Tick live loop breaker: %v", err)
	}
	if _, err := f.MoveWork(ctx, "work-unaffected", "review", work.WorkStateChangeSourceCLI, ""); err != nil {
		t.Fatalf("MoveWork live unaffected sibling: %v", err)
	}
	for phase := 0; phase < 2; phase++ {
		if err := tickable.Tick(ctx); err != nil {
			t.Fatalf("Tick live unaffected sibling phase %d: %v", phase+1, err)
		}
	}
	return f, history
}

type nextTickCompletionPlanner struct{}

func (nextTickCompletionPlanner) DeliveryTickForDispatch(dispatch work.WorkDispatch) (int, bool, error) {
	return dispatch.Execution.DispatchCreatedTick + 1, true, nil
}

func buildVisitCountSiblingIsolationNet(maxReviews int) *state.Net {
	workType := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "held", Category: state.StateCategoryInitial},
			{Value: "review", Category: state.StateCategoryProcessing},
			{Value: "done", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, place := range workType.GeneratePlaces() {
		places[place.ID] = place
	}
	reviewInput := petri.Arc{
		ID: "review-in", Name: "work", PlaceID: "task:review", Direction: petri.ArcInput,
		Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
	}
	return &state.Net{
		ID: "visit-count-sibling-isolation", Places: places,
		WorkTypes: map[string]*state.WorkType{"task": workType}, Resources: make(map[string]*state.ResourceDef),
		Transitions: map[string]*petri.Transition{
			"quality-check": {
				ID: "quality-check", Name: "quality-check", Type: petri.TransitionNormal, WorkerType: "mock",
				InputArcs:  []petri.Arc{reviewInput},
				OutputArcs: []petri.Arc{{ID: "review-again", PlaceID: "task:review", Direction: petri.ArcOutput}},
			},
			"review-loop-breaker": {
				ID: "review-loop-breaker", Name: "review-loop-breaker", Type: petri.TransitionExhaustion,
				InputArcs: []petri.Arc{{
					ID: "exhausted-review", Name: "work", PlaceID: "task:review", Direction: petri.ArcInput,
					Cardinality: petri.ArcCardinality{Mode: petri.CardinalityOne},
					Guard:       &petri.VisitCountGuard{TransitionID: "quality-check", MaxVisits: maxReviews},
				}},
				OutputArcs: []petri.Arc{{ID: "review-failed", PlaceID: "task:failed", Direction: petri.ArcOutput}},
			},
		},
	}
}

func sharedTraceSiblingSubmissions(sharedTrace string) []work.SubmitRequest {
	return []work.SubmitRequest{
		{RequestID: "request-shared-siblings", WorkID: "work-repeated", Name: "repeated", WorkTypeID: "task", TargetState: "review", CurrentChainingTraceID: sharedTrace, TraceID: sharedTrace},
		{RequestID: "request-shared-siblings", WorkID: "work-unaffected", Name: "unaffected", WorkTypeID: "task", TargetState: "held", CurrentChainingTraceID: sharedTrace, TraceID: sharedTrace},
	}
}

func runtimeSnapshot(t *testing.T, f factoryhost.Engine) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	snapshot, err := f.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	return snapshot
}

func assertWorkVisitState(t *testing.T, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID, placeID string, visits int) {
	t.Helper()
	for _, token := range snapshot.Marking.Tokens {
		if token.Color.WorkID != workID {
			continue
		}
		if token.PlaceID != placeID {
			t.Fatalf("work %s place = %q, want %q", workID, token.PlaceID, placeID)
		}
		if got := token.History.TotalVisits["quality-check"]; got != visits {
			t.Fatalf("work %s quality-check visits = %d, want %d", workID, got, visits)
		}
		return
	}
	t.Fatalf("work %s missing from marking", workID)
}

func assertReviewDispatch(t *testing.T, snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], index int, workID string) {
	t.Helper()
	if len(snapshot.DispatchHistory) <= index {
		t.Fatalf("dispatch history count = %d, want index %d", len(snapshot.DispatchHistory), index)
	}
	dispatch := snapshot.DispatchHistory[index]
	if dispatch.TransitionID != "quality-check" {
		t.Fatalf("dispatch[%d] transition = %q, want quality-check", index, dispatch.TransitionID)
	}
	for _, token := range dispatch.ConsumedTokens {
		if token.Color.WorkID == workID {
			return
		}
	}
	t.Fatalf("dispatch[%d] did not consume work %s", index, workID)
}

func TestRestoredDispatchRequestEventPreservesRestartMetadataAndResources(t *testing.T) {
	now := time.Date(2026, time.April, 10, 12, 0, 0, 0, time.UTC)
	cfg := &runtimeConfig{
		clock:              platformclock.NewDeterministic(now, time.Second),
		restoredWorldState: &interfaces.FactoryWorldState{Tick: 9},
	}
	dispatch := interfaces.FactoryWorldDispatch{
		DispatchID:               "dispatch-restart",
		TransitionID:             "t-process",
		StartedTick:              11,
		StartedAt:                now.Add(-time.Minute),
		RunnerID:                 "runner-restart",
		RunnerSelectionSource:    workerexecution.RunnerSelectionSourceFactory,
		WorkItemIDs:              []string{"work-restart", "work-restart"},
		CurrentChainingTraceID:   "trace-current",
		PreviousChainingTraceIDs: []string{"trace-previous", "trace-previous", ""},
		TraceIDs:                 []string{"trace-restart", "trace-restart", ""},
		Inputs: []interfaces.WorkstationInput{
			{TokenID: "work-restart", PlaceID: "task:init", WorkItem: &work.FactoryWorkItem{ID: "work-restart"}},
			{TokenID: "resource-token", PlaceID: "gpu:available", Resource: &interfaces.FactoryResourceUnit{ResourceID: "gpu", TokenID: "resource-token"}},
		},
		Resources: []interfaces.FactoryResourceUnit{
			{ResourceID: "gpu", TokenID: "resource-token"},
			{ResourceID: " ", TokenID: "ignored"},
		},
	}

	event, err := restoredDispatchRequestEvent(cfg, dispatch)
	if err != nil {
		t.Fatalf("restoredDispatchRequestEvent: %v", err)
	}
	var payload interfaces.DispatchRequestEventPayload
	if err := event.DecodePayload(&payload); err != nil {
		t.Fatalf("decode restored dispatch request: %v", err)
	}
	assertRestoredDispatchRequestEnvelope(t, event, dispatch)
	assertRestoredDispatchRequestMetadata(t, payload, dispatch)
	assertRestoredDispatchRequestResources(t, payload)
	assertRestoredDispatchRequestInputs(t, payload)
	assertEmptyRestoredDispatchResourceRefs(t)
}

func assertRestoredDispatchRequestEnvelope(t *testing.T, event interfaces.FactoryEvent, dispatch interfaces.FactoryWorldDispatch) {
	t.Helper()
	if event.Type != interfaces.FactoryEventTypeDispatchRequest {
		t.Fatalf("restored dispatch request type = %q, want %q", event.Type, interfaces.FactoryEventTypeDispatchRequest)
	}
	if stringPointerValue(event.Context.DispatchID) != dispatch.DispatchID {
		t.Fatalf("restored dispatch request dispatch ID = %q, want %q", stringPointerValue(event.Context.DispatchID), dispatch.DispatchID)
	}
}

func assertRestoredDispatchRequestMetadata(t *testing.T, payload interfaces.DispatchRequestEventPayload, dispatch interfaces.FactoryWorldDispatch) {
	t.Helper()
	if payload.TransitionID != dispatch.TransitionID {
		t.Fatalf("restored dispatch transition = %q, want %q", payload.TransitionID, dispatch.TransitionID)
	}
	if payload.Metadata == nil {
		t.Fatal("restored dispatch metadata = nil, want runner facts")
	}
	if stringPointerValue(payload.Metadata.RunnerID) != dispatch.RunnerID {
		t.Fatalf("restored dispatch runner ID = %q, want %q", stringPointerValue(payload.Metadata.RunnerID), dispatch.RunnerID)
	}
	if payload.Metadata.RunnerSelectionSource == nil {
		t.Fatal("restored dispatch runner selection source = nil")
	}
	if *payload.Metadata.RunnerSelectionSource != dispatch.RunnerSelectionSource {
		t.Fatalf("restored dispatch runner selection source = %q, want %q", *payload.Metadata.RunnerSelectionSource, dispatch.RunnerSelectionSource)
	}
}

func assertRestoredDispatchRequestResources(t *testing.T, payload interfaces.DispatchRequestEventPayload) {
	t.Helper()
	if payload.Resources == nil {
		t.Fatal("restored dispatch resources = nil, want one gpu resource")
	}
	if len(*payload.Resources) != 1 {
		t.Fatalf("restored dispatch resource count = %d, want one", len(*payload.Resources))
	}
	if (*payload.Resources)[0].Name != "gpu" {
		t.Fatalf("restored dispatch resource name = %q, want gpu", (*payload.Resources)[0].Name)
	}
}

func assertRestoredDispatchRequestInputs(t *testing.T, payload interfaces.DispatchRequestEventPayload) {
	t.Helper()
	if len(payload.Inputs) != 1 {
		t.Fatalf("restored dispatch input count = %d, want one deduplicated Work input", len(payload.Inputs))
	}
	if payload.Inputs[0].WorkID != "work-restart" {
		t.Fatalf("restored dispatch Work input = %q, want work-restart", payload.Inputs[0].WorkID)
	}
}

func assertEmptyRestoredDispatchResourceRefs(t *testing.T) {
	t.Helper()
	if got := restoredDispatchResourceRefs(nil); got != nil {
		t.Fatalf("empty restored resource refs = %#v, want nil", got)
	}
	if got := restoredDispatchResourceRefs([]interfaces.FactoryResourceUnit{{ResourceID: " "}}); got != nil {
		t.Fatalf("blank restored resource refs = %#v, want nil", got)
	}
}

func TestRestoredWorkPlacementHandlesApprovalAndTokenIdentityCollisions(t *testing.T) {
	net := buildSimpleNet()
	net.Transitions["t-approval"] = &petri.Transition{ID: "t-approval", Type: petri.TransitionHumanApproval}
	item := work.FactoryWorkItem{ID: "work-restored", WorkTypeID: "task", State: "init"}
	pendingApproval := interfaces.FactoryWorldDispatch{DispatchID: "dispatch-approval", TransitionID: "t-process"}
	approvalWorld := &interfaces.FactoryWorldState{
		PendingHumanApprovalsByID: map[string]interfaces.FactoryWorldHumanApproval{
			"approval": {ApprovalID: "approval", DispatchID: pendingApproval.DispatchID},
		},
	}
	if !restoredDispatchIsHumanApproval(approvalWorld, net, pendingApproval) {
		t.Fatal("pending approval dispatch was not recognized as human approval")
	}
	if !restoredDispatchIsHumanApproval(nil, net, interfaces.FactoryWorldDispatch{TransitionID: "t-approval"}) {
		t.Fatal("human-approval transition was not recognized")
	}
	if restoredDispatchIsHumanApproval(nil, nil, interfaces.FactoryWorldDispatch{DispatchID: "dispatch-normal"}) {
		t.Fatal("ordinary dispatch was recognized as human approval")
	}

	marking := petri.NewMarking("restart-test")
	existing := restoredWorkToken(item, "task:init", "", nil, time.Unix(0, 0).UTC())
	existing.ID = "restored-work:" + item.ID
	marking.AddToken(existing)
	first := uniqueRestoredWorkTokenID(marking, item.ID)
	if first != "restored-work:work-restored:2" {
		t.Fatalf("first collision-safe token ID = %q, want restored-work:work-restored:2", first)
	}
	second := restoredWorkToken(item, "task:init", "", nil, time.Unix(0, 0).UTC())
	second.ID = first
	marking.AddToken(second)
	if got := uniqueRestoredWorkTokenID(marking, item.ID); got != "restored-work:work-restored:3" {
		t.Fatalf("repeated collision-safe token ID = %q, want restored-work:work-restored:3", got)
	}
}
