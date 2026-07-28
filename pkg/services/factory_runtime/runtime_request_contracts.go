package factory

import (
	"fmt"
	"strings"
)

// Runtime-owned request construction published at the Factory Runtime root.
// Recordings consumer edges use these helpers to build Runtime control,
// observation, and dispatch-plan requests without importing nested Runtime
// implementation packages.

// ObservationScopeRequest constructs the Runtime root observation request for
// one peer-selected scope. Empty scope means ObservationScopeFull.
func ObservationScopeRequest(scope ObservationScope) ObserveRequest {
	return ObserveRequest{Scope: scope}
}

// PauseControlRequest constructs the Runtime root pause control request.
func PauseControlRequest() PauseRequest {
	return PauseRequest{}
}

// PlanDispatchIntent carries the neutral dispatch-plan fields Recordings replay
// edges hand to Runtime through the root contract.
type PlanDispatchIntent struct {
	DispatchID      string
	CorrelationID   string
	WorkIDs         []string
	WorkstationName string
	WorkerType      string
	ReplayKey       string
}

// PlanDispatchRequestFromIntent constructs the Runtime root dispatch-plan
// request from neutral peer intent fields.
func PlanDispatchRequestFromIntent(intent PlanDispatchIntent) (PlanDispatchRequest, error) {
	if strings.TrimSpace(intent.DispatchID) == "" {
		return PlanDispatchRequest{}, fmt.Errorf("dispatch id is required")
	}
	if strings.TrimSpace(intent.CorrelationID) == "" {
		return PlanDispatchRequest{}, fmt.Errorf("correlation id is required")
	}
	if strings.TrimSpace(intent.WorkstationName) == "" {
		return PlanDispatchRequest{}, fmt.Errorf("workstation name is required")
	}
	if strings.TrimSpace(intent.WorkerType) == "" {
		return PlanDispatchRequest{}, fmt.Errorf("worker type is required")
	}
	if strings.TrimSpace(intent.ReplayKey) == "" {
		return PlanDispatchRequest{}, fmt.Errorf("replay key is required")
	}
	if len(intent.WorkIDs) == 0 {
		return PlanDispatchRequest{}, fmt.Errorf("work ids must contain at least one Work identifier")
	}
	workIDs := make([]string, 0, len(intent.WorkIDs))
	for i, workID := range intent.WorkIDs {
		if strings.TrimSpace(workID) == "" {
			return PlanDispatchRequest{}, fmt.Errorf("work ids[%d] must not be empty", i)
		}
		workIDs = append(workIDs, workID)
	}
	return PlanDispatchRequest{
		DispatchID:      intent.DispatchID,
		CorrelationID:   intent.CorrelationID,
		WorkIDs:         workIDs,
		WorkstationName: intent.WorkstationName,
		WorkerType:      intent.WorkerType,
		ReplayKey:       intent.ReplayKey,
	}, nil
}
