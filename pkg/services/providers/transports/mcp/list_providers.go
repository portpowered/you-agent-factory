package providersmcp

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ListProvidersInput is the MCP request shape for you.provider.list_providers.
type ListProvidersInput struct{}

// ListProviders returns detached catalog descriptors through the
// you.provider.list_providers MCP tool.
func ListProviders(
	ctx context.Context,
	service providers.Service,
	_ ListProvidersInput,
) ToolResponse[providers.ListProvidersResult] {
	if response, done := requestContextErrorResponse[providers.ListProvidersResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[providers.ListProvidersResult]{Error: &envelope}
	}
	result, err := service.ListProviders(ctx, providers.ListProvidersRequest{})
	if err != nil {
		envelope := executionErrorEnvelope(err)
		return ToolResponse[providers.ListProvidersResult]{Error: &envelope}
	}
	return ToolResponse[providers.ListProvidersResult]{Result: &result}
}
