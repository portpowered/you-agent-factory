package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelassets "github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"go.uber.org/zap"
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
