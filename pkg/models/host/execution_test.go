package modelhost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
)

type recordingLeaseHost struct {
	*CatalogHost
	acquires int
	releases int
}

func (h *recordingLeaseHost) AcquireLease(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string, opts LeaseOptions) (Lease, error) {
	h.acquires++
	return h.CatalogHost.AcquireLease(ctx, runtimeCfg, modelName, opts)
}

func (h *recordingLeaseHost) ReleaseLease(ctx context.Context, leaseID string) error {
	h.releases++
	return h.CatalogHost.ReleaseLease(ctx, leaseID)
}

type executionTestRuntime struct {
	mu                  sync.Mutex
	loads               int
	invocations         int
	lastServingEndpoint string
}

func (r *executionTestRuntime) Supports(resource factoryresource.Config, worker *workerconfig.Config) bool {
	return localmodels.CanonicalBackendName(resource.Backend) == "LLAMACPP" &&
		localmodels.CanonicalModelName(worker.Model) == localmodels.CanonicalModelName("OMNIVOICE_Q4_K_M")
}

func (r *executionTestRuntime) Load(_ context.Context, request localmodels.LoadRequest) (localmodels.Handle, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.loads++
	r.lastServingEndpoint = strings.TrimSpace(request.ServingEndpoint)
	return executionTestHandle{runtime: r}, nil
}

type executionTestHandle struct {
	runtime *executionTestRuntime
}

func (h executionTestHandle) Invoke(_ context.Context, _ localmodels.InvocationRequest) (workerexecution.InferenceResponse, error) {
	h.runtime.mu.Lock()
	defer h.runtime.mu.Unlock()
	h.runtime.invocations++
	return workerexecution.InferenceResponse{Content: "ok"}, nil
}

type executionTestAssetPuller struct {
	cache localmodels.CacheLayout
}

func (p executionTestAssetPuller) PullModel(context.Context, *factoryconfig.LoadedFactoryConfig, string) (apisurface.ModelPullResult, error) {
	return apisurface.ModelPullResult{}, apisurface.ErrModelNotFound
}

func (p executionTestAssetPuller) EnsureModelAvailable(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) error {
	return nil
}

func (p executionTestAssetPuller) ResolveModelCache(context.Context, *factoryconfig.LoadedFactoryConfig, *workerconfig.Config) (localmodels.CacheLayout, error) {
	return p.cache, nil
}

func (p executionTestAssetPuller) InspectRuntimeCache(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, _ string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{
		Supported:          true,
		Installed:          true,
		CachePath:          p.cache.CachePath,
		Revision:           p.cache.Revision,
		InstalledFileCount: len(p.cache.Files),
	}, nil
}

type noopProviderRunner struct{}

func (noopProviderRunner) Execute(context.Context, workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	return workerexecution.RunnerExecutionResult{Content: "provider"}, nil
}

func TestLeaseExecution_AcquiresAndReleasesHostLeaseAroundInference(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	baseHost := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})
	host := &recordingLeaseHost{CatalogHost: baseHost}
	runtime := &executionTestRuntime{}
	cachePath := t.TempDir()
	assets := executionTestAssetPuller{
		cache: localmodels.CacheLayout{
			ModelName: "OMNIVOICE_Q4_K_M",
			CachePath: cachePath,
			Revision:  "rev-test",
			Files:     []string{cachePath + "/model.gguf", cachePath + "/tokenizer.gguf"},
		},
	}
	leaseExec := NewLeaseExecution(host, assets, runtime, localmodels.Hooks{})
	workerDef, ok := loaded.Worker("voice-local")
	if !ok {
		t.Fatal("expected voice-local worker in loaded config")
	}
	runner := leaseExec.WrapRunner(noopProviderRunner{}, loaded, loaded.FactoryConfig(), workerDef)
	if _, ok := runner.(*leaseBoundRunner); !ok {
		t.Fatalf("runner type = %T, want *leaseBoundRunner", runner)
	}
	result, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("content = %q, want host-owned inference result", result.Content)
	}
	if host.acquires != 1 || host.releases != 1 {
		t.Fatalf("lease acquire/release = %d/%d, want 1/1", host.acquires, host.releases)
	}
	if runtime.loads != 1 || runtime.invocations != 1 {
		t.Fatalf("runtime load/invoke = %d/%d, want 1/1", runtime.loads, runtime.invocations)
	}
	runtime.mu.Lock()
	servingEndpoint := runtime.lastServingEndpoint
	runtime.mu.Unlock()
	if servingEndpoint != healthServer.URL {
		t.Fatalf("serving endpoint = %q, want supervised lease endpoint %q", servingEndpoint, healthServer.URL)
	}
}

func TestLeaseExecution_AcquiresAndReleasesHostLeaseAroundAgentWorker(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	cfg := supervisedCatalogFactoryConfig()
	cfg.Workers[0].Type = workertaxonomy.WorkerTypeAgent
	loaded := mustLoadedCatalogConfig(t, cfg)
	baseHost := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})
	host := &recordingLeaseHost{CatalogHost: baseHost}
	runtime := &executionTestRuntime{}
	cachePath := t.TempDir()
	assets := executionTestAssetPuller{
		cache: localmodels.CacheLayout{
			ModelName: "OMNIVOICE_Q4_K_M",
			CachePath: cachePath,
			Revision:  "rev-test",
			Files:     []string{cachePath + "/model.gguf", cachePath + "/tokenizer.gguf"},
		},
	}
	leaseExec := NewLeaseExecution(host, assets, runtime, localmodels.Hooks{})
	workerDef, ok := loaded.Worker("voice-local")
	if !ok {
		t.Fatal("expected voice-local worker in loaded config")
	}
	if workerDef.Type != workertaxonomy.WorkerTypeAgent {
		t.Fatalf("worker type = %q, want %q", workerDef.Type, workertaxonomy.WorkerTypeAgent)
	}

	runner := leaseExec.WrapRunner(noopProviderRunner{}, loaded, loaded.FactoryConfig(), workerDef)
	if _, ok := runner.(*leaseBoundRunner); !ok {
		t.Fatalf("runner type = %T, want *leaseBoundRunner", runner)
	}
	result, err := runner.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-agent-1"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Content != "ok" {
		t.Fatalf("content = %q, want host-owned inference result", result.Content)
	}
	if host.acquires != 1 || host.releases != 1 {
		t.Fatalf("lease acquire/release = %d/%d, want 1/1", host.acquires, host.releases)
	}
}
