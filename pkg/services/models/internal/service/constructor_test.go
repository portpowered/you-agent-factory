package service_test

import (
	"context"
	"go.uber.org/zap"
	"testing"
	"time"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
)

type modelServiceFixture struct {
	RuntimeConfig    func() *modelRuntimeConfig
	ModelHost        modelhost.Host
	ModelAssetPuller localmodels.AssetPuller
	Logger           *zap.Logger
	Clock            func() time.Time
	ModelPullMetrics models.PullMetricsRecorder
}

func mustConstructModelService(t *testing.T, fixture modelServiceFixture) *modelsservice.Service {
	t.Helper()
	if fixture.ModelAssetPuller == nil {
		fixture.ModelAssetPuller = externalConstructionAssetPuller{}
	}
	if fixture.ModelHost == nil {
		fixture.ModelHost = externalConstructionHost{puller: fixture.ModelAssetPuller}
	}
	if fixture.Logger == nil {
		fixture.Logger = zap.NewNop()
	}
	if fixture.Clock == nil {
		fixture.Clock = time.Now
	}
	svc, err := modelsservice.NewService(
		fixture.RuntimeConfig,
		fixture.ModelHost,
		fixture.ModelAssetPuller,
		fixture.Logger,
		fixture.Clock,
		fixture.ModelPullMetrics,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type externalConstructionAssetPuller struct{}

func (externalConstructionAssetPuller) PullModel(context.Context, *modelRuntimeConfig, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (externalConstructionAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}
func (externalConstructionAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}
func (externalConstructionAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}

type externalConstructionHost struct {
	puller localmodels.AssetPuller
}

func (externalConstructionHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}
func (externalConstructionHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}
func (h externalConstructionHost) Pull(ctx context.Context, runtimeCfg *modelRuntimeConfig, modelName string) (modelhost.PullSnapshot, error) {
	result, err := localmodels.PullModelWithOptions(h.puller, ctx, runtimeCfg, modelName, localmodels.PullOptions{
		RuntimeCacheInspector: h.puller,
		SourceResolver:        localmodels.DefaultManagedRuntimeSourceResolver(),
	})
	files := make([]modelhost.PullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		files = append(files, modelhost.PullDownloadedFile{Path: file.Path, Bytes: file.Bytes, SHA256: file.SHA256})
	}
	return modelhost.PullSnapshot{
		ReadinessSnapshot: modelhost.ReadinessSnapshot{
			Identity: modelhost.Identity{
				Name:          result.ModelName,
				Locality:      managedruntime.Locality(result.ProviderLocality),
				SourceKind:    result.SourceKind,
				SourceID:      result.SourceID,
				ResolverNotes: result.ResolverNotes,
			},
			ReadinessState: managedruntime.ReadinessState(result.ReadinessState),
			LifecycleState: managedruntime.LifecycleState(result.LifecycleState),
		},
		PullOutcome:     managedruntime.PullOutcome(result.ManagedPullOutcome),
		LegacyOutcome:   result.Outcome,
		CachePath:       result.CachePath,
		Revision:        result.Revision,
		DownloadedFiles: files,
	}, err
}
func (externalConstructionHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}
func (externalConstructionHost) ReleaseLease(context.Context, string) error { return nil }
func (externalConstructionHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return nil
}

type modelRuntimeConfig = models.RuntimeConfig
type modelRuntimeWorker = models.RuntimeWorker
type modelRuntimeResource = models.RuntimeResource

type testFactoryConfig struct {
	Name             string
	Workers          []modelRuntimeWorker
	Resources        []modelRuntimeResource
	ResourceManifest *testResourceManifest
}

type testResourceManifest struct{ RequiredTools []testRequiredTool }
type testRequiredTool struct{ Name, Command string }

func projectTestModelsRuntimeConfig(factoryDir string, cfg *testFactoryConfig) *modelRuntimeConfig {
	if cfg == nil {
		return nil
	}
	return &modelRuntimeConfig{
		FactoryDirectory: factoryDir,
		Workers:          append([]modelRuntimeWorker(nil), cfg.Workers...),
		Resources:        append([]modelRuntimeResource(nil), cfg.Resources...),
	}
}
