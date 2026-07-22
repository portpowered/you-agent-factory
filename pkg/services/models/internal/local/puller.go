package local

import (
	"context"
	"fmt"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
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
	PullModel(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (assets.PullResult, error)
	EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error
	ResolveModelCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error)
	InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error)
}

type assetPuller struct {
	inner *assets.Puller
}

// NewAssetPuller constructs the local adapter around an asset puller
// whose exact network and source boundaries come from composition.
func NewAssetPuller(
	cacheDir string,
	hostPlatform HostPlatform,
	client assets.HTTPDoer,
	endpoints assets.Endpoints,
	makeDirectories assets.MakeDirectories,
	inspectPath assets.InspectPath,
	resolveHome assets.ResolveHomeDirectory,
	writeFile assets.WriteFile,
	renamePath assets.RenamePath,
	removePath assets.RemovePath,
	readFile assets.ReadFile,
	readDirectory assets.ReadDirectory,
	createFile assets.CreateFile,
	openFile assets.OpenFile,
) (AssetPuller, error) {
	operatingSystem := strings.TrimSpace(hostPlatform.OperatingSystem)
	if operatingSystem == "" {
		return nil, fmt.Errorf("construct local model asset puller: host operating system is required")
	}
	architecture := strings.TrimSpace(hostPlatform.Architecture)
	if architecture == "" {
		return nil, fmt.Errorf("construct local model asset puller: host architecture is required")
	}
	inner, err := assets.NewPuller(
		cacheDir, operatingSystem, architecture, client, endpoints,
		makeDirectories, inspectPath, resolveHome, writeFile, renamePath,
		removePath, readFile, readDirectory, createFile, openFile,
	)
	if err != nil {
		return nil, err
	}
	return assetPuller{inner: inner}, nil
}

func (p assetPuller) PullModel(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (assets.PullResult, error) {
	return p.inner.PullModel(ctx, runtimeCfg, modelName)
}

func (p assetPuller) EnsureModelAvailable(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) error {
	return p.inner.EnsureModelAvailable(ctx, runtimeCfg, worker)
}

func (p assetPuller) ResolveModelCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, worker *models.RuntimeWorker) (CacheLayout, error) {
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

func (p assetPuller) InspectRuntimeCache(ctx context.Context, runtimeCfg *models.RuntimeConfig, modelName string) (RuntimeCacheInspection, error) {
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
