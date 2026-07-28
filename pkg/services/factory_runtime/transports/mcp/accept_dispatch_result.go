package mcp

import (
	"context"
	"fmt"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

// AcceptDispatchResultInput is the MCP request shape for
// you.factory_runtime.accept_dispatch_result.
type AcceptDispatchResultInput struct {
	DispatchID    string `json:"dispatchId"`
	CorrelationID string `json:"correlationId"`
	WorkID        string `json:"workId"`
	ResultOutcome string `json:"resultOutcome"`
}

// AcceptDispatchResult accepts or retires one correlated worker result through the
// you.factory_runtime.accept_dispatch_result MCP tool.
func AcceptDispatchResult(
	ctx context.Context,
	runtime factoryruntime.Service,
	input AcceptDispatchResultInput,
) ToolResponse[factoryruntime.AcceptDispatchResultResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[factoryruntime.AcceptDispatchResultResult]{Error: &envelope}
	}
	if runtime == nil {
		envelope := unavailableRuntimeErrorEnvelope()
		return ToolResponse[factoryruntime.AcceptDispatchResultResult]{Error: &envelope}
	}

	request, err := toAcceptDispatchResultRequest(input)
	if err != nil {
		envelope := validationErrorEnvelope(err)
		return ToolResponse[factoryruntime.AcceptDispatchResultResult]{Error: &envelope}
	}

	result, err := runtime.AcceptDispatchResult(ctx, request)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[factoryruntime.AcceptDispatchResultResult]{Error: &envelope}
	}
	return ToolResponse[factoryruntime.AcceptDispatchResultResult]{Result: &result}
}

func toAcceptDispatchResultRequest(
	input AcceptDispatchResultInput,
) (factoryruntime.AcceptDispatchResultRequest, error) {
	if strings.TrimSpace(input.DispatchID) == "" {
		return factoryruntime.AcceptDispatchResultRequest{}, fmt.Errorf("dispatchId is required")
	}
	if strings.TrimSpace(input.CorrelationID) == "" {
		return factoryruntime.AcceptDispatchResultRequest{}, fmt.Errorf("correlationId is required")
	}
	if strings.TrimSpace(input.WorkID) == "" {
		return factoryruntime.AcceptDispatchResultRequest{}, fmt.Errorf("workId is required")
	}
	outcome := factoryruntime.DispatchResultOutcome(input.ResultOutcome)
	if !validDispatchResultOutcome(outcome) {
		return factoryruntime.AcceptDispatchResultRequest{}, fmt.Errorf(
			`unsupported Factory Runtime dispatch result outcome %q`,
			input.ResultOutcome,
		)
	}
	return factoryruntime.AcceptDispatchResultRequest{
		DispatchID:    input.DispatchID,
		CorrelationID: input.CorrelationID,
		WorkID:        input.WorkID,
		ResultOutcome: outcome,
	}, nil
}

func validDispatchResultOutcome(outcome factoryruntime.DispatchResultOutcome) bool {
	switch outcome {
	case factoryruntime.DispatchResultOutcomeSuccess,
		factoryruntime.DispatchResultOutcomeFailure,
		factoryruntime.DispatchResultOutcomeCancelled:
		return true
	default:
		return false
	}
}
