// Package catalog defines the parent-private Providers catalog service.
package catalog

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// Service serves detached, deterministically ordered provider descriptors
// projected from the accepted standardized provider catalog.
type Service interface {
	ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error)
	GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error)
}
