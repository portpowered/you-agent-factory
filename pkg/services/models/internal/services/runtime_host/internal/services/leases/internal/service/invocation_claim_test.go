package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
)

func TestClaimedInvocationRetainsCapacityPastDetachedExpiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	clock := &mutexAdvanceableHostClock{now: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)}
	coordinator := &recordingSlotCapacityCoordinator{}
	svc := internalservice.New(clock, readySlotFacts{capacity: 1}, coordinator)
	scope := mustRuntimeScopeRef(t, "claimed-invocation")
	acquired, err := svc.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{Scope: scope, Name: "model", Holder: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	lease := acquired.Lease.Lease
	request := models.InvokeModelRequest{Scope: scope, Lease: lease, ModelName: "model", Holder: "worker", Operation: models.OperationOMNI}
	claimed := make(chan error, 1)
	complete := make(chan struct{})
	releasedCh := make(chan error, 1)
	go func() {
		_, err := svc.ClaimInvocationLease(ctx, request)
		claimed <- err
		if err != nil {
			return
		}
		<-complete // Simulated backend generation remains in flight.
		_, err = svc.ReleaseInvocationLease(ctx, models.ReleaseModelLeaseRequest{Scope: scope, Lease: lease})
		releasedCh <- err
	}()
	if err := <-claimed; err != nil {
		t.Fatalf("claim: %v", err)
	}
	clock.advance(hostleases.DefaultLeaseTTL + time.Hour)
	if _, err := svc.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{Scope: scope, Name: "model", Holder: "other"}); !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("concurrent admission = %v, want capacity exhausted", err)
	}
	if _, err := svc.GetModelLease(ctx, models.GetModelLeaseRequest{Scope: scope, Lease: lease}); err != nil {
		t.Fatalf("claimed lease expired during invocation: %v", err)
	}
	if _, err := svc.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{Scope: scope, Lease: lease}); !errors.Is(err, models.ErrHostCapacityContended) {
		t.Fatalf("detached release of claimed lease = %v, want contended", err)
	}
	if _, err := svc.ClaimInvocationLease(ctx, request); !errors.Is(err, models.ErrHostCapacityContended) {
		t.Fatalf("duplicate claim = %v, want contended", err)
	}
	close(complete)
	if err := <-releasedCh; err != nil {
		t.Fatalf("invocation cleanup: %v", err)
	}
	if _, err := svc.ReleaseInvocationLease(ctx, models.ReleaseModelLeaseRequest{Scope: scope, Lease: lease}); !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("second cleanup = %v, want already released", err)
	}
	if _, err := svc.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{Scope: scope, Name: "model", Holder: "other"}); err != nil {
		t.Fatalf("capacity not returned after invocation: %v", err)
	}
	if coordinator.acquired != 2 || coordinator.released != 1 {
		t.Fatalf("claimed notifications = %d/%d, want 2 acquired / 1 released", coordinator.acquired, coordinator.released)
	}

}
