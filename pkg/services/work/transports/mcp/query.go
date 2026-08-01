package workmcp

import (
	"context"

	work "github.com/portpowered/infinite-you/pkg/services/work"
)

// ListInput is the MCP request shape for you.work.list.
type ListInput struct {
	SessionID    string `json:"sessionId"`
	StateName    string `json:"stateName,omitempty"`
	StateType    string `json:"stateType,omitempty"`
	Name         string `json:"name,omitempty"`
	WorkTypeName string `json:"workTypeName,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
	SortBy       string `json:"sortBy,omitempty"`
	MaxResults   int    `json:"maxResults,omitempty"`
	NextToken    string `json:"nextToken,omitempty"`
}

func (input ListInput) listOptions() work.ListOptions {
	return work.ListOptions{
		StateName:    input.StateName,
		StateType:    input.StateType,
		Name:         input.Name,
		WorkTypeName: input.WorkTypeName,
		TraceID:      input.TraceID,
		SortBy:       input.SortBy,
		MaxResults:   input.MaxResults,
		NextToken:    input.NextToken,
	}
}

// List returns detached Work list results through the you.work.list MCP tool.
func List(
	ctx context.Context,
	service work.Service,
	input ListInput,
) ToolResponse[work.ListResult] {
	if response, done := requestContextErrorResponse[work.ListResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[work.ListResult]{Error: &envelope}
	}
	result, err := service.ListWork(ctx, input.SessionID, input.listOptions())
	if err != nil {
		envelope := stateAccessErrorEnvelope(err)
		return ToolResponse[work.ListResult]{Error: &envelope}
	}
	return ToolResponse[work.ListResult]{Result: &result}
}

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
	if response, done := requestContextErrorResponse[work.ReadModel](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[work.ReadModel]{Error: &envelope}
	}
	result, err := service.GetWork(ctx, input.SessionID, input.WorkID)
	if err != nil {
		envelope := stateAccessErrorEnvelope(err)
		return ToolResponse[work.ReadModel]{Error: &envelope}
	}
	return ToolResponse[work.ReadModel]{Result: &result}
}
