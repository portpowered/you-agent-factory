package runtime

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/workstationpool"
)

// recordingRootBoundaryExecutor records Workers-root executor invocations so
// boundary tests can prove Runtime does not invoke executors directly.
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

func requireWorkersRootPoolBoundary(t *testing.T, impl *factoryImpl) workstationpool.WorkstationPoolBoundary {
	t.Helper()
	if impl.workers == nil {
		t.Fatal("production runtime workers boundary is nil; pool execution regressed into Runtime")
	}
	boundary, ok := impl.workers.(workstationpool.WorkstationPoolBoundary)
	if !ok {
		t.Fatalf("workers = %T, want workstationpool.WorkstationPoolBoundary", impl.workers)
	}
	return boundary
}

// TestFactoryImpl_PlanDispatchExecutesThroughWorkersRootBoundary proves a
// Runtime-root PlanDispatch publication is executed through the Workers root
// WorkstationPoolBoundary without Runtime invoking Workers executors directly.
func TestFactoryImpl_PlanDispatchExecutesThroughWorkersRootBoundary(t *testing.T) {
	executor := &recordingRootBoundaryExecutor{}
	runtime, err := newTestFactory(
		withNet(buildSimpleNet()),
		withInlineDispatch(),
		withWorkerExecutor("mock", executor),
		withLogger(logging.NoopLogger{}),
	)
	requireNoRootErr(t, err, "New")

	impl, ok := runtime.(*factoryImpl)
	if !ok {
		t.Fatalf("factory type = %T, want *factoryImpl", runtime)
	}
	requireWorkersRootPoolBoundary(t, impl)

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
	if executor.calls.Load() != 1 {
		t.Fatalf(
			"Workers executor calls = %d, want 1 through Workers root boundary",
			executor.calls.Load(),
		)
	}
	lastDispatchID, _ := executor.lastDispatchID.Load().(string)
	if lastDispatchID != plan.DispatchID {
		t.Fatalf("executed dispatch ID = %q, want %q", lastDispatchID, plan.DispatchID)
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
	requireWorkersRootPoolBoundary(t, impl)
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

// TestWorkersRootPoolBoundaryAdmitsRuntimePlannedDispatchRequest proves the
// Workers root pool boundary executes a dispatch request shaped like Runtime
// planning publishes without importing nested Workers implementation packages.
func TestWorkersRootPoolBoundaryAdmitsRuntimePlannedDispatchRequest(t *testing.T) {
	executor := &recordingRootBoundaryExecutor{}
	service := &testWorkstationBoundary{}
	boundary := workstationpool.NewWorkstationPoolBoundary(workstationpool.WorkstationPoolBoundaryConfig{
		Service:    service,
		Executors:  map[string]workers.WorkerExecutor{"mock": executor},
		RouteNames: []string{"Process", "mock", "t-process"},
		Async:      false,
	})

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
			WorkerType: "mock",
			Dispatch:   dispatch,
			FactorySessionID: "~default",
		},
	}

	done := make(chan workers.WorkstationDispatchResult, 1)
	err := boundary.Publish(t.Context(), request, func(
		_ context.Context,
		_ workers.WorkstationDispatchRequest,
		result workers.WorkstationDispatchResult,
		publishErr error,
	) {
		if publishErr != nil {
			t.Errorf("Workers root publish callback error = %v", publishErr)
			return
		}
		done <- result
	})
	requireNoRootErr(t, err, "Publish")
	result := <-done
	if result.TerminalOutcome != workers.WorkstationDispatchTerminalOutcomeCompleted {
		t.Fatalf("terminal outcome = %q, want COMPLETED", result.TerminalOutcome)
	}
	if executor.calls.Load() != 1 {
		t.Fatalf("executor calls = %d, want 1 through Workers root boundary", executor.calls.Load())
	}
	lastDispatchID, _ := executor.lastDispatchID.Load().(string)
	if lastDispatchID != dispatch.DispatchID {
		t.Fatalf("executed dispatch ID = %q, want %q", lastDispatchID, dispatch.DispatchID)
	}
}
