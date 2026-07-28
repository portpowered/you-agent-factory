package http

import (
	"context"
)

// Execute decodes one execute HTTP request, invokes the accepted Providers root,
// and encodes the adapter-owned success response shape.
func (a *Adapter) Execute(ctx context.Context, input ExecuteInput) (ExecuteResponse, error) {
	request, err := ExecuteRequestFromHTTP(input)
	if err != nil {
		return ExecuteResponse{}, err
	}
	result, err := a.invokeExecute(ctx, request)
	if err != nil {
		return ExecuteResponse{}, err
	}
	return ExecuteResponseToHTTP(result), nil
}
