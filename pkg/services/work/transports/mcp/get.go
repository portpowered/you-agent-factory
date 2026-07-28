package workmcp

import (
	"context"
	"strings"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

// GetInput is the MCP request shape for you.work.get.
type GetInput struct {
	SessionID string `json:"sessionId"`
	WorkID    string `json:"workId"`
}

// Get returns one detached Work ReadModel through the you.work.get MCP tool.
func Get(
	ctx context.Context,
	service work.Service,
	input GetInput,
) ToolResponse[work.ReadModel] {
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[work.ReadModel]{Error: &envelope}
	}
	result, err := service.GetWork(ctx, input.SessionID, input.WorkID)
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[work.ReadModel]{Error: &envelope}
	}
	return ToolResponse[work.ReadModel]{Result: &result}
}

func executionErrorEnvelope(err error) ToolErrorEnvelope {
	return ToolErrorEnvelope{
		Code:      errorCodeBadRequest,
		Message:   strings.TrimSpace(err.Error()),
		Retryable: false,
		Details: map[string]any{
			"reason": err.Error(),
		},
	}
}
