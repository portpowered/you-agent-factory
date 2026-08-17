package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/recordingfixtures"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// recordingRootExecutionService records the detached request at the Workers
// root so the boundary test proves Runtime calls Service.Execute directly.
type recordingRootExecutionService struct {
	*testWorkstationBoundary
	calls       atomic.Int32
	lastRequest atomic.Value
}

func (service *recordingRootExecutionService) Execute(
	_ context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	service.calls.Add(1)
	service.lastRequest.Store(request)
	return workers.ExecuteResult{
		Correlation: request.Correlation,
		Outcome:     workers.ExecutionOutcomeAccepted,
		Output: workers.ProposedOutput{
			Primary: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "workers-root-boundary",
			}},
		},
	}, nil
}

// recordingRootBoundaryExecutor remains for the pool-boundary compatibility
// test below; Runtime's migrated path does not call this legacy executor.
type recordingRootBoundaryExecutor struct {
	calls          atomic.Int32
	lastDispatchID atomic.Value
}

func (executor *recordingRootBoundaryExecutor) Execute(
	_ context.Context,
	dispatch work.WorkDispatch,
) (workers.WorkResult, error) {
	executor.calls.Add(1)
	executor.lastDispatchID.Store(dispatch.DispatchID)
	return workers.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workers.OutcomeAccepted,
		Output:       "workers-root-boundary",
	}, nil
}

// TestFactoryImpl_PlanDispatchExecutesThroughStatelessWorkers proves a
// Runtime-root PlanDispatch publication reaches the detached Workers Execute
// boundary without Runtime invoking a WorkerExecutor directly.
func TestFactoryImpl_PlanDispatchExecutesThroughWorkersRootBoundary(t *testing.T) {
	service := &recordingRootExecutionService{
		testWorkstationBoundary: &testWorkstationBoundary{},
	}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(service),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")

	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.state = interfaces.FactoryStateRunning
	ctx := context.Background()
	plan := factory.PlanDispatchRequest{
		DispatchID:      "boundary-dispatch-1",
		CorrelationID:   "boundary-corr-1",
		WorkIDs:         []string{"boundary-work-1"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/boundary-trace/boundary-work-1",
	}

	planned, err := impl.PlanDispatch(ctx, plan)
	requireNoRootErr(t, err, "PlanDispatch")
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
	if service.calls.Load() != 1 {
		t.Fatalf(
			"Workers Execute calls = %d, want 1 through Workers root boundary",
			service.calls.Load(),
		)
	}
	request, ok := service.lastRequest.Load().(workers.ExecuteRequest)
	if !ok {
		t.Fatal("Workers Execute request was not recorded")
	}
	if request.Correlation.DispatchID != plan.DispatchID {
		t.Fatalf("executed dispatch correlation = %q, want %q", request.Correlation.DispatchID, plan.DispatchID)
	}
	if request.Target.WorkerType != plan.WorkerType {
		t.Fatalf("executed worker type = %q, want %q", request.Target.WorkerType, plan.WorkerType)
	}
	if request.Target.WorkstationName != plan.WorkstationName {
		t.Fatalf("executed workstation = %q, want %q", request.Target.WorkstationName, plan.WorkstationName)
	}

	accepted, err := impl.AcceptDispatchResult(ctx, factory.AcceptDispatchResultRequest{
		DispatchID:    plan.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        "boundary-work-1",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult")
	if accepted.Outcome != factory.DispatchPlanOutcomeDuplicateIdempotent {
		t.Fatalf(
			"AcceptDispatchResult outcome = %q, want DUPLICATE_IDEMPOTENT after Workers completion",
			accepted.Outcome,
		)
	}
}

// TestFactoryImpl_PlannedDispatchAcceptsWorkersResultThroughRuntimeRoot proves
// a planned dispatch can be result-accepted across the Runtime/Workers boundary
// using Runtime-root AcceptDispatchResult after Workers-root execution.
func TestFactoryImpl_PlannedDispatchAcceptsWorkersResultThroughRuntimeRoot(t *testing.T) {
	boundary := newControlledWorkstationBoundary()
	executor := &countingWorkerExecutor{}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")

	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "planned-result-dispatch",
		CorrelationID:   "planned-result-corr",
		WorkIDs:         []string{"planned-result-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/planned-result-trace/planned-result-work",
	}

	plannedCh := make(chan factory.PlanDispatchResult, 1)
	planErrCh := make(chan error, 1)
	go func() {
		planned, err := impl.PlanDispatch(t.Context(), plan)
		plannedCh <- planned
		planErrCh <- err
	}()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	if executor.calls.Load() != 0 {
		t.Fatalf(
			"Runtime invoked executor %d times outside Workers boundary",
			executor.calls.Load(),
		)
	}

	accepted, err := impl.AcceptDispatchResult(t.Context(), factory.AcceptDispatchResultRequest{
		DispatchID:    plan.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        "planned-result-work",
		ResultOutcome: factory.DispatchResultOutcomeSuccess,
	})
	requireNoRootErr(t, err, "AcceptDispatchResult")
	if accepted.Outcome != factory.DispatchPlanOutcomeRetired {
		t.Fatalf("AcceptDispatchResult outcome = %q, want RETIRED", accepted.Outcome)
	}

	boundary.results <- completedWorkersResult(request)
	requireNoRootErr(t, <-planErrCh, "PlanDispatch")
	planned := <-plannedCh
	if planned.Outcome != factory.DispatchPlanOutcomeAccepted {
		t.Fatalf("PlanDispatch outcome = %q, want ACCEPTED", planned.Outcome)
	}
}

// TestWorkersServiceExecutesRuntimePlannedRequest proves the injected Workers
// service executes a request shaped like Runtime planning publishes without
// importing nested implementation packages or assembling a pool in Runtime.
func TestWorkersServiceExecutesRuntimePlannedRequest(t *testing.T) {
	executor := &recordingRootBoundaryExecutor{}
	service := &testWorkstationBoundary{executors: map[string]workers.WorkerExecutor{"mock": executor}}

	dispatch := work.WorkDispatch{
		DispatchID:      "workers-root-dispatch",
		TransitionID:    "t-process",
		WorkerType:      "mock",
		WorkstationName: "t-process",
		Execution: work.ExecutionMetadata{
			WorkIDs:   []string{"workers-root-work"},
			ReplayKey: "t-process/workers-root-trace/workers-root-work",
		},
	}
	request := workers.WorkstationDispatchRequest{
		WorkstationName: "Process",
		Execution: workers.WorkstationExecutionRequest{
			WorkerType:       "mock",
			Dispatch:         dispatch,
			FactorySessionID: "~default",
		},
	}

	result, err := service.Execute(t.Context(), testExecuteRequestFromDispatch(request))
	requireNoRootErr(t, err, "Execute")
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("execution outcome = %q, want ACCEPTED", result.Outcome)
	}
	if executor.calls.Load() != 1 {
		t.Fatalf("executor calls = %d, want 1 through Workers execution service", executor.calls.Load())
	}
	lastDispatchID, _ := executor.lastDispatchID.Load().(string)
	if lastDispatchID != dispatch.DispatchID {
		t.Fatalf("executed dispatch ID = %q, want %q", lastDispatchID, dispatch.DispatchID)
	}
}

// TestFactoryImpl_ConcurrentDuplicateCompletionResolvesExactlyOnceForDirectAndChildDispatch
// proves duplicate terminal delivery for one in-flight dispatch resolves to
// exactly one accepted Runtime terminal result for both dispatch origins: a
// Runtime-root PlanDispatch (direct) and a scheduler-originated Factory
// dispatch (child). Every losing concurrent caller observes the deterministic
// DUPLICATE_IDEMPOTENT outcome, the dispatch records one Worker Session
// association and no duplicate canonical response once the Workers callback
// that follows is released, nothing is left in flight, and both origins map the
// delivered result to the identical canonical terminal outcome.
func TestFactoryImpl_ConcurrentDuplicateCompletionResolvesExactlyOnceForDirectAndChildDispatch(t *testing.T) {
	tests := []struct {
		name     string
		deliver  func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult
		accepted factory.DispatchResultOutcome
		want     dispatchplanning.TerminalResultOutcome
	}{
		{"success", completedWorkersResult, factory.DispatchResultOutcomeSuccess, dispatchplanning.TerminalResultOutcomeSuccess},
		{"failure", failedWorkersResult, factory.DispatchResultOutcomeFailure, dispatchplanning.TerminalResultOutcomeFailure},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			direct := concurrentDirectDispatchCompletion(t, tc.deliver, tc.accepted)
			child := concurrentChildDispatchCompletion(t, tc.deliver, tc.accepted)
			if direct != tc.want || child != tc.want || direct != child {
				t.Fatalf(
					"terminal outcomes under duplicate completion = direct:%q child:%q, want both %q",
					direct, child, tc.want,
				)
			}
		})
	}
}

// concurrentDirectDispatchCompletion parks a Runtime-root PlanDispatch inside
// the Workers boundary, races duplicate terminal acceptance against it, then
// releases the real Workers callback as a late duplicate.
func concurrentDirectDispatchCompletion(
	t *testing.T,
	deliver func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult,
	accepted factory.DispatchResultOutcome,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	boundary := newControlledWorkstationBoundary()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "concurrent-direct-completion"}
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withInlineDispatch(),
		withWorkerService(boundary),
		withFactoryEventHistory(ledger),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	impl.state = interfaces.FactoryStateRunning

	plan := factory.PlanDispatchRequest{
		DispatchID:      "concurrent-direct-" + t.Name(),
		CorrelationID:   "concurrent-direct-corr-" + t.Name(),
		WorkIDs:         []string{"concurrent-direct-work"},
		WorkstationName: "t-process",
		WorkerType:      "mock",
		ReplayKey:       "t-process/concurrent-direct-trace/concurrent-direct-work",
	}
	planErrCh := make(chan error, 1)
	go func() {
		_, planErr := impl.PlanDispatch(t.Context(), plan)
		planErrCh <- planErr
	}()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	raceDuplicateTerminalAcceptance(t, impl, factory.AcceptDispatchResultRequest{
		DispatchID:    request.Execution.Dispatch.DispatchID,
		CorrelationID: plan.CorrelationID,
		WorkID:        plan.WorkIDs[0],
		ResultOutcome: accepted,
	})
	boundary.results <- deliver(request)
	requireNoRootErr(t, <-planErrCh, "PlanDispatch")

	requireSingleCanonicalDispatchRecord(t, impl, ledger)
	return recordedTerminalOutcome(t, impl, plan.DispatchID)
}

// concurrentChildDispatchCompletion runs the same duplicate-completion race
// against a scheduler-originated Factory child dispatch.
func concurrentChildDispatchCompletion(
	t *testing.T,
	deliver func(workers.WorkstationDispatchRequest) workers.WorkstationDispatchResult,
	accepted factory.DispatchResultOutcome,
) dispatchplanning.TerminalResultOutcome {
	t.Helper()
	boundary := newControlledWorkstationBoundary()
	ledger := &recordingfixtures.ScriptedRuntimeLedger{GenerationID: "concurrent-child-completion"}
	runtime, err := newTestFactory(
		withNet(buildSimpleNetWithFailureArc()),
		withWorkerService(boundary),
		withFactoryEventHistory(ledger),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")
	impl := runtime.(*factoryImpl)
	workID := "concurrent-child-work-" + t.Name()
	if _, err := submitWorkRequests(t.Context(), runtime, []work.SubmitRequest{{
		WorkID: workID, WorkTypeID: "task", TraceID: "concurrent-child-trace-" + t.Name(),
	}}); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- runtime.Run(t.Context()) }()

	request := awaitCanonicalWorkersRequest(t, boundary.requests)
	raceDuplicateTerminalAcceptance(t, impl, factory.AcceptDispatchResultRequest{
		DispatchID:    request.Execution.Dispatch.DispatchID,
		CorrelationID: request.Execution.Dispatch.DispatchID,
		WorkID:        workID,
		ResultOutcome: accepted,
	})
	boundary.results <- deliver(request)
	if err := <-runDone; err != nil {
		t.Fatalf("Run: %v", err)
	}

	requireSingleCanonicalDispatchRecord(t, impl, ledger)
	return recordedTerminalOutcome(t, impl, request.Execution.Dispatch.DispatchID)
}

// raceDuplicateTerminalAcceptance releases concurrentTerminalCallers duplicate
// AcceptDispatchResult callers together and requires exactly one accepted
// retirement with every other caller deterministically DUPLICATE_IDEMPOTENT.
func raceDuplicateTerminalAcceptance(t *testing.T, impl *factoryImpl, terminal factory.AcceptDispatchResultRequest) {
	t.Helper()
	const concurrentTerminalCallers = 8
	var retired, duplicate, failed atomic.Int32
	var start, done sync.WaitGroup
	start.Add(1)
	done.Add(concurrentTerminalCallers)
	for range concurrentTerminalCallers {
		go func() {
			defer done.Done()
			start.Wait()
			result, err := impl.AcceptDispatchResult(t.Context(), terminal)
			switch {
			case err != nil:
				failed.Add(1)
			case result.Outcome == factory.DispatchPlanOutcomeRetired:
				retired.Add(1)
			case result.Outcome == factory.DispatchPlanOutcomeDuplicateIdempotent:
				duplicate.Add(1)
			}
		}()
	}
	start.Done()
	done.Wait()

	if failed.Load() != 0 || retired.Load() != 1 || duplicate.Load() != concurrentTerminalCallers-1 {
		t.Fatalf(
			"duplicate terminal acceptance outcomes = retired:%d duplicate:%d errors:%d, want 1, %d and 0",
			retired.Load(), duplicate.Load(), failed.Load(), concurrentTerminalCallers-1,
		)
	}
}

// requireSingleCanonicalDispatchRecord requires the raced dispatch to have
// recorded exactly one Worker Session association and at most one canonical
// workstation response, and to leave nothing in flight -- so a duplicate
// acceptance that leaked a second terminal fact or restarted the dispatch would
// be observable in recorded history rather than only in planner bookkeeping.
func requireSingleCanonicalDispatchRecord(
	t *testing.T,
	impl *factoryImpl,
	ledger *recordingfixtures.ScriptedRuntimeLedger,
) {
	t.Helper()
	if associations := ledger.CallCount("RecordDispatchWorkerSessionAssociation"); associations != 1 {
		t.Fatalf("dispatch/Worker Session association count = %d, want exactly 1", associations)
	}
	if responses := ledger.CallCount("RecordWorkstationResponse"); responses > 1 {
		t.Fatalf("canonical workstation response count = %d, want at most 1", responses)
	}
	observed, err := impl.Observe(t.Context(), factory.ObserveRequest{Scope: factory.ObservationScopeFull})
	requireNoRootErr(t, err, "Observe")
	if observed.Observation.Progress.InFlightDispatchCount != 0 {
		t.Fatalf(
			"in-flight dispatch count after terminal acceptance = %d, want 0",
			observed.Observation.Progress.InFlightDispatchCount,
		)
	}
}
