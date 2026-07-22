package service_test

import (
	"context"
	"errors"
	"testing"

	apisurface "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/services/models/internal/service"
)

func TestService_GetModel_ReturnsMissingWhenManagedCacheNotInstalled(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     missingCacheInspectHost{},
	})

	model, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if model.ManagedRuntime.ReadinessState != managedruntime.ReadinessStateMissing {
		t.Fatalf("managed readiness = %s, want MISSING", model.ManagedRuntime.ReadinessState)
	}
	if model.ManagedRuntime.LifecycleState != managedruntime.LifecycleStateNotInstalled {
		t.Fatalf("managed lifecycle = %s, want NOT_INSTALLED", model.ManagedRuntime.LifecycleState)
	}
}

func TestService_GetModel_PreservesInstalledAssetReadinessFromModelHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
		ModelHost:     installedCacheInspectHost{},
	})

	model, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if model.ManagedRuntime.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("managed readiness = %s, want READY", model.ManagedRuntime.ReadinessState)
	}
	if model.ManagedRuntime.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("managed lifecycle = %s, want INSTALLED", model.ManagedRuntime.LifecycleState)
	}
}

func TestService_GetModel_ReturnsNotFoundForUnknownModel(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, &testFactoryConfig{Name: "factory"})
	svc := mustConstructModelService(t, modelServiceFixture{
		RuntimeConfig: func() *modelRuntimeConfig { return runtimeCfg },
	})

	_, err := svc.GetModel(context.Background(), "missing")
	if !errors.Is(err, apisurface.ErrNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func TestService_GetModel_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc, err := modelsservice.NewService(nil, nil, nil, nil, nil, nil)
	if svc != nil || !errors.Is(err, modelsservice.ErrInvalidDependencies) {
		t.Fatalf("NewService = (%v, %v), want missing runtime construction error", svc, err)
	}
}

type missingCacheInspectHost struct{}

type installedCacheInspectHost struct {
	missingCacheInspectHost
}

func (installedCacheInspectHost) InspectReadiness(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
	}, nil
}

func (missingCacheInspectHost) ResolveIdentity(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal}, nil
}

func (missingCacheInspectHost) InspectReadiness(_ context.Context, _ *modelRuntimeConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateMissing,
		LifecycleState: managedruntime.LifecycleStateNotInstalled,
	}, nil
}

func (missingCacheInspectHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("pull unavailable in test host")
}

func (missingCacheInspectHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, errors.New("lease unavailable in test host")
}

func (missingCacheInspectHost) ReleaseLease(context.Context, string) error {
	return errors.New("lease unavailable in test host")
}

func (missingCacheInspectHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return errors.New("unload unavailable in test host")
}
