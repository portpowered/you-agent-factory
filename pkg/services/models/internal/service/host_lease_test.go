package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/legacyhost"
	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

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
