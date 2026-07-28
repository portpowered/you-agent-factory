package mcp

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// PlanDispatchInput is the MCP request shape for you.factory_runtime.plan_dispatch.
type PlanDispatchInput struct {
	DispatchID      string   `json:"dispatchId"`
	CorrelationID   string   `json:"correlationId"`
	WorkIDs         []string `json:"workIds"`
	WorkstationName string   `json:"workstationName"`
	WorkerType      string   `json:"workerType"`
	ReplayKey       string   `json:"replayKey"`
}

// PlanDispatch publishes one dispatch intent through the you.factory_runtime.plan_dispatch
// MCP tool.
func PlanDispatch(
	ctx context.Context,
	runtime factoryruntime.Service,
	input PlanDispatchInput,
) ToolResponse[factoryruntime.PlanDispatchResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryruntime.PlanDispatchResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryruntime.PlanDispatchResult](ctx); done {
		return response
	}
	if runtime == nil {
		envelope := unavailableRuntimeErrorEnvelope()
		return ToolResponse[factoryruntime.PlanDispatchResult]{Error: &envelope}
	}

	request, err := toPlanDispatchRequest(input)
	if err != nil {
		envelope := validationErrorEnvelope(err)
		return ToolResponse[factoryruntime.PlanDispatchResult]{Error: &envelope}
	}

	result, err := runtime.PlanDispatch(ctx, request)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryruntime.PlanDispatchResult]{Error: &envelope}
	}
	return ToolResponse[factoryruntime.PlanDispatchResult]{Result: &result}
}

func toPlanDispatchRequest(input PlanDispatchInput) (factoryruntime.PlanDispatchRequest, error) {
	if strings.TrimSpace(input.DispatchID) == "" {
		return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("dispatchId is required")
	}
	if strings.TrimSpace(input.CorrelationID) == "" {
		return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("correlationId is required")
	}
	if strings.TrimSpace(input.WorkstationName) == "" {
		return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("workstationName is required")
	}
	if strings.TrimSpace(input.WorkerType) == "" {
		return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("workerType is required")
	}
	if strings.TrimSpace(input.ReplayKey) == "" {
		return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("replayKey is required")
	}
	if len(input.WorkIDs) == 0 {
		return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("workIds must contain at least one Work identifier")
	}
	workIDs := make([]string, 0, len(input.WorkIDs))
	for i, workID := range input.WorkIDs {
		if strings.TrimSpace(workID) == "" {
			return factoryruntime.PlanDispatchRequest{}, fmt.Errorf("workIds[%d] must not be empty", i)
		}
		workIDs = append(workIDs, workID)
	}
	return factoryruntime.PlanDispatchRequest{
		DispatchID:      input.DispatchID,
		CorrelationID:   input.CorrelationID,
		WorkIDs:         workIDs,
		WorkstationName: input.WorkstationName,
		WorkerType:      input.WorkerType,
		ReplayKey:       input.ReplayKey,
	}, nil
}
