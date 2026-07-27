package service_test

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestListCatalogClassifiesScopeBeforeProjection(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-service-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}

	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{},
	); !errors.Is(err, models.ErrRuntimeScopeInvalid) {
		t.Fatalf("ListCatalog empty scope error = %v, want ErrRuntimeScopeInvalid", err)
	}

	stale, err := (models.RuntimeScopeRef{}).Parse("stale")
	if err != nil {
		t.Fatalf("parse stale scope: %v", err)
	}
	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: stale},
	); !errors.Is(err, models.ErrRuntimeScopeStale) {
		t.Fatalf("ListCatalog stale scope error = %v, want ErrRuntimeScopeStale", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ListCatalog(
		ctx,
		models.ListModelsRequest{Scope: stale},
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListCatalog canceled error = %v, want context.Canceled", err)
	}
}

func TestListCatalogRejectsUnavailableScopedConfiguration(t *testing.T) {
	t.Parallel()

	scopes, err := runtimescopeswire.NewService(func() string { return "catalog-unavailable-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateRef, err := scopes.Open(models.RuntimeBinding{
		RuntimeConfig: func() *models.RuntimeConfig { return nil },
	})
	if err != nil {
		t.Fatalf("open unavailable scope: %v", err)
	}
	scope := publicScope(t, privateRef)
	service, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Catalog: %v", err)
	}

	if _, err := service.ListCatalog(
		context.Background(),
		models.ListModelsRequest{Scope: scope},
	); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("ListCatalog unavailable error = %v, want ErrUnavailable", err)
	}
}

func TestConstructionRejectsMissingRuntimeScopes(t *testing.T) {
	t.Parallel()

	service, err := catalogwire.NewService(nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil) = (%#v, %v), want nil service and error", service, err)
	}
}

func publicScope(t *testing.T, ref runtimescopes.Reference) models.RuntimeScopeRef {
	t.Helper()
	scope, err := (models.RuntimeScopeRef{}).Parse(string(ref))
	if err != nil {
		t.Fatalf("parse public scope: %v", err)
	}
	return scope
}
