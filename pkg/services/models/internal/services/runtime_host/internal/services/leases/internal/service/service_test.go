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

func TestGetModelLeaseReportsExpiredLeaseAndFreesCapacity(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	clock := &advanceableHostClock{now: start}
	scope := mustRuntimeScopeRef(t, "leases-get-expired")
	service := internalservice.New(clock, readySlotFacts{capacity: 1})
	request := models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	}

	acquired, err := service.AcquireModelLease(context.Background(), request)
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	_, err = service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "worker-b",
	})
	if !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("AcquireModelLease before expiry = %v, want ErrHostCapacityExhausted", err)
	}

	clock.now = start.Add(hostleases.DefaultLeaseTTL)
	expired, err := service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if !errors.Is(err, models.ErrHostLeaseExpired) {
		t.Fatalf("GetModelLease expired = %v, want ErrHostLeaseExpired", err)
	}
	if expired.Lease.Status != models.ModelLeaseStatusExpired {
		t.Fatalf("expired lease status = %q, want EXPIRED", expired.Lease.Status)
	}

	afterExpiry, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  request.Scope,
		Name:   request.Name,
		Holder: "worker-c",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease after expiry freed capacity: %v", err)
	}
	if afterExpiry.Lease.Lease.IsZero() {
		t.Fatal("expected lease after expiry freed capacity")
	}
}

func TestReleaseModelLeaseRejectsExpiredWithoutDoubleFree(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	clock := &advanceableHostClock{now: start}
	scope := mustRuntimeScopeRef(t, "leases-release-expired")
	service := internalservice.New(clock, readySlotFacts{capacity: 1})

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}

	clock.now = start.Add(hostleases.DefaultLeaseTTL)
	_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if !errors.Is(err, models.ErrHostLeaseExpired) {
		t.Fatalf("ReleaseModelLease expired = %v, want ErrHostLeaseExpired", err)
	}

	_, err = service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if !errors.Is(err, models.ErrHostLeaseExpired) {
		t.Fatalf("ReleaseModelLease already expired = %v, want ErrHostLeaseExpired", err)
	}

	reacquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-b",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease after expired release did not double-free: %v", err)
	}
	if reacquired.Lease.Lease.IsZero() {
		t.Fatal("capacity should remain available exactly once after expiry")
	}
}

func TestAcquireModelLeaseRejectsContendedCapacity(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-acquire-contended")
	service := internalservice.New(
		fixedHostClock{},
		readySlotFacts{capacity: 2, contendedHolder: "worker-a"},
	)

	_, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-b",
	})
	if !errors.Is(err, models.ErrHostCapacityContended) {
		t.Fatalf("AcquireModelLease contended = %v, want ErrHostCapacityContended", err)
	}

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease contending holder = %v, want success", err)
	}
	if acquired.Lease.Lease.IsZero() {
		t.Fatal("contending holder should acquire when capacity is available")
	}
}

func TestHostLeaseFailureClassificationsRemainDistinct(t *testing.T) {
	t.Parallel()

	scope := mustRuntimeScopeRef(t, "leases-failure-distinct")
	start := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	clock := &advanceableHostClock{now: start}
	service := internalservice.New(clock, readySlotFacts{capacity: 1})

	assertHostLeaseFailureIsOnly(t, "invalid holder", mustAcquireErr(service, models.AcquireModelLeaseRequest{
		Scope: scope, Name: "local-model", Holder: "",
	}), models.ErrHostInvalidHolder)
	assertHostLeaseFailureIsOnly(t, "runtime not ready", mustAcquireErr(service, models.AcquireModelLeaseRequest{
		Scope: scope, Name: "not-ready", Holder: "worker",
	}), models.ErrHostRuntimeNotReady)

	exhaustedService := internalservice.New(
		fixedHostClock{},
		readySlotFacts{capacity: 1},
	)
	_, err := exhaustedService.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: scope, Name: "local-model", Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("seed exhausted capacity: %v", err)
	}
	assertHostLeaseFailureIsOnly(t, "capacity exhausted", mustAcquireErr(exhaustedService, models.AcquireModelLeaseRequest{
		Scope: scope, Name: "local-model", Holder: "worker-b",
	}), models.ErrHostCapacityExhausted)

	contendedService := internalservice.New(
		fixedHostClock{},
		readySlotFacts{capacity: 2, contendedHolder: "holder-a"},
	)
	assertHostLeaseFailureIsOnly(t, "capacity contended", mustAcquireErr(contendedService, models.AcquireModelLeaseRequest{
		Scope: scope, Name: "local-model", Holder: "holder-b",
	}), models.ErrHostCapacityContended)

	unknown, parseErr := (models.ModelLeaseRef{}).Parse("model-lease-unknown")
	if parseErr != nil {
		t.Fatalf("parse unknown lease: %v", parseErr)
	}
	_, err = service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope, Lease: unknown,
	})
	assertHostLeaseFailureIsOnly(t, "lease not found", err, models.ErrHostLeaseNotFound)

	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: scope, Name: "expiring", Holder: "worker",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease expiring: %v", err)
	}
	clock.now = start.Add(hostleases.DefaultLeaseTTL)
	_, err = service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope, Lease: acquired.Lease.Lease,
	})
	assertHostLeaseFailureIsOnly(t, "lease expired", err, models.ErrHostLeaseExpired)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.AcquireModelLease(cancelled, models.AcquireModelLeaseRequest{
		Scope: scope, Name: "local-model", Holder: "worker",
	})
	assertHostLeaseFailureIsOnly(t, "cancelled acquire", err, models.ErrHostCancelled)
}

func mustAcquireErr(
	service hostleases.Service,
	request models.AcquireModelLeaseRequest,
) error {
	_, err := service.AcquireModelLease(context.Background(), request)
	return err
}

func assertHostLeaseFailureIsOnly(t *testing.T, label string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s = %v, want %v", label, err, want)
	}
	for _, other := range []error{
		models.ErrHostCapacityExhausted,
		models.ErrHostCapacityContended,
		models.ErrHostLeaseExpired,
		models.ErrHostLeaseNotFound,
		models.ErrHostInvalidHolder,
		models.ErrHostRuntimeNotReady,
		models.ErrHostCancelled,
	} {
		if other != want && errors.Is(err, other) {
			t.Fatalf("%s must keep %v distinct from %v: %v", label, want, other, err)
		}
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

type advanceableHostClock struct {
	now time.Time
}

func (clock *advanceableHostClock) Now() time.Time {
	return clock.now
}

func (clock *advanceableHostClock) NewTimer(time.Duration) models.HostTimer {
	panic("host timer created during leases expiry tests")
}

type readySlotFacts struct {
	readiness       models.ReadinessState
	capacity        int
	contendedHolder string
}

func (facts readySlotFacts) SlotFacts(
	ctx context.Context,
	scope models.RuntimeScopeRef,
	name string,
) (hostleases.SlotFacts, error) {
	if name == "not-ready" {
		return hostleases.SlotFacts{
			Readiness: models.ReadinessStateLoading,
			Capacity:  facts.capacity,
		}, nil
	}
	readiness := facts.readiness
	if readiness == "" {
		readiness = models.ReadinessStateReady
	}
	return hostleases.SlotFacts{
		Readiness:       readiness,
		Capacity:        facts.capacity,
		ContendedHolder: facts.contendedHolder,
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
