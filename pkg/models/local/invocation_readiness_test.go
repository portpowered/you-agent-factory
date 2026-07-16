package local

import (
	"context"
	"errors"
	"testing"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestEnsureManagedRuntimeReadyForInvocation_BlocksMissingRuntime(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	opts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, MissingAssets: []string{"omnivoice-base-Q4_K_M.gguf"}},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	_, err := EnsureManagedRuntimeReadyForInvocation(loaded, "OMNIVOICE_Q4_K_M", opts)
	if err == nil || !apisurface.IsManagedRuntimeInvocationBlocked(err) {
		t.Fatalf("error = %v, want managed runtime invocation block", err)
	}
	if !apisurface.IsManagedRuntimeMissing(err) {
		t.Fatalf("error = %v, want missing readiness", err)
	}
}

func TestEnsureManagedRuntimeReadyForInvocation_BlocksLoadingAndFailedRuntimes(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	loadingOpts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, InstalledFileCount: 1},
		}},
	}
	_, loadingErr := EnsureManagedRuntimeReadyForInvocation(loaded, "OMNIVOICE_Q4_K_M", loadingOpts)
	if loadingErr == nil || !apisurface.IsManagedRuntimeInvocationBlocked(loadingErr) {
		t.Fatalf("loading error = %v, want blocked invocation", loadingErr)
	}
	if !errors.Is(loadingErr, apisurface.ErrManagedRuntimeLoading) {
		t.Fatalf("loading error = %v, want ErrManagedRuntimeLoading", loadingErr)
	}

	failedOpts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: false, PartialArtifacts: true},
		}},
	}
	_, failedErr := EnsureManagedRuntimeReadyForInvocation(loaded, "OMNIVOICE_Q4_K_M", failedOpts)
	if failedErr == nil || !apisurface.IsManagedRuntimeInvocationBlocked(failedErr) {
		t.Fatalf("failed error = %v, want blocked invocation", failedErr)
	}
	if !errors.Is(failedErr, apisurface.ErrManagedRuntimeFailed) {
		t.Fatalf("failed error = %v, want ErrManagedRuntimeFailed", failedErr)
	}
}

func TestEnsureManagedRuntimeReadyForInvocation_AllowsReadyRuntime(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	opts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	managed, err := EnsureManagedRuntimeReadyForInvocation(loaded, "OMNIVOICE_Q4_K_M", opts)
	if err != nil {
		t.Fatalf("EnsureManagedRuntimeReadyForInvocation: %v", err)
	}
	if managed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
}

func TestEnsureManagedRuntimeReadyForInvocation_PackagedAndAuthoredFactoriesMatch(t *testing.T) {
	authored := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	packaged := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	opts := CatalogOptions{
		RuntimeCacheInspector: stubRuntimeCacheInspector{byModel: map[string]RuntimeCacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2},
		}},
		SourceResolver: DefaultManagedRuntimeSourceResolver(),
	}

	authoredManaged, err := EnsureManagedRuntimeReadyForInvocation(authored, "OMNIVOICE_Q4_K_M", opts)
	if err != nil {
		t.Fatalf("authored readiness: %v", err)
	}
	packagedManaged, err := EnsureManagedRuntimeReadyForInvocation(packaged, "OMNIVOICE_Q4_K_M", opts)
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
	runtime := &countingLocalRuntime{}
	manager := mustNewManagedRuntime(t, stubInvocationReadinessAssetPuller{
		inspection: RuntimeCacheInspection{
			Supported:     true,
			Installed:     false,
			MissingAssets: []string{"omnivoice-base-Q4_K_M.gguf"},
		},
	}, runtime, Hooks{})

	factoryCfg := managerTestFactoryConfig()
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	runner := manager.WrapRunner(stubRunner{}, loaded, factoryCfg, worker)
	_, err = runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{ModelOperation: "TTS"})
	if err == nil || !apisurface.IsManagedRuntimeMissing(err) {
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

func (s stubInvocationReadinessAssetPuller) PullModel(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, nil
}

func (s stubInvocationReadinessAssetPuller) EnsureModelAvailable(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *workerconfig.Config) error {
	return nil
}

func (s stubInvocationReadinessAssetPuller) ResolveModelCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ *workerconfig.Config) (CacheLayout, error) {
	return s.cache, nil
}

func (s stubInvocationReadinessAssetPuller) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (RuntimeCacheInspection, error) {
	if CanonicalModelName(modelName) == CanonicalModelName("OMNIVOICE_Q4_K_M") {
		return s.inspection, nil
	}
	return RuntimeCacheInspection{}, nil
}
