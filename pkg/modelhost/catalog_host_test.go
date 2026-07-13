package modelhost

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestClassifyReadiness_CoversReadyMissingLoadingFailedUnsupported(t *testing.T) {
	identity := Identity{
		Name:     "OMNIVOICE_Q4_K_M",
		Locality: factoryapi.WorkerModelLocalityLocal,
	}

	cases := []struct {
		name        string
		inspection  CacheInspection
		unsupported bool
		readiness   factoryapi.ManagedRuntimeReadinessState
		lifecycle   factoryapi.ManagedRuntimeLifecycleState
		failure     FailureClass
	}{
		{
			name: "ready",
			inspection: CacheInspection{
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
			readiness: factoryapi.ManagedRuntimeReadinessStateREADY,
			lifecycle: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
			failure:   FailureClassNone,
		},
		{
			name: "missing",
			inspection: CacheInspection{
				Supported:     true,
				MissingAssets: []string{"model.gguf"},
			},
			readiness: factoryapi.ManagedRuntimeReadinessStateMISSING,
			lifecycle: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			failure:   FailureClassMissingAssets,
		},
		{
			name: "loading",
			inspection: CacheInspection{
				Supported:          true,
				InstalledFileCount: 1,
			},
			readiness: factoryapi.ManagedRuntimeReadinessStateLOADING,
			lifecycle: factoryapi.ManagedRuntimeLifecycleStateINSTALLING,
			failure:   FailureClassLoadingTimeout,
		},
		{
			name: "failed",
			inspection: CacheInspection{
				Supported:        true,
				PartialArtifacts: true,
			},
			readiness: factoryapi.ManagedRuntimeReadinessStateFAILED,
			lifecycle: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			failure:   FailureClassMissingAssets,
		},
		{
			name:        "unsupported",
			unsupported: true,
			readiness:   factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
			lifecycle:   factoryapi.ManagedRuntimeLifecycleStateNOTAPPLICABLE,
			failure:     FailureClassUnsupportedRuntime,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := ClassifyReadiness(identity, tc.inspection, tc.unsupported)
			if snapshot.ReadinessState != tc.readiness {
				t.Fatalf("readiness = %s, want %s", snapshot.ReadinessState, tc.readiness)
			}
			if snapshot.LifecycleState != tc.lifecycle {
				t.Fatalf("lifecycle = %s, want %s", snapshot.LifecycleState, tc.lifecycle)
			}
			if snapshot.FailureClass != tc.failure {
				t.Fatalf("failure class = %s, want %s", snapshot.FailureClass, tc.failure)
			}
		})
	}
}

func TestFailureClassFromError_ClassifiesCancelled(t *testing.T) {
	if got := FailureClassFromError(errors.Join(ErrCancelled, context.Canceled)); got != FailureClassCancelled {
		t.Fatalf("failure class = %s, want %s", got, FailureClassCancelled)
	}
}

func TestManagedRuntimeFromSnapshot_PreservesPublicVocabulary(t *testing.T) {
	snapshot := ReadinessSnapshot{
		Identity: Identity{
			Name:     "OMNIVOICE_Q4_K_M",
			Locality: factoryapi.WorkerModelLocalityLocal,
			SupportedOperations: []factoryapi.ModelOperation{{
				Name: "TTS",
			}},
		},
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
		FailureClass:   FailureClassNone,
		Diagnostics: map[string]string{
			"readinessState": "READY",
			"lifecycleState": "INSTALLED",
			"locality":       "LOCAL",
		},
	}

	managed := ManagedRuntimeFromSnapshot(snapshot)
	if managed.Identity != snapshot.Identity.Name {
		t.Fatalf("identity = %q, want %q", managed.Identity, snapshot.Identity.Name)
	}
	if managed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
	if managed.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("lifecycle = %s, want INSTALLED", managed.LifecycleState)
	}
	if len(managed.SupportedOperations) != 1 || managed.SupportedOperations[0].Name != "TTS" {
		t.Fatalf("operations = %#v, want one TTS operation", managed.SupportedOperations)
	}
}

func TestCatalogHost_InspectReadinessAndLeaseLifecycle(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := NewCatalogHost(stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, Options{SourceResolver: DefaultManagedRuntimeSourceResolverAdapter()})

	ctx := context.Background()
	ready, err := host.InspectReadiness(ctx, loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectReadiness: %v", err)
	}
	if ready.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("ready state = %s, want READY", ready.ReadinessState)
	}

	lease, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{Holder: "test"})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease.ID == "" {
		t.Fatal("lease id is empty")
	}
	if err := host.Unload(ctx, loaded, "OMNIVOICE_Q4_K_M"); !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("Unload with active lease = %v, want capacity exhausted", err)
	}
	if err := host.ReleaseLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := host.Unload(ctx, loaded, "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("Unload after release: %v", err)
	}
}

func TestCatalogHost_BlocksLeaseForNonReadyStates(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := NewCatalogHost(stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, InstalledFileCount: 1},
		},
	}, Options{})

	_, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	var readinessErr *ReadinessError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %v, want *ReadinessError", err)
	}
	if readinessErr.Snapshot.ReadinessState != factoryapi.ManagedRuntimeReadinessStateLOADING {
		t.Fatalf("readiness = %s, want LOADING", readinessErr.Snapshot.ReadinessState)
	}
	if !errors.Is(err, ErrLoadingTimeout) {
		t.Fatalf("error = %v, want ErrLoadingTimeout", err)
	}
}

func TestCatalogHost_InspectReadinessHonoursCancellation(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := NewCatalogHost(stubAssetGateway{}, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := host.InspectReadiness(ctx, loaded, "OMNIVOICE_Q4_K_M")
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("error = %v, want ErrCancelled", err)
	}
	if FailureClassFromError(err) != FailureClassCancelled {
		t.Fatalf("failure class = %s, want cancelled", FailureClassFromError(err))
	}
}

type stubAssetGateway struct {
	byModel map[string]CacheInspection
}

func (s stubAssetGateway) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (AssetPullResult, error) {
	return AssetPullResult{}, apisurface.ErrModelNotFound
}

func (s stubAssetGateway) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (CacheInspection, error) {
	if inspection, ok := s.byModel[modelName]; ok {
		return inspection, nil
	}
	return CacheInspection{}, nil
}

func mustLoadedCatalogConfig(t *testing.T, factoryCfg *interfaces.FactoryConfig) *factoryconfig.LoadedFactoryConfig {
	t.Helper()
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func catalogFactoryConfig(includeResource bool) *interfaces.FactoryConfig {
	worker := interfaces.WorkerConfig{
		Name:          "voice-local",
		Type:          interfaces.WorkerTypeModel,
		Model:         "OMNIVOICE_Q4_K_M",
		ModelLocality: interfaces.ModelLocalityLocal,
		Operations: []interfaces.ModelOperation{{
			Name: "TTS",
		}},
	}
	if includeResource {
		worker.Resources = []interfaces.ResourceConfig{{Name: "omnivoice-cache", Capacity: 1}}
	}
	cfg := &interfaces.FactoryConfig{
		Name:    "factory",
		Workers: []interfaces.WorkerConfig{worker},
	}
	if includeResource {
		cfg.Resources = []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "GGUF",
			LoadPolicy: "ON_DEMAND",
		}}
	}
	return cfg
}
