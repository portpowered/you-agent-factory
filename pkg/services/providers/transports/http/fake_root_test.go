package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// rootFake is a focused Providers root fake for adapter-edge tests. It avoids
// constructing catalog assemblers, execution adapters, or service-local Wire
// graphs.
type rootFake struct {
	providers.Service

	listProviders func(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error)
	getProvider   func(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error)
	execute       func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error)
}

func (fake *rootFake) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	if fake.listProviders != nil {
		return fake.listProviders(ctx, request)
	}
	return providers.ListProvidersResult{}, providers.ErrUnknownProvider
}

func (fake *rootFake) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if fake.getProvider != nil {
		return fake.getProvider(ctx, request)
	}
	return providers.GetProviderResult{}, providers.ErrUnknownProvider
}

func (fake *rootFake) Execute(
	ctx context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if fake.execute != nil {
		return fake.execute(ctx, request)
	}
	return providers.ExecuteResult{}, providers.ErrExecuteFailed
}
