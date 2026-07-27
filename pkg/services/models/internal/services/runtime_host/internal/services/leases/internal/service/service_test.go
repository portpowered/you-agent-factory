package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestConstructionAllocatesLeaseStateWithoutStartingLifecycle(t *testing.T) {
	t.Parallel()

	clock := &recordingHostClock{}
	service := internalservice.New(clock)
	if service == nil {
		t.Fatal("New returned nil service")
	}
	if clock.timerCreates != 0 {
		t.Fatalf("timer creates during construction = %d, want 0", clock.timerCreates)
	}
}

func TestLeaseOperationsAreContractOnlyUntilBehaviorPacket(t *testing.T) {
	t.Parallel()

	service := internalservice.New(&recordingHostClock{})
	ctx := context.Background()
	_, err := service.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{})
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("AcquireModelLease error = %v, want ErrUnsupportedOperation", err)
	}
	_, err = service.GetModelLease(ctx, models.GetModelLeaseRequest{})
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("GetModelLease error = %v, want ErrUnsupportedOperation", err)
	}
	_, err = service.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{})
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("ReleaseModelLease error = %v, want ErrUnsupportedOperation", err)
	}
}

func TestOpenRuntimeScopeDoesNotConstructAnotherLeasesOwner(t *testing.T) {
	t.Parallel()

	clock := &recordingHostClock{}
	service := internalservice.New(clock)
	scopes, err := runtimescopeswire.NewService(func() string { return "leases-scope-binding" })
	if err != nil {
		t.Fatalf("construct runtime scopes: %v", err)
	}
	_, err = scopes.Open(models.RuntimeBinding{
		CacheDirectory: t.TempDir(),
		RuntimeConfig: func() *models.RuntimeConfig {
			return &models.RuntimeConfig{FactoryDirectory: "factory"}
		},
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if service == nil {
		t.Fatal("leases service is nil after scope open")
	}
	if clock.timerCreates != 0 {
		t.Fatalf("timer creates after scope open = %d, want 0", clock.timerCreates)
	}
}

type recordingHostClock struct {
	timerCreates int
}

func (clock *recordingHostClock) Now() time.Time {
	return time.Unix(0, 0)
}

func (clock *recordingHostClock) NewTimer(time.Duration) models.HostTimer {
	clock.timerCreates++
	panic("host timer created during inert leases owner")
}
