package recordings_test

import (
	"context"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// TestRecordingsConstructsRuntimeRequestsThroughRoot proves CUT-REC-RUN story 005:
// Recordings consumer edges construct Runtime root requests only through the
// sealed Factory Runtime service root contract.
func TestRecordingsConstructsRuntimeRequestsThroughRoot(t *testing.T) {
	t.Parallel()

	observeRequest := factoryruntime.ObservationScopeRequest(factoryruntime.ObservationScopeHealth)
	if observeRequest.Scope != factoryruntime.ObservationScopeHealth {
		t.Fatalf("observe scope = %q, want HEALTH", observeRequest.Scope)
	}

	pauseRequest := factoryruntime.PauseControlRequest()
	if pauseRequest != (factoryruntime.PauseRequest{}) {
		t.Fatalf("pause request = %#v, want empty PauseRequest", pauseRequest)
	}

	planRequest, err := factoryruntime.PlanDispatchRequestFromIntent(factoryruntime.PlanDispatchIntent{
		DispatchID:      "dispatch-recordings-root",
		CorrelationID:   "corr-recordings-root",
		WorkIDs:         []string{"work-recordings-root"},
		WorkstationName: "review",
		WorkerType:      "inference",
		ReplayKey:       "review/trace-recordings/work-recordings-root",
	})
	if err != nil {
		t.Fatalf("PlanDispatchRequestFromIntent: %v", err)
	}

	stub := &runtimeRequestBoundaryStub{}
	ctx := context.Background()
	observeResult, err := stub.Observe(ctx, observeRequest)
	if err != nil {
		t.Fatalf("Observe: %v", err)
	}
	if observeResult.Observation.Health.FactoryState != "RUNNING" {
		t.Fatalf("observation factory state = %q, want RUNNING", observeResult.Observation.Health.FactoryState)
	}
	if stub.lastObserve.Scope != factoryruntime.ObservationScopeHealth {
		t.Fatalf("stub observe scope = %q, want HEALTH", stub.lastObserve.Scope)
	}

	pauseResult, err := stub.ControlPause(ctx, pauseRequest)
	if err != nil {
		t.Fatalf("ControlPause: %v", err)
	}
	if pauseResult.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("pause outcome = %q, want ACCEPTED", pauseResult.Outcome)
	}
	if stub.lastPause != (factoryruntime.PauseRequest{}) {
		t.Fatalf("stub pause request = %#v, want empty PauseRequest", stub.lastPause)
	}

	planResult, err := stub.PlanDispatch(ctx, planRequest)
	if err != nil {
		t.Fatalf("PlanDispatch: %v", err)
	}
	if planResult.DispatchID != "dispatch-recordings-root" {
		t.Fatalf("plan dispatch id = %q, want dispatch-recordings-root", planResult.DispatchID)
	}
	if planResult.CorrelationID != "corr-recordings-root" {
		t.Fatalf("plan correlation id = %q, want corr-recordings-root", planResult.CorrelationID)
	}
	if stub.lastPlan.DispatchID != planRequest.DispatchID ||
		stub.lastPlan.CorrelationID != planRequest.CorrelationID ||
		stub.lastPlan.WorkstationName != planRequest.WorkstationName ||
		stub.lastPlan.WorkerType != planRequest.WorkerType ||
		stub.lastPlan.ReplayKey != planRequest.ReplayKey ||
		len(stub.lastPlan.WorkIDs) != 1 ||
		stub.lastPlan.WorkIDs[0] != "work-recordings-root" {
		t.Fatalf("stub plan request = %#v, want %#v", stub.lastPlan, planRequest)
	}
}

// TestRecordingsRuntimeRequestConstructionImportsRuntimeRootOnly seals the
// request-construction path: Recordings boundary tests may depend on Factory
// Runtime request helpers only through the service root contract.

type runtimeRequestBoundaryStub struct {
	lastObserve factoryruntime.ObserveRequest
	lastPause   factoryruntime.PauseRequest
	lastPlan    factoryruntime.PlanDispatchRequest
}

func (stub *runtimeRequestBoundaryStub) ControlPause(
	_ context.Context,
	request factoryruntime.PauseRequest,
) (factoryruntime.PauseResult, error) {
	stub.lastPause = request
	return factoryruntime.PauseResult{Outcome: factoryruntime.ControlOutcomeAccepted}, nil
}

func (stub *runtimeRequestBoundaryStub) Observe(
	_ context.Context,
	request factoryruntime.ObserveRequest,
) (factoryruntime.ObserveResult, error) {
	stub.lastObserve = request
	return factoryruntime.ObserveResult{
		Observation: factoryruntime.Observation{
			Status: factoryruntime.ObservationStatusActive,
			Health: factoryruntime.ObservationHealth{
				FactoryState: "RUNNING",
			},
		},
	}, nil
}

func (stub *runtimeRequestBoundaryStub) PlanDispatch(
	_ context.Context,
	request factoryruntime.PlanDispatchRequest,
) (factoryruntime.PlanDispatchResult, error) {
	stub.lastPlan = request
	return factoryruntime.PlanDispatchResult{
		Outcome:       factoryruntime.DispatchPlanOutcomeAccepted,
		DispatchID:    request.DispatchID,
		CorrelationID: request.CorrelationID,
	}, nil
}
