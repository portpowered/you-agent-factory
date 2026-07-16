package service_test

import (
	"context"
	"errors"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestService_GetModel_ReturnsMissingWhenManagedCacheNotInstalled(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     missingCacheInspectHost{},
	})

	model, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING {
		t.Fatalf("managed readiness = %s, want MISSING", model.ManagedRuntime.ReadinessState)
	}
	if model.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("managed lifecycle = %s, want NOT_INSTALLED", model.ManagedRuntime.LifecycleState)
	}
}

func TestService_GetModel_PreservesInstalledAssetReadinessFromModelHost(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     installedCacheInspectHost{},
	})

	model, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if model.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("managed readiness = %s, want READY", model.ManagedRuntime.ReadinessState)
	}
	if model.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateINSTALLED {
		t.Fatalf("managed lifecycle = %s, want INSTALLED", model.ManagedRuntime.LifecycleState)
	}
}

func TestService_GetModel_ReturnsNotFoundForUnknownModel(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, &interfaces.FactoryConfig{Name: "factory"})
	svc := mustConstructModelService(t, modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
	})

	_, err := svc.GetModel(context.Background(), "missing")
	if !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func TestService_GetModel_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc, err := modelsservice.NewService(modelsservice.Dependencies{})
	if svc != nil || !errors.Is(err, modelsservice.ErrInvalidDependencies) {
		t.Fatalf("NewService = (%v, %v), want missing runtime construction error", svc, err)
	}
}

type missingCacheInspectHost struct{}

type installedCacheInspectHost struct {
	missingCacheInspectHost
}

func (installedCacheInspectHost) InspectReadiness(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateReady,
		LifecycleState: managedruntime.LifecycleStateInstalled,
	}, nil
}

func (missingCacheInspectHost) ResolveIdentity(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal}, nil
}

func (missingCacheInspectHost) InspectReadiness(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: managedruntime.LocalityLocal},
		ReadinessState: managedruntime.ReadinessStateMissing,
		LifecycleState: managedruntime.LifecycleStateNotInstalled,
	}, nil
}

func (missingCacheInspectHost) Pull(context.Context, *factoryconfig.LoadedFactoryConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, errors.New("pull unavailable in test host")
}

func (missingCacheInspectHost) AcquireLease(context.Context, *factoryconfig.LoadedFactoryConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	return modelhost.Lease{}, errors.New("lease unavailable in test host")
}

func (missingCacheInspectHost) ReleaseLease(context.Context, string) error {
	return errors.New("lease unavailable in test host")
}

func (missingCacheInspectHost) Unload(context.Context, *factoryconfig.LoadedFactoryConfig, string) error {
	return errors.New("unload unavailable in test host")
}
