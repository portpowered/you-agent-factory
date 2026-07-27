package service_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
)

func TestWiredHostDelegatesLeaseOperationsToNestedOwner(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "lease-cutover-delegate")
	cfg := runtimeConfig()
	cfg.Resources[0].Capacity = 2
	ref := openScope(t, scopes, cacheDirectory, cfg)
	launcher := &recordingProcessLauncher{}
	service, err := runtimehostwire.NewService(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		testHostClock{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  ref,
		Name:   "OMNIVOICE_Q4_K_M",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	if acquired.Lease.Lease.IsZero() || acquired.Lease.Holder != "worker-a" {
		t.Fatalf("AcquireModelLease = %#v, want active lease facts", acquired)
	}

	got, err := service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: ref,
		Lease: acquired.Lease.Lease,
	})
	if err != nil {
		t.Fatalf("GetModelLease: %v", err)
	}
	if got.Lease.Status != models.ModelLeaseStatusActive {
		t.Fatalf("GetModelLease = %#v, want active lease", got)
	}

	released, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: ref,
		Lease: acquired.Lease.Lease,
	})
	if err != nil || released.Outcome != models.ModelLeaseReleased {
		t.Fatalf("ReleaseModelLease = (%#v, %v), want released", released, err)
	}

	leases := internalservice.LeasesService(service)
	if leases == nil {
		t.Fatal("LeasesService returned nil nested owner")
	}
}

func TestWiredHostSlotFactsRejectMissingAssetsWithoutConsumingCapacity(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	scopes := newScopes(t, "lease-cutover-missing")
	cfg := runtimeConfig()
	cfg.Resources[0].Capacity = 1
	ref := openScope(t, scopes, cacheDirectory, cfg)
	service, err := runtimehostwire.NewService(
		scopes,
		mustAssetsService(t, scopes),
		&recordingProcessLauncher{},
		http.DefaultClient,
		testHostClock{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  ref,
		Name:   "OMNIVOICE_Q4_K_M",
		Holder: "worker-a",
	})
	if !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("AcquireModelLease missing assets = %v, want ErrHostRuntimeNotReady", err)
	}

	writeCacheFixture(t, cacheDirectory, true)
	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  ref,
		Name:   "OMNIVOICE_Q4_K_M",
		Holder: "worker-b",
	})
	if err != nil {
		t.Fatalf("second AcquireModelLease after not-ready = %v, want success once assets installed", err)
	}
	if acquired.Lease.Lease.IsZero() {
		t.Fatal("expected lease after not-ready did not consume capacity")
	}
}
