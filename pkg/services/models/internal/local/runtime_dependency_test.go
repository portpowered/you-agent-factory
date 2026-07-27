package local

import (
	"context"
	"errors"
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

func TestManagedRuntimeReadinessForFactory_MatchesCatalogProjection(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loaded.Resources[0].Backend = "LLAMACPP"

	readiness, err := ManagedRuntimeReadinessForFactory(loaded, "OMNIVOICE_Q4_K_M", nil, nil)
	if err != nil {
		t.Fatalf("ManagedRuntimeReadinessForFactory: %v", err)
	}
	catalog := BuildCatalog(loaded)
	entry := catalog[CanonicalModelName("OMNIVOICE_Q4_K_M")]
	if readiness.Identity != entry.Summary.ManagedRuntime.Identity ||
		string(readiness.ReadinessState) != string(entry.Summary.ManagedRuntime.ReadinessState) ||
		string(readiness.LifecycleState) != string(entry.Summary.ManagedRuntime.LifecycleState) {
		t.Fatalf("readiness = %#v, want catalog summary %#v", readiness, entry.Summary.ManagedRuntime)
	}
}

func TestManagedRuntimeReadinessForFactory_PackagedAndAuthoredFactoriesMatch(t *testing.T) {
	t.Parallel()
	authored := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	authored.Resources[0].Backend = "LLAMACPP"

	packagedCfg := catalogFactoryConfig(true)
	packagedCfg.Resources[0].Backend = "LLAMACPP"
	packagedCfg.ResourceManifest = &testResourceManifest{
		RequiredTools: []testRequiredTool{{
			Name:    "Go toolchain",
			Command: "go",
		}},
	}
	packaged := mustLoadedCatalogConfig(t, packagedCfg)

	authoredReadiness, err := ManagedRuntimeReadinessForFactory(authored, "OMNIVOICE_Q4_K_M", nil, nil)
	if err != nil {
		t.Fatalf("authored readiness: %v", err)
	}
	packagedReadiness, err := ManagedRuntimeReadinessForFactory(packaged, "OMNIVOICE_Q4_K_M", nil, nil)
	if err != nil {
		t.Fatalf("packaged readiness: %v", err)
	}
	if authoredReadiness.Identity != packagedReadiness.Identity ||
		authoredReadiness.ReadinessState != packagedReadiness.ReadinessState ||
		authoredReadiness.LifecycleState != packagedReadiness.LifecycleState ||
		authoredReadiness.Locality != packagedReadiness.Locality {
		t.Fatalf("authored = %#v, packaged = %#v, want identical managed runtime readiness", authoredReadiness, packagedReadiness)
	}
	if authoredReadiness.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readinessState = %s, want READY when dependency is declared", authoredReadiness.ReadinessState)
	}
}

func TestManagedRuntimeReadinessForFactory_ReportsLoadingAndFailedStates(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loaded.Resources[0].Backend = "LLAMACPP"

	loading, err := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", staticRuntimeCacheInspector{
			inspection: RuntimeCacheInspection{
				Supported:          true,
				Installed:          false,
				InstalledFileCount: 1,
				MissingAssets:      []string{"omnivoice-tokenizer-Q4_K_M.gguf"},
			},
		}, nil,
	)
	if err != nil {
		t.Fatalf("loading readiness: %v", err)
	}
	if loading.ReadinessState != managedruntime.ReadinessStateLoading {
		t.Fatalf("loading readiness = %s, want LOADING", loading.ReadinessState)
	}

	failed, err := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", staticRuntimeCacheInspector{
			inspection: RuntimeCacheInspection{
				Supported:        true,
				Installed:        false,
				PartialArtifacts: true,
			},
		}, nil,
	)
	if err != nil {
		t.Fatalf("failed readiness: %v", err)
	}
	if failed.ReadinessState != managedruntime.ReadinessStateFailed {
		t.Fatalf("failed readiness = %s, want FAILED", failed.ReadinessState)
	}
}

func TestManagedRuntimeReadinessForFactory_PackagedAndAuthoredFactoriesShareLoadingState(t *testing.T) {
	t.Parallel()
	authored := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	packagedCfg := catalogFactoryConfig(true)
	packagedCfg.ResourceManifest = &testResourceManifest{
		RequiredTools: []testRequiredTool{{Name: "Go toolchain", Command: "go"}},
	}
	packaged := mustLoadedCatalogConfig(t, packagedCfg)
	inspector := staticRuntimeCacheInspector{
		inspection: RuntimeCacheInspection{
			Supported:          true,
			Installed:          false,
			InstalledFileCount: 1,
		},
	}

	authoredReadiness, err := ManagedRuntimeReadinessForFactory(authored, "OMNIVOICE_Q4_K_M", inspector, nil)
	if err != nil {
		t.Fatalf("authored readiness: %v", err)
	}
	packagedReadiness, err := ManagedRuntimeReadinessForFactory(packaged, "OMNIVOICE_Q4_K_M", inspector, nil)
	if err != nil {
		t.Fatalf("packaged readiness: %v", err)
	}
	if authoredReadiness.ReadinessState != packagedReadiness.ReadinessState ||
		authoredReadiness.LifecycleState != packagedReadiness.LifecycleState {
		t.Fatalf("authored = %#v, packaged = %#v, want identical loading readiness", authoredReadiness, packagedReadiness)
	}
	if authoredReadiness.ReadinessState != managedruntime.ReadinessStateLoading {
		t.Fatalf("readinessState = %s, want LOADING", authoredReadiness.ReadinessState)
	}
}

func TestManagedRuntimeReadinessForFactory_UsesCacheInspectionWhenProvided(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loaded.Resources[0].Backend = "LLAMACPP"

	readiness, err := ManagedRuntimeReadinessForFactory(
		loaded, "OMNIVOICE_Q4_K_M", staticRuntimeCacheInspector{
			inspection: RuntimeCacheInspection{
				Supported: true,
				Installed: true,
			},
		}, nil,
	)
	if err != nil {
		t.Fatalf("ManagedRuntimeReadinessForFactory: %v", err)
	}
	if readiness.ReadinessState != managedruntime.ReadinessStateReady ||
		readiness.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("readiness = %#v, want READY/INSTALLED", readiness)
	}
}

func TestManagedRuntimeReadinessForFactoryContext_ReportsDetailedCurrentCacheFacts(t *testing.T) {
	t.Parallel()

	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	readiness, err := ManagedRuntimeReadinessForFactoryContext(
		context.Background(),
		loaded,
		"OMNIVOICE_Q4_K_M",
		staticRuntimeCacheInspector{inspection: RuntimeCacheInspection{
			Supported:          true,
			Installed:          true,
			Revision:           "rev-current",
			CachePath:          "/cache/current",
			InstalledFileCount: 2,
		}},
		DefaultManagedRuntimeSourceResolver(),
	)
	if err != nil {
		t.Fatalf("ManagedRuntimeReadinessForFactoryContext: %v", err)
	}
	if readiness.ReadinessState != managedruntime.ReadinessStateReady ||
		readiness.LifecycleState != managedruntime.LifecycleStateInstalled ||
		readiness.Diagnostics["revision"] != "rev-current" ||
		readiness.Diagnostics["cachePath"] != "/cache/current" ||
		readiness.Diagnostics["installedFileCount"] != "2" {
		t.Fatalf("current readiness = %#v, want detached detailed cache facts", readiness)
	}
}

func TestManagedRuntimeReadinessForFactoryContext_PreservesCancellationAndDependencyFailures(t *testing.T) {
	t.Parallel()

	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		canceled, loaded, "OMNIVOICE_Q4_K_M", nil, nil,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled readiness error = %v, want context.Canceled", err)
	}
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		context.Background(), nil, "OMNIVOICE_Q4_K_M", nil, nil,
	); err == nil {
		t.Fatal("nil runtime config unexpectedly succeeded")
	}
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		context.Background(), loaded, " ", nil, nil,
	); !errors.Is(err, managedruntime.ErrNotFound) {
		t.Fatalf("empty model readiness error = %v, want not found", err)
	}
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		context.Background(), loaded, "OMNIVOICE_Q4_K_M", nil, nil,
	); err != nil {
		t.Fatalf("readiness without cache inspector: %v", err)
	}
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		context.Background(),
		loaded,
		"unknown-model",
		staticRuntimeCacheInspector{},
		nil,
	); !errors.Is(err, managedruntime.ErrNotFound) {
		t.Fatalf("unknown model readiness error = %v, want not found", err)
	}

	dependencyErr := errors.New("cache inspection failed")
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		context.Background(),
		loaded,
		"OMNIVOICE_Q4_K_M",
		staticRuntimeCacheInspector{err: dependencyErr},
		nil,
	); !errors.Is(err, dependencyErr) {
		t.Fatalf("cache dependency error = %v, want %v", err, dependencyErr)
	}

	during, cancelDuring := context.WithCancel(context.Background())
	if _, err := ManagedRuntimeReadinessForFactoryContext(
		during,
		loaded,
		"OMNIVOICE_Q4_K_M",
		cancelingRuntimeCacheInspector{cancel: cancelDuring},
		nil,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("during-query cancellation error = %v, want context.Canceled", err)
	}
}

type staticRuntimeCacheInspector struct {
	inspection RuntimeCacheInspection
	err        error
}

func (s staticRuntimeCacheInspector) InspectRuntimeCache(_ context.Context, _ *modelRuntimeConfig, _ string) (RuntimeCacheInspection, error) {
	return s.inspection, s.err
}

type cancelingRuntimeCacheInspector struct {
	cancel context.CancelFunc
}

func (s cancelingRuntimeCacheInspector) InspectRuntimeCache(
	context.Context,
	*modelRuntimeConfig,
	string,
) (RuntimeCacheInspection, error) {
	s.cancel()
	return RuntimeCacheInspection{}, nil
}
