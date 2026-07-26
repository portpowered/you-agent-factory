package service

import (
	"context"
	"errors"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelassets "github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
	"strings"
	"testing"
	"time"
)

func TestNewRootRejectsMissingHostPlatform(t *testing.T) {
	t.Parallel()

	for _, platform := range []localmodels.HostPlatform{
		{Architecture: "amd64"},
		{OperatingSystem: "linux"},
		{OperatingSystem: " ", Architecture: "amd64"},
		{OperatingSystem: "linux", Architecture: " "},
	} {
		opener, err := NewRoot(
			platform,
			nil, modelassets.Endpoints{},
			nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil, nil, nil, nil,
		)
		if opener != nil || !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), "model asset host platform") {
			t.Fatalf("NewRoot(%#v) = (%#v, %v), want missing host-platform dependency", platform, opener, err)
		}
	}
}

func TestNewServiceRetainsExplicitDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	puller := constructionAssetPuller{}
	logger := zap.NewNop()
	metrics := &constructionPullMetrics{}
	host := constructionModelHost{}

	svc, err := NewService(
		func() *modelRuntimeConfig { return runtimeCfg },
		host,
		puller,
		logger,
		func() time.Time { return time.Unix(123, 0) },
		metrics,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc.runtimeConfig() != runtimeCfg || svc.modelHost() != host || svc.modelAssetPuller() != puller {
		t.Fatal("NewService did not retain required dependencies")
	}
	if svc.logger() != logger || svc.pullMetrics != metrics {
		t.Fatal("NewService did not retain optional dependencies")
	}
}

func TestNewServiceRejectsMissingClockAndLogger(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	for _, test := range []struct {
		name   string
		logger *zap.Logger
		clock  func() time.Time
		want   string
	}{
		{name: "logger", clock: time.Now, want: "logger is required"},
		{name: "clock", logger: zap.NewNop(), want: "clock is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, err := NewService(
				func() *modelRuntimeConfig { return runtimeCfg }, constructionModelHost{},
				constructionAssetPuller{}, test.logger, test.clock, nil,
			)
			if svc != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewService = (%#v, %v), want %q", svc, err, test.want)
			}
		})
	}
}

func TestServiceNilReceiverPreservesUnavailableRuntimeErrors(t *testing.T) {
	t.Parallel()

	var svc *Service
	assertUnavailable := func(operation string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), "runtime is not available") {
			t.Fatalf("%s error = %v, want runtime unavailable", operation, err)
		}
	}

	_, err := svc.ListModels(context.Background())
	assertUnavailable("ListModels", err)
	_, err = svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	assertUnavailable("GetModel", err)
	_, err = svc.PullModel(context.Background(), "OMNIVOICE_Q4_K_M")
	assertUnavailable("PullModel", err)
	if svc.logger() != nil || svc.modelHost() != nil {
		t.Fatal("nil service accessors returned configured collaborators")
	}
}

func TestNewServiceRejectsMissingRequiredDependencies(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustConstructionRuntimeConfig(t)
	runtimeConfig := func() *modelRuntimeConfig { return runtimeCfg }
	host := constructionModelHost{}
	puller := constructionAssetPuller{}
	tests := []struct {
		name       string
		dependency string
		construct  func() (*Service, error)
	}{
		{
			name: "runtime lookup", dependency: "runtime configuration lookup",
			construct: func() (*Service, error) { return NewService(nil, host, puller, zap.NewNop(), time.Now, nil) },
		},
		{
			name: "model host", dependency: "model host",
			construct: func() (*Service, error) { return NewService(runtimeConfig, nil, puller, zap.NewNop(), time.Now, nil) },
		},
		{
			name: "typed nil model host", dependency: "model host",
			construct: func() (*Service, error) {
				var typedNilHost *constructionModelHost
				return NewService(runtimeConfig, typedNilHost, puller, zap.NewNop(), time.Now, nil)
			},
		},
		{
			name: "asset puller", dependency: "model asset puller",
			construct: func() (*Service, error) { return NewService(runtimeConfig, host, nil, zap.NewNop(), time.Now, nil) },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, err := test.construct()
			if svc != nil {
				t.Fatalf("NewService service = %#v, want nil", svc)
			}
			if !errors.Is(err, ErrInvalidDependencies) || !strings.Contains(err.Error(), test.dependency) {
				t.Fatalf("NewService error = %v, want invalid-dependencies error naming %q", err, test.dependency)
			}
		})
	}
}

type constructionPullMetrics struct{}

func (*constructionPullMetrics) RecordModelPullMetric(models.PullMetric) {}

type constructionAssetPuller struct{}

func (constructionAssetPuller) PullModel(context.Context, *modelRuntimeConfig, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (constructionAssetPuller) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}
func (constructionAssetPuller) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{}, nil
}
func (constructionAssetPuller) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{}, nil
}

func mustConstructionRuntimeConfig(t *testing.T) *modelRuntimeConfig {
	t.Helper()
	return projectTestModelsRuntimeConfig(t.TempDir(), &testFactoryConfig{
		Name: "construction-test",
	})
}

type constructionModelHost struct{}

func (constructionModelHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}

func (constructionModelHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}

func (constructionModelHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, nil
}

func (constructionModelHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, nil
}

func (constructionModelHost) ReleaseLease(context.Context, string) error { return nil }

func (constructionModelHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return nil
}

func TestService_AcquireLease_ReturnsRuntimeNotReadyWhenHostMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		runtimeConfigLookup: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		clock:               time.Now,
	}

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "local-model"})
	if !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("AcquireLease nil host = %v, want ErrHostRuntimeNotReady", err)
	}
}

func TestService_ReleaseLease_ReturnsLeaseNotFoundWhenHostMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		runtimeConfigLookup: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		loggerValue:         zap.NewNop(),
		clock:               time.Now,
	}

	err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease-1"})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseLease nil host = %v, want ErrHostLeaseNotFound", err)
	}
}

func assertContractOnlyUnsupported(t *testing.T, operation string, err error) {
	t.Helper()
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("%s error = %v, want ErrUnsupportedOperation", operation, err)
	}
}

func TestRootContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	root := &Root{}
	_, err := root.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "OpenRuntimeScope", err)
	_, err = root.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "CloseRuntimeScope", err)
	_, err = root.ListCatalog(ctx, models.ListModelsRequest{})
	assertContractOnlyUnsupported(t, "ListCatalog", err)
	_, err = root.GetCatalogModel(ctx, models.GetModelRequest{})
	assertContractOnlyUnsupported(t, "GetCatalogModel", err)
	_, err = root.GetModelReadiness(ctx, models.GetModelReadinessRequest{})
	assertContractOnlyUnsupported(t, "GetModelReadiness", err)
	_, err = root.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = root.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = root.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = root.EnsureModelHost(ctx, models.EnsureModelHostRequest{})
	assertContractOnlyUnsupported(t, "EnsureModelHost", err)
	_, err = root.InspectModelHost(ctx, models.InspectModelHostRequest{})
	assertContractOnlyUnsupported(t, "InspectModelHost", err)
	_, err = root.StopModelHost(ctx, models.StopModelHostRequest{})
	assertContractOnlyUnsupported(t, "StopModelHost", err)
	_, err = root.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "AcquireModelLease", err)
	_, err = root.GetModelLease(ctx, models.GetModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "GetModelLease", err)
	_, err = root.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "ReleaseModelLease", err)
	_, err = root.InvokeModelWithLease(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModelWithLease", err)
	_, err = root.CancelInvocation(ctx, models.CancelInvocationRequest{})
	assertContractOnlyUnsupported(t, "CancelInvocation", err)
}

func TestBoundServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &Service{}
	_, err := svc.ListCatalog(ctx, models.ListModelsRequest{})
	assertContractOnlyUnsupported(t, "ListCatalog", err)
	_, err = svc.GetCatalogModel(ctx, models.GetModelRequest{})
	assertContractOnlyUnsupported(t, "GetCatalogModel", err)
	_, err = svc.GetModelReadiness(ctx, models.GetModelReadinessRequest{})
	assertContractOnlyUnsupported(t, "GetModelReadiness", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
	_, err = svc.EnsureModelHost(ctx, models.EnsureModelHostRequest{})
	assertContractOnlyUnsupported(t, "EnsureModelHost", err)
	_, err = svc.InspectModelHost(ctx, models.InspectModelHostRequest{})
	assertContractOnlyUnsupported(t, "InspectModelHost", err)
	_, err = svc.StopModelHost(ctx, models.StopModelHostRequest{})
	assertContractOnlyUnsupported(t, "StopModelHost", err)
	_, err = svc.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "AcquireModelLease", err)
	_, err = svc.GetModelLease(ctx, models.GetModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "GetModelLease", err)
	_, err = svc.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{})
	assertContractOnlyUnsupported(t, "ReleaseModelLease", err)
	_, err = svc.InvokeModelWithLease(ctx, models.InvokeModelRequest{})
	assertContractOnlyUnsupported(t, "InvokeModelWithLease", err)
	_, err = svc.CancelInvocation(ctx, models.CancelInvocationRequest{})
	assertContractOnlyUnsupported(t, "CancelInvocation", err)
}

func TestRuntimeServiceContractOnlyOperationsFailExplicitly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := &runtimeService{}
	_, err := svc.OpenRuntimeScope(ctx, models.OpenRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "OpenRuntimeScope", err)
	_, err = svc.CloseRuntimeScope(ctx, models.CloseRuntimeScopeRequest{})
	assertContractOnlyUnsupported(t, "CloseRuntimeScope", err)
	_, err = svc.PrepareModelAssets(ctx, models.PrepareModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "PrepareModelAssets", err)
	_, err = svc.InspectModelAssets(ctx, models.InspectModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "InspectModelAssets", err)
	_, err = svc.RemoveModelAssets(ctx, models.RemoveModelAssetsRequest{})
	assertContractOnlyUnsupported(t, "RemoveModelAssets", err)
}
