package localmodels

import (
	"context"
	"runtime"

	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/localmodels/assets"
)

// AssetPuller resolves managed local model assets and pull outcomes.
type AssetPuller interface {
	PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (apisurface.ModelPullResult, error)
	EnsureModelAvailable(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) error
	ResolveModelCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) (CacheLayout, error)
}

type assetPuller struct {
	inner *assets.Puller
}

// NewAssetPuller constructs a managed local-model asset puller for the given cache directory.
func NewAssetPuller(cacheDir string) AssetPuller {
	return assetPuller{inner: assets.NewPuller(cacheDir, runtime.GOOS, runtime.GOARCH)}
}

func (p assetPuller) PullModel(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, modelName string) (apisurface.ModelPullResult, error) {
	return p.inner.PullModel(ctx, runtimeCfg, modelName)
}

func (p assetPuller) EnsureModelAvailable(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) error {
	return p.inner.EnsureModelAvailable(ctx, runtimeCfg, worker)
}

func (p assetPuller) ResolveModelCache(ctx context.Context, runtimeCfg *factoryconfig.LoadedFactoryConfig, worker *interfaces.WorkerConfig) (CacheLayout, error) {
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
