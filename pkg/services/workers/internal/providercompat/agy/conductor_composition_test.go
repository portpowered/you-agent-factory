package agy_test

import (
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/providercompat/registry"
)

func TestBuiltInRegistrySelectsAntigravityThroughAuthoritativeManifestIdentity(t *testing.T) {
	t.Parallel()

	providers := newProductionAgyRegistry(t)
	entry, err := providers.Lookup(" ANTIGRAVITY ")
	if err != nil {
		t.Fatalf("Lookup(antigravity) error = %v", err)
	}
	if entry.Identity() != inference.Identity(modelprovider.ProviderAntigravity) {
		t.Fatalf("Lookup identity = %q, want antigravity", entry.Identity())
	}
	integration, err := providers.Integration(string(modelprovider.ProviderAntigravity))
	if err != nil {
		t.Fatalf("Integration(agy) error = %v", err)
	}
	if integration.Identity() != inference.Identity(modelprovider.ProviderAntigravity) {
		t.Fatalf("Integration identity = %q, want agy", integration.Identity())
	}
	maximum := integration.MaximumCapabilities()
	if !maximum.Has(inference.CapabilityPromptSubmission) || !maximum.Has(inference.CapabilityMessageSnapshots) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt_submission and message_snapshots", maximum.Values())
	}
	if providers.UsesNativeRunner(string(modelprovider.ProviderAntigravity)) {
		t.Fatal("UsesNativeRunner(agy) = true, want conductor route for migrated Agy")
	}
}

func newProductionAgyRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	registrations, err := registry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	providers, err := registry.New(registrations...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers
}
