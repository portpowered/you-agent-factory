package local

import (
	"context"
	"errors"
	"testing"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestEnsureManagedRuntimeReadyForInvocation_BlocksMissingRuntime(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	inspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, MissingAssets: []string{"omnivoice-base-Q4_K_M.gguf"}},
	}}

	_, err := EnsureManagedRuntimeReadyForInvocation(
		loaded, "OMNIVOICE_Q4_K_M", inspector, DefaultManagedRuntimeSourceResolver(),
	)
	if err == nil {
		t.Fatalf("error = %v, want managed runtime invocation block", err)
	}
	if !errors.Is(err, apisurface.ErrMissing) {
		t.Fatalf("error = %v, want missing readiness", err)
	}
}

func TestEnsureManagedRuntimeReadyForInvocation_BlocksLoadingAndFailedRuntimes(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loadingInspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, InstalledFileCount: 1},
	}}
	_, loadingErr := EnsureManagedRuntimeReadyForInvocation(
		loaded, "OMNIVOICE_Q4_K_M", loadingInspector, nil,
	)
	if loadingErr == nil {
		t.Fatalf("loading error = %v, want blocked invocation", loadingErr)
	}
	if !errors.Is(loadingErr, apisurface.ErrLoading) {
		t.Fatalf("loading error = %v, want ErrManagedRuntimeLoading", loadingErr)
	}

	failedInspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, PartialArtifacts: true},
	}}
	_, failedErr := EnsureManagedRuntimeReadyForInvocation(
		loaded, "OMNIVOICE_Q4_K_M", failedInspector, nil,
	)
	if failedErr == nil {
		t.Fatalf("failed error = %v, want blocked invocation", failedErr)
	}
	if !errors.Is(failedErr, apisurface.ErrFailed) {
		t.Fatalf("failed error = %v, want ErrManagedRuntimeFailed", failedErr)
	}
}

func TestEnsureManagedRuntimeReadyForInvocation_AllowsReadyRuntime(t *testing.T) {
	t.Parallel()
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	inspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2},
	}}

	managed, err := EnsureManagedRuntimeReadyForInvocation(
		loaded, "OMNIVOICE_Q4_K_M", inspector, DefaultManagedRuntimeSourceResolver(),
	)
	if err != nil {
		t.Fatalf("EnsureManagedRuntimeReadyForInvocation: %v", err)
	}
	if managed.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
}

func TestEnsureManagedRuntimeReadyForInvocation_PackagedAndAuthoredFactoriesMatch(t *testing.T) {
	t.Parallel()
	authored := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	packaged := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	inspector := stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
		"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2},
	}}
	resolver := DefaultManagedRuntimeSourceResolver()

	authoredManaged, err := EnsureManagedRuntimeReadyForInvocation(authored, "OMNIVOICE_Q4_K_M", inspector, resolver)
	if err != nil {
		t.Fatalf("authored readiness: %v", err)
	}
	packagedManaged, err := EnsureManagedRuntimeReadyForInvocation(packaged, "OMNIVOICE_Q4_K_M", inspector, resolver)
	if err != nil {
		t.Fatalf("packaged readiness: %v", err)
	}
	if authoredManaged.Identity != packagedManaged.Identity ||
		authoredManaged.ReadinessState != packagedManaged.ReadinessState ||
		authoredManaged.LifecycleState != packagedManaged.LifecycleState {
		t.Fatalf("authored = %#v, packaged = %#v, want identical readiness", authoredManaged, packagedManaged)
	}
}

func TestManager_BlocksInvocationWhenManagedRuntimeMissing(t *testing.T) {
	t.Parallel()
	runtime := &countingLocalRuntime{}
	manager := mustNewManagedRuntime(t, stubInvocationReadinessAssetPuller{
		inspection: RuntimeCacheInspection{
			Supported:     true,
			Installed:     false,
			MissingAssets: []string{"omnivoice-base-Q4_K_M.gguf"},
		},
	}, runtime, Hooks{})

	factoryCfg := managerTestFactoryConfig()
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	_, handled, err := manager.Invoke(context.Background(), loaded, loaded, worker, ModelInvocation{ModelOperation: "TTS"})
	if !handled {
		t.Fatal("Invoke handled = false, want true")
	}
	if err == nil || !errors.Is(err, apisurface.ErrMissing) {
		t.Fatalf("Execute error = %v, want managed runtime missing", err)
	}
	if runtime.loadCount() != 0 {
		t.Fatalf("load count = %d, want 0 when readiness blocks invocation", runtime.loadCount())
	}
}

type stubInvocationReadinessAssetPuller struct {
	inspection RuntimeCacheInspection
	cache      CacheLayout
}

func (s stubInvocationReadinessAssetPuller) PullModel(_ context.Context, _ *modelRuntimeConfig, _ string) (apisurface.PullResult, error) {
	return apisurface.PullResult{}, nil
}

func (s stubInvocationReadinessAssetPuller) EnsureModelAvailable(_ context.Context, _ *modelRuntimeConfig, _ *modelRuntimeWorker) error {
	return nil
}

func (s stubInvocationReadinessAssetPuller) ResolveModelCache(_ context.Context, _ *modelRuntimeConfig, _ *modelRuntimeWorker) (CacheLayout, error) {
	return s.cache, nil
}

func (s stubInvocationReadinessAssetPuller) InspectRuntimeCache(_ context.Context, _ *modelRuntimeConfig, modelName string) (RuntimeCacheInspection, error) {
	if CanonicalModelName(modelName) == CanonicalModelName("OMNIVOICE_Q4_K_M") {
		return s.inspection, nil
	}
	return RuntimeCacheInspection{}, nil
}
