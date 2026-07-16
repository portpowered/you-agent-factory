package local

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func mustNewManagedRuntime(t *testing.T, assets AssetPuller, runtime Runtime, hooks Hooks) *Manager {
	t.Helper()
	manager, err := NewManagedRuntime(ManagedRuntimeDependencies{
		AssetPuller: assets, Runtime: runtime, Hooks: hooks,
	})
	if err != nil {
		t.Fatalf("NewManagedRuntime: %v", err)
	}
	return manager
}

type staticCatalogAssetPuller struct {
	cache CacheLayout
}

func (s staticCatalogAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, nil
}

func (s staticCatalogAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) error {
	return nil
}

func (s staticCatalogAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *interfaces.WorkerConfig) (CacheLayout, error) {
	return s.cache, nil
}

func (s staticCatalogAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

type countingLocalRuntime struct {
	mu    sync.Mutex
	loads int
}

func (r *countingLocalRuntime) Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool {
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

func (stubHandle) Invoke(context.Context, InvocationRequest) (interfaces.InferenceResponse, error) {
	return interfaces.InferenceResponse{Content: "ok"}, nil
}

func TestManager_ReusesLoadedHandleForRepeatExecution(t *testing.T) {
	runtime := &countingLocalRuntime{}
	cache := CacheLayout{ModelName: "OMNIVOICE_Q4_K_M", CachePath: t.TempDir()}
	manager, err := NewManagedRuntime(ManagedRuntimeDependencies{
		AssetPuller: staticCatalogAssetPuller{cache: cache},
		Runtime:     runtime,
	})
	if err != nil {
		t.Fatalf("NewManagedRuntime: %v", err)
	}

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
	request := interfaces.RunnerExecutionRequest{
		ModelOperation: "TTS",
	}

	if _, err := runner.Execute(context.Background(), request); err != nil {
		t.Fatalf("first Execute: %v", err)
	}
	if _, err := runner.Execute(context.Background(), request); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want 1", got)
	}
}

func TestNewManagedRuntime_ValidatesDependenciesBeforeRuntimeMutation(t *testing.T) {
	puller := staticCatalogAssetPuller{}
	runtime := &countingLocalRuntime{}
	tests := []struct {
		name string
		deps ManagedRuntimeDependencies
		want string
	}{
		{name: "asset and cache edge", deps: ManagedRuntimeDependencies{Runtime: runtime}, want: "asset puller and cache resolver is required"},
		{name: "invocation runtime", deps: ManagedRuntimeDependencies{AssetPuller: puller}, want: "local invocation runtime is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := NewManagedRuntime(tc.deps)
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

func TestManager_WrapRunner_AcceptsInferenceWorkerTaxonomyAlias(t *testing.T) {
	runtime := &countingLocalRuntime{}
	manager := mustNewManagedRuntime(t, staticCatalogAssetPuller{cache: CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: t.TempDir(),
	}}, runtime, Hooks{})

	factoryCfg := managerTestFactoryConfig()
	factoryCfg.Workers[0].Type = interfaces.WorkerTypeInference
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	runner := manager.WrapRunner(stubRunner{}, loaded, factoryCfg, worker)
	if _, err := runner.Execute(context.Background(), interfaces.RunnerExecutionRequest{ModelOperation: "TTS"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want inference worker to route through managed runtime", got)
	}
}

func TestManager_WrapRunner_AcceptsAgentWorkerLocalModel(t *testing.T) {
	runtime := &countingLocalRuntime{}
	manager := mustNewManagedRuntime(t, staticCatalogAssetPuller{cache: CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: t.TempDir(),
	}}, runtime, Hooks{})

	factoryCfg := managerTestFactoryConfig()
	factoryCfg.Workers[0].Type = interfaces.WorkerTypeAgent
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	runner := manager.WrapRunner(stubRunner{}, loaded, factoryCfg, worker)
	if _, err := runner.Execute(context.Background(), interfaces.RunnerExecutionRequest{ModelOperation: "TTS"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want agent worker to route through managed runtime", got)
	}
}

type stubRunner struct{}

func (stubRunner) Execute(context.Context, interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	return interfaces.RunnerExecutionResult{}, nil
}

func managerTestFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{
			Name:       "omnivoice-cache",
			Type:       interfaces.ResourceTypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []interfaces.WorkerConfig{{
			Name:          "tts-worker",
			Type:          interfaces.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: interfaces.ModelLocalityLocal,
			Resources: []interfaces.ResourceConfig{{
				Name:     "omnivoice-cache",
				Capacity: 1,
			}},
			Operations: []interfaces.ModelOperation{{
				Name: "TTS",
				Inputs: []interfaces.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{interfaces.ModelOperationContentTypeText},
					Required:     true,
				}},
			}},
		}},
	}
}
