package support

import (
	"encoding/json"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providercontract "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// ProgressingExternalProvider is a functional provider effect whose manifest
// and integration are composed into the process edge bag. Keeping the wire
// registration details here prevents behavior packages from constructing a
// peer or importing a Providers construction contract.
type ProgressingExternalProvider struct {
	integration  *providercontract.ProgressingIntegration
	registration providercontract.Registration
}

// ExternalProviderStats is the observable side-effect count for one
// ProgressingExternalProvider.
type ExternalProviderStats struct {
	DiscoverCalls, CapabilityCalls, InvokeCalls, ProgressWrites, TerminalCloses int
}

// NewProgressingExternalProvider returns a deterministic external provider
// effect for functional provider-selection scenarios.
func NewProgressingExternalProvider(
	t testing.TB,
	identity, alias, content string,
) *ProgressingExternalProvider {
	t.Helper()
	integration := providercontract.ProgressingExternalIntegration(
		providercontract.Identity(identity),
		content,
	)
	return &ProgressingExternalProvider{
		integration: integration,
		registration: providercontract.Registration{
			Manifest:    externalProviderManifest(t, identity, alias),
			Integration: integration,
		},
	}
}

// Stats returns the observed effect counts without exposing the Providers
// registration contract to behavior packages.
func (provider *ProgressingExternalProvider) Stats() ExternalProviderStats {
	if provider == nil || provider.integration == nil {
		return ExternalProviderStats{}
	}
	stats := provider.integration.Stats()
	return ExternalProviderStats{
		DiscoverCalls:   stats.DiscoverCalls,
		CapabilityCalls: stats.CapabilityCalls,
		InvokeCalls:     stats.InvokeCalls,
		ProgressWrites:  stats.ProgressWrites,
		TerminalCloses:  stats.TerminalCloses,
	}
}

// ProviderEdges returns the exact process-edge replacement consumed by
// root.BuildProcess. It is the only registration seam exposed to functional
// behavior packages.
func ProviderEdges(providers ...*ProgressingExternalProvider) serviceedges.Edges {
	registrations := make(providercontract.ProviderRegistrations, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		registrations = append(registrations, provider.registration)
	}
	return serviceedges.Edges{ProviderRegistrations: registrations}
}

func externalProviderManifest(t testing.TB, identity, alias string) providercontract.Manifest {
	t.Helper()
	var catalog struct {
		Providers []providercontract.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	if len(catalog.Providers) == 0 {
		t.Fatal("embedded provider catalog is empty")
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = providercontract.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = providercontract.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = providercontract.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = providercontract.ResponseFidelityCapabilities{}
	return manifest
}
