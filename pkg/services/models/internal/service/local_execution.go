package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
)

// localExecutor owns all Models implementation collaborators required for
// managed local invocation.
type localExecutor struct {
	runtimeConfig models.RuntimeConfigLoader
	host          modelhost.Host
	assets        localmodels.AssetPuller
	runtime       localmodels.Runtime
	manager       *localmodels.Manager
	resources     *localmodels.ResourceLimiter
	hooks         models.LocalRuntimeHooks
	now           func() time.Time

	mu      sync.Mutex
	entries map[string]*localExecutionEntry
}

type localExecutionEntry struct {
	mu     sync.Mutex
	handle localmodels.Handle
}

func newLocalExecutor(
	runtimeConfig models.RuntimeConfigLoader,
	host modelhost.Host,
	assets localmodels.AssetPuller,
	runtime localmodels.Runtime,
	manager *localmodels.Manager,
	resources *localmodels.ResourceLimiter,
	hooks models.LocalRuntimeHooks,
	now func() time.Time,
) (*localExecutor, error) {
	if runtimeConfig == nil {
		return nil, missingDependencyError("local executor runtime configuration lookup")
	}
	if now == nil {
		return nil, missingDependencyError("local executor clock")
	}
	return &localExecutor{
		runtimeConfig: runtimeConfig,
		host:          host,
		assets:        assets,
		runtime:       runtime,
		manager:       manager,
		resources:     resources,
		hooks:         hooks,
		now:           now,
		entries:       make(map[string]*localExecutionEntry),
	}, nil
}

func (e *localExecutor) InvokeLocal(
	ctx context.Context,
	request models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	if e == nil || !request.Worker.UsesManagedRuntime() {
		return models.LocalInvocationResult{}, nil
	}
	runtimeConfig := e.runtimeConfig()
	if runtimeConfig == nil {
		return models.LocalInvocationResult{Handled: true}, fmt.Errorf("loaded runtime config is required for local model execution")
	}
	factoryConfig, worker := localExecutionConfiguration(request)

	if e.resources != nil {
		release, err := e.resources.Acquire(ctx, factoryConfig, worker)
		if err != nil {
			return models.LocalInvocationResult{Handled: true}, err
		}
		if release != nil {
			defer release()
		}
	}

	invocation := localmodels.ModelInvocation{
		Dispatch:         request.Dispatch,
		ModelOperation:   request.ModelOperation,
		ModelBindings:    append([]models.ResolvedModelOperationBinding(nil), request.ModelBindings...),
		WorkingDirectory: request.WorkingDirectory,
	}
	if e.host != nil && e.assets != nil && e.runtime != nil {
		return e.invokeWithLease(ctx, runtimeConfig, factoryConfig, worker, request.Holder, invocation)
	}
	if e.manager == nil {
		return models.LocalInvocationResult{}, nil
	}
	response, handled, err := e.manager.Invoke(ctx, runtimeConfig, factoryConfig, worker, invocation)
	return models.LocalInvocationResult{Handled: handled, Content: response.Content}, err
}

func (e *localExecutor) invokeWithLease(
	ctx context.Context,
	runtimeConfig *models.RuntimeConfig,
	factoryConfig *models.RuntimeConfig,
	worker *models.RuntimeWorker,
	holder string,
	invocation localmodels.ModelInvocation,
) (models.LocalInvocationResult, error) {
	resource, resourceKey, ok := localmodels.RuntimeResource(factoryConfig, worker)
	if !ok || !e.runtime.Supports(resource, worker) {
		return models.LocalInvocationResult{}, nil
	}
	lease, err := e.host.AcquireLease(ctx, runtimeConfig, worker.Model, modelhost.LeaseOptions{
		Holder: strings.TrimSpace(holder),
	})
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	defer func() {
		_ = e.host.ReleaseLease(ctx, lease.ID)
	}()

	cacheLayout, err := e.assets.ResolveModelCache(ctx, runtimeConfig, worker)
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	loadWorker := worker.Clone()
	handle, err := e.loadHandle(ctx, localExecutionCacheKey(resourceKey, lease.Endpoint), localmodels.LoadRequest{
		Resource:        resource,
		Worker:          &loadWorker,
		ModelName:       cacheLayout.ModelName,
		CachePath:       cacheLayout.CachePath,
		Revision:        cacheLayout.Revision,
		Files:           append([]string(nil), cacheLayout.Files...),
		ServingEndpoint: strings.TrimSpace(lease.Endpoint),
	})
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	invokeWorker := worker.Clone()
	response, err := handle.Invoke(ctx, localmodels.InvocationRequest{
		Resource: resource,
		Worker:   &invokeWorker,
		Request:  invocation,
	})
	return models.LocalInvocationResult{Handled: true, Content: response.Content}, err
}

func (e *localExecutor) loadHandle(
	ctx context.Context,
	key string,
	request localmodels.LoadRequest,
) (localmodels.Handle, error) {
	entry := e.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.handle != nil {
		if e.hooks.MarkLoadReused != nil {
			e.hooks.MarkLoadReused(ctx)
		}
		return entry.handle, nil
	}
	if e.hooks.MarkLoadRequested != nil {
		e.hooks.MarkLoadRequested(ctx, e.now())
	}
	handle, err := e.runtime.Load(ctx, request)
	if e.hooks.MarkLoadFinished != nil {
		e.hooks.MarkLoadFinished(ctx, e.now())
	}
	if err != nil {
		return nil, err
	}
	entry.handle = handle
	return handle, nil
}

func (e *localExecutor) entry(key string) *localExecutionEntry {
	e.mu.Lock()
	defer e.mu.Unlock()
	if entry, ok := e.entries[key]; ok {
		return entry
	}
	entry := &localExecutionEntry{}
	e.entries[key] = entry
	return entry
}

func localExecutionConfiguration(
	request models.LocalInvocationRequest,
) (*models.RuntimeConfig, *models.RuntimeWorker) {
	factoryConfig := &models.RuntimeConfig{
		Resources: localResourceConfigs(request.Resources),
	}
	worker := &models.RuntimeWorker{
		Name:          request.Worker.Name,
		Type:          request.Worker.Type,
		Model:         request.Worker.Model,
		ModelLocality: request.Worker.ModelLocality,
		Resources:     localResourceConfigs(request.Worker.Resources),
	}
	return factoryConfig, worker
}

func localResourceConfigs(resources []models.LocalResource) []models.RuntimeResource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]models.RuntimeResource, len(resources))
	for index, resource := range resources {
		result[index] = models.RuntimeResource{
			ID: resource.ID, Name: resource.Name, Type: resource.Type,
			Capacity: resource.Capacity, Model: resource.Model,
			Backend: resource.Backend, LoadPolicy: resource.LoadPolicy,
			Provider: resource.Provider,
		}
	}
	return result
}

func localExecutionCacheKey(resourceKey, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return resourceKey
	}
	return resourceKey + "|" + endpoint
}
