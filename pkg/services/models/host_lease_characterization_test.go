package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
)

// hostLeasePeerService is a fake peer implementer of Models root Service that
// exercises plain host/lease contracts using only root-package types.
type hostLeasePeerService struct {
	runtimes map[string]models.Runtime
	leases   map[string]models.HostLease
	acquire  map[string]error
	release  map[string]error
	inspect  map[string]error
}

func (hostLeasePeerService) ForRuntime(models.RuntimeBinding) (models.Service, error) {
	return hostLeasePeerService{}, nil
}

func (hostLeasePeerService) ListModels(context.Context) (models.List, error) {
	return models.List{Results: []models.Summary{}}, nil
}

func (hostLeasePeerService) GetModel(context.Context, string) (models.Detail, error) {
	return models.Detail{}, models.ErrNotFound
}

func (hostLeasePeerService) PullModel(context.Context, string) (models.PullResult, error) {
	return models.PullResult{}, models.ErrUnsupportedOperation
}

func (s hostLeasePeerService) InspectRuntime(_ context.Context, name string) (models.Runtime, error) {
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: name}); err != nil {
		return models.Runtime{}, err
	}
	if fail, ok := s.inspect[name]; ok {
		return models.Runtime{}, fail
	}
	runtime, ok := s.runtimes[name]
	if !ok {
		return models.Runtime{}, fmt.Errorf("%w: %s", models.ErrNotFound, name)
	}
	return runtime, nil
}

func (s hostLeasePeerService) AcquireLease(_ context.Context, request models.AcquireLeaseRequest) (models.HostLease, error) {
	if err := models.ValidateAcquireLeaseRequest(request); err != nil {
		return models.HostLease{}, err
	}
	if fail, ok := s.acquire[request.ModelName]; ok {
		return models.HostLease{}, fail
	}
	lease, ok := s.leases[request.ModelName]
	if !ok {
		return models.HostLease{}, models.ErrHostRuntimeNotReady
	}
	if request.Holder != "" {
		lease.Holder = request.Holder
	}
	return lease, nil
}

func (s hostLeasePeerService) ReleaseLease(_ context.Context, request models.ReleaseLeaseRequest) error {
	if err := models.ValidateReleaseLeaseRequest(request); err != nil {
		return err
	}
	if fail, ok := s.release[request.LeaseID]; ok {
		return fail
	}
	for _, lease := range s.leases {
		if lease.ID == request.LeaseID {
			return nil
		}
	}
	return models.ErrHostLeaseNotFound
}

func (hostLeasePeerService) InvokeLocal(context.Context, models.LocalInvocationRequest) (models.LocalInvocationResult, error) {
	return models.LocalInvocationResult{Handled: false}, nil
}

func TestHostLease_ReadinessInspectAndAcquireReleaseReturnModelsOwnedShapes(t *testing.T) {
	t.Parallel()

	wantRuntime := models.Runtime{
		Identity:       "local-model",
		ReadinessState: models.ReadinessStateReady,
		LifecycleState: models.LifecycleStateLoaded,
		Locality:       models.LocalityLocal,
	}
	wantLease := models.HostLease{
		ID:       "lease-1",
		Identity: models.HostIdentity{Name: "local-model", Locality: models.LocalityLocal},
		Endpoint: "http://127.0.0.1:8080",
		Holder:   "worker-a",
	}
	var service models.Service = hostLeasePeerService{
		runtimes: map[string]models.Runtime{"local-model": wantRuntime},
		leases:   map[string]models.HostLease{"local-model": wantLease},
	}

	gotRuntime, err := service.InspectRuntime(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("InspectRuntime: %v", err)
	}
	if gotRuntime.Identity != wantRuntime.Identity || gotRuntime.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("InspectRuntime = %#v, want Models-owned Runtime readiness", gotRuntime)
	}

	gotLease, err := service.AcquireLease(context.Background(), models.AcquireLeaseRequest{
		ModelName: "local-model",
		Holder:    "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if gotLease.ID != wantLease.ID || gotLease.Endpoint != wantLease.Endpoint || gotLease.Holder != "worker-a" {
		t.Fatalf("AcquireLease = %#v, want Models-owned HostLease", gotLease)
	}
	if gotLease.Identity.Name != "local-model" {
		t.Fatalf("AcquireLease identity = %#v, want local-model HostIdentity", gotLease.Identity)
	}

	if err := service.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease-1"}); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func assertHostLeaseErrorIsOnly(t *testing.T, label string, err error, want error, notWants ...error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s = %v, want %v", label, err, want)
	}
	for _, notWant := range notWants {
		if errors.Is(err, notWant) {
			t.Fatalf("%s must stay distinct from %v: %v", want, notWant, err)
		}
	}
}

func TestHostLease_MissingAssetsLoadingTimeoutCapacityLeaseNotFoundRuntimeNotReadyAreDistinct(t *testing.T) {
	t.Parallel()

	var service models.Service = hostLeasePeerService{
		inspect: map[string]error{
			"missing-assets": models.ErrHostMissingAssets,
			"loading-slow":   models.ErrHostLoadingTimeout,
		},
		acquire: map[string]error{
			"full-capacity":     models.ErrHostCapacityExhausted,
			"runtime-not-ready": models.ErrHostRuntimeNotReady,
		},
		release: map[string]error{
			"unknown-lease": models.ErrHostLeaseNotFound,
		},
		leases: map[string]models.HostLease{},
	}

	_, err := service.InspectRuntime(context.Background(), "missing-assets")
	assertHostLeaseErrorIsOnly(t, "InspectRuntime missing-assets", err, models.ErrHostMissingAssets,
		models.ErrHostLoadingTimeout, models.ErrHostCapacityExhausted)

	_, err = service.InspectRuntime(context.Background(), "loading-slow")
	assertHostLeaseErrorIsOnly(t, "InspectRuntime loading-slow", err, models.ErrHostLoadingTimeout,
		models.ErrHostMissingAssets, models.ErrHostRuntimeNotReady)

	_, err = service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "full-capacity"})
	assertHostLeaseErrorIsOnly(t, "AcquireLease full-capacity", err, models.ErrHostCapacityExhausted,
		models.ErrHostLeaseNotFound, models.ErrHostRuntimeNotReady)

	_, err = service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "runtime-not-ready"})
	assertHostLeaseErrorIsOnly(t, "AcquireLease runtime-not-ready", err, models.ErrHostRuntimeNotReady,
		models.ErrHostCapacityExhausted, models.ErrHostMissingAssets)

	err = service.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "unknown-lease"})
	assertHostLeaseErrorIsOnly(t, "ReleaseLease unknown-lease", err, models.ErrHostLeaseNotFound,
		models.ErrHostCapacityExhausted, models.ErrHostRuntimeNotReady)
}

func TestHostLease_PeerCompilesWithoutNestedHostImport(t *testing.T) {
	t.Parallel()

	// Compiling this fake peer against only root package types proves peers can
	// use host/lease without models/internal/host or local managed-runtime imports.
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{Name: "local-model"}); err != nil {
		t.Fatalf("ValidateInspectRuntimeRequest: %v", err)
	}
	if err := models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{}); err == nil {
		t.Fatal("ValidateInspectRuntimeRequest empty name = nil, want error")
	}
	if !errors.Is(models.ValidateInspectRuntimeRequest(models.InspectRuntimeRequest{}), models.ErrNotFound) {
		t.Fatal("ValidateInspectRuntimeRequest empty name must wrap ErrNotFound")
	}

	if err := models.ValidateAcquireLeaseRequest(models.AcquireLeaseRequest{ModelName: "local-model"}); err != nil {
		t.Fatalf("ValidateAcquireLeaseRequest: %v", err)
	}
	if !errors.Is(models.ValidateAcquireLeaseRequest(models.AcquireLeaseRequest{}), models.ErrNotFound) {
		t.Fatal("ValidateAcquireLeaseRequest empty model must wrap ErrNotFound")
	}

	if err := models.ValidateReleaseLeaseRequest(models.ReleaseLeaseRequest{LeaseID: "lease-1"}); err != nil {
		t.Fatalf("ValidateReleaseLeaseRequest: %v", err)
	}
	if !errors.Is(models.ValidateReleaseLeaseRequest(models.ReleaseLeaseRequest{}), models.ErrHostLeaseNotFound) {
		t.Fatal("ValidateReleaseLeaseRequest empty lease id must wrap ErrHostLeaseNotFound")
	}

	var service models.Service = hostLeasePeerService{
		runtimes: map[string]models.Runtime{
			"local-model": {
				Identity:       "local-model",
				ReadinessState: models.ReadinessStateReady,
				LifecycleState: models.LifecycleStateInstalled,
			},
		},
		leases: map[string]models.HostLease{
			"local-model": {ID: "lease-1", Endpoint: "http://local", Identity: models.HostIdentity{Name: "local-model"}},
		},
	}
	if _, err := service.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "local-model"}); err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
}
