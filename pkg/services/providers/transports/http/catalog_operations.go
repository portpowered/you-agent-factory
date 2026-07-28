package http

import (
	"context"
)

// ListProviders decodes one list-providers HTTP request, invokes the accepted
// Providers root, and encodes the adapter-owned success response shape.
func (a *Adapter) ListProviders(ctx context.Context) (ListProvidersResponse, error) {
	request := ListProvidersRequestFromHTTP()
	result, err := a.invokeListProviders(ctx, request)
	if err != nil {
		return ListProvidersResponse{}, err
	}
	return ListProvidersResponseToHTTP(result), nil
}

// GetProvider decodes one get-provider HTTP request, invokes the accepted
// Providers root, and encodes the adapter-owned success response shape.
func (a *Adapter) GetProvider(
	ctx context.Context,
	input GetProviderInput,
) (GetProviderResponse, error) {
	request, err := GetProviderRequestFromHTTP(input)
	if err != nil {
		return GetProviderResponse{}, err
	}
	result, err := a.invokeGetProvider(ctx, request)
	if err != nil {
		return GetProviderResponse{}, err
	}
	return GetProviderResponseToHTTP(result), nil
}
