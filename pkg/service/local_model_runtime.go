package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
)

type localModelCacheLayout struct {
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

type localModelLoadRequest struct {
	Resource  interfaces.ResourceConfig
	Worker    *interfaces.WorkerConfig
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

type localModelInvocationRequest struct {
	Resource interfaces.ResourceConfig
	Worker   *interfaces.WorkerConfig
	Request  interfaces.RunnerExecutionRequest
}

type localModelHandle interface {
	Invoke(context.Context, localModelInvocationRequest) (interfaces.InferenceResponse, error)
}

type localModelRuntime interface {
	Supports(resource interfaces.ResourceConfig, worker *interfaces.WorkerConfig) bool
	Load(context.Context, localModelLoadRequest) (localModelHandle, error)
}

type managedLocalModelManager struct {
	mu          sync.Mutex
	entries     map[string]*managedLocalModelEntry
	assetPuller modelAssetPuller
	runtime     localModelRuntime
}

type managedLocalModelEntry struct {
	mu     sync.Mutex
	handle localModelHandle
}

type localModelRunner struct {
	inner      workers.Runner
	manager    *managedLocalModelManager
	runtimeCfg interfaces.RuntimeConfigLookup
	factoryCfg *interfaces.FactoryConfig
	workerDef  *interfaces.WorkerConfig
}

func newManagedLocalModelManager(assetPuller modelAssetPuller, runtime localModelRuntime) *managedLocalModelManager {
	if assetPuller == nil || runtime == nil {
		return nil
	}
	return &managedLocalModelManager{
		entries:     make(map[string]*managedLocalModelEntry),
		assetPuller: assetPuller,
		runtime:     runtime,
	}
}

func (m *managedLocalModelManager) wrapRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
) workers.Runner {
	if inner == nil || m == nil || runtimeCfg == nil || factoryCfg == nil || workerDef == nil {
		return inner
	}
	if workerDef.Type != interfaces.WorkerTypeModel || workerDef.ModelLocality != interfaces.ModelLocalityLocal {
		return inner
	}
	return &localModelRunner{
		inner:      inner,
		manager:    m,
		runtimeCfg: runtimeCfg,
		factoryCfg: factoryCfg,
		workerDef:  workerDef,
	}
}

func (r *localModelRunner) Execute(ctx context.Context, request interfaces.RunnerExecutionRequest) (interfaces.RunnerExecutionResult, error) {
	if r == nil || r.manager == nil {
		return r.inner.Execute(ctx, request)
	}
	response, handled, err := r.manager.execute(ctx, r.runtimeCfg, r.factoryCfg, r.workerDef, request)
	if !handled {
		return r.inner.Execute(ctx, request)
	}
	return response, err
}

func (m *managedLocalModelManager) execute(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	request interfaces.RunnerExecutionRequest,
) (interfaces.InferenceResponse, bool, error) {
	resource, resourceKey, ok := localModelRuntimeResource(factoryCfg, workerDef)
	if !ok || !m.runtime.Supports(resource, workerDef) {
		return interfaces.InferenceResponse{}, false, nil
	}
	loaded, err := runtimeCfgForLocalModel(runtimeCfg)
	if err != nil {
		return interfaces.InferenceResponse{}, true, err
	}
	cacheLayout, err := m.assetPuller.ResolveModelCache(ctx, loaded, workerDef)
	if err != nil {
		return interfaces.InferenceResponse{}, true, err
	}
	handle, err := m.loadHandle(ctx, resourceKey, localModelLoadRequest{
		Resource:  resource,
		Worker:    cloneWorkerForLocalModel(workerDef),
		ModelName: cacheLayout.ModelName,
		CachePath: cacheLayout.CachePath,
		Revision:  cacheLayout.Revision,
		Files:     append([]string(nil), cacheLayout.Files...),
	})
	if err != nil {
		return interfaces.InferenceResponse{}, true, err
	}
	response, err := handle.Invoke(ctx, localModelInvocationRequest{
		Resource: resource,
		Worker:   cloneWorkerForLocalModel(workerDef),
		Request:  interfaces.CloneProviderInferenceRequest(request),
	})
	return response, true, err
}

func runtimeCfgForLocalModel(runtimeCfg interfaces.RuntimeConfigLookup) (*factoryconfig.LoadedFactoryConfig, error) {
	loaded, ok := runtimeCfg.(*factoryconfig.LoadedFactoryConfig)
	if !ok || loaded == nil {
		return nil, fmt.Errorf("loaded runtime config is required for local model execution")
	}
	return loaded, nil
}

func (m *managedLocalModelManager) loadHandle(ctx context.Context, key string, request localModelLoadRequest) (localModelHandle, error) {
	entry := m.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.handle != nil {
		markModelExecutionLoadReused(ctx)
		return entry.handle, nil
	}
	markModelExecutionLoadRequested(ctx, time.Now())
	handle, err := m.runtime.Load(ctx, request)
	if err != nil {
		markModelExecutionLoadFinished(ctx, time.Now())
		return nil, err
	}
	markModelExecutionLoadFinished(ctx, time.Now())
	entry.handle = handle
	return handle, nil
}

func (m *managedLocalModelManager) entry(key string) *managedLocalModelEntry {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.entries[key]
	if ok {
		return entry
	}
	entry = &managedLocalModelEntry{}
	m.entries[key] = entry
	return entry
}

func localModelRuntimeResource(factoryCfg *interfaces.FactoryConfig, workerDef *interfaces.WorkerConfig) (interfaces.ResourceConfig, string, bool) {
	for _, match := range eligibleLocalModelResourceMatches(factoryCfg, workerDef) {
		return match.resource, match.key, true
	}
	return interfaces.ResourceConfig{}, "", false
}

func cloneWorkerForLocalModel(workerDef *interfaces.WorkerConfig) *interfaces.WorkerConfig {
	if workerDef == nil {
		return nil
	}
	clone := *workerDef
	clone.Args = append([]string(nil), workerDef.Args...)
	clone.Resources = append([]interfaces.ResourceConfig(nil), workerDef.Resources...)
	clone.Operations = append([]interfaces.ModelOperation(nil), workerDef.Operations...)
	return &clone
}

func canonicalBackendName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
