package service

import (
	"context"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

type service struct {
	providers []providers.Descriptor
}

var _ catalog.Service = (*service)(nil)

// New constructs an inert catalog over the accepted standardized provider
// catalog publication.
func New() (catalog.Service, error) {
	descriptors, err := projectPublishedCatalog()
	if err != nil {
		return nil, err
	}
	return &service{providers: descriptors}, nil
}

func (s *service) ListProviders(
	ctx context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	if err := ctx.Err(); err != nil {
		return providers.ListProvidersResult{}, err
	}
	results := make([]providers.Descriptor, len(s.providers))
	for i, descriptor := range s.providers {
		results[i] = descriptor.Clone()
	}
	return providers.ListProvidersResult{Providers: results}, nil
}
