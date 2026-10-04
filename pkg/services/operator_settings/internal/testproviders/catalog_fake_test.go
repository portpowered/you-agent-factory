package testproviders_test

import (
	"context"
	"errors"
	"testing"

	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestStandardCatalog_GetProviderRejectsRemovedCursorIdentity(t *testing.T) {
	t.Parallel()

	_, err := internaltestproviders.StandardCatalog().GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: "cursor"},
	)
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("GetProvider(cursor) = %v, want ErrUnknownProvider", err)
	}
}

func TestCatalogFake_GetProviderRejectsInvalidRequest(t *testing.T) {
	t.Parallel()

	_, err := internaltestproviders.StandardCatalog().GetProvider(
		context.Background(),
		providers.GetProviderRequest{},
	)
	if err == nil {
		t.Fatal("GetProvider with empty ID = nil, want validation error")
	}
}

func TestCatalogFake_ExecuteIsNotImplemented(t *testing.T) {
	t.Parallel()

	_, err := internaltestproviders.StandardCatalog().Execute(
		context.Background(),
		providers.ExecuteRequest{},
	)
	if err == nil {
		t.Fatal("Execute() = nil, want not implemented error")
	}
}

func TestCatalogFakeCatalogResultsAreDetached(t *testing.T) {
	t.Parallel()
	entry := providers.Descriptor{
		ID: providers.IDCodex, Aliases: []string{"openai"},
		Availability: providers.AvailabilitySelectable, Readiness: providers.ReadinessReady,
	}
	catalog := internaltestproviders.NewCatalogFake(entry)
	entry.Aliases[0] = "changed-input"
	listed, err := catalog.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil || len(listed.Providers) != 1 || listed.Providers[0].Aliases[0] != "openai" {
		t.Fatalf("ListProviders() = %#v, %v, want detached original alias", listed, err)
	}
	listed.Providers[0].Aliases[0] = "changed-output"
	result, err := catalog.GetProvider(context.Background(), providers.GetProviderRequest{ID: "OPENAI"})
	if err != nil || result.Provider.ID != providers.IDCodex || result.Provider.Aliases[0] != "openai" {
		t.Fatalf("GetProvider(OPENAI) = %#v, %v, want original descriptor", result, err)
	}
	result.Provider.Aliases[0] = "changed-lookup"
	again, err := catalog.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil || again.Provider.Aliases[0] != "openai" {
		t.Fatalf("GetProvider(codex) = %#v, %v, want detached lookup", again, err)
	}
}

func TestCatalogFakeRejectsUnavailableProvider(t *testing.T) {
	t.Parallel()
	catalog := internaltestproviders.NewCatalogFake(providers.Descriptor{ID: providers.IDCodex})
	_, err := catalog.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if !errors.Is(err, providers.ErrProviderUnavailable) {
		t.Fatalf("GetProvider() = %v, want ErrProviderUnavailable", err)
	}
}
