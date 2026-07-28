package workmcp

import (
	"context"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

// MoveInput is the MCP request shape for you.work.move.
type MoveInput struct {
	SessionID string `json:"sessionId"`
	WorkID    string `json:"workId"`
	StateName string `json:"stateName"`
	RequestID string `json:"requestId"`
}

// Move applies one operator move through the you.work.move MCP tool.
func Move(
	ctx context.Context,
	service work.Service,
	input MoveInput,
) ToolResponse[work.OperatorMoveResult] {
	if response, done := requestContextErrorResponse[work.OperatorMoveResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[work.OperatorMoveResult]{Error: &envelope}
	}
	result, err := service.MoveWorkForSession(
		ctx,
		input.SessionID,
		input.WorkID,
		input.StateName,
		input.RequestID,
	)
	if err != nil {
		envelope := stateAccessErrorEnvelope(err)
		return ToolResponse[work.OperatorMoveResult]{Error: &envelope}
	}
	return ToolResponse[work.OperatorMoveResult]{Result: &result}
}
