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
