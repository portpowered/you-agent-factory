package workmcp

import (
	"context"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

// SubmitInput is the MCP request shape for you.work.submit.
type SubmitInput struct {
	SessionID   string           `json:"sessionId"`
	WorkRequest work.WorkRequest `json:"workRequest"`
}

// Submit admits one already-decoded Work Request through the you.work.submit MCP tool.
func Submit(
	ctx context.Context,
	service work.Service,
	input SubmitInput,
) ToolResponse[work.WorkRequestSubmitResult] {
	if response, done := requestContextErrorResponse[work.WorkRequestSubmitResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[work.WorkRequestSubmitResult]{Error: &envelope}
	}
	result, err := service.SubmitWorkRequestForSession(ctx, input.SessionID, input.WorkRequest)
	if err != nil {
		envelope := submitErrorEnvelope(err)
		return ToolResponse[work.WorkRequestSubmitResult]{Error: &envelope}
	}
	return ToolResponse[work.WorkRequestSubmitResult]{Result: &result}
}
