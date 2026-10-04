package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// localExecutor owns all Models implementation collaborators required for
// managed local invocation. Configuration and the selected scoped asset adapter
// are operation inputs so shared execution does not retain the first scope.
type localExecutor struct {
	acquire   func(context.Context, models.RuntimeScopeRef, *models.RuntimeConfig, string, string) (modelhost.Lease, error)
	release   func(context.Context, models.RuntimeScopeRef, string) error
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

func newLocalExecutorWithLeases(
	acquire func(context.Context, models.RuntimeScopeRef, *models.RuntimeConfig, string, string) (modelhost.Lease, error),
	release func(context.Context, models.RuntimeScopeRef, string) error,
	runtime localmodels.Runtime, resources *localmodels.ResourceLimiter,
	hooks modelseffects.LocalRuntimeHooks, now func() time.Time,
) (*localExecutor, error) {
	if acquire == nil || release == nil {
		return nil, missingDependencyError("local executor lease effects")
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
		acquire: acquire, release: release,
		runtime:      runtime,
		resources:    resources,
		hooks:        hooks,
		now:          now,
		entries:      make(map[localExecutionKey]*localExecutionEntry),
		closedScopes: make(map[models.RuntimeScopeRef]bool),
	}, nil
}

// ScopedLocalExecution shares fixed execution behavior and retains only scoped
// handles. Pull receives configuration and assets selected for each operation.
type ScopedLocalExecution interface {
	PullModelForScope(context.Context, models.PullModelRequest) (models.PullResult, error)
	InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error)
	CloseScope(models.RuntimeScopeRef)
	Close()
}

type scopedLocalExecution struct {
	scopes   runtimescopes.Service
	assets   scopedassets.Service
	host     runtimehost.Service
	executor *localExecutor
	sequence atomic.Uint64
}

// NewScopedLocalExecution constructs an inert owner over completed services.
func NewScopedLocalExecution(scopes runtimescopes.Service, assets scopedassets.Service,
	host runtimehost.Service, runtime localmodels.Runtime, resources *localmodels.ResourceLimiter,
	hooks modelseffects.LocalRuntimeHooks, now func() time.Time) (ScopedLocalExecution, error) {
	for _, dependency := range []struct {
		value any
		name  string
	}{
		{scopes, "local execution scopes"}, {assets, "local execution assets"}, {host, "local execution host"},
	} {
		if isNilDependency(dependency.value) {
			return nil, missingDependencyError(dependency.name)
		}
	}
	s := &scopedLocalExecution{scopes: scopes, assets: assets, host: host}
	executor, err := newLocalExecutorWithLeases(s.acquire, s.release, runtime, resources, hooks, now)
	if err != nil {
		return nil, err
	}
	s.executor = executor
	return s, nil
}

func (s *scopedLocalExecution) InvokeLocal(ctx context.Context, request models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	if err := models.ValidateLocalInvocationRequest(request); err != nil {
		return models.LocalInvocationResult{}, err
	}
	if request.Scope.IsZero() {
		return models.LocalInvocationResult{}, models.ErrRuntimeScopeInvalid
	}
	binding, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String()))
	if err != nil {
		return models.LocalInvocationResult{}, runtimeScopeError(err)
	}
	if !request.Worker.UsesManagedRuntime() {
		return models.LocalInvocationResult{}, nil
	}
	if err := s.executor.admitInvocation(ctx, request.Scope); err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	if binding.RuntimeConfig == nil {
		return models.LocalInvocationResult{Handled: true}, models.ErrUnavailable
	}
	config := binding.RuntimeConfig()
	// Configuration lookup may race close. Never begin an effect for a retired scope.
	if _, err := s.scopes.Resolve(runtimescopes.Reference(request.Scope.String())); err != nil {
		return models.LocalInvocationResult{Handled: true}, runtimeScopeError(err)
	}
	assets, err := localmodels.NewScopedAssetPuller(s.assets, request.Scope)
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	return s.executor.InvokeLocal(ctx, request, config, assets)
}

func (s *scopedLocalExecution) acquire(ctx context.Context, scope models.RuntimeScopeRef,
	config *models.RuntimeConfig, name, holder string) (modelhost.Lease, error) {
	if holder == "" {
		holder = fmt.Sprintf("local-execution-%d", s.sequence.Add(1))
	}
	ready, err := s.host.EnsureModelHost(ctx, models.EnsureModelHostRequest{Scope: scope, Name: name})
	if err != nil {
		return modelhost.Lease{}, err
	}
	if err := s.executor.admitInvocation(ctx, scope); err != nil {
		return modelhost.Lease{}, err
	}
	endpoint, err := modelhost.LocalInvocationEndpoint(config, name, ready.Host.Diagnostics)
	if err != nil {
		return modelhost.Lease{}, err
	}
	acquired, err := s.host.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{Scope: scope, Name: name, Holder: holder})
	if err != nil {
		return modelhost.Lease{}, err
	}
	return modelhost.Lease{ID: acquired.Lease.Lease.String(), Endpoint: endpoint, Holder: holder}, nil
}

func (s *scopedLocalExecution) release(ctx context.Context, scope models.RuntimeScopeRef, id string) error {
	lease, err := (models.ModelLeaseRef{}).Parse(id)
	if err != nil {
		return err
	}
	_, err = s.host.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{Scope: scope, Lease: lease})
	return err
}

func (s *scopedLocalExecution) CloseScope(scope models.RuntimeScopeRef) {
	s.executor.CloseScope(scope)
}

func (s *scopedLocalExecution) Close() { s.executor.Close() }

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
	lease, err := e.acquire(ctx, scope, runtimeConfig, worker.Model, strings.TrimSpace(holder))
	if err != nil {
		return models.LocalInvocationResult{Handled: true}, err
	}
	defer func() {
		_ = e.release(context.WithoutCancel(ctx), scope, lease.ID)
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
