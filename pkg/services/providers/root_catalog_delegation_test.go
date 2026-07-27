package providers_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

func TestRootCatalogDelegation_FulfillsPublishedListAndGet(t *testing.T) {
	t.Parallel()

	var root providers.Service
	root, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	list, err := root.ListProviders(context.Background(), providers.ListProvidersRequest{})
	if err != nil {
		t.Fatalf("ListProviders() = %v", err)
	}
	if len(list.Providers) < 2 {
		t.Fatalf("ListProviders() = %#v, want multiple known providers", list.Providers)
	}

	got, err := root.GetProvider(context.Background(), providers.GetProviderRequest{ID: providers.IDCodex})
	if err != nil {
		t.Fatalf("GetProvider(codex) = %v", err)
	}
	if got.Provider.ID != providers.IDCodex {
		t.Fatalf("GetProvider(codex).Provider.ID = %q", got.Provider.ID)
	}

	assertGetErrorIs(t, root, providers.GetProviderRequest{}, providers.ErrInvalidID)
	assertGetErrorIs(t, root, providers.GetProviderRequest{ID: providers.IDAgy}, providers.ErrProviderUnavailable)
}

func TestRootCatalogDelegation_ConstructionIsInert(t *testing.T) {
	t.Parallel()

	service, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	if service == nil {
		t.Fatal("NewService() returned nil")
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "root-delegation-attempt",
	})
	if !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("Execute() error = %v, want ErrExecuteFailed until IMP-PROV-02", err)
	}
}
