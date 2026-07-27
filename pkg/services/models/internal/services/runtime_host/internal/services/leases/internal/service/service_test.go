package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/internal/service"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
)

func TestConstructionAllocatesLeaseStateWithoutStartingLifecycle(t *testing.T) {
	t.Parallel()

	clock := &recordingHostClock{}
	service := internalservice.New(clock, readySlotFacts{capacity: 1})
	if service == nil {
		t.Fatal("New returned nil service")
	}
	if clock.timerCreates != 0 {
		t.Fatalf("timer creates during construction = %d, want 0", clock.timerCreates)
	}
}

func TestAcquireModelLeaseReturnsDetachedActiveLeaseFacts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	scope := mustRuntimeScopeRef(t, "leases-acquire-ready")
	service := internalservice.New(
		fixedHostClock{now: now},
		readySlotFacts{capacity: 2},
	)

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	if acquired.Lease.Lease.IsZero() ||
		acquired.Lease.Scope != scope ||
		acquired.Lease.ModelName != "local-model" ||
		acquired.Lease.Holder != "worker-a" ||
		acquired.Lease.Status != models.ModelLeaseStatusActive ||
		acquired.Lease.HostReadiness != models.ReadinessStateReady ||
		acquired.Lease.ExpiresAt != now.Add(hostleases.DefaultLeaseTTL) {
		t.Fatalf("AcquireModelLease = %#v, want opaque active lease facts", acquired)
	}
}

func TestAcquireModelLeaseRejectsBlankHolderWithoutConsumingCapacity(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-acquire-holder")
	facts := readySlotFacts{capacity: 1}
	service := internalservice.New(fixedHostClock{}, facts)

	_, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: " ",
	})
	if !errors.Is(err, models.ErrHostInvalidHolder) {
		t.Fatalf("AcquireModelLease blank holder = %v, want ErrHostInvalidHolder", err)
	}

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease after invalid holder = %v, want success", err)
	}
	if acquired.Lease.Lease.IsZero() {
		t.Fatal("expected lease after invalid holder did not consume capacity")
	}
}

func TestAcquireModelLeaseIssuesUniqueLeaseIdentities(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-acquire-unique")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 2})
	request := models.AcquireModelLeaseRequest{
		Scope: scope,
		Name:  "local-model",
	}

	first, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "caller-1",
	})
	if err != nil {
		t.Fatalf("first AcquireModelLease: %v", err)
	}
	second, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "caller-2",
	})
	if err != nil {
		t.Fatalf("second AcquireModelLease: %v", err)
	}
	if first.Lease.Lease.String() == second.Lease.Lease.String() {
		t.Fatalf("lease identities must be unique: %q", first.Lease.Lease.String())
	}
}

func TestAcquireModelLeaseRejectsCapacityExhaustion(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-acquire-exhausted")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 1})
	request := models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	}

	first, err := service.AcquireModelLease(context.Background(), request)
	if err != nil {
		t.Fatalf("first AcquireModelLease: %v", err)
	}
	if first.Lease.Lease.IsZero() {
		t.Fatal("first lease identity is zero")
	}

	_, err = service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "worker-b",
	})
	if !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("second AcquireModelLease = %v, want ErrHostCapacityExhausted", err)
	}
}

func TestAcquireModelLeaseRejectsRuntimeNotReady(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-acquire-not-ready")
	service := internalservice.New(
		fixedHostClock{},
		readySlotFacts{readiness: models.ReadinessStateLoading, capacity: 1},
	)

	_, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("AcquireModelLease not ready = %v, want ErrHostRuntimeNotReady", err)
	}
}

func TestGetModelLeaseRemainsContractOnlyUntilExpiryPacket(t *testing.T) {
	t.Parallel()

	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 1})
	ctx := context.Background()
	_, err := service.GetModelLease(ctx, models.GetModelLeaseRequest{})
	if !errors.Is(err, models.ErrUnsupportedOperation) {
		t.Fatalf("GetModelLease error = %v, want ErrUnsupportedOperation", err)
	}
}

func TestReleaseModelLeaseFreesCapacityForSubsequentAcquire(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-release-free")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 1})
	request := models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	}

	first, err := service.AcquireModelLease(context.Background(), request)
	if err != nil {
		t.Fatalf("first AcquireModelLease: %v", err)
	}
	_, err = service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "worker-b",
	})
	if !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("second AcquireModelLease = %v, want ErrHostCapacityExhausted", err)
	}

	released, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: first.Lease.Lease,
	})
	if err != nil {
		t.Fatalf("ReleaseModelLease: %v", err)
	}
	if released.Outcome != models.ModelLeaseReleased ||
		released.Lease.Status != models.ModelLeaseStatusReleased {
		t.Fatalf("ReleaseModelLease = %#v, want released lease", released)
	}

	third, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "worker-c",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease after release: %v", err)
	}
	if third.Lease.Lease.IsZero() {
		t.Fatal("expected lease after release freed capacity")
	}
}

func TestReleaseModelLeaseRejectsUnknownAndAlreadyReleased(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-release-not-found")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 2})
	unknown, err := (models.ModelLeaseRef{}).Parse("model-lease-unknown")
	if err != nil {
		t.Fatalf("parse lease ref: %v", err)
	}
	_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: unknown,
	})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseModelLease unknown = %v, want ErrHostLeaseNotFound", err)
	}

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if err != nil {
		t.Fatalf("first ReleaseModelLease: %v", err)
	}
	_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseModelLease already released = %v, want ErrHostLeaseNotFound", err)
	}
}

func TestAcquireModelLeaseHonoursCancelledContextWithoutConsumingCapacity(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-acquire-cancel")
	service := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 1})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if !errors.Is(err, models.ErrHostCancelled) {
		t.Fatalf("AcquireModelLease cancelled = %v, want ErrHostCancelled", err)
	}

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-b",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease after cancel = %v, want success", err)
	}
	if acquired.Lease.Lease.IsZero() {
		t.Fatal("cancelled acquire should not consume capacity")
	}
}

func TestReleaseModelLeaseNotifiesCapacityCoordinator(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-coordinator")
	coordinator := &recordingSlotCapacityCoordinator{}
	leasesSvc := internalservice.New(fixedHostClock{}, readySlotFacts{capacity: 1})
	hostleases.BindCoordinator(leasesSvc, coordinator)

	acquired, err := leasesSvc.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	if coordinator.acquired != 1 || coordinator.released != 0 {
		t.Fatalf("coordinator after acquire = (%d, %d), want (1, 0)",
			coordinator.acquired, coordinator.released)
	}

	_, err = leasesSvc.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if err != nil {
		t.Fatalf("ReleaseModelLease: %v", err)
	}
	if coordinator.acquired != 1 || coordinator.released != 1 {
		t.Fatalf("coordinator after release = (%d, %d), want (1, 1)",
			coordinator.acquired, coordinator.released)
	}
}

type recordingSlotCapacityCoordinator struct {
	acquired int
	released int
}

func (coordinator *recordingSlotCapacityCoordinator) OnLeaseCapacityAcquired(
	models.RuntimeScopeRef,
	string,
) {
	coordinator.acquired++
}

func (coordinator *recordingSlotCapacityCoordinator) OnLeaseCapacityReleased(
	models.RuntimeScopeRef,
	string,
) {
	coordinator.released++
}

func TestOpenRuntimeScopeDoesNotConstructAnotherLeasesOwner(t *testing.T) {
	t.Parallel()

	clock := &recordingHostClock{}
	service := internalservice.New(clock, readySlotFacts{capacity: 1})
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

type fixedHostClock struct {
	now time.Time
}

func (clock fixedHostClock) Now() time.Time {
	if clock.now.IsZero() {
		return time.Unix(0, 0)
	}
	return clock.now
}

func (clock fixedHostClock) NewTimer(time.Duration) models.HostTimer {
	panic("host timer created during leases acquire tests")
}

type readySlotFacts struct {
	readiness models.ReadinessState
	capacity  int
}

func (facts readySlotFacts) SlotFacts(
	context.Context,
	models.RuntimeScopeRef,
	string,
) (hostleases.SlotFacts, error) {
	readiness := facts.readiness
	if readiness == "" {
		readiness = models.ReadinessStateReady
	}
	return hostleases.SlotFacts{
		Readiness: readiness,
		Capacity:  facts.capacity,
	}, nil
}

func mustRuntimeScopeRef(t *testing.T, value string) models.RuntimeScopeRef {
	t.Helper()
	ref, err := models.RuntimeScopeRef{}.Parse(value)
	if err != nil {
		t.Fatalf("parse runtime scope ref: %v", err)
	}
	return ref
}
