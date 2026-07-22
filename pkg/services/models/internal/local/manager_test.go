package local

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
)

func mustNewManagedRuntime(t *testing.T, assets AssetPuller, runtime Runtime, hooks Hooks) *Manager {
	t.Helper()
	manager, err := NewManagedRuntime(assets, runtime, hooks, time.Now)
	if err != nil {
		t.Fatalf("NewManagedRuntime: %v", err)
	}
	return manager
}

type staticCatalogAssetPuller struct {
	cache CacheLayout
}

func (s staticCatalogAssetPuller) PullModel(context.Context, *modelRuntimeConfig, string) (apisurface.PullResult, error) {
	return apisurface.PullResult{}, nil
}

func (s staticCatalogAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}

func (s staticCatalogAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (CacheLayout, error) {
	return s.cache, nil
}

func (s staticCatalogAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

type countingLocalRuntime struct {
	mu    sync.Mutex
	loads int
}

func (r *countingLocalRuntime) Supports(resource modelRuntimeResource, worker *modelRuntimeWorker) bool {
	return CanonicalBackendName(resource.Backend) == "LLAMACPP" && CanonicalModelName(worker.Model) == CanonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *countingLocalRuntime) Load(context.Context, LoadRequest) (Handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	return stubHandle{}, nil
}

func (r *countingLocalRuntime) loadCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loads
}

type stubHandle struct{}

func (stubHandle) Invoke(context.Context, InvocationRequest) (InvocationResponse, error) {
	return InvocationResponse{Content: "ok"}, nil
}

func TestManager_ReusesLoadedHandleForRepeatExecution(t *testing.T) {
	t.Parallel()
	runtime := &countingLocalRuntime{}
	cache := CacheLayout{ModelName: "OMNIVOICE_Q4_K_M", CachePath: t.TempDir()}
	manager, err := NewManagedRuntime(staticCatalogAssetPuller{cache: cache}, runtime, Hooks{}, time.Now)
	if err != nil {
		t.Fatalf("NewManagedRuntime: %v", err)
	}

	factoryCfg := managerTestFactoryConfig()
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	request := ModelInvocation{ModelOperation: "TTS"}

	if _, handled, err := manager.Invoke(context.Background(), loaded, loaded, worker, request); err != nil || !handled {
		t.Fatalf("first Invoke: handled=%t err=%v", handled, err)
	}
	if _, handled, err := manager.Invoke(context.Background(), loaded, loaded, worker, request); err != nil || !handled {
		t.Fatalf("second Invoke: handled=%t err=%v", handled, err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want 1", got)
	}
}

func TestNewManagedRuntime_ValidatesDependenciesBeforeRuntimeMutation(t *testing.T) {
	t.Parallel()
	puller := staticCatalogAssetPuller{}
	runtime := &countingLocalRuntime{}
	tests := []struct {
		name      string
		construct func() (*Manager, error)
		want      string
	}{
		{name: "asset and cache edge", construct: func() (*Manager, error) {
			return NewManagedRuntime(nil, runtime, Hooks{}, time.Now)
		}, want: "asset puller and cache resolver is required"},
		{name: "invocation runtime", construct: func() (*Manager, error) {
			return NewManagedRuntime(puller, nil, Hooks{}, time.Now)
		}, want: "local invocation runtime is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := tc.construct()
			if manager != nil {
				t.Fatal("manager constructed with a missing required dependency")
			}
			if !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want classified error containing %q", err, tc.want)
			}
		})
	}
	if got := runtime.loadCount(); got != 0 {
		t.Fatalf("runtime loads during validation = %d, want 0", got)
	}
}

func TestManager_Invoke_AcceptsInferenceWorkerTaxonomyAlias(t *testing.T) {
	t.Parallel()
	runtime := &countingLocalRuntime{}
	manager := mustNewManagedRuntime(t, staticCatalogAssetPuller{cache: CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: t.TempDir(),
	}}, runtime, Hooks{})

	factoryCfg := managerTestFactoryConfig()
	factoryCfg.Workers[0].Type = apisurface.RuntimeWorkerTypeInference
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	if _, handled, err := manager.Invoke(context.Background(), loaded, loaded, worker, ModelInvocation{ModelOperation: "TTS"}); err != nil || !handled {
		t.Fatalf("Invoke: handled=%t err=%v", handled, err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want inference worker to route through managed runtime", got)
	}
}

func TestManager_Invoke_AcceptsAgentWorkerLocalModel(t *testing.T) {
	t.Parallel()
	runtime := &countingLocalRuntime{}
	manager := mustNewManagedRuntime(t, staticCatalogAssetPuller{cache: CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: t.TempDir(),
	}}, runtime, Hooks{})

	factoryCfg := managerTestFactoryConfig()
	factoryCfg.Workers[0].Type = apisurface.RuntimeWorkerTypeAgent
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), factoryCfg)
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	if _, handled, err := manager.Invoke(context.Background(), loaded, loaded, worker, ModelInvocation{ModelOperation: "TTS"}); err != nil || !handled {
		t.Fatalf("Invoke: handled=%t err=%v", handled, err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want agent worker to route through managed runtime", got)
	}
}

func managerTestFactoryConfig() *testFactoryConfig {
	return &testFactoryConfig{
		Resources: []modelRuntimeResource{{
			Name:       "omnivoice-cache",
			Type:       apisurface.RuntimeResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []modelRuntimeWorker{{
			Name:          "tts-worker",
			Type:          apisurface.RuntimeWorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: apisurface.RuntimeModelLocalityLocal,
			Resources: []modelRuntimeResource{{
				Name:     "omnivoice-cache",
				Capacity: 1,
			}},
			Operations: []apisurface.RuntimeOperation{{
				Name: "TTS",
				Inputs: []apisurface.RuntimeOperationSlot{{
					Name:         "text",
					ContentTypes: []string{apisurface.RuntimeContentTypeText},
					Required:     true,
				}},
			}},
		}},
	}
}
