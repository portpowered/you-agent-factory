package localmodels

import (
	"context"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestManagedRuntimeReadinessForFactory_MatchesCatalogProjection(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loaded.FactoryConfig().Resources[0].Backend = "LLAMACPP"

	readiness, err := ManagedRuntimeReadinessForFactory(loaded, "OMNIVOICE_Q4_K_M", CatalogOptions{})
	if err != nil {
		t.Fatalf("ManagedRuntimeReadinessForFactory: %v", err)
	}
	catalog := BuildCatalog(loaded)
	entry := catalog[CanonicalModelName("OMNIVOICE_Q4_K_M")]
	if readiness.Identity != entry.Summary.ManagedRuntime.Identity ||
		readiness.ReadinessState != entry.Summary.ManagedRuntime.ReadinessState ||
		readiness.LifecycleState != entry.Summary.ManagedRuntime.LifecycleState {
		t.Fatalf("readiness = %#v, want catalog summary %#v", readiness, entry.Summary.ManagedRuntime)
	}
}

func TestManagedRuntimeReadinessForFactory_PackagedAndAuthoredFactoriesMatch(t *testing.T) {
	authored := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	authored.FactoryConfig().Resources[0].Backend = "LLAMACPP"

	packagedCfg := catalogFactoryConfig(true)
	packagedCfg.Resources[0].Backend = "LLAMACPP"
	packagedCfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		RequiredTools: []interfaces.RequiredToolConfig{{
			Name:    "Go toolchain",
			Command: "go",
		}},
	}
	packaged := mustLoadedCatalogConfig(t, packagedCfg)

	authoredReadiness, err := ManagedRuntimeReadinessForFactory(authored, "OMNIVOICE_Q4_K_M", CatalogOptions{})
	if err != nil {
		t.Fatalf("authored readiness: %v", err)
	}
	packagedReadiness, err := ManagedRuntimeReadinessForFactory(packaged, "OMNIVOICE_Q4_K_M", CatalogOptions{})
	if err != nil {
		t.Fatalf("packaged readiness: %v", err)
	}
	if authoredReadiness.Identity != packagedReadiness.Identity ||
		authoredReadiness.ReadinessState != packagedReadiness.ReadinessState ||
		authoredReadiness.LifecycleState != packagedReadiness.LifecycleState ||
		authoredReadiness.Locality != packagedReadiness.Locality {
		t.Fatalf("authored = %#v, packaged = %#v, want identical managed runtime readiness", authoredReadiness, packagedReadiness)
	}
	if authoredReadiness.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readinessState = %s, want READY when dependency is declared", authoredReadiness.ReadinessState)
	}
}

func TestManagedRuntimeReadinessForFactory_UsesCacheInspectionWhenProvided(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loaded.FactoryConfig().Resources[0].Backend = "LLAMACPP"

	readiness, err := ManagedRuntimeReadinessForFactory(loaded, "OMNIVOICE_Q4_K_M", CatalogOptions{
		RuntimeCacheInspector: staticRuntimeCacheInspector{
			inspection: RuntimeCacheInspection{
				Supported: true,
				Installed: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("ManagedRuntimeReadinessForFactory: %v", err)
	}
	if readiness.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		readiness.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("readiness = %#v, want READY/INSTALLED", readiness)
	}
}

type staticRuntimeCacheInspector struct {
	inspection RuntimeCacheInspection
	err        error
}

func (s staticRuntimeCacheInspector) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (RuntimeCacheInspection, error) {
	return s.inspection, s.err
}
