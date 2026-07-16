package modelhost

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"

	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// LeaseExecution routes local model worker execution through host lease ownership.
type LeaseExecution struct {
	Host    Host
	Assets  localmodels.AssetPuller
	Runtime localmodels.Runtime
	Hooks   localmodels.Hooks

	mu      sync.Mutex
	entries map[string]*leaseExecutionEntry
}

type leaseExecutionEntry struct {
	mu     sync.Mutex
	handle localmodels.Handle
}

type leaseBoundRunner struct {
	execution  *LeaseExecution
	inner      workers.Runner
	runtimeCfg interfaces.RuntimeConfigLookup
	factoryCfg *interfaces.FactoryConfig
	workerDef  *workerconfig.Config
}

// NewLeaseExecution constructs a host-owned local model execution seam.
func NewLeaseExecution(host Host, assets localmodels.AssetPuller, runtime localmodels.Runtime, hooks localmodels.Hooks) *LeaseExecution {
	if host == nil || assets == nil || runtime == nil {
		return nil
	}
	return &LeaseExecution{
		Host:    host,
		Assets:  assets,
		Runtime: runtime,
		Hooks:   hooks,
		entries: make(map[string]*leaseExecutionEntry),
	}
}

// WrapRunner acquires and releases host leases around local model inference.
func (l *LeaseExecution) WrapRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *workerconfig.Config,
) workers.Runner {
	if inner == nil || l == nil || runtimeCfg == nil || factoryCfg == nil || workerDef == nil {
		return inner
	}
	if !workertaxonomy.UsesModelhostLease(workerDef.Type, workerDef.ModelLocality) {
		return inner
	}
	return &leaseBoundRunner{
		execution:  l,
		inner:      inner,
		runtimeCfg: runtimeCfg,
		factoryCfg: factoryCfg,
		workerDef:  workerDef,
	}
}

func (r *leaseBoundRunner) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	if r == nil || r.execution == nil {
		return r.inner.Execute(ctx, request)
	}
	resource, resourceKey, ok := localmodels.RuntimeResource(r.factoryCfg, r.workerDef)
	if !ok || !r.execution.Runtime.Supports(resource, r.workerDef) {
		return r.inner.Execute(ctx, request)
	}
	loaded, err := runtimeCfgForLeaseExecution(r.runtimeCfg)
	if err != nil {
		return workerexecution.RunnerExecutionResult{}, err
	}
	lease, err := r.execution.Host.AcquireLease(ctx, loaded, r.workerDef.Model, LeaseOptions{
		Holder: leaseHolderFromRequest(request),
	})
	if err != nil {
		return workerexecution.RunnerExecutionResult{}, err
	}
	defer func() {
		_ = r.execution.Host.ReleaseLease(ctx, lease.ID)
	}()

	response, err := r.execution.executeWithLease(ctx, loaded, resource, resourceKey, r.workerDef, request, lease)
	if err != nil {
		return workerexecution.RunnerExecutionResult{}, err
	}
	return response, nil
}

func (l *LeaseExecution) executeWithLease(
	ctx context.Context,
	loaded *factoryconfig.LoadedFactoryConfig,
	resource factoryresource.Config,
	resourceKey string,
	workerDef *workerconfig.Config,
	request workerexecution.RunnerExecutionRequest,
	lease Lease,
) (workerexecution.InferenceResponse, error) {
	if workerDef == nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("worker config is required for host-owned local model execution")
	}
	cacheLayout, err := l.Assets.ResolveModelCache(ctx, loaded, workerDef)
	if err != nil {
		return workerexecution.InferenceResponse{}, err
	}
	loadWorker := factoryconfig.CloneWorkerConfig(*workerDef)
	handle, err := l.loadHandle(ctx, leaseExecutionCacheKey(resourceKey, lease.Endpoint), localmodels.LoadRequest{
		Resource:        resource,
		Worker:          &loadWorker,
		ModelName:       cacheLayout.ModelName,
		CachePath:       cacheLayout.CachePath,
		Revision:        cacheLayout.Revision,
		Files:           append([]string(nil), cacheLayout.Files...),
		ServingEndpoint: strings.TrimSpace(lease.Endpoint),
	})
	if err != nil {
		return workerexecution.InferenceResponse{}, err
	}
	invokeWorker := factoryconfig.CloneWorkerConfig(*workerDef)
	return handle.Invoke(ctx, localmodels.InvocationRequest{
		Resource: resource,
		Worker:   &invokeWorker,
		Request:  workerexecution.CloneProviderInferenceRequest(request),
	})
}

func (l *LeaseExecution) loadHandle(ctx context.Context, key string, request localmodels.LoadRequest) (localmodels.Handle, error) {
	entry := l.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.handle != nil {
		if l.Hooks.MarkLoadReused != nil {
			l.Hooks.MarkLoadReused(ctx)
		}
		return entry.handle, nil
	}
	if l.Hooks.MarkLoadRequested != nil {
		l.Hooks.MarkLoadRequested(ctx, time.Now())
	}
	handle, err := l.Runtime.Load(ctx, request)
	if err != nil {
		if l.Hooks.MarkLoadFinished != nil {
			l.Hooks.MarkLoadFinished(ctx, time.Now())
		}
		return nil, err
	}
	if l.Hooks.MarkLoadFinished != nil {
		l.Hooks.MarkLoadFinished(ctx, time.Now())
	}
	entry.handle = handle
	return entry.handle, nil
}

func (l *LeaseExecution) entry(key string) *leaseExecutionEntry {
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if ok {
		return entry
	}
	entry = &leaseExecutionEntry{}
	l.entries[key] = entry
	return entry
}

func runtimeCfgForLeaseExecution(runtimeCfg interfaces.RuntimeConfigLookup) (*factoryconfig.LoadedFactoryConfig, error) {
	loaded, ok := runtimeCfg.(*factoryconfig.LoadedFactoryConfig)
	if !ok || loaded == nil {
		return nil, fmt.Errorf("loaded runtime config is required for host-owned local model execution")
	}
	return loaded, nil
}

func leaseHolderFromRequest(request workerexecution.RunnerExecutionRequest) string {
	dispatchID := strings.TrimSpace(request.Dispatch.DispatchID)
	if dispatchID != "" {
		return dispatchID
	}
	return strings.TrimSpace(request.Dispatch.WorkstationName)
}

func leaseExecutionCacheKey(resourceKey, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return resourceKey
	}
	return resourceKey + "|" + endpoint
}
