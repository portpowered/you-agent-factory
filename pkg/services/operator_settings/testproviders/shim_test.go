package testproviders_test

import (
	"context"
	"testing"

	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	testproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestShimStandardCatalogMatchesInternalOwner(t *testing.T) {
	t.Parallel()

	shimResult, err := testproviders.StandardCatalog().GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: "cursor"},
	)
	if err != nil {
		t.Fatalf("shim GetProvider(cursor) = %v", err)
	}
	ownerResult, err := internaltestproviders.StandardCatalog().GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: "cursor"},
	)
	if err != nil {
		t.Fatalf("internal GetProvider(cursor) = %v", err)
	}
	if shimResult.Provider.ID != ownerResult.Provider.ID {
		t.Fatalf("shim provider ID = %q, want %q", shimResult.Provider.ID, ownerResult.Provider.ID)
	}
}
