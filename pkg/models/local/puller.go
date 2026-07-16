package local

import (
	"context"
	"runtime"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/models/assets"
)

// AssetPuller resolves managed local model assets and pull outcomes.
type AssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (assets.PullResult, error)
	EnsureModelAvailable(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *workerconfig.Config) error
	ResolveModelCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *workerconfig.Config) (CacheLayout, error)
	InspectRuntimeCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (RuntimeCacheInspection, error)
}

type assetPuller struct {
	inner *assets.Puller
}

// NewAssetPuller constructs a managed local-model asset puller for the given cache directory.
func NewAssetPuller(cacheDir string) AssetPuller {
	return assetPuller{inner: assets.NewPuller(cacheDir, runtime.GOOS, runtime.GOARCH)}
}

func (p assetPuller) PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (assets.PullResult, error) {
	return p.inner.PullModel(ctx, runtimeCfg, modelName)
}

func (p assetPuller) EnsureModelAvailable(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *workerconfig.Config) error {
	return p.inner.EnsureModelAvailable(ctx, runtimeCfg, worker)
}

func (p assetPuller) ResolveModelCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *workerconfig.Config) (CacheLayout, error) {
	layout, err := p.inner.ResolveModelCache(ctx, runtimeCfg, worker)
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

func (p assetPuller) InspectRuntimeCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (RuntimeCacheInspection, error) {
	inspection, err := p.inner.InspectRuntimeCache(ctx, runtimeCfg, modelName)
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
