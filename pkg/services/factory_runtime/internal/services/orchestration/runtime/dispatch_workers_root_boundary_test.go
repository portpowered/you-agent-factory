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
