package local

import (
	"context"
	"fmt"
	"strings"
	"sync"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	assets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
)

// HostPlatform identifies the operating-system and architecture pair used to
// select compatible managed-model assets. Composition observes this process
// fact and injects it; the Models service never reads process globals.
type HostPlatform struct {
	OperatingSystem string
	Architecture    string
}

// AssetPuller resolves managed local model assets and pull outcomes.
type AssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (models.PullResult, error)
	EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error
	ResolveModelCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error)
	InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error)
}

type assetPuller struct {
	inner        assets.Service
	resolveScope func() (models.RuntimeScopeRef, error)
	scopeMu      sync.Mutex
	scope        models.RuntimeScopeRef
}

// NewScopedAssetPuller adapts the already-constructed Models Assets service to
// the existing local runtime without constructing another puller or effect
// bundle for a selected Factory.
func NewScopedAssetPuller(inner assets.Service, scope models.RuntimeScopeRef) (AssetPuller, error) {
	if inner == nil {
		return nil, fmt.Errorf("construct local model asset adapter: Models Assets service is required")
	}
	if scope.IsZero() {
		return nil, fmt.Errorf("construct local model asset adapter: runtime scope is required")
	}
	return &assetPuller{
		inner: inner,
		resolveScope: func() (models.RuntimeScopeRef, error) {
			return scope, nil
		},
	}, nil
}

// NewDeferredScopedAssetPuller creates a compatibility adapter whose immutable
// runtime scope is opened on first use. Factory Session assembly can therefore
// retain its existing deferred runtime-config lookup without snapshotting it
// before the runtime exists.
func NewDeferredScopedAssetPuller(
	inner assets.Service,
	resolveScope func() (models.RuntimeScopeRef, error),
) (AssetPuller, error) {
	if inner == nil {
		return nil, fmt.Errorf("construct local model asset adapter: Models Assets service is required")
	}
	if resolveScope == nil {
		return nil, fmt.Errorf("construct local model asset adapter: runtime scope resolver is required")
	}
	return &assetPuller{inner: inner, resolveScope: resolveScope}, nil
}

func (p *assetPuller) PullModel(ctx context.Context, _ *models.RuntimeConfig, modelName string) (models.PullResult, error) {
	scope, err := p.currentScope()
	if err != nil {
		return models.PullResult{}, err
	}
	result, err := p.inner.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	projected := pullResultFromAssets(result)
	if err != nil {
		projected.ManagedPullOutcome = ""
		projected.ReadinessState = "FAILED"
		projected.LifecycleState = "NOT_INSTALLED"
		return projected, err
	}
	layout, layoutErr := p.inner.ResolveRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if layoutErr != nil {
		return projected, layoutErr
	}
	projected.CachePath = layout.CachePath
	return projected, nil
}

func (p *assetPuller) EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error {
	_, err := p.ResolveModelCache(ctx, runtimeCfg, worker)
	return err
}

func (p *assetPuller) ResolveModelCache(ctx context.Context, _ *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error) {
	if worker == nil || strings.TrimSpace(worker.ModelLocality) != models.RuntimeModelLocalityLocal {
		return CacheLayout{}, nil
	}
	scope, err := p.currentScope()
	if err != nil {
		return CacheLayout{}, err
	}
	layout, err := p.inner.ResolveRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  worker.Model,
	})
	if err != nil {
		return CacheLayout{}, err
	}
	return CacheLayout{
		ModelName: layout.ModelName,
		CachePath: layout.CachePath,
		Revision:  layout.Revision,
		Files:     layout.Files,
	}, nil
}

func (p *assetPuller) InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error) {
	if runtimeCfg == nil {
		return RuntimeCacheInspection{}, fmt.Errorf("runtime config is not available")
	}
	if modelScopedResource(runtimeCfg, modelName) == nil {
		return RuntimeCacheInspection{}, nil
	}
	scope, err := p.currentScope()
	if err != nil {
		return RuntimeCacheInspection{}, err
	}
	inspection, err := p.inner.InspectRuntimeCache(ctx, models.InspectModelAssetsRequest{
		Scope: scope,
		Name:  modelName,
	})
	if err != nil {
		return RuntimeCacheInspection{}, err
	}
	return RuntimeCacheInspection{
		Supported:          inspection.Supported,
		Installed:          inspection.Installed,
		Revision:           inspection.Revision,
		CachePath:          inspection.CachePath,
		InstalledFileCount: inspection.InstalledFileCount,
		MissingAssets:      inspection.MissingAssets,
		PartialArtifacts:   inspection.PartialArtifacts,
	}, nil
}

func (p *assetPuller) currentScope() (models.RuntimeScopeRef, error) {
	if p == nil || p.resolveScope == nil {
		return models.RuntimeScopeRef{}, fmt.Errorf("Models runtime scope is unavailable")
	}
	p.scopeMu.Lock()
	defer p.scopeMu.Unlock()
	if !p.scope.IsZero() {
		return p.scope, nil
	}
	scope, err := p.resolveScope()
	if err != nil {
		return models.RuntimeScopeRef{}, err
	}
	if scope.IsZero() {
		return models.RuntimeScopeRef{}, fmt.Errorf("Models runtime scope is unavailable")
	}
	p.scope = scope
	return scope, nil
}

func pullResultFromAssets(result models.PrepareModelAssetsResult) models.PullResult {
	files := make([]models.DownloadedFile, 0, len(result.Asset.Artifacts))
	for _, artifact := range result.Asset.Artifacts {
		files = append(files, models.DownloadedFile{
			Path: artifact.Name, Bytes: artifact.Bytes, SHA256: artifact.SHA256,
		})
	}
	outcome := "PULLED"
	managedOutcome := "INSTALLED_SUCCESSFULLY"
	if result.Outcome == models.AssetPreparationAlreadyAvailable {
		outcome = "ALREADY_PRESENT"
		managedOutcome = "ALREADY_READY"
	}
	return models.PullResult{
		ModelName:          result.Asset.ModelName,
		ProviderLocality:   models.RuntimeModelLocalityLocal,
		Outcome:            outcome,
		Revision:           result.Asset.Revision,
		DownloadedFiles:    files,
		ManagedPullOutcome: managedOutcome,
		ReadinessState:     "READY",
		LifecycleState:     "INSTALLED",
		SourceKind:         result.Asset.Source.Provider,
		SourceID:           result.Asset.Source.Reference,
	}
}
