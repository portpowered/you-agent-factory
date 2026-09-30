package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
	"go.uber.org/zap"
)

type modelServiceFixture struct {
	RuntimeConfig    func() *modelRuntimeConfig
	ModelHost        modelhost.Host
	ModelAssetPuller localmodels.AssetPuller
	Logger           *zap.Logger
	Clock            func() time.Time
	ModelPullMetrics modelseffects.PullMetricsRecorder
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

func TestService_AcquireLease_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     hostLeaseTestHost{},
	})

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{})
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("AcquireLease empty model = %v, want ErrNotFound", err)
	}
}

func TestService_ReleaseLease_RejectsEmptyLeaseID(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     hostLeaseTestHost{},
	})

	err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseLease empty lease id = %v, want ErrHostLeaseNotFound", err)
	}
}

func TestService_AcquireLease_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return nil },
		ModelHost:     hostLeaseTestHost{},
	})

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "OMNIVOICE_Q4_K_M"})
	if err == nil || !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("AcquireLease missing runtime = %v, want runtime unavailable", err)
	}
}

func TestService_AcquireLeaseAndReleaseLease_HappyPathThroughStubHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := hostLeaseTestHost{}
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     host,
	})

	lease, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{
		ModelName: "OMNIVOICE_Q4_K_M",
		Holder:    "dispatch-1",
	})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease.ID != "lease-OMNIVOICE_Q4_K_M" || lease.Holder != "dispatch-1" || lease.Endpoint != "http://127.0.0.1:8080" {
		t.Fatalf("AcquireLease = %#v, want stub host lease", lease)
	}

	if err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: lease.ID}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func TestService_GetModel_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.GetModel(context.Background(), " ")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("GetModel empty name = %v, want ErrNotFound", err)
	}
}

func TestService_PullModel_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.PullModel(context.Background(), "")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("PullModel empty name = %v, want ErrNotFound", err)
	}
}

func TestService_InspectRuntime_RejectsEmptyModelName(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.InspectRuntime(context.Background(), "")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("InspectRuntime empty name = %v, want ErrNotFound", err)
	}
}

func TestService_InspectRuntime_ProjectsReadyHostAndPreservesFailure(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	ready, err := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     hostLeaseTestHost{},
	}).InspectRuntime(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectRuntime ready: %v", err)
	}
	if ready.Identity != "OMNIVOICE_Q4_K_M" ||
		ready.ReadinessState != models.ReadinessStateReady ||
		ready.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("InspectRuntime ready = %#v, want ready installed model", ready)
	}

	failing, err := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     failingInspectHost{},
	}).InspectRuntime(context.Background(), "OMNIVOICE_Q4_K_M")
	if err == nil || !strings.Contains(err.Error(), "inspect failed") {
		t.Fatalf("InspectRuntime failure = (%#v, %v), want host inspection error", failing, err)
	}
}

type hostLeaseTestHost struct{}

func (hostLeaseTestHost) ResolveIdentity(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal}, nil
}

func (hostLeaseTestHost) InspectReadiness(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
	}, nil
}

func (hostLeaseTestHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("pull unavailable in test host")
}

func (hostLeaseTestHost) AcquireLease(_ context.Context, _ *modelRuntimeConfig, modelName string, opts modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{
		ID:       "lease-" + modelName,
		Holder:   opts.Holder,
		Endpoint: "http://127.0.0.1:8080",
		Identity: modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
	}, nil
}

func (hostLeaseTestHost) ReleaseLease(context.Context, string) error {
	return nil
}

func (hostLeaseTestHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return errors.New("unload unavailable in test host")
}
