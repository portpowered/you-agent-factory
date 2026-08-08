package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	dispatchplanningwire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning/wire"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingswire "github.com/portpowered/infinite-you/pkg/services/recordings/wire"
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

func (s *countingWorkerSessionsService) StreamObservations(
	ctx context.Context, req workersessions.StreamObservationsRequest,
) (workersessions.ObservationSubscription, error) {
	return s.inner.StreamObservations(ctx, req)
}

func (s *countingWorkerSessionsService) ReadTranscript(
	ctx context.Context, req workersessions.ReadTranscriptRequest,
) (workersessions.ReadTranscriptResult, error) {
	return s.inner.ReadTranscript(ctx, req)
}

func (s *countingWorkerSessionsService) InvokeSession(
	ctx context.Context, req workersessions.InvokeSessionRequest,
) (workersessions.InvokeSessionResult, error) {
	s.mu.Lock()
	s.startCallsByID[req.ID]++
	s.mu.Unlock()
	return s.inner.InvokeSession(ctx, req)
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

// TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalReplay
// holds a Worker Session terminal callback and an explicit Runtime-root result
// acceptance behind one channel barrier. The contenders are released together,
// then the resulting canonical history is serialized and reloaded into a new
// Recordings ledger and projection. This proves the W4 cutover has one terminal
// effect even when the callback and an explicit delivery contend concurrently,
// and that the retained canonical facts—not the live Runtime object—are enough
// to preserve its identity and Work lineage after replay.
func TestFactoryImpl_WorkerSessionCompletionRacesExplicitAcceptanceAndCanonicalReplay(t *testing.T) {
	recordedAt := time.Date(2026, time.August, 4, 15, 0, 0, 0, time.UTC)
	liveLedger := recordingswire.NewRuntimeLedger(
		nil,
		func() time.Time { return recordedAt },
		"w4-terminal-race-live",
		nil,
	)
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

	assertTerminalRaceLiveState(t, runtime, liveLedger, request, terminal)
	replayed := reloadCanonicalRuntimeLedger(t, liveLedger.CanonicalEvents(), recordedAt, terminal.DispatchID)
	assertTerminalRaceReplayState(t, replayed, request, terminal)
	replayedRuntime := reconstructTerminalReplayAuthority(t, replayed)

	eventsBeforeDuplicate := replayed.ledger.CanonicalEvents()
	projectionBeforeDuplicate := replayed.projection
	duplicate, err := replayedRuntime.AcceptDispatchResult(t.Context(), replayed.terminal)
	requireNoRootErr(t, err, "AcceptDispatchResult(after replay)")
	if duplicate.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf("terminal redelivery outcome = %q, want DUPLICATE_IDEMPOTENT", duplicate.Outcome)
	}
	if eventsAfterDuplicate := replayed.ledger.CanonicalEvents(); !reflect.DeepEqual(eventsBeforeDuplicate, eventsAfterDuplicate) {
		t.Fatalf("replay terminal redelivery changed canonical events:\n before=%#v\n after=%#v", eventsBeforeDuplicate, eventsAfterDuplicate)
	}
	projectionAfterDuplicate, err := recordingswire.NewProjectionService().ReconstructFactoryWorldState(
		replayed.ledger.CanonicalEvents(),
		maxCanonicalTick(replayed.ledger.CanonicalEvents()),
	)
	requireNoRootErr(t, err, "reconstruct replay projection after duplicate")
	if !reflect.DeepEqual(projectionBeforeDuplicate, projectionAfterDuplicate) {
		t.Fatalf("replay terminal redelivery changed reconstructed projection:\n before=%#v\n after=%#v", projectionBeforeDuplicate, projectionAfterDuplicate)
	}
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
	runtime factory.Factory,
	ledger interface {
		CanonicalEvents() []interfaces.FactoryEvent
	},
	request workers.WorkstationDispatchRequest,
	terminal factory.AcceptDispatchResultRequest,
) {
	t.Helper()
	impl := runtime.(*factoryImpl)
	intent, ok := impl.dispatchPlan.Intent(request.Execution.Dispatch.DispatchID)
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
	observed, err := impl.Observe(t.Context(), factory.ObserveRequest{Scope: factory.ObservationScopeFull})
	requireNoRootErr(t, err, "Observe")
	if len(observed.Observation.Results) != 1 ||
		observed.Observation.Results[0].WorkID != terminal.WorkID ||
		observed.Observation.Results[0].Outcome != "ACCEPTED" {
		t.Fatalf("terminal Runtime observation = %#v, want one accepted result for %q", observed.Observation, terminal.WorkID)
	}
	assertTerminalRaceCanonicalFacts(t, ledger.CanonicalEvents(), request, terminal)
}

func reloadCanonicalRuntimeLedger(
	t *testing.T,
	events []interfaces.FactoryEvent,
	recordedAt time.Time,
	dispatchID string,
) *canonicalTerminalReplay {
	t.Helper()

	// The JSON round trip is the persistence boundary: replay is loaded from
	// detached canonical event data, never by retaining or rereading the live
	// ledger's in-memory events.
	persisted, err := json.Marshal(events)
	requireNoRootErr(t, err, "persist canonical events")
	var loaded []interfaces.FactoryEvent
	requireNoRootErr(t, json.Unmarshal(persisted, &loaded), "load canonical events")

	replayLedger := recordingswire.NewRuntimeLedger(
		nil,
		func() time.Time { return recordedAt.Add(time.Second) },
		"w4-terminal-race-replay",
		nil,
	)
	for _, event := range loaded {
		replayLedger.AppendRecordedEvent(event)
	}
	projection, err := recordingswire.NewProjectionService().ReconstructFactoryWorldState(
		replayLedger.CanonicalEvents(),
		maxCanonicalTick(replayLedger.CanonicalEvents()),
	)
	if err != nil {
		t.Fatalf("reconstruct fresh canonical replay projection: %v", err)
	}
	return &canonicalTerminalReplay{
		ledger:     replayLedger,
		projection: projection,
		terminal:   terminalReplayResultFromCanonicalEvents(t, replayLedger.CanonicalEvents(), dispatchID),
	}
}

type canonicalTerminalReplay struct {
	ledger     recordings.RuntimeEventLedger
	projection interfaces.FactoryWorldState
	terminal   factory.AcceptDispatchResultRequest
}

func assertTerminalRaceReplayState(
	t *testing.T,
	replay *canonicalTerminalReplay,
	request workers.WorkstationDispatchRequest,
	terminal factory.AcceptDispatchResultRequest,
) {
	t.Helper()
	if !reflect.DeepEqual(replay.terminal, terminal) {
		t.Fatalf("replayed terminal result = %#v, want canonical live result %#v", replay.terminal, terminal)
	}
	assertTerminalRaceCanonicalFacts(t, replay.ledger.CanonicalEvents(), request, terminal)
	assertTerminalRaceReplayProjection(t, replay.projection, request, terminal)
}

func assertTerminalRaceReplayProjection(
	t *testing.T,
	projection interfaces.FactoryWorldState,
	request workers.WorkstationDispatchRequest,
	terminal factory.AcceptDispatchResultRequest,
) {
	t.Helper()

	var completion *interfaces.FactoryWorldDispatchCompletion
	for index := range projection.CompletedDispatches {
		candidate := &projection.CompletedDispatches[index]
		if candidate.DispatchID == terminal.DispatchID {
			completion = candidate
			break
		}
	}
	if completion == nil {
		t.Fatalf("replayed projection completed dispatches = %#v, want dispatch %q", projection.CompletedDispatches, terminal.DispatchID)
	}
	if completion.Result.Outcome != "ACCEPTED" || !containsCanonicalWorkID(&completion.WorkItemIDs, terminal.WorkID) {
		t.Fatalf("replayed completion = %#v, want accepted terminal result with Work lineage %q", completion, terminal.WorkID)
	}
	if item, ok := projection.WorkItemsByID[terminal.WorkID]; !ok || item.PlaceID != "task:done" || item.State != "done" {
		t.Fatalf("replayed materialized Work = (%#v, %t), want %q at task:done", item, ok, terminal.WorkID)
	}
	if request.Execution.Dispatch.DispatchID != terminal.DispatchID || terminal.CorrelationID != terminal.DispatchID {
		t.Fatalf("replayed dispatch/correlation identity = (%q, %q), want stable dispatch-derived correlation", request.Execution.Dispatch.DispatchID, terminal.CorrelationID)
	}
}

// reconstructTerminalReplayAuthority builds a new Factory Runtime authority
// from detached canonical replay facts. The full Petri marking is already the
// Recordings projection above; this narrowly restores the Runtime planner's
// terminal tombstone so a new process classifies repeated delivery without
// re-emitting Work or canonical dispatch effects.
func reconstructTerminalReplayAuthority(
	t *testing.T,
	replay *canonicalTerminalReplay,
) *factoryImpl {
	t.Helper()

	plan := terminalReplayPlanFromCanonicalEvents(t, replay.ledger.CanonicalEvents(), replay.terminal)
	readOnlyLedger := replayReadOnlyRuntimeLedger{RuntimeEventLedger: replay.ledger}
	replayedFactory, err := newTestFactory(
		withNet(buildSimpleNet()),
		withWorkerService(&testWorkstationBoundary{}),
		withWorkerExecutor("mock", &passExecutor{}),
		withFactoryEventHistory(readOnlyLedger),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New(replay terminal authority)")
	replayed := replayedFactory.(*factoryImpl)

	// Restoring an already-terminal dispatch must not call Worker Sessions or
	// append another association. A no-op publisher reconstructs the planner's
	// accepted outbox intent from the persisted dispatch request, then Retire
	// restores its persisted terminal tombstone.
	planner := dispatchplanningwire.New(func(context.Context, workers.WorkstationDispatchRequest) error {
		return nil
	}, nil)
	replayed.dispatchPlan = planner
	replayed.dispatchFlow.planner = planner
	planned, err := replayed.PlanDispatch(t.Context(), plan)
	requireNoRootErr(t, err, "PlanDispatch(reconstruct replay terminal authority)")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("replayed PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
	retired, err := planner.Retire(t.Context(), dispatchplanning.TerminalResult{
		DispatchID:    replay.terminal.DispatchID,
		CorrelationID: replay.terminal.CorrelationID,
		WorkID:        replay.terminal.WorkID,
		Outcome:       terminalResultOutcomeForReplay(t, replay.terminal.ResultOutcome),
	})
	requireNoRootErr(t, err, "Retire(reconstruct replay terminal authority)")
	if retired.Outcome != dispatchplanning.RetirementOutcomeRetired {
		t.Fatalf("replayed retirement outcome = %q, want RETIRED", retired.Outcome)
	}
	replayed.state = interfaces.FactoryStateCompleted
	return replayed
}

func terminalReplayResultFromCanonicalEvents(
	t *testing.T,
	events []interfaces.FactoryEvent,
	dispatchID string,
) factory.AcceptDispatchResultRequest {
	t.Helper()

	var workID, workerSessionID string
	var response workers.DispatchResponseEventPayload
	foundResponse := false
	for _, event := range events {
		if event.Context.DispatchID == nil || *event.Context.DispatchID != dispatchID {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest:
			if event.Context.WorkIDs != nil && len(*event.Context.WorkIDs) == 1 {
				workID = (*event.Context.WorkIDs)[0]
			}
		case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			var association interfaces.DispatchWorkerSessionAssociationEventPayload
			requireNoRootErr(t, event.DecodePayload(&association), "decode replay Worker Session association")
			workerSessionID = association.WorkerSessionID
		case interfaces.FactoryEventTypeDispatchResponse:
			requireNoRootErr(t, event.DecodePayload(&response), "decode replay terminal dispatch response")
			foundResponse = true
		}
	}
	if workID == "" || workerSessionID != dispatchID || !foundResponse {
		t.Fatalf(
			"replayed terminal facts = work:%q workerSession:%q response:%t, want canonical dispatch %q",
			workID,
			workerSessionID,
			foundResponse,
			dispatchID,
		)
	}
	return factory.AcceptDispatchResultRequest{
		DispatchID: dispatchID,
		// Scheduler-originated dispatches intentionally use DispatchID as their
		// stable correlation identity. That identity is reconstructed from the
		// canonical dispatch envelope, not retained from the live factory.
		CorrelationID: dispatchID,
		WorkID:        workID,
		ResultOutcome: replayDispatchResultOutcome(t, response.Outcome),
	}
}

func replayDispatchResultOutcome(
	t *testing.T,
	outcome workers.WorkOutcome,
) factory.DispatchResultOutcome {
	t.Helper()
	switch outcome {
	case workers.OutcomeAccepted, workers.OutcomeContinue, workers.OutcomeRejected:
		return factory.DispatchResultOutcomeSuccess
	case workers.OutcomeFailed:
		return factory.DispatchResultOutcomeFailure
	default:
		t.Fatalf("replayed terminal Workers outcome = %q, want terminal outcome", outcome)
		return ""
	}
}

func terminalResultOutcomeForReplay(
	t *testing.T,
	outcome factory.DispatchResultOutcome,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	switch outcome {
	case factory.DispatchResultOutcomeSuccess:
		return dispatchplanning.TerminalResultOutcomeSuccess
	case factory.DispatchResultOutcomeFailure:
		return dispatchplanning.TerminalResultOutcomeFailure
	case factory.DispatchResultOutcomeCancelled:
		return dispatchplanning.TerminalResultOutcomeCancelled
	default:
		t.Fatalf("replayed Runtime outcome = %q, want terminal outcome", outcome)
		return ""
	}
}

func terminalReplayPlanFromCanonicalEvents(
	t *testing.T,
	events []interfaces.FactoryEvent,
	terminal factory.AcceptDispatchResultRequest,
) factory.PlanDispatchRequest {
	t.Helper()

	var request interfaces.DispatchRequestEventPayload
	for _, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchRequest || event.Context.DispatchID == nil ||
			*event.Context.DispatchID != terminal.DispatchID {
			continue
		}
		requireNoRootErr(t, event.DecodePayload(&request), "decode replay dispatch request")
		break
	}
	if request.TransitionID == "" || request.Metadata == nil || request.Metadata.ReplayKey == nil ||
		*request.Metadata.ReplayKey == "" {
		t.Fatalf("replayed dispatch request = %#v, want transition and replay key", request)
	}
	transition := buildSimpleNet().Transitions[request.TransitionID]
	if transition == nil || transition.WorkerType == "" {
		t.Fatalf("replayed transition %q has no Worker type in fresh Factory definition", request.TransitionID)
	}
	return factory.PlanDispatchRequest{
		DispatchID:      terminal.DispatchID,
		CorrelationID:   terminal.CorrelationID,
		WorkIDs:         []string{terminal.WorkID},
		WorkstationName: request.TransitionID,
		WorkerType:      transition.WorkerType,
		ReplayKey:       *request.Metadata.ReplayKey,
	}
}

// replayReadOnlyRuntimeLedger makes construction of the fresh Factory Runtime
// side-effect free. Its embedded real ledger remains the single canonical
// source, while constructor-only recording calls cannot append a second run or
// structure event to the persisted replay history.
type replayReadOnlyRuntimeLedger struct {
	recordings.RuntimeEventLedger
}

func (replayReadOnlyRuntimeLedger) AddEventRecorder(func(interfaces.FactoryEvent)) {}

func (replayReadOnlyRuntimeLedger) RecordRunRequest() {}

func (replayReadOnlyRuntimeLedger) RecordInitialStructure() {}

func (replayReadOnlyRuntimeLedger) RecordSessionLifecycleFromFactoryConfig(
	string,
	*interfaces.FactoryConfig,
	int,
	time.Time,
) {
}

func assertTerminalRaceCanonicalFacts(
	t *testing.T,
	events []interfaces.FactoryEvent,
	request workers.WorkstationDispatchRequest,
	terminal factory.AcceptDispatchResultRequest,
) {
	t.Helper()

	associationCount := 0
	responseCount := 0
	for _, event := range events {
		if event.Context.DispatchID == nil || *event.Context.DispatchID != terminal.DispatchID {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchWorkerSessionAssoc:
			associationCount++
			var payload interfaces.DispatchWorkerSessionAssociationEventPayload
			requireNoRootErr(t, event.DecodePayload(&payload), "decode Worker Session association")
			if payload.WorkerSessionID != terminal.DispatchID {
				t.Fatalf("replayed Worker Session ID = %q, want %q", payload.WorkerSessionID, terminal.DispatchID)
			}
		case interfaces.FactoryEventTypeDispatchResponse:
			responseCount++
			var payload workers.DispatchResponseEventPayload
			requireNoRootErr(t, event.DecodePayload(&payload), "decode terminal dispatch response")
			if payload.Outcome != workers.OutcomeAccepted {
				t.Fatalf("replayed terminal response outcome = %q, want ACCEPTED", payload.Outcome)
			}
			if !containsCanonicalWorkID(event.Context.WorkIDs, terminal.WorkID) {
				t.Fatalf("replayed terminal Work lineage = %#v, want %q", event.Context.WorkIDs, terminal.WorkID)
			}
		}
	}
	if associationCount != 1 || responseCount != 1 {
		t.Fatalf(
			"canonical race facts = associations:%d responses:%d, want exactly one each for dispatch %q",
			associationCount,
			responseCount,
			request.Execution.Dispatch.DispatchID,
		)
	}
}

func containsCanonicalWorkID(workIDs *[]string, want string) bool {
	if workIDs == nil {
		return false
	}
	for _, workID := range *workIDs {
		if workID == want {
			return true
		}
	}
	return false
}

func maxCanonicalTick(events []interfaces.FactoryEvent) int {
	maxTick := 0
	for _, event := range events {
		if event.Context.Tick > maxTick {
			maxTick = event.Context.Tick
		}
	}
	return maxTick
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
