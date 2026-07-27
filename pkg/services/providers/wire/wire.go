// Package wire constructs the published Providers root Service from the
// parent-private catalog subservice.
package wire

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
)

// Option configures catalog construction for the root facade.
type Option = catalogwire.Option

// NewService constructs an inert Providers root Service that fulfills
// ListProviders and GetProvider through one private catalog instance.
func NewService(options ...Option) (providers.Service, error) {
	catalogService, err := catalogwire.NewService(options...)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalogService)
}
