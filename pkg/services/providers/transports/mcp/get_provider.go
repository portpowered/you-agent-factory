package providersmcp

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// GetProviderInput is the MCP request shape for you.provider.get_provider.
type GetProviderInput struct {
	ID string `json:"id"`
}

// GetProvider returns one detached catalog descriptor through the
// you.provider.get_provider MCP tool.
func GetProvider(
	ctx context.Context,
	service providers.Service,
	input GetProviderInput,
) ToolResponse[providers.GetProviderResult] {
	if response, done := requestContextErrorResponse[providers.GetProviderResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[providers.GetProviderResult]{Error: &envelope}
	}
	result, err := service.GetProvider(ctx, providers.GetProviderRequest{
		ID: providers.ID(input.ID),
	})
	if err != nil {
		envelope := getProviderErrorEnvelope(err)
		return ToolResponse[providers.GetProviderResult]{Error: &envelope}
	}
	return ToolResponse[providers.GetProviderResult]{Result: &result}
}
