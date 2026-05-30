package localmodels

import (
	"context"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
	manager := NewManager(staticCatalogAssetPuller{cache: cache}, runtime, Hooks{})
	if manager == nil {
		t.Fatal("NewManager returned nil")
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
