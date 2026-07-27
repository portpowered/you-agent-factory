// Package wire constructs the published Providers root Service from the
// parent-private catalog subservice.
package wire

import (
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

// Option configures catalog construction for the root facade.
type Option = catalogwire.Option

// NewService constructs one inert Providers root over sibling Catalog and
// Execution capabilities sharing the same private catalog identity authority.
func NewService(options ...Option) (providers.Service, error) {
	catalogService, err := catalogwire.NewService(options...)
	if err != nil {
		return nil, err
	}
	return newRoot(catalogService)
}

func newRoot(catalogService catalog.Service) (providers.Service, error) {
	executionService, err := executionwire.NewService(catalogService)
	if err != nil {
		return nil, err
	}
	return providerservice.New(catalogService, executionService)
}
