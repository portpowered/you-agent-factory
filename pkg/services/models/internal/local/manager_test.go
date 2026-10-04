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

func TestResourceLimiterScopeCapacityAndIdempotentRelease(t *testing.T) {
	t.Parallel()
	limiter, err := NewResourceLimiter(Hooks{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scopeA := resourceTestScope(t, "factory-session:capacity:a")
	scopeB := resourceTestScope(t, "factory-session:capacity:b")
	configA, workerA := resourceTestConfiguration(1)
	configB, workerB := resourceTestConfiguration(2)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	releaseA := acquireTestResources(t, limiter, ctx, scopeA, configA, workerA)
	defer releaseA()
	// Identical model/backend/load-policy names do not share capacity or the
	// lower capacity chosen by A.
	releaseB := acquireTestResources(t, limiter, ctx, scopeB, configB, workerB)
	defer releaseB()
	assertResourceCapacityFull(t, limiter, scopeA, configA, workerA)
	assertResourceCapacityFull(t, limiter, scopeB, configB, workerB)

	releaseA()
	releaseNextA := acquireTestResources(t, limiter, ctx, scopeA, configA, workerA)
	defer releaseNextA()
	releaseA() // Must not release the newer reservation.
	assertResourceCapacityFull(t, limiter, scopeA, configA, workerA)
	assertResourceCapacityFull(t, limiter, scopeB, configB, workerB)

	releaseNextA()
	releaseB()
	for range 3 {
		release := acquireTestResources(t, limiter, ctx, scopeB, configB, workerB)
		release()
	}
}

func TestResourceLimiterScopeCancellationRollsBackPartialReservations(t *testing.T) {
	t.Parallel()
	var outcomes []bool
	limiter, err := NewResourceLimiter(Hooks{
		MarkResourceWaitFinished: func(_ context.Context, _ time.Time, success bool) {
			outcomes = append(outcomes, success)
		},
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scope := resourceTestScope(t, "factory-session:capacity:rollback")
	config, worker := resourceTestConfiguration(1)
	second := config.Resources[0]
	second.Name, second.Model = "second", "SECOND_MODEL"
	config.Resources = append(config.Resources, second)
	worker.Resources = append(worker.Resources, apisurface.RuntimeResource{Name: second.Name, Capacity: 1})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	secondWorker := *worker
	secondWorker.Resources = secondWorker.Resources[1:]
	releaseSecond := acquireTestResources(t, limiter, ctx, scope, config, &secondWorker)
	defer releaseSecond()
	canceled, cancelWait := context.WithCancel(ctx)
	cancelWait()
	// The first resource is free; the second is held. The cancelled wait must
	// roll back the first reservation before returning.
	release, err := limiter.Acquire(canceled, scope, config, worker)
	if !errors.Is(err, context.Canceled) || release != nil {
		t.Fatalf("partial acquisition = %v, has release=%t; want cancellation without release", err, release != nil)
	}
	firstWorker := *worker
	firstWorker.Resources = firstWorker.Resources[:1]
	releaseFirst := acquireTestResources(t, limiter, ctx, scope, config, &firstWorker)
	defer releaseFirst()
	assertResourceCapacityFull(t, limiter, scope, config, &secondWorker)
	releaseFirst()
	releaseSecond()
	release = acquireTestResources(t, limiter, ctx, scope, config, worker)
	release()
	if len(outcomes) != 5 || outcomes[0] != true || outcomes[1] != false ||
		outcomes[2] != true || outcomes[3] != false || outcomes[4] != true {
		t.Fatalf("resource wait outcomes = %v, want [true false true false true]", outcomes)
	}
}

func TestResourceLimiterScopeWaitingAcquisitionResumesAfterRelease(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 1)
	limiter, err := NewResourceLimiter(Hooks{
		MarkResourceWaitStarted: func(_ context.Context, _ time.Time) { started <- struct{}{} },
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	scope := resourceTestScope(t, "factory-session:capacity:waiting")
	config, worker := resourceTestConfiguration(1)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	release := acquireTestResources(t, limiter, ctx, scope, config, worker)
	defer release()
	<-started
	type acquisition struct {
		release func()
		err     error
	}
	completed := make(chan acquisition, 1)
	go func() {
		release, err := limiter.Acquire(ctx, scope, config, worker)
		completed <- acquisition{release: release, err: err}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("second acquisition did not start")
	}
	release()
	select {
	case result := <-completed:
		if result.err != nil || result.release == nil {
			t.Fatalf("waiting acquisition = %v, has release=%t", result.err, result.release != nil)
		}
		result.release()
	case <-ctx.Done():
		t.Fatal("release did not resume waiting acquisition")
	}
}

func resourceTestScope(t *testing.T, value string) apisurface.RuntimeScopeRef {
	t.Helper()
	scope, err := (apisurface.RuntimeScopeRef{}).Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

func resourceTestConfiguration(capacity int) (*apisurface.RuntimeConfig, *apisurface.RuntimeWorker) {
	return &apisurface.RuntimeConfig{Resources: []apisurface.RuntimeResource{{
			Name: "model", Type: apisurface.RuntimeResourceTypeModel, Model: "OMNIVOICE_Q4_K_M",
			Backend: "LLAMACPP", LoadPolicy: "ON_DEMAND", Capacity: capacity,
		}}}, &apisurface.RuntimeWorker{
			ModelLocality: apisurface.RuntimeModelLocalityLocal,
			Resources:     []apisurface.RuntimeResource{{Name: "model", Capacity: capacity}},
		}
}

func acquireTestResources(t *testing.T, limiter *ResourceLimiter, ctx context.Context,
	scope apisurface.RuntimeScopeRef, config *apisurface.RuntimeConfig, worker *apisurface.RuntimeWorker,
) func() {
	t.Helper()
	release, err := limiter.Acquire(ctx, scope, config, worker)
	if err != nil || release == nil {
		t.Fatalf("Acquire(%s) = %v, has release=%t; want reservation", scope.String(), err, release != nil)
	}
	return release
}

func assertResourceCapacityFull(t *testing.T, limiter *ResourceLimiter, scope apisurface.RuntimeScopeRef,
	config *apisurface.RuntimeConfig, worker *apisurface.RuntimeWorker,
) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if release, err := limiter.Acquire(ctx, scope, config, worker); !errors.Is(err, context.Canceled) || release != nil {
		if release != nil {
			release()
		}
		t.Fatalf("full capacity acquisition = %v, has release=%t; want cancellation", err, release != nil)
	}
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
	return RuntimeCacheInspection{
		Supported: true,
		Installed: true,
	}, nil
}

type countingLocalRuntime struct {
	mu    sync.Mutex
	loads int
}

func (r *countingLocalRuntime) Supports(resource modelRuntimeResource, worker *modelRuntimeWorker) bool {
	return apisurface.IsManagedRuntimeBackend(resource.Backend) && CanonicalModelName(worker.Model) == CanonicalModelName("OMNIVOICE_Q4_K_M")
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
