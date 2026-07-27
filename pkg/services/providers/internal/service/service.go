// Package service implements the published Providers root Service by
// delegating catalog list/get to the parent-private catalog subservice.
package service

import (
	"context"
	"fmt"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

// Service fulfills the published Providers root contract.
type Service struct {
	catalog catalog.Service
}

var _ providers.Service = (*Service)(nil)

// New constructs an inert Providers root facade over the supplied catalog.
func New(catalogService catalog.Service) (providers.Service, error) {
	if catalogService == nil {
		return nil, fmt.Errorf("construct Providers: catalog is required")
	}
	return &Service{catalog: catalogService}, nil
}

func (s *Service) ListProviders(
	ctx context.Context,
	request providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return s.catalog.ListProviders(ctx, request)
}

func (s *Service) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return s.catalog.GetProvider(ctx, request)
}

// Execute remains unimplemented in IMP-PROV-01; IMP-PROV-02 owns execution
// absorption behind the published root slice.
func (s *Service) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, err
	}
	return providers.ExecuteResult{}, providers.ErrExecuteFailed
}
