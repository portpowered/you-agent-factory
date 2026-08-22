// Package testproviders supplies root-typed Providers fakes for Operator
// Settings boundary tests without importing providers/wire.
package testproviders

import (
	"context"
	"errors"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// CatalogFake is a behavioral double for providers.Service used by Settings
// wire and resolution tests.
type CatalogFake struct {
	providers.Service
	providers map[providers.ID]providers.Descriptor
}

var _ providers.Service = (*CatalogFake)(nil)

// NewCatalogFake constructs a catalog fake from the supplied descriptors.
func NewCatalogFake(entries ...providers.Descriptor) *CatalogFake {
	catalog := make(map[providers.ID]providers.Descriptor, len(entries))
	for _, entry := range entries {
		catalog[entry.ID] = entry.Clone()
	}
	return &CatalogFake{providers: catalog}
}

// StandardCatalog returns a root-typed Providers fake with selectable catalog
// entries used by Operator Settings wire, resolution, and servicewire tests.
func StandardCatalog() providers.Service {
	return NewCatalogFake(
		selectable(providers.IDCodex),
		selectable(providers.IDClaude),
		selectable(providers.IDGemini),
	)
}

func selectable(id providers.ID) providers.Descriptor {
	var aliases []string
	switch id {
	case providers.IDCodex:
		aliases = []string{"openai"}
	case providers.IDClaude:
		aliases = []string{"anthropic"}
	}
	return providers.Descriptor{
		ID:           id,
		Aliases:      aliases,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}
}

// ListProviders returns detached catalog entries.
func (fake *CatalogFake) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	results := make([]providers.Descriptor, 0, len(fake.providers))
	for _, descriptor := range fake.providers {
		results = append(results, descriptor.Clone())
	}
	return providers.ListProvidersResult{Providers: results}, nil
}

// GetProvider resolves canonical IDs and accepted aliases.
func (fake *CatalogFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	descriptor, ok := fake.lookup(request.ID)
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	if descriptor.Availability != providers.AvailabilitySelectable ||
		descriptor.Readiness != providers.ReadinessReady {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
}

// Execute is not implemented for Settings catalog boundary tests.
func (fake *CatalogFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, errors.New("not implemented")
}

func (fake *CatalogFake) lookup(id providers.ID) (providers.Descriptor, bool) {
	if descriptor, ok := fake.providers[id]; ok {
		return descriptor, true
	}
	normalized := strings.ToLower(strings.TrimSpace(id.String()))
	for _, descriptor := range fake.providers {
		if strings.ToLower(descriptor.ID.String()) == normalized {
			return descriptor, true
		}
		for _, alias := range descriptor.Aliases {
			if alias == normalized {
				return descriptor, true
			}
		}
	}
	return providers.Descriptor{}, false
}
