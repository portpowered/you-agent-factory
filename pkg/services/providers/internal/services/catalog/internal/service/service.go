package service

import (
	"context"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	catalog "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

type service struct {
	providers []providers.Descriptor
	byID      map[providers.ID]providers.Descriptor
	aliases   map[string]providers.ID
}

var _ catalog.Service = (*service)(nil)

// New constructs an inert catalog over the accepted standardized provider
// catalog publication.
func New() (catalog.Service, error) {
	descriptors, err := projectPublishedCatalog()
	if err != nil {
		return nil, err
	}
	byID, aliases := indexDescriptors(descriptors)
	return &service{
		providers: descriptors,
		byID:      byID,
		aliases:   aliases,
	}, nil
}

func indexDescriptors(descriptors []providers.Descriptor) (
	map[providers.ID]providers.Descriptor,
	map[string]providers.ID,
) {
	byID := make(map[providers.ID]providers.Descriptor, len(descriptors))
	aliases := make(map[string]providers.ID, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
		aliases[strings.ToLower(descriptor.ID.String())] = descriptor.ID
		for _, alias := range descriptor.Aliases {
			aliases[alias] = descriptor.ID
		}
	}
	return byID, aliases
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

func (s *service) GetProvider(
	ctx context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return providers.GetProviderResult{}, err
	}
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	canonical, ok := s.resolveID(request.ID)
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	descriptor, ok := s.byID[canonical]
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
}

func (s *service) resolveID(id providers.ID) (providers.ID, bool) {
	normalized := strings.ToLower(strings.TrimSpace(id.String()))
	canonical, ok := s.aliases[normalized]
	return canonical, ok
}
