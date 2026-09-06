package runtime

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	factory_context "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/context"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

func (s *countingWorkerSessionsService) ListObservations(
	ctx context.Context, req workersessions.ListObservationsRequest,
) (workersessions.ListObservationsResult, error) {
	return s.inner.ListObservations(ctx, req)
}

func (s *countingWorkerSessionsService) GetObservation(
	ctx context.Context, req workersessions.GetObservationRequest,
) (workersessions.Observation, error) {
	return s.inner.GetObservation(ctx, req)
}

func (s *countingWorkerSessionsService) GetObservationByWorkerSessionID(
	ctx context.Context, req workersessions.GetObservationByWorkerSessionIDRequest,
) (workersessions.Observation, error) {
	return s.inner.GetObservationByWorkerSessionID(ctx, req)
}

func (s *countingWorkerSessionsService) ListWorkerSessionObservations(ctx context.Context, req workersessions.ListWorkerSessionObservationsRequest) (workersessions.ListWorkerSessionObservationsResult, error) {
	return s.inner.ListWorkerSessionObservations(ctx, req)
}

func (s *countingWorkerSessionsService) StreamObservations(
	ctx context.Context, req workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, error) {
	return s.inner.StreamObservations(ctx, req)
}

func (s *countingWorkerSessionsService) StreamObservationsByWorkerSessionID(
	ctx context.Context, req workersessions.StreamObservationsByWorkerSessionIDRequest,
) (workersessions.ObservationSubscription, error) {
	return s.inner.StreamObservationsByWorkerSessionID(ctx, req)
}

func (s *countingWorkerSessionsService) ReadTranscript(
	ctx context.Context, req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	return s.inner.ReadTranscript(ctx, req)
}

func (s *countingWorkerSessionsService) ReadTranscriptByWorkerSessionID(ctx context.Context, req workersessions.ReadTranscriptByWorkerSessionIDRequest) (workersessions.ReadTranscriptResult, error) {
	return s.inner.ReadTranscriptByWorkerSessionID(ctx, req)
}

func (s *countingWorkerSessionsService) InvokeSession(
	ctx context.Context, req workersessions.InvokeSessionRequest,
) (workersessions.InvokeSessionResult, error) {
	s.mu.Lock()
	s.startCallsByID[req.ID]++
	s.mu.Unlock()
	return s.inner.InvokeSession(ctx, req)
}

func (s *countingWorkerSessionsService) Start(
	ctx context.Context, req workersessions.StartRequest,
) (workersessions.StartResult, error) {
	s.mu.Lock()
	s.startCallsByID[req.ID]++
	s.mu.Unlock()
	return s.inner.Start(ctx, req)
}

func (s *countingWorkerSessionsService) Continue(ctx context.Context, req workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	return s.inner.Continue(ctx, req)
}

func (s *countingWorkerSessionsService) Interrupt(ctx context.Context, req workersessions.InterruptRequest) (workersessions.InterruptResult, error) {
	return s.inner.Interrupt(ctx, req)
}

func (s *countingWorkerSessionsService) PublishRecord(
	ctx context.Context, req workersessions.PublishRecordRequest,
) (workersessions.PublishRecordResult, error) {
	return s.inner.PublishRecord(ctx, req)
}

func (s *countingWorkerSessionsService) AssociateProviderSession(
	ctx context.Context, req workersessions.ProviderSessionAssociationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return s.inner.AssociateProviderSession(ctx, req)
}

func (s *countingWorkerSessionsService) ObserveProviderSession(
	ctx context.Context, req workersessions.ProviderSessionObservationRequest,
) (workersessions.ProviderSessionAssociationResult, error) {
	return s.inner.ObserveProviderSession(ctx, req)
}

func (s *countingWorkerSessionsService) EnsureProviderBinding(
	ctx context.Context, req workersessions.ProviderBindingRequest,
) (workersessions.ProviderBindingResult, error) {
	return s.inner.EnsureProviderBinding(ctx, req)
}

func (s *countingWorkerSessionsService) WorkerSessionIDForDispatch(
	ctx context.Context, dispatchID string,
) (string, error) {
	return s.inner.WorkerSessionIDForDispatch(ctx, dispatchID)
}

func (s *countingWorkerSessionsService) Pause(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.inner.Pause(ctx, req)
}

func (s *countingWorkerSessionsService) Resume(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.inner.Resume(ctx, req)
}

func (s *countingWorkerSessionsService) Cancel(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.inner.Cancel(ctx, req)
}

func (s *countingWorkerSessionsService) Terminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return s.inner.Terminate(ctx, req)
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

// TestFactoryImpl_DuplicatePlanDispatchReusesAssociationAndDoesNotRestartWorkers
// proves Runtime's idempotency requirement: repeating an identical planned
// dispatch reuses the original association and does not start a legacy Worker
// Session execution path.
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
	if got := counting.startCallCount(plan.DispatchID); got != 0 {
		t.Fatalf("legacy worker_sessions.Service.Start calls for %q = %d, want 0", plan.DispatchID, got)
	}
	if got := counting.totalStartCalls(); got != 0 {
		t.Fatalf("legacy worker_sessions.Service.Start calls = %d, want 0", got)
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

// TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalIdempotency
// holds a Worker Session terminal callback and an explicit Runtime-root result
// acceptance behind one channel barrier. The contenders are released together
// and the injected Recordings root-contract fake records the single terminal
// association and response. Canonical Recordings replay and projection are
// covered by the owner package's behavioral tests; Runtime owns the concurrent
// terminal/idempotency invariant here.
func TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalIdempotency(t *testing.T) {
	liveLedger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "w4-terminal-race-live"}
	boundary := newControlledWorkstationBoundary()
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withWorkerService(boundary),
		withWorkerExecutor("mock", &passExecutor{}),
		withFactoryEventHistory(liveLedger),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	const workID = "work-terminal-race"
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: workID, WorkTypeID: "task", TraceID: "trace-terminal-race",
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	terminal := factory.AcceptDispatchResultRequest{
		DispatchID:    request.Execution.Dispatch.DispatchID,
		CorrelationID: request.Execution.Dispatch.DispatchID,
		WorkID:        workID,
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	}

	runTerminalAcceptanceRace(t, impl, boundary, request, terminal)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertTerminalRaceLiveState(t, impl, liveLedger, request, terminal)
}

func runTerminalAcceptanceRace(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	request workers.WorkstationDispatchRequest,
	terminal factory.AcceptDispatchResultRequest,
) {
	t.Helper()
	acceptance := newTerminalAcceptanceBarrierPlanner(impl.dispatchPlan)
	impl.dispatchPlan = acceptance
	impl.dispatchFlow.planner = acceptance

	callbackDelivered := make(chan struct{}, 1)
	explicitDelivered := make(chan struct {
		result factory.AcceptDispatchResultResult
		err    error
	}, 1)
	go func() {
		boundary.results <- completedWorkersResult(request)
		callbackDelivered <- struct{}{}
	}()
	go func() {
		result, acceptErr := impl.AcceptDispatchResult(t.Context(), terminal)
		explicitDelivered <- struct {
			result factory.AcceptDispatchResultResult
			err    error
		}{result: result, err: acceptErr}
	}()

	// Both arrivals are observed at Runtime's shared terminal acceptance
	// operation before either completion path may retire the dispatch.
	<-acceptance.arrived
	<-acceptance.arrived
	close(acceptance.release)
	assertTerminalAcceptanceRace(t, []terminalAcceptanceCall{<-acceptance.calls, <-acceptance.calls})

	explicit := <-explicitDelivered
	requireNoRootErr(t, explicit.err, "AcceptDispatchResult(racing)")
	if explicit.result.Outcome != factory.DispatchPlanOutcomeRetired &&
		explicit.result.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("racing AcceptDispatchResult outcome = %q, want RETIRED or DUPLICATE_IDEMPOTENT", explicit.result.Outcome)
	}
	<-callbackDelivered
}

type terminalAcceptanceCall struct {
	result dispatchplanning.RetirementResult
	err    error
}

// terminalAcceptanceBarrierPlanner parks callers immediately before the
// Runtime-owned terminal acceptance operation. It embeds the production
// planner, so after release both contenders execute the real Retire logic.
type terminalAcceptanceBarrierPlanner struct {
	dispatchplanning.Service

	arrived chan struct{}
	release chan struct{}
	calls   chan terminalAcceptanceCall
}

func newTerminalAcceptanceBarrierPlanner(
	inner dispatchplanning.Service,
) *terminalAcceptanceBarrierPlanner {
	return &terminalAcceptanceBarrierPlanner{
		Service: inner,
		arrived: make(chan struct{}, 2),
		release: make(chan struct{}),
		calls:   make(chan terminalAcceptanceCall, 2),
	}
}

func (p *terminalAcceptanceBarrierPlanner) Retire(
	ctx context.Context,
	terminal dispatchplanning.TerminalResult,
) (dispatchplanning.RetirementResult, error) {
	p.arrived <- struct{}{}
	<-p.release
	result, err := p.Service.Retire(ctx, terminal)
	p.calls <- terminalAcceptanceCall{result: result, err: err}
	return result, err
}

func assertTerminalAcceptanceRace(t *testing.T, calls []terminalAcceptanceCall) {
	t.Helper()
	retired := 0
	duplicates := 0
	for _, call := range calls {
		requireNoRootErr(t, call.err, "Retire(racing completion)")
		switch call.result.Outcome {
		case dispatchplanning.RetirementOutcomeRetired:
			retired++
		case dispatchplanning.RetirementOutcomeDuplicateIdempotent:
			duplicates++
		default:
			t.Fatalf("terminal retirement outcome = %q, want RETIRED or DUPLICATE_IDEMPOTENT", call.result.Outcome)
		}
	}
	if retired != 1 || duplicates != 1 {
		t.Fatalf("terminal retirement outcomes = retired:%d duplicates:%d, want exactly one of each", retired, duplicates)
	}
}

func assertTerminalRaceLiveState(
	t *testing.T,
	runtime *factoryImpl,
	ledger *recordingfixtures.ScriptedRuntimeLedger,
	request workers.WorkstationDispatchRequest,
	terminal factory.AcceptDispatchResultRequest,
) {
	t.Helper()
	intent, ok := runtime.dispatchPlan.Intent(request.Execution.Dispatch.DispatchID)
	if !ok || intent.Result == nil {
		t.Fatalf("terminal dispatch intent = (%#v, %t), want one accepted result", intent, ok)
	}
	if intent.Action.CorrelationID != terminal.CorrelationID ||
		intent.Result.CorrelationID != terminal.CorrelationID ||
		intent.Result.WorkID != terminal.WorkID {
		t.Fatalf("terminal intent = %#v, want preserved correlation %q and Work lineage %q", intent, terminal.CorrelationID, terminal.WorkID)
	}
	if intent.Result.Outcome != "SUCCESS" {
		t.Fatalf("terminal outcome = %q, want SUCCESS", intent.Result.Outcome)
	}

	snapshot, err := runtime.GetEngineStateSnapshot(t.Context())
	requireNoRootErr(t, err, "GetEngineStateSnapshot")
	if count := countTokensAtPlace(snapshot, "task:done"); count != 1 {
		t.Fatalf("materialized task:done token count = %d, want exactly 1", count)
	}
	observed, err := runtime.Observe(t.Context(), factory.ObserveRequest{Scope: factory.ObservationScopeFull})
	requireNoRootErr(t, err, "Observe")
	if len(observed.Observation.Results) != 1 ||
		observed.Observation.Results[0].WorkID != terminal.WorkID ||
		observed.Observation.Results[0].Outcome != "ACCEPTED" {
		t.Fatalf("terminal Runtime observation = %#v, want one accepted result for %q", observed.Observation, terminal.WorkID)
	}
	assertTerminalRaceLedgerFacts(t, ledger, terminal.DispatchID)
}

func assertTerminalRaceLedgerFacts(t *testing.T, ledger *recordingfixtures.ScriptedRuntimeLedger, dispatchID string) {
	t.Helper()
	if count := ledger.CallCount("RecordDispatchWorkerSessionAssociation"); count != 1 {
		t.Fatalf("dispatch/Worker Session association call count = %d, want exactly 1", count)
	}
	if count := ledger.CallCount("RecordWorkstationResponse"); count != 1 {
		t.Fatalf("dispatch response call count = %d, want exactly 1", count)
	}
	associations := ledger.DispatchWorkerSessionAssociationsSnapshot()
	if len(associations) != 1 || associations[0].DispatchID != dispatchID || associations[0].WorkerSessionID != dispatchID {
		t.Fatalf("dispatch/Worker Session associations = %#v, want one association for %q", associations, dispatchID)
	}
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

// TestInvokeWorker_ARerunDispatchReachesWorkersUnderItsOwnIdentity pins the
// identity a Worker carries into Workers.
//
// A JavaScript workflow resumed after an interruption re-runs the child that
// was cut off under its original orchestrator-minted dispatch ID. Workers
// treats a dispatch ID as single-use for the whole life of its pool -- an
// accepted dispatch is never removed from the pool's record map -- so a re-run
// that reuses that ID is refused before it reaches an executor, and the caller
// sees START_FAILURE rather than a second Worker.
//
// The Worker Session identity is the one Runtime already mints uniquely, so it
// is the identity Workers is given. What the caller sees is unchanged: its own
// dispatch ID comes back on the result, because that is the identity its own
// records are keyed by.
func TestInvokeWorker_ARerunDispatchReachesWorkersUnderItsOwnIdentity(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)

	firstWorkersID, first := runOneInvokeWorker(t, impl, boundary, "child-1")
	secondWorkersID, second := runOneInvokeWorker(t, impl, boundary, "child-1")

	if secondWorkersID == firstWorkersID {
		t.Fatalf(
			"re-run Workers dispatch ID = %q, want an identity distinct from the first attempt's %q",
			secondWorkersID,
			firstWorkersID,
		)
	}
	if secondWorkersID != second.WorkerSessionID {
		t.Fatalf(
			"re-run Workers dispatch ID = %q, want the reserved Worker Session identity %q",
			secondWorkersID,
			second.WorkerSessionID,
		)
	}
	for _, result := range []factory.InvokeWorkerResult{first, second} {
		if result.DispatchID != "child-1" {
			t.Fatalf("result dispatch ID = %q, want the caller's own %q", result.DispatchID, "child-1")
		}
		if result.Outcome != factory.InvokeWorkerOutcomeCompleted {
			t.Fatalf("result outcome = %q, want COMPLETED", result.Outcome)
		}
	}
}

// TestInvokeWorker_FirstAttemptUsesTheCallerDispatchIdentity keeps the common
// case honest: an uncontended Worker still reaches Workers under exactly the
// identity its caller minted, so the resume suffix above is visibly the
// exception rather than the rule.
func TestInvokeWorker_FirstAttemptUsesTheCallerDispatchIdentity(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)

	workersID, result := runOneInvokeWorker(t, impl, boundary, "child-1")
	if workersID != "child-1" {
		t.Fatalf("Workers dispatch ID = %q, want the caller's own %q", workersID, "child-1")
	}
	if result.WorkerSessionID != "child-1" {
		t.Fatalf("Worker Session ID = %q, want %q", result.WorkerSessionID, "child-1")
	}
}

func TestInvokeWorker_UsesRuntimeRetryBudget(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	impl := newInvokeWorkerTestFactory(t, boundary)
	done := make(chan struct {
		result factory.InvokeWorkerResult
		err    error
	}, 1)
	go func() {
		result, err := impl.InvokeWorker(context.Background(), factory.InvokeWorkerRequest{
			DispatchID:  "child-retry",
			Prompt:      "run",
			MaxAttempts: 2,
		})
		done <- struct {
			result factory.InvokeWorkerResult
			err    error
		}{result: result, err: err}
	}()

	first := awaitCanonicalWorkersRequest(t, boundary.requests)
	if got := first.Execution.Dispatch.DispatchID; got != "child-retry" {
		t.Fatalf("first dispatch ID = %q, want child-retry", got)
	}
	boundary.results <- workers.WorkstationDispatchResult{
		DispatchID:      first.Execution.Dispatch.DispatchID,
		WorkstationName: first.WorkstationName,
		TerminalOutcome: workers.WorkstationDispatchTerminalOutcomeFailed,
		Result: workers.WorkResult{
			DispatchID: first.Execution.Dispatch.DispatchID,
			Outcome:    workers.OutcomeFailed,
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyRetryable,
				Type:   workers.WorkFailureTypeInternalServerError,
			},
		},
	}

	second := awaitCanonicalWorkersRequest(t, boundary.requests)
	if got := second.Execution.Dispatch.DispatchID; got != "child-retry/attempt/2" {
		t.Fatalf("second dispatch ID = %q, want child-retry/attempt/2", got)
	}
	boundary.results <- completedWorkersResult(second)

	got := <-done
	if got.err != nil {
		t.Fatalf("InvokeWorker: %v", got.err)
	}
	if got.result.Outcome != factory.InvokeWorkerOutcomeCompleted || got.result.Attempts != 2 {
		t.Fatalf("InvokeWorker result = %#v, want completed after two attempts", got.result)
	}
}

// TestInvokeWorker_CarriesTheAuthoredWorkerNameAndPermissionPolicy pins the
// two facts a Worker with no authored workstation can only get from its
// caller.
//
// The worker name is what --with-mock-workers matches a named preset on, at
// the subprocess boundary, so a Worker that arrives without it is never the
// mock the operator configured. The permission policy is the invocation
// -effective one the caller already resolved; dropping it runs the Worker
// under a policy its own dispatch record says it does not have.
func TestInvokeWorker_CarriesTheAuthoredWorkerNameAndPermissionPolicy(t *testing.T) {
	for _, test := range []struct {
		name string
		skip bool
	}{
		{name: "true", skip: true},
		{name: "false", skip: false},
		{name: "omitted", skip: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			boundary := newControlledWorkstationBoundary()
			impl := newInvokeWorkerTestFactory(t, boundary)
			capabilities := &workers.Capabilities{NativeStreaming: true, ToolLifecycle: true}

			observed, _ := runInvokeWorker(t, impl, boundary, factory.InvokeWorkerRequest{
				DispatchID:      "child-1",
				Prompt:          "run",
				WorkerName:      "worker-a",
				SkipPermissions: test.skip,
				RecordingID:     "recording-1",
				Capabilities:    capabilities,
			})
			if observed.Execution.WorkerType != "worker-a" {
				t.Fatalf("Workers worker type = %q, want the authored worker name %q", observed.Execution.WorkerType, "worker-a")
			}
			if observed.Execution.Dispatch.WorkerType != "worker-a" {
				t.Fatalf(
					"dispatch worker type = %q, want the authored worker name %q",
					observed.Execution.Dispatch.WorkerType,
					"worker-a",
				)
			}
			if observed.Execution.SkipPermissions != test.skip {
				t.Fatalf("Workers skip-permissions = %v, want %v", observed.Execution.SkipPermissions, test.skip)
			}
			if observed.Execution.RecordingID != "recording-1" {
				t.Fatalf("Workers recording ID = %q, want recording-1", observed.Execution.RecordingID)
			}
			if observed.Execution.Capabilities == nil || !observed.Execution.Capabilities.NativeStreaming || !observed.Execution.Capabilities.ToolLifecycle {
				t.Fatalf("Workers capabilities = %+v, want caller-supplied capability facts", observed.Execution.Capabilities)
			}
		})
	}
}

// runOneInvokeWorker drives one minimal InvokeWorker to its terminal result
// and reports the dispatch identity Workers actually observed.
func runOneInvokeWorker(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	dispatchID string,
) (string, factory.InvokeWorkerResult) {
	t.Helper()
	observed, result := runInvokeWorker(t, impl, boundary, factory.InvokeWorkerRequest{
		DispatchID: dispatchID,
		Prompt:     "run",
	})
	return observed.Execution.Dispatch.DispatchID, result
}

// runInvokeWorker drives one InvokeWorker to its terminal result and reports
// the request Workers actually observed alongside it.
func runInvokeWorker(
	t *testing.T,
	impl *factoryImpl,
	boundary *controlledWorkstationBoundary,
	req factory.InvokeWorkerRequest,
) (workers.WorkstationDispatchRequest, factory.InvokeWorkerResult) {
	t.Helper()
	type outcome struct {
		result factory.InvokeWorkerResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := impl.InvokeWorker(context.Background(), req)
		done <- outcome{result: result, err: err}
	}()
	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	boundary.results <- completedWorkersResult(request)
	got := <-done
	if got.err != nil {
		t.Fatalf("InvokeWorker(%q): %v", req.DispatchID, got.err)
	}
	return request, got.result
}

func TestRecordedWorkerSessionObservationScopesCanonicalEventsToFactorySession(t *testing.T) {
	firstSessionID := "factory-session-first"
	secondSessionID := "factory-session-second"
	events := []interfaces.FactoryEvent{
		{Context: interfaces.FactoryEventContext{SessionID: &firstSessionID, Tick: 1, Sequence: 1}},
		{Context: interfaces.FactoryEventContext{SessionID: &secondSessionID, Tick: 1, Sequence: 2}},
		{Context: interfaces.FactoryEventContext{SessionID: &firstSessionID, Tick: 2, Sequence: 3}},
	}

	service := &recordedWorkerSessionObservation{
		ledger:           &recordingfixtures.ScriptedRuntimeLedger{Events: events},
		factorySessionID: firstSessionID,
	}

	scoped := service.canonicalEvents()
	if len(scoped) != 2 {
		t.Fatalf("canonicalEvents() returned %d events, want two scoped events: %#v", len(scoped), scoped)
	}
	for _, event := range scoped {
		if event.Context.SessionID == nil || *event.Context.SessionID != firstSessionID {
			t.Fatalf("canonicalEvents() returned foreign session event: %#v", event)
		}
	}
}

func TestRecordedWorkerSessionObservationFiltersLiveFleetPageToFactorySession(t *testing.T) {
	page := []workersessions.Observation{
		{WorkerSessionID: "worker-first", FactorySessionID: "factory-session-first"},
		{WorkerSessionID: "worker-second", FactorySessionID: "factory-session-second"},
		{WorkerSessionID: "worker-recorded", FactorySessionID: ""},
	}
	recorded := []workersessions.Observation{{WorkerSessionID: "worker-recorded"}}

	filtered := filterObservationPageForFactorySession(page, "factory-session-first", recorded)
	if got := []string{filtered[0].WorkerSessionID, filtered[1].WorkerSessionID}; !reflect.DeepEqual(got, []string{"worker-first", "worker-recorded"}) {
		t.Fatalf("filtered Worker Session IDs = %v, want first-session and recorded identities", got)
	}

	defaultFiltered := filterObservationPageForFactorySession(
		[]workersessions.Observation{{WorkerSessionID: "worker-direct"}},
		factory_context.DefaultSessionID,
		nil,
	)
	if got := []string{defaultFiltered[0].WorkerSessionID}; !reflect.DeepEqual(got, []string{"worker-direct"}) {
		t.Fatalf("default filtered Worker Session IDs = %v, want direct identity", got)
	}
}
