package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// countingWorkerSessionsService wraps a worker_sessions.Service and counts
// Start calls per session ID so tests can prove a duplicate/retried dispatch
// does not start a second Worker Session.
type countingWorkerSessionsService struct {
	inner workersessions.Service

	mu             sync.Mutex
	startCallsByID map[string]int
}

func newCountingWorkerSessionsService(inner workersessions.Service) *countingWorkerSessionsService {
	return &countingWorkerSessionsService{inner: inner, startCallsByID: map[string]int{}}
}

func (s *countingWorkerSessionsService) Reserve(
	ctx context.Context, req workersessions.ReserveRequest,
) (workersessions.Session, error) {
	return s.inner.Reserve(ctx, req)
}

func (s *countingWorkerSessionsService) Get(
	ctx context.Context, req workersessions.GetRequest,
) (workersessions.Session, error) {
	return s.inner.Get(ctx, req)
}

func (s *countingWorkerSessionsService) List(
	ctx context.Context, req workersessions.ListRequest,
) (workersessions.ListResult, error) {
	return s.inner.List(ctx, req)
}

func (s *countingWorkerSessionsService) Start(
	ctx context.Context, req workersessions.StartRequest,
) (workersessions.StartResult, error) {
	s.mu.Lock()
	s.startCallsByID[req.ID]++
	s.mu.Unlock()
	return s.inner.Start(ctx, req)
}

func (s *countingWorkerSessionsService) PublishRecord(
	ctx context.Context, req workersessions.PublishRecordRequest,
) (workersessions.PublishRecordResult, error) {
	return s.inner.PublishRecord(ctx, req)
}

func (s *countingWorkerSessionsService) startCallCount(sessionID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.startCallsByID[sessionID]
}

func (s *countingWorkerSessionsService) totalStartCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, count := range s.startCallsByID {
		total += count
	}
	return total
}

// TestFactoryImpl_DuplicatePlanDispatchReusesAssociationAndDoesNotRestartWorkerSessions
// proves story 003's core idempotency requirement: repeating an identical
// planned dispatch reuses the original Worker Session association and starts
// worker_sessions.Service.Start exactly once, never twice.
func TestFactoryImpl_DuplicatePlanDispatchReusesAssociationAndDoesNotRestartWorkerSessions(t *testing.T) {
	boundary := &testWorkstationBoundary{}
	counting := newCountingWorkerSessionsService(&fakeWorkerSessionsService{execution: boundary})
	runtime, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withWorkerSessions(counting),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "dup-dispatch-1",
		CorrelationID:   "dup-corr-1",
		WorkIDs:         []string{"dup-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/dup-trace/dup-work-1",
	}

	first, err := impl.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch(first)")
	if first.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch(first) outcome = %q, want ACCEPTED", first.Outcome)
	}

	second, err := impl.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch(duplicate)")
	if second.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("PlanDispatch(duplicate) outcome = %q, want DUPLICATE_IDEMPOTENT", second.Outcome)
	}

	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 1 {
		t.Fatalf("associations recorded = %#v, want exactly one for a repeated identical dispatch", associations)
	}
	if associations[0].DispatchID != plan.DispatchID || associations[0].WorkerSessionID != plan.DispatchID {
		t.Fatalf("association = %#v, want dispatch/session identity %q", associations[0], plan.DispatchID)
	}
	if got := counting.startCallCount(plan.DispatchID); got != 1 {
		t.Fatalf("worker_sessions.Service.Start calls for %q = %d, want exactly 1", plan.DispatchID, got)
	}
	if got := counting.totalStartCalls(); got != 1 {
		t.Fatalf("total worker_sessions.Service.Start calls = %d, want exactly 1", got)
	}
}

// TestFactoryImpl_DistinctRetryDispatchesGetDistinctWorkerSessionAssociations
// proves that a retry dispatch carrying a distinct dispatch ID for the same
// underlying Work lineage receives its own distinct Worker Session identity
// and canonical association, with no cross-association between the two
// attempts.
func TestFactoryImpl_DistinctRetryDispatchesGetDistinctWorkerSessionAssociations(t *testing.T) {
	boundary := &testWorkstationBoundary{}
	runtime, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	impl.state = interfaces.FactoryStateRunning

	original := factory.PlanDispatchRequest{
		DispatchID:      "retry-dispatch-original",
		CorrelationID:   "retry-corr-original",
		WorkIDs:         []string{"retry-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/retry-trace/retry-work-1",
	}
	retry := factory.PlanDispatchRequest{
		DispatchID:      "retry-dispatch-attempt-2",
		CorrelationID:   "retry-corr-attempt-2",
		WorkIDs:         []string{"retry-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/retry-trace/retry-work-1",
	}

	firstPlanned, err := impl.PlanDispatch(t.Context(), original)
	requireNoRootErr(t, err, "PlanDispatch(original)")
	if firstPlanned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch(original) outcome = %q, want ACCEPTED", firstPlanned.Outcome)
	}

	retryPlanned, err := impl.PlanDispatch(t.Context(), retry)
	requireNoRootErr(t, err, "PlanDispatch(retry)")
	if retryPlanned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch(retry) outcome = %q, want ACCEPTED (distinct dispatch ID)", retryPlanned.Outcome)
	}
	if retryPlanned.DispatchID == firstPlanned.DispatchID {
		t.Fatalf("retry dispatch ID = %q, want distinct from original %q", retryPlanned.DispatchID, firstPlanned.DispatchID)
	}

	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 2 {
		t.Fatalf("associations recorded = %#v, want exactly two (one per distinct dispatch ID)", associations)
	}
	byDispatch := map[string]string{}
	for _, association := range associations {
		byDispatch[association.DispatchID] = association.WorkerSessionID
	}
	originalSession, ok := byDispatch[original.DispatchID]
	if !ok || originalSession != original.DispatchID {
		t.Fatalf("original association = %#v, want session ID %q", byDispatch, original.DispatchID)
	}
	retrySession, ok := byDispatch[retry.DispatchID]
	if !ok || retrySession != retry.DispatchID {
		t.Fatalf("retry association = %#v, want session ID %q", byDispatch, retry.DispatchID)
	}
	if originalSession == retrySession {
		t.Fatalf("original and retry share Worker Session ID %q, want pairwise-distinct identities", originalSession)
	}
}

// TestFactoryImpl_ConcurrentAcceptDispatchResultResolvesExactlyOnce proves
// concurrent duplicate AcceptDispatchResult delivery for the same dispatch
// resolves to exactly one accepted Runtime terminal result and one Work
// materialization, with every other concurrent caller observing the
// deterministic DUPLICATE_IDEMPOTENT outcome.
func TestFactoryImpl_ConcurrentAcceptDispatchResultResolvesExactlyOnce(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	runtime, ledger, err := newTestFactoryWithScriptedLedger(
		withNet(buildSimpleNet()),
		withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: "work-result-ingress", WorkTypeID: "task", TraceID: "trace-concurrent-accept",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)

	accepted := factory.AcceptDispatchResultRequest{
		DispatchID: request.Execution.Dispatch.DispatchID, CorrelationID: request.Execution.Dispatch.DispatchID,
		WorkID: "work-result-ingress", ResultOutcome: factory.DispatchResultOutcomeSuccess,
	}

	const concurrentCallers = 16
	var retiredCount, duplicateCount, errorCount atomic.Int32
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(concurrentCallers)
	for range concurrentCallers {
		go func() {
			defer done.Done()
			start.Wait()
			result, err := impl.AcceptDispatchResult(t.Context(), accepted)
			switch {
			case err != nil:
				errorCount.Add(1)
			case result.Outcome == factory.DispatchPlanOutcomeRetired:
				retiredCount.Add(1)
			case result.Outcome == factory.DispatchPlanOutcomeDuplicateIdempotent:
				duplicateCount.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()

	if errorCount.Load() != 0 {
		t.Fatalf("concurrent AcceptDispatchResult errors = %d, want 0", errorCount.Load())
	}
	if retiredCount.Load() != 1 {
		t.Fatalf("concurrent AcceptDispatchResult RETIRED count = %d, want exactly 1", retiredCount.Load())
	}
	if duplicateCount.Load() != concurrentCallers-1 {
		t.Fatalf(
			"concurrent AcceptDispatchResult DUPLICATE_IDEMPOTENT count = %d, want %d",
			duplicateCount.Load(), concurrentCallers-1,
		)
	}

	boundary.results <- completedWorkersResult(request)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCanonicalResultProgression(t, runtime, ledger)
}

// TestFactoryImpl_UnknownCallbackIdentityRejectedWithoutMutatingKnownDispatch
// proves that a callback whose dispatch identity does not match any recorded
// plan is rejected through the existing typed Runtime boundary and cannot
// mutate the state of a real, known dispatch.
func TestFactoryImpl_UnknownCallbackIdentityRejectedWithoutMutatingKnownDispatch(t *testing.T) {
	boundary := &testWorkstationBoundary{}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "known-dispatch-1",
		CorrelationID:   "known-corr-1",
		WorkIDs:         []string{"known-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/known-trace/known-work-1",
	}
	planned, err := impl.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}

	_, err = impl.AcceptDispatchResult(t.Context(), factory.AcceptDispatchResultRequest{
		DispatchID: "unknown-dispatch-does-not-exist", CorrelationID: "unknown-corr-does-not-exist",
		WorkID: "unknown-work-does-not-exist", ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireRootErrIs(t, err, factory.ErrUnknownDispatchCorrelation, "AcceptDispatchResult(unknown callback identity)")

	intent, ok := impl.dispatchPlan.Intent(plan.DispatchID)
	if !ok || intent.Result == nil {
		t.Fatalf("known dispatch intent = (%#v, %t), want an already-resolved terminal result", intent, ok)
	}

	stillKnown, err := impl.AcceptDispatchResult(t.Context(), factory.AcceptDispatchResultRequest{
		DispatchID: plan.DispatchID, CorrelationID: plan.CorrelationID,
		WorkID: "known-work-1", ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult(known, after unknown callback)")
	if stillKnown.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf(
			"known dispatch outcome after unrelated unknown callback = %q, want DUPLICATE_IDEMPOTENT (unmutated)",
			stillKnown.Outcome,
		)
	}
}
