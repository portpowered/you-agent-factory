package wire_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
)

func TestPublishedConstructionPorts_ExposeRootCompositionHelpers(t *testing.T) {
	t.Parallel()

	discovery, err := factorydefinitionswire.NewEffectiveCatalogDiscovery(
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

	normalize := factorydefinitionswire.EffectiveFactoryDefinitionNormalizerFromMapper()
	catalog, err := factorydefinitionswire.NewEffectiveCatalog(discovery, normalize)
	if err != nil {
		t.Fatalf("NewEffectiveCatalog() error = %v", err)
	}
	if catalog == nil {
		t.Fatal("NewEffectiveCatalog() returned nil operation")
	}

	packagedCatalog, err := factorydefinitionswire.NewPackagedFactoryCatalog(nil)
	if err != nil {
		t.Fatalf("NewPackagedFactoryCatalog() error = %v", err)
	}
	if packagedCatalog.List == nil || packagedCatalog.Resolve == nil {
		t.Fatal("NewPackagedFactoryCatalog() returned incomplete catalog operations")
	}
}
