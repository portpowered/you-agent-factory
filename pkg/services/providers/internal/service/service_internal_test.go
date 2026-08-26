package service

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	_ "github.com/portpowered/infinite-you/pkg/services/providers/internal/testutil/execution"
)

func TestServiceInternalDelegationCoverage(t *testing.T) {
	catalog := internalCatalogStub{}
	execution := internalExecutionStub{}
	root, err := New(catalog, execution, logging.NoopLogger{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := root.ListProviders(t.Context(), providers.ListProvidersRequest{}); err != nil {
		t.Fatalf("ListProviders() error = %v", err)
	}
	if _, err := root.GetProvider(
		t.Context(),
		providers.GetProviderRequest{ID: providers.IDCodex},
	); err != nil {
		t.Fatalf("GetProvider() error = %v", err)
	}
	if _, err := root.Execute(
		t.Context(),
		providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt"},
	); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if root, err := New(nil, execution, logging.NoopLogger{}); err == nil || root != nil {
		t.Fatalf("New(nil, execution) = (%v, %v), want error", root, err)
	}
	if root, err := New(catalog, nil, logging.NoopLogger{}); err == nil || root != nil {
		t.Fatalf("New(catalog, nil) = (%v, %v), want error", root, err)
	}
}

func TestEffectiveACPIntegrationsPreservesUnchangedPackageRuntimeBinding(t *testing.T) {
	packaged := []providers.ACPIntegration{{
		ID: "entry-1", Name: "cursor-acp", Aliases: []string{"factory-cursor"},
		Transport: "stdio", Command: "cursor-agent acp", Arguments: []string{"acp"},
		RuntimePosture: "installed_executable", ImplementationProfile: "cursor-acp",
	}}
	configured := []providers.ACPIntegration{{
		ID: "saved-entry", Name: "cursor-acp", Transport: "stdio", Command: "cursor-agent acp",
	}}

	got := effectiveACPIntegrations(packaged, configured)
	if len(got) != 1 || got[0].ImplementationProfile != "cursor-acp" || got[0].RuntimePosture != "installed_executable" || len(got[0].Arguments) != 1 || got[0].Arguments[0] != "acp" || len(got[0].Aliases) != 1 || got[0].Aliases[0] != "factory-cursor" {
		t.Fatalf("effectiveACPIntegrations() = %#v, want package runtime binding preserved", got)
	}
}

type internalCatalogStub struct{}

func (internalCatalogStub) ResolveProviderID(id providers.ID) (providers.ID, error) {
	return id, nil
}

func (internalCatalogStub) RegistrationProvider(
	id providers.ID,
) (providers.Descriptor, error) {
	return providers.Descriptor{
		ID:           id,
		Availability: providers.AvailabilitySelectable,
	}, nil
}

func (internalCatalogStub) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (internalCatalogStub) GetProvider(
	_ context.Context,
	request providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{
		Provider: providers.Descriptor{ID: request.ID},
	}, nil
}

type internalExecutionStub struct{}

func (internalExecutionStub) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{Content: "ok"}, nil
}
