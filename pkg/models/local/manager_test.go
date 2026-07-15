package local

import (
	"context"
	"sync"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

type staticCatalogAssetPuller struct {
	cache CacheLayout
}

func (s staticCatalogAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, nil
}

func (s staticCatalogAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (s staticCatalogAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (CacheLayout, error) {
	return s.cache, nil
}

func (s staticCatalogAssetPuller) InspectRuntimeCache(context.Context, *factoryconfig.LoadedFactoryConfig, string) (RuntimeCacheInspection, error) {
	return RuntimeCacheInspection{}, nil
}

type countingLocalRuntime struct {
	mu    sync.Mutex
	loads int
}

func (r *countingLocalRuntime) Supports(resource factoryresource.Config, worker *workerconfig.Config) bool {
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

func (stubHandle) Invoke(context.Context, InvocationRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{Content: "ok"}, nil
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
	request := workerexecution.RunnerExecutionRequest{
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

func TestManager_WrapRunner_AcceptsInferenceWorkerTaxonomyAlias(t *testing.T) {
	runtime := &countingLocalRuntime{}
	manager := NewManager(staticCatalogAssetPuller{cache: CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: t.TempDir(),
	}}, runtime, Hooks{})
	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	factoryCfg := managerTestFactoryConfig()
	factoryCfg.Workers[0].Type = workertaxonomy.WorkerTypeInference
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	runner := manager.WrapRunner(stubRunner{}, loaded, factoryCfg, worker)
	if _, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{ModelOperation: "TTS"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want inference worker to route through managed runtime", got)
	}
}

func TestManager_WrapRunner_AcceptsAgentWorkerLocalModel(t *testing.T) {
	runtime := &countingLocalRuntime{}
	manager := NewManager(staticCatalogAssetPuller{cache: CacheLayout{
		ModelName: "OMNIVOICE_Q4_K_M",
		CachePath: t.TempDir(),
	}}, runtime, Hooks{})
	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	factoryCfg := managerTestFactoryConfig()
	factoryCfg.Workers[0].Type = workertaxonomy.WorkerTypeAgent
	loaded, err := factoryconfig.NewLoadedFactoryConfig(t.TempDir(), factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	worker, ok := loaded.Worker("tts-worker")
	if !ok || worker == nil {
		t.Fatal("worker tts-worker not found in loaded config")
	}

	runner := manager.WrapRunner(stubRunner{}, loaded, factoryCfg, worker)
	if _, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{ModelOperation: "TTS"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := runtime.loadCount(); got != 1 {
		t.Fatalf("load count = %d, want agent worker to route through managed runtime", got)
	}
}

type stubRunner struct{}

func (stubRunner) Execute(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	return workerexecution.RunnerExecutionResult{}, nil
}

func managerTestFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		Resources: []factoryresource.Config{{
			Name:       "omnivoice-cache",
			Type:       factoryresource.TypeModel,
			Capacity:   1,
			Model:      "OMNIVOICE_Q4_K_M",
			Backend:    "LLAMACPP",
			LoadPolicy: "ON_DEMAND",
		}},
		Workers: []workerconfig.Config{{
			Name:          "tts-worker",
			Type:          workertaxonomy.WorkerTypeModel,
			Model:         "OMNIVOICE_Q4_K_M",
			ModelLocality: workerconfig.ModelLocalityLocal,
			Resources: []factoryresource.Config{{
				Name:     "omnivoice-cache",
				Capacity: 1,
			}},
			Operations: []workerconfig.ModelOperation{{
				Name: "TTS",
				Inputs: []workerconfig.ModelOperationSlot{{
					Name:         "text",
					ContentTypes: []string{workerconfig.ModelOperationContentTypeText},
					Required:     true,
				}},
			}},
		}},
	}
}
