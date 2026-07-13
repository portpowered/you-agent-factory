package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
)

func TestService_GetModel_ReturnsMissingWhenManagedCacheNotInstalled(t *testing.T) {
	t.Parallel()

	runtimeCfg := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     func() modelhost.Host { return missingCacheInspectHost{} },
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
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
		ModelHost:     func() modelhost.Host { return installedCacheInspectHost{} },
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
	svc := modelsservice.New(modelsservice.Dependencies{
		RuntimeConfig: func() *factoryconfig.LoadedFactoryConfig { return runtimeCfg },
	})

	_, err := svc.GetModel(context.Background(), "missing")
	if !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("GetModel error = %v, want ErrModelNotFound", err)
	}
}

func TestService_GetModel_ReturnsUnavailableWhenRuntimeMissing(t *testing.T) {
	t.Parallel()

	svc := modelsservice.New(modelsservice.Dependencies{})

	_, err := svc.GetModel(context.Background(), "OMNIVOICE_Q4_K_M")
	if err == nil {
		t.Fatal("GetModel: nil error, want runtime unavailable")
	}
	if !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("GetModel error = %v, want runtime unavailable message", err)
	}
}

type missingCacheInspectHost struct{}

type installedCacheInspectHost struct {
	missingCacheInspectHost
}

func (installedCacheInspectHost) InspectReadiness(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: factoryapi.WorkerModelLocalityLocal},
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateREADY,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateINSTALLED,
	}, nil
}

func (missingCacheInspectHost) ResolveIdentity(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.Identity, error) {
	return modelhost.Identity{Name: modelName, Locality: factoryapi.WorkerModelLocalityLocal}, nil
}

func (missingCacheInspectHost) InspectReadiness(_ context.Context, _ *factoryconfig.LoadedFactoryConfig, modelName string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{
		Identity:       modelhost.Identity{Name: modelName, Locality: factoryapi.WorkerModelLocalityLocal},
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateMISSING,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
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
