package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// LocalModelDomain wires local-model runtime dependencies constructed at host
// build time and copied onto each Bundle.
type LocalModelDomain struct {
	Resources      *localmodels.ResourceLimiter
	Assets         localmodels.AssetPuller
	Runtime        localmodels.Runtime
	Manager        *localmodels.Manager
	Host           modelhost.Host
	LeaseExecution *modelhost.LeaseExecution
}

// NewLocalModelDomain constructs local-model collaborators for one bundle build.
func NewLocalModelDomain(cfg Config) (LocalModelDomain, error) {
	hooks := cfg.LocalModelHooks
	if hooks.MarkResourceWaitStarted == nil {
		hooks = LocalModelHooks()
	}
	modelResources := localmodels.NewResourceLimiter(hooks)
	modelAssets := localmodels.NewAssetPuller(cfg.ModelCacheDir)
	if cfg.ModelAssetsOverride != nil {
		modelAssets = cfg.ModelAssetsOverride
	}
	localModelRuntime := cfg.LocalModelRuntimeOverride
	if localModelRuntime == nil {
		localModelRuntime = localmodels.DefaultRuntime()
	}
	manager, err := localmodels.NewManagedRuntime(localmodels.ManagedRuntimeDependencies{
		AssetPuller: modelAssets, Runtime: localModelRuntime, Hooks: hooks,
	})
	if err != nil {
		return LocalModelDomain{}, fmt.Errorf("construct managed local runtime: %w", err)
	}
	host := cfg.ModelHostOverride
	if host == nil {
		gateway := modelhost.NewLocalAssetGateway(modelAssets)
		host, err = modelhost.NewHost(modelhost.Dependencies{
			AssetPuller: gateway, CacheInspector: gateway,
			ProcessLauncher: modelhost.DefaultProcessLauncher(),
			Options: modelhost.Options{
				SourceResolver: modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
				Diagnostics:    ModelHostDiagnostics(cfg),
			},
		})
		if err != nil {
			return LocalModelDomain{}, fmt.Errorf("construct model host: %w", err)
		}
	}
	domain := LocalModelDomain{
		Resources: modelResources,
		Assets:    modelAssets,
		Runtime:   localModelRuntime,
		Manager:   manager,
		Host:      host,
	}
	domain.LeaseExecution = modelhost.NewLeaseExecution(host, modelAssets, localModelRuntime, hooks)
	return domain, nil
}

func localModelHooks() localmodels.Hooks {
	return LocalModelHooks()
}

// LocalModelHooks returns hooks that annotate model execution traces for event recording.
func LocalModelHooks() localmodels.Hooks {
	return localmodels.Hooks{
		MarkResourceWaitStarted:  markModelExecutionResourceWaitStarted,
		MarkResourceWaitFinished: markModelExecutionResourceWaitFinished,
		MarkLoadRequested:        markModelExecutionLoadRequested,
		MarkLoadFinished:         markModelExecutionLoadFinished,
		MarkLoadReused:           markModelExecutionLoadReused,
	}
}

// WrapLocalModelRunner applies managed-runtime lease wrapping for model workers.
func WrapLocalModelRunner(
	inner workers.Runner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	factoryCfg *interfaces.FactoryConfig,
	workerDef *interfaces.WorkerConfig,
	modelDomain LocalModelDomain,
) workers.Runner {
	if modelDomain.LeaseExecution != nil {
		return modelDomain.LeaseExecution.WrapRunner(inner, runtimeCfg, factoryCfg, workerDef)
	}
	if modelDomain.Host != nil && modelDomain.Runtime != nil && modelDomain.Assets != nil {
		if leaseExec := modelhost.NewLeaseExecution(
			modelDomain.Host,
			modelDomain.Assets,
			modelDomain.Runtime,
			localModelHooks(),
		); leaseExec != nil {
			return leaseExec.WrapRunner(inner, runtimeCfg, factoryCfg, workerDef)
		}
	}
	if modelDomain.Manager != nil {
		return modelDomain.Manager.WrapRunner(inner, runtimeCfg, factoryCfg, workerDef)
	}
	return inner
}

type modelExecutionEventTrace struct {
	mu sync.Mutex

	resourceWaitStartedAt time.Time
	resourceWaitMillis    int64
	resourceAcquired      bool

	loadRequested bool
	loadReused    bool
	loadStartedAt time.Time
	loadMillis    int64
}

type modelExecutionEventTraceKey struct{}

func modelExecutionTraceFromContext(ctx context.Context) *modelExecutionEventTrace {
	if ctx == nil {
		return nil
	}
	trace, _ := ctx.Value(modelExecutionEventTraceKey{}).(*modelExecutionEventTrace)
	return trace
}

func markModelExecutionResourceWaitStarted(ctx context.Context, startedAt time.Time) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.resourceWaitStartedAt = startedAt
	trace.mu.Unlock()
}

func markModelExecutionResourceWaitFinished(ctx context.Context, finishedAt time.Time, acquired bool) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	if !trace.resourceWaitStartedAt.IsZero() {
		trace.resourceWaitMillis = finishedAt.Sub(trace.resourceWaitStartedAt).Milliseconds()
	}
	trace.resourceAcquired = acquired
	trace.mu.Unlock()
}

func markModelExecutionLoadRequested(ctx context.Context, startedAt time.Time) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.loadRequested = true
	trace.loadStartedAt = startedAt
	trace.mu.Unlock()
}

func markModelExecutionLoadFinished(ctx context.Context, finishedAt time.Time) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	if !trace.loadStartedAt.IsZero() {
		trace.loadMillis = finishedAt.Sub(trace.loadStartedAt).Milliseconds()
	}
	trace.mu.Unlock()
}

func markModelExecutionLoadReused(ctx context.Context) {
	trace := modelExecutionTraceFromContext(ctx)
	if trace == nil {
		return
	}
	trace.mu.Lock()
	trace.loadRequested = true
	trace.loadReused = true
	trace.mu.Unlock()
}
