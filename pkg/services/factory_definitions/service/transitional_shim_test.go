package service_test

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionsservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/service"
)

func TestTransitionalShim_ExposesOwnerLocalConstructionHelpers(t *testing.T) {
	t.Parallel()

	discovery, err := factorydefinitionsservice.NewEffectiveCatalogDiscovery(
		func(string) ([]factorydefinitions.NamedFactoryListEntry, error) {
			return nil, nil
		},
		func(string) ([]byte, error) { return nil, nil },
		nil,
	)
	if err != nil {
		t.Fatalf("NewEffectiveCatalogDiscovery() error = %v", err)
	}
	if discovery.ListRoot == nil || discovery.ListPackaged == nil {
		t.Fatal("NewEffectiveCatalogDiscovery() returned incomplete discovery ports")
	}

	normalize := func(
		context.Context,
		factorydefinitions.EffectiveFactoryCatalogCandidate,
	) (*factorydefinitions.FactoryConfig, error) {
		return &factorydefinitions.FactoryConfig{}, nil
	}
	catalog, err := factorydefinitionsservice.NewEffectiveCatalog(discovery, normalize)
	if err != nil {
		t.Fatalf("NewEffectiveCatalog() error = %v", err)
	}
	if catalog == nil {
		t.Fatal("NewEffectiveCatalog() returned nil operation")
	}

	effectiveCatalogService, err := factorydefinitionsservice.NewEffectiveCatalogService(catalog)
	if err != nil {
		t.Fatalf("NewEffectiveCatalogService() error = %v", err)
	}
	if effectiveCatalogService == nil {
		t.Fatal("NewEffectiveCatalogService() returned nil service")
	}

	packagedCatalog, err := factorydefinitionsservice.NewPackagedFactoryCatalog(nil)
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}
	if packagedCatalog.List == nil || packagedCatalog.Resolve == nil {
		t.Fatal("NewPackagedFactoryCatalog() returned incomplete catalog operations")
	}

	installationService := factorydefinitionsservice.NewPackagedFactoryInstallationService(nil, nil)
	if installationService == nil {
		t.Fatal("NewPackagedFactoryInstallationService() returned nil service")
	}

	_ = factorydefinitionsservice.WithDistributionScaffold(nil, nil)
}
