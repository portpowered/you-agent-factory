package providers_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// catalogPeerFake implements the published Providers Service catalog slices
// using only Providers root contracts.
type catalogPeerFake struct {
	providers map[providers.ID]providers.Descriptor
}

var _ providers.Service = (*catalogPeerFake)(nil)

func newCatalogPeerFake(entries ...providers.Descriptor) *catalogPeerFake {
	catalog := make(map[providers.ID]providers.Descriptor, len(entries))
	for _, entry := range entries {
		catalog[entry.ID] = entry.Clone()
	}
	return &catalogPeerFake{providers: catalog}
}

func (fake *catalogPeerFake) ListProviders(
	_ context.Context,
	_ providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	ids := make([]providers.ID, 0, len(fake.providers))
	for id := range fake.providers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})

	results := make([]providers.Descriptor, 0, len(ids))
	for _, id := range ids {
		results = append(results, fake.providers[id].Clone())
	}
	return providers.ListProvidersResult{Providers: results}, nil
}

func (fake *catalogPeerFake) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	if err := request.Validate(); err != nil {
		return providers.GetProviderResult{}, err
	}
	descriptor, ok := fake.providers[request.ID]
	if !ok {
		return providers.GetProviderResult{}, providers.ErrUnknownProvider
	}
	if descriptor.Availability != providers.AvailabilitySelectable ||
		descriptor.Readiness != providers.ReadinessReady ||
		hasMissingPrerequisite(descriptor.Prerequisites) {
		return providers.GetProviderResult{}, providers.ErrProviderUnavailable
	}
	return providers.GetProviderResult{Provider: descriptor.Clone()}, nil
}

func hasMissingPrerequisite(prerequisites []providers.Prerequisite) bool {
	for _, prerequisite := range prerequisites {
		if prerequisite.Status == providers.PrerequisiteMissing {
			return true
		}
	}
	return false
}

func TestCatalogContract_Characterization_ListAndGetSuccess(t *testing.T) {
	t.Parallel()

	codex := providers.Descriptor{
		ID:           providers.IDCodex,
		Aliases:      []string{"openai-codex"},
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{
			providers.CapabilityPromptSubmission,
			providers.CapabilityNativeStreaming,
		},
	}
	cursor := providers.Descriptor{
		ID:           providers.IDCursor,
		DisplayName:  "Cursor",
		Availability: providers.AvailabilitySupportedButUnavailable,
		Readiness:    providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:        providers.PrerequisiteConfiguration,
			Name:        "executable",
			Status:      providers.PrerequisiteMissing,
			Description: "cursor-agent must be installed",
		}},
		Capabilities: []providers.Capability{providers.CapabilityPromptSubmission},
	}

	var service providers.Service = newCatalogPeerFake(codex, cursor)

	list, err := service.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) != 2 {
		t.Fatalf("len(Providers) = %d, want 2", len(list.Providers))
	}
	byID := make(map[providers.ID]providers.Descriptor, len(list.Providers))
	for _, provider := range list.Providers {
		byID[provider.ID] = provider
	}
	if byID[providers.IDCodex].DisplayName != "Codex" ||
		byID[providers.IDCursor].Availability != providers.AvailabilitySupportedButUnavailable {
		t.Fatalf("Providers = %#v", list.Providers)
	}
	if byID[providers.IDCodex].Capabilities[0] != providers.CapabilityPromptSubmission {
		t.Fatalf("codex capabilities = %#v", byID[providers.IDCodex].Capabilities)
	}

	got, err := service.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.DisplayName != "Codex" ||
		got.Provider.Availability != providers.AvailabilitySelectable ||
		len(got.Provider.Capabilities) != 2 {
		t.Fatalf("GetProvider(codex) = %#v", got.Provider)
	}
}

func TestCatalogContract_Characterization_TypedFailures(t *testing.T) {
	t.Parallel()

	service := newCatalogPeerFake(providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	})

	assertGetErrorIs(t, service, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDClaude}, providers.ErrUnknownProvider)
	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDCodex + "-stale"}, providers.ErrUnknownProvider)

	unavailable := providers.Descriptor{
		ID:           providers.IDCursor,
		DisplayName:  "Cursor",
		Availability: providers.AvailabilitySupportedButUnavailable,
		Readiness:    providers.ReadinessUnavailable,
		Prerequisites: []providers.Prerequisite{{
			Kind:        providers.PrerequisiteDependency,
			Name:        "cursor-agent",
			Status:      providers.PrerequisiteMissing,
			Description: "install cursor-agent",
		}},
	}
	service = newCatalogPeerFake(unavailable)
	assertGetErrorIs(t, service, providers.GetProviderRequest{ID: providers.IDCursor}, providers.ErrProviderUnavailable)
}

func assertGetErrorIs(
	t *testing.T,
	service providers.Service,
	request providers.GetProviderRequest,
	want error,
) {
	t.Helper()

	_, err := service.GetProvider(context.Background(), request)
	if !errors.Is(err, want) {
		t.Fatalf("GetProvider(%#v) error = %v, want %v", request, err, want)
	}
}
