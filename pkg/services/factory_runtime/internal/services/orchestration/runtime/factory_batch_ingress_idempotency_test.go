package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factorycontext "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestConcurrentBatchIngressSameRequestIDReplayIsIdempotent_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	batchRequest := ingressBatchRequest(
		"request-idempotent-replay",
		"work-idempotent-replay",
		"trace-idempotent-replay",
	)

	first, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	assertIngressObservable(t, history, h.Factory, batchRequest.RequestID, "work-idempotent-replay")

	replayed, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest)
	if err != nil {
		t.Fatalf("replay SubmitWorkRequest: %v", err)
	}
	if replayed.Accepted {
		t.Fatalf("replay accepted = true, want idempotent no-op")
	}
	if replayed.RequestID != first.RequestID || replayed.WorkID != first.WorkID {
		t.Fatalf("replay identity = %#v, want preserved %#v", replayed, first)
	}
	if countWorkRequestRecords(history, batchRequest.RequestID) != 1 {
		t.Fatalf("WORK_REQUEST count for %q = %d, want 1", batchRequest.RequestID, countWorkRequestRecords(history, batchRequest.RequestID))
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if countObservedWorkMaterializations(snap, "work-idempotent-replay") != 1 {
		t.Fatalf("observed work materializations for work-idempotent-replay = %d, want 1", countObservedWorkMaterializations(snap, "work-idempotent-replay"))
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func TestConcurrentBatchIngressDifferentRequestIDAfterVisibleAcceptanceDoesNotDuplicateWork_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor, history, h := startBlockedDispatchIngressHarness(t)

	const workID = "work-visible-retry"
	firstRequest := ingressBatchRequest("request-visible-first", workID, "trace-visible-first")
	secondRequest := ingressBatchRequest("request-visible-retry", workID, "trace-visible-retry")

	first, err := h.Factory.SubmitWorkRequest(context.Background(), firstRequest)
	if err != nil {
		t.Fatalf("first SubmitWorkRequest: %v", err)
	}
	if !first.Accepted {
		t.Fatalf("first submit accepted = false, want true")
	}
	assertIngressObservable(t, history, h.Factory, firstRequest.RequestID, workID)

	second, err := h.Factory.SubmitWorkRequest(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("retry SubmitWorkRequest: %v", err)
	}
	if second.Accepted {
		t.Fatalf("different-request-ID retry accepted = true, want rejection after visible acceptance")
	}
	if countWorkRequestRecords(history, secondRequest.RequestID) != 0 {
		t.Fatalf("WORK_REQUEST count for retry request %q = %d, want 0", secondRequest.RequestID, countWorkRequestRecords(history, secondRequest.RequestID))
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if countObservedWorkMaterializations(snap, workID) != 1 {
		t.Fatalf("observed work materializations for %q = %d, want 1 durable materialization", workID, countObservedWorkMaterializations(snap, workID))
	}

	releaseBlockedDispatchHarness(t, executor, h)
}

func startBlockedDispatchIngressHarness(t *testing.T) (*ingressBlockingExecutor, *recordingfixtures.ScriptedRuntimeLedger, *serviceModeRunHarness) {
	t.Helper()

	executor := &ingressBlockingExecutor{
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	history := &recordingfixtures.ScriptedRuntimeLedger{}
	h := startServiceModeRunHarness(t,
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		WorkID:     "work-blocked-dispatch",
		WorkTypeID: "task",
		TraceID:    "trace-blocked-dispatch",
	}}); err != nil {
		t.Fatalf("submit initial work: %v", err)
	}

	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking dispatch to start")
	}

	return executor, history, h
}

func releaseBlockedDispatchHarness(t *testing.T, executor *ingressBlockingExecutor, h *serviceModeRunHarness) {
	t.Helper()
	close(executor.release)
	h.cancel()
	select {
	case <-h.errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func ingressBatchRequest(requestID, workID, traceID string) work.WorkRequest {
	return work.WorkRequest{
		RequestID: requestID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "ingress-batch",
			WorkID:     workID,
			WorkTypeID: "task",
			TraceID:    traceID,
		}},
	}
}

func assertIngressObservable(
	t *testing.T,
	history *recordingfixtures.ScriptedRuntimeLedger,
	factoryInstance factoryhost.Engine,
	requestID string,
	workID string,
) {
	t.Helper()
	if !workRequestRecorded(history, requestID) {
		t.Fatalf("WORK_REQUEST not recorded for %q; work requests=%#v", requestID, history.WorkRequests)
	}
	snap, err := factoryInstance.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !snapshotObservesWork(snap, workID) {
		t.Fatalf("work %q not observable before retry; marking=%#v", workID, snap.Marking.Tokens)
	}
}

func countWorkRequestRecords(history *recordingfixtures.ScriptedRuntimeLedger, requestID string) int {
	count := 0
	for _, record := range history.WorkRequests {
		if record.RequestID == requestID {
			count++
		}
	}
	return count
}

func countMarkingTokensForWorkID(marking *petri.MarkingSnapshot, workID string) int {
	count := 0
	for _, token := range marking.Tokens {
		if token != nil && token.Color.WorkID == workID {
			count++
		}
	}
	return count
}

func countObservedWorkMaterializations(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) int {
	if snap == nil {
		return 0
	}
	count := countMarkingTokensForWorkID(&snap.Marking, workID)
	for _, entry := range snap.Dispatches {
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				count++
			}
		}
	}
	return count
}

type ingressBlockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *ingressBlockingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	select {
	case e.started <- struct{}{}:
	default:
	}
	<-e.release
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

func TestConcurrentBatchIngressProjectsWhileDispatchBlocked_ServiceModeWorkerPool(t *testing.T) {
	t.Helper()

	executor := &blockingExecutor{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	history := &recordingfixtures.ScriptedRuntimeLedger{}

	opts := []testFactoryOption{
		withNet(buildSimpleNet()),
		withServiceMode(),
		withWorkerExecutor("mock", executor),
		withFactoryEventHistory(history),
		withLogger(logging.NoopLogger{}),
	}

	h := startServiceModeRunHarness(t, opts...)

	if _, err := submitWorkRequests(context.Background(), h.Factory, []work.SubmitRequest{{
		WorkID:     "work-blocked-dispatch",
		WorkTypeID: "task",
		TraceID:    "trace-blocked-dispatch",
	}}); err != nil {
		t.Fatalf("submit initial work: %v", err)
	}

	waitForAggregateSnapshot(t, h.Factory, func(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
		return snapshot.InFlightCount > 0
	})

	select {
	case <-executor.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for blocking dispatch to start")
	}

	batchRequest := work.WorkRequest{
		RequestID: "request-concurrent-ingress",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "concurrent-ingress",
			WorkID:     "work-concurrent-ingress",
			WorkTypeID: "task",
			TraceID:    "trace-concurrent-ingress",
		}},
	}
	if _, err := h.Factory.SubmitWorkRequest(context.Background(), batchRequest); err != nil {
		t.Fatalf("SubmitWorkRequest concurrent batch: %v", err)
	}

	if !workRequestRecorded(history, batchRequest.RequestID) {
		t.Fatalf("WORK_REQUEST not recorded before submit returned; work requests=%#v", history.WorkRequests)
	}
	snap, err := h.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	if !snapshotObservesWork(snap, "work-concurrent-ingress") {
		t.Fatalf(
			"submit returned before work became observable; work requests=%#v marking tokens=%#v",
			history.WorkRequests,
			snap.Marking.Tokens,
		)
	}

	close(executor.release)
	h.cancel()
	select {
	case <-h.errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service-mode runtime to stop")
	}
}

func workRequestRecorded(history *recordingfixtures.ScriptedRuntimeLedger, requestID string) bool {
	for _, record := range history.WorkRequests {
		if record.RequestID == requestID {
			return true
		}
	}
	return false
}

func snapshotObservesWork(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workID string) bool {
	if snap == nil {
		return false
	}
	if markingContainsWorkAtPlace(&snap.Marking, workID, "task:init") {
		return true
	}
	for _, entry := range snap.Dispatches {
		for _, token := range entry.ConsumedTokens {
			if token.Color.WorkID == workID {
				return true
			}
		}
	}
	return false
}

func TestRuntimeCompletionDurablyClosesSourceForSuccessorMetrics(t *testing.T) {
	history := newCompletionTestHistory(t)
	completionPublished := 0
	history.AddDeferredSessionCompletionRecorder(func() { completionPublished++ })

	var flushed [][]recordings.FactoryEvent
	factory := completionTestFactory(history, func() error {
		flushed = append(flushed, history.CanonicalEvents())
		return nil
	})

	if err := recordSessionLifecycleCompletionFromFactory(
		factory, 2, testCompletionFactoryState(), "", completionTestTime(),
	); err != nil {
		t.Fatalf("record terminal source lifecycle: %v", err)
	}
	if completionPublished != 0 {
		t.Fatalf("completion callbacks before durability publication = %d, want 0", completionPublished)
	}
	if len(flushed) != 2 {
		t.Fatalf("durability flushes = %d, want result and completion flushes", len(flushed))
	}
	if countCompletionEvents(flushed[0]) != 0 {
		t.Fatalf("pre-completion durable snapshot contains SESSION_COMPLETED: %#v", flushed[0])
	}
	if countCompletionEvents(flushed[1]) != 1 {
		t.Fatalf("post-completion durable snapshot has %d SESSION_COMPLETED events, want 1", countCompletionEvents(flushed[1]))
	}

	publishDeferredSessionCompletion(history)
	publishDeferredSessionCompletion(history)
	if completionPublished != 1 {
		t.Fatalf("completion callbacks = %d, want exactly one", completionPublished)
	}
	if countCompletionEvents(history.CanonicalEvents()) != 1 {
		t.Fatalf("in-memory SESSION_COMPLETED count = %d, want exactly one", countCompletionEvents(history.CanonicalEvents()))
	}
}

func TestRuntimeCompletionFlushFailureLeavesSourceIncompleteAndRetryable(t *testing.T) {
	history := newCompletionTestHistory(t)
	completionPublished := 0
	history.AddDeferredSessionCompletionRecorder(func() { completionPublished++ })
	flushErr := errors.New("durable source flush failed")
	flushCalls := 0
	factory := completionTestFactory(history, func() error {
		flushCalls++
		if flushCalls == 1 {
			return flushErr
		}
		return nil
	})

	err := recordSessionLifecycleCompletionFromFactory(
		factory, 2, testCompletionFactoryState(), "", completionTestTime(),
	)
	if !errors.Is(err, flushErr) {
		t.Fatalf("first completion error = %v, want flush cause", err)
	}
	if countCompletionEvents(history.CanonicalEvents()) != 0 || completionPublished != 0 {
		t.Fatalf("failed completion advertised close: events=%d callbacks=%d", countCompletionEvents(history.CanonicalEvents()), completionPublished)
	}

	if err := recordSessionLifecycleCompletionFromFactory(
		factory, 2, testCompletionFactoryState(), "", completionTestTime(),
	); err != nil {
		t.Fatalf("retry terminal source lifecycle: %v", err)
	}
	publishDeferredSessionCompletion(history)
	if countCompletionEvents(history.CanonicalEvents()) != 1 || completionPublished != 1 {
		t.Fatalf("retry completion = events:%d callbacks:%d, want one durable close and callback", countCompletionEvents(history.CanonicalEvents()), completionPublished)
	}
}

func TestRuntimeCompletionConcurrentCallbacksRemainExactlyOnce(t *testing.T) {
	history := newCompletionTestHistory(t)
	factory := completionTestFactory(history, nil)
	const callbacks = 24
	var group sync.WaitGroup
	group.Add(callbacks)
	for index := 0; index < callbacks; index++ {
		go func() {
			defer group.Done()
			if err := recordSessionLifecycleCompletionFromFactory(
				factory, 2, testCompletionFactoryState(), "", completionTestTime(),
			); err != nil {
				t.Errorf("concurrent completion callback: %v", err)
			}
		}()
	}
	group.Wait()

	if got := countCompletionEvents(history.CanonicalEvents()); got != 1 {
		t.Fatalf("concurrent SESSION_COMPLETED count = %d, want exactly one", got)
	}
}

type completionTestLedger struct {
	recordings.RuntimeLedger
	mu             sync.Mutex
	events         []recordings.FactoryEvent
	completionJobs []func()
	pending        bool
	published      bool
}

func (ledger *completionTestLedger) CanonicalEvents() []recordings.FactoryEvent {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return append([]recordings.FactoryEvent(nil), ledger.events...)
}

func (ledger *completionTestLedger) RecordSessionLifecycleResultUpdated(
	string, *interfaces.FactoryConfig, int, interfaces.FactoryState, string, time.Time,
) {
	ledger.mu.Lock()
	ledger.events = append(ledger.events, recordings.FactoryEvent{
		Type: interfaces.FactoryEventTypeSessionResultUpdated,
	})
	ledger.mu.Unlock()
}

func (ledger *completionTestLedger) RecordSessionLifecycleCompleted(
	string, *interfaces.FactoryConfig, int, interfaces.FactoryState, string, time.Time,
) {
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	for _, event := range ledger.events {
		if event.Type == interfaces.FactoryEventTypeSessionCompleted {
			return
		}
	}
	ledger.events = append(ledger.events, recordings.FactoryEvent{
		Type: interfaces.FactoryEventTypeSessionCompleted,
	})
	ledger.pending = true
}

func (ledger *completionTestLedger) AddDeferredSessionCompletionRecorder(recorder func()) {
	if recorder == nil {
		return
	}
	ledger.mu.Lock()
	ledger.completionJobs = append(ledger.completionJobs, recorder)
	if countCompletionEvents(ledger.events) > 0 {
		ledger.pending = true
	}
	ledger.mu.Unlock()
}

func (ledger *completionTestLedger) PublishDeferredSessionCompletion() {
	ledger.mu.Lock()
	if !ledger.pending || ledger.published {
		ledger.mu.Unlock()
		return
	}
	ledger.published = true
	ledger.pending = false
	jobs := append([]func(){}, ledger.completionJobs...)
	ledger.completionJobs = nil
	ledger.mu.Unlock()
	for _, job := range jobs {
		job()
	}
}

func newCompletionTestHistory(t *testing.T) *completionTestLedger {
	t.Helper()
	return &completionTestLedger{}
}

func completionTestFactory(history recordings.RuntimeLedger, flush func() error) *factoryImpl {
	return &factoryImpl{
		cfg: &runtimeConfig{
			workflowContext: &factorycontext.FactoryContext{SessionID: "source-session"},
		},
		eventHistory:             history,
		completionDurabilityGate: flush,
	}
}

func testCompletionFactoryState() interfaces.FactoryState {
	return interfaces.FactoryStateCompleted
}

func completionTestTime() time.Time {
	return time.Date(2026, 8, 29, 2, 0, 0, 0, time.UTC)
}

func countCompletionEvents(events []recordings.FactoryEvent) int {
	count := 0
	for _, event := range events {
		if event.Type == interfaces.FactoryEventTypeSessionCompleted {
			count++
		}
	}
	return count
}
