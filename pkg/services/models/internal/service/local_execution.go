package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
)

// localExecutor owns all Models implementation collaborators required for
// managed local invocation. Configuration and the selected scoped asset adapter
// are operation inputs so shared execution does not retain the first scope.
type localExecutor struct {
	host      modelhost.Host
	runtime   localmodels.Runtime
	resources *localmodels.ResourceLimiter
	hooks     modelseffects.LocalRuntimeHooks
	now       func() time.Time

	mu           sync.Mutex
	entries      map[localExecutionKey]*localExecutionEntry
	closedScopes map[models.RuntimeScopeRef]bool
	closed       bool
}

type localExecutionKey struct {
	scope                                   models.RuntimeScopeRef
	resource, endpoint, cachePath, revision string
}

type localExecutionEntry struct {
	mu     sync.Mutex
	handle localmodels.Handle
}

func newLocalExecutor(
	host modelhost.Host,
	runtime localmodels.Runtime,
	resources *localmodels.ResourceLimiter,
	hooks modelseffects.LocalRuntimeHooks,
	now func() time.Time,
) (*localExecutor, error) {
	if isNilDependency(host) {
		return nil, missingDependencyError("local executor model host")
	}
	if isNilDependency(runtime) {
		return nil, missingDependencyError("local executor model runtime")
	}
	if resources == nil {
		return nil, missingDependencyError("local executor resource limiter")
	}
	if now == nil {
		return nil, missingDependencyError("local executor clock")
	}
	return &localExecutor{
		host:         host,
		runtime:      runtime,
		resources:    resources,
		hooks:        hooks,
		now:          now,
		entries:      make(map[localExecutionKey]*localExecutionEntry),
		closedScopes: make(map[models.RuntimeScopeRef]bool),
	}, nil
}

func (e *localExecutor) InvokeLocal(
	ctx context.Context,
	request models.LocalInvocationRequest,
	runtimeConfig *models.RuntimeConfig,
	assets localmodels.AssetPuller,
) (models.LocalInvocationResult, error) {
	if e == nil || !request.Worker.UsesManagedRuntime() {
		return models.LocalInvocationResult{}, nil
	}
	if err := e.admitInvocation(ctx, request.Scope); err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	if runtimeConfig == nil {
		return models.LocalInvocationResult{Handled: true}, fmt.Errorf("loaded runtime config is required for local model execution")
	}
	if isNilDependency(assets) {
		return models.LocalInvocationResult{Handled: true}, missingDependencyError("local invocation model assets")
	}
	factoryConfig, worker := localExecutionConfiguration(request)

	release, err := e.resources.Acquire(ctx, request.Scope, factoryConfig, worker)
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	if release != nil {
		defer release()
	}

	invocation := localmodels.ModelInvocation{
		Dispatch:         request.Dispatch,
		ModelOperation:   request.ModelOperation,
		ModelBindings:    append([]models.ResolvedModelOperationBinding(nil), request.ModelBindings...),
		WorkingDirectory: request.WorkingDirectory,
	}
	return e.invokeWithLease(ctx, request.Scope, runtimeConfig, assets, factoryConfig, worker, request.Holder, invocation)
}

func (e *localExecutor) invokeWithLease(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	runtimeConfig *models.RuntimeConfig,
	assets localmodels.AssetPuller,
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
		_ = e.host.ReleaseLease(context.WithoutCancel(ctx), lease.ID)
	}()
	if err := e.admitInvocation(ctx, scope); err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}

	cacheLayout, err := assets.ResolveModelCache(ctx, runtimeConfig, worker)
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	if err := e.admitInvocation(ctx, scope); err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	loadWorker := worker.Clone()
	key := localExecutionKey{scope: scope, resource: resourceKey,
		endpoint: strings.TrimSpace(lease.Endpoint), cachePath: cacheLayout.CachePath, revision: cacheLayout.Revision}
	handle, err := e.loadHandle(ctx, key, localmodels.LoadRequest{
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
	if err := e.admitInvocation(ctx, scope); err != nil {
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
	key localExecutionKey,
	request localmodels.LoadRequest,
) (localmodels.Handle, error) {
	entry, err := e.entry(key)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if err := e.admitInvocation(ctx, key.scope); err != nil {
		return nil, err
	}

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
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.closedScopes[key.scope] {
		return nil, models.ErrRuntimeScopeClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entry.handle = handle
	return handle, nil
}

func (e *localExecutor) entry(key localExecutionKey) (*localExecutionEntry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.closedScopes[key.scope] {
		return nil, models.ErrRuntimeScopeClosed
	}
	if entry, ok := e.entries[key]; ok {
		return entry, nil
	}
	entry := &localExecutionEntry{}
	e.entries[key] = entry
	return entry, nil
}

func (e *localExecutor) admitInvocation(ctx context.Context, scope models.RuntimeScopeRef) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.closedScopes[scope] {
		return models.ErrRuntimeScopeClosed
	}
	return ctx.Err()
}

func (e *localExecutor) CloseScope(scope models.RuntimeScopeRef) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closedScopes[scope] = true
	for key := range e.entries {
		if key.scope == scope {
			delete(e.entries, key)
		}
	}
}

func (e *localExecutor) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closed = true
	clear(e.entries)
	clear(e.closedScopes)
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
