package testproviders_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestStandardCatalog_GetProviderResolvesAlias(t *testing.T) {
	t.Parallel()

	result, err := testproviders.StandardCatalog().GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: "cursor"},
	)
	if err != nil {
		t.Fatalf("GetProvider(cursor) = %v", err)
	}
	if result.Provider.ID != providers.IDCursor {
		t.Fatalf("provider ID = %q, want %q", result.Provider.ID, providers.IDCursor)
	}
}

func TestCatalogFake_ListProvidersReturnsDetachedEntries(t *testing.T) {
	t.Parallel()

	fake := testproviders.NewCatalogFake(
		providers.Descriptor{ID: providers.IDCodex, Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady},
	)
	result, err := fake.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(result.Providers) != 1 || result.Providers[0].ID != providers.IDCodex {
		t.Fatalf("providers = %#v, want one codex entry", result.Providers)
	}
}

func TestCatalogFake_GetProviderRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	_, err := testproviders.StandardCatalog().GetProvider(
		context.Background(),
		providers.GetProviderRequest{},
	)
	if err == nil {
		t.Fatal("GetProvider with empty ID = nil, want validation error")
	}
}

func TestCatalogFake_ExecuteIsNotImplemented(t *testing.T) {
	t.Parallel()

	_, err := testproviders.StandardCatalog().Execute(
		context.Background(),
		providers.ExecuteRequest{},
	)
	if err == nil {
		t.Fatal("Execute() = nil, want not implemented error")
	}
}
