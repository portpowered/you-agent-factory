package models_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func (unsupportedRuntimeScopePeer) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (unsupportedRuntimeScopePeer) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) EnsureModelHost(
	context.Context,
	models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	return models.EnsureModelHostResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) InspectModelHost(
	context.Context,
	models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	return models.InspectModelHostResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) StopModelHost(
	context.Context,
	models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	return models.StopModelHostResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) AcquireModelLease(
	context.Context,
	models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	return models.AcquireModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) GetModelLease(
	context.Context,
	models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	return models.GetModelLeaseResult{}, models.ErrUnsupportedOperation
}

func (*runtimeScopePeerService) ReleaseModelLease(
	context.Context,
	models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	return models.ReleaseModelLeaseResult{}, models.ErrUnsupportedOperation
}

// scopedHostLeasePeer proves that a peer can implement supervised host and
// nested lease behavior using only detached Models root values.
type scopedHostLeasePeer struct {
	*runtimeScopePeerService
	now             func() time.Time
	hosts           map[string]models.ModelHostSnapshot
	leases          map[models.ModelLeaseRef]models.ModelLease
	hostFailures    map[string]error
	acquireFailures map[string]error
	nextLease       int
}

func newScopedHostLeasePeer(now func() time.Time) *scopedHostLeasePeer {
	return &scopedHostLeasePeer{
		runtimeScopePeerService: newRuntimeScopePeerService("host-peer"),
		now:                     now,
		hosts:                   make(map[string]models.ModelHostSnapshot),
		leases:                  make(map[models.ModelLeaseRef]models.ModelLease),
		hostFailures:            make(map[string]error),
		acquireFailures:         make(map[string]error),
	}
}

func (s *scopedHostLeasePeer) EnsureModelHost(
	ctx context.Context,
	request models.EnsureModelHostRequest,
) (models.EnsureModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.EnsureModelHostResult{}, err
	}
	if err := s.hostFailures[request.Name]; err != nil {
		return models.EnsureModelHostResult{}, err
	}
	key := scopedModelKey(request.Scope, request.Name)
	host, exists := s.hosts[key]
	outcome := models.HostEnsureAlreadyReady
	if !exists || host.ReadinessState != models.ReadinessStateReady {
		host = models.ModelHostSnapshot{
			Scope:          request.Scope,
			ModelName:      request.Name,
			ReadinessState: models.ReadinessStateReady,
			LifecycleState: models.LifecycleStateLoaded,
			Diagnostics:    map[string]string{"supervision": "ready"},
		}
		s.hosts[key] = host
		outcome = models.HostEnsureBecameReady
	}
	return models.EnsureModelHostResult{Host: host.Clone(), Outcome: outcome}, nil
}

func (s *scopedHostLeasePeer) InspectModelHost(
	ctx context.Context,
	request models.InspectModelHostRequest,
) (models.InspectModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.InspectModelHostResult{}, err
	}
	if err := s.hostFailures[request.Name]; err != nil {
		return models.InspectModelHostResult{}, err
	}
	host, ok := s.hosts[scopedModelKey(request.Scope, request.Name)]
	if !ok {
		return models.InspectModelHostResult{}, models.ErrHostRuntimeNotReady
	}
	return models.InspectModelHostResult{Host: host.Clone()}, nil
}

func (s *scopedHostLeasePeer) StopModelHost(
	ctx context.Context,
	request models.StopModelHostRequest,
) (models.StopModelHostResult, error) {
	if err := request.Validate(); err != nil {
		return models.StopModelHostResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.StopModelHostResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.StopModelHostResult{}, err
	}
	key := scopedModelKey(request.Scope, request.Name)
	host, exists := s.hosts[key]
	outcome := models.HostStopAlreadyStopped
	if exists && host.LifecycleState == models.LifecycleStateLoaded {
		host.LifecycleState = models.LifecycleStateInstalled
		s.hosts[key] = host
		outcome = models.HostStopStopped
	}
	return models.StopModelHostResult{Host: host.Clone(), Outcome: outcome}, nil
}

func (s *scopedHostLeasePeer) AcquireModelLease(
	ctx context.Context,
	request models.AcquireModelLeaseRequest,
) (models.AcquireModelLeaseResult, error) {
	if err := request.Validate(); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	if err := s.acquireFailures[request.Name]; err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	host, ok := s.hosts[scopedModelKey(request.Scope, request.Name)]
	if !ok || host.ReadinessState != models.ReadinessStateReady {
		return models.AcquireModelLeaseResult{}, models.ErrHostRuntimeNotReady
	}
	s.nextLease++
	ref, err := (models.ModelLeaseRef{}).Parse(fmt.Sprintf("host-peer:lease:%d", s.nextLease))
	if err != nil {
		return models.AcquireModelLeaseResult{}, err
	}
	lease := models.ModelLease{
		Lease:         ref,
		Scope:         request.Scope,
		ModelName:     request.Name,
		Holder:        request.Holder,
		ExpiresAt:     s.now().Add(time.Minute),
		Status:        models.ModelLeaseStatusActive,
		HostReadiness: host.ReadinessState,
	}
	s.leases[ref] = lease
	return models.AcquireModelLeaseResult{Lease: lease}, nil
}

func (s *scopedHostLeasePeer) GetModelLease(
	ctx context.Context,
	request models.GetModelLeaseRequest,
) (models.GetModelLeaseResult, error) {
	if err := request.Validate(); err != nil {
		return models.GetModelLeaseResult{}, err
	}
	if err := s.scopeUseError(request.Scope); err != nil {
		return models.GetModelLeaseResult{}, err
	}
	if err := hostContextError(ctx); err != nil {
		return models.GetModelLeaseResult{}, err
	}
	lease, ok := s.leases[request.Lease]
	if !ok || lease.Scope != request.Scope {
		return models.GetModelLeaseResult{}, models.ErrHostLeaseNotFound
	}
	if lease.Status == models.ModelLeaseStatusActive && !s.now().Before(lease.ExpiresAt) {
		lease.Status = models.ModelLeaseStatusExpired
		s.leases[request.Lease] = lease
		return models.GetModelLeaseResult{Lease: lease}, models.ErrHostLeaseExpired
	}
	return models.GetModelLeaseResult{Lease: lease}, nil
}

func (s *scopedHostLeasePeer) ReleaseModelLease(
	ctx context.Context,
	request models.ReleaseModelLeaseRequest,
) (models.ReleaseModelLeaseResult, error) {
	current, err := s.GetModelLease(ctx, models.GetModelLeaseRequest(request))
	if err != nil {
		return models.ReleaseModelLeaseResult{Lease: current.Lease}, err
	}
	outcome := models.ModelLeaseReleased
	if current.Lease.Status == models.ModelLeaseStatusReleased {
		outcome = models.ModelLeaseAlreadyReleased
	} else {
		current.Lease.Status = models.ModelLeaseStatusReleased
		s.leases[request.Lease] = current.Lease
	}
	return models.ReleaseModelLeaseResult{Lease: current.Lease, Outcome: outcome}, nil
}

func scopedModelKey(scope models.RuntimeScopeRef, name string) string {
	return scope.String() + ":" + name
}

func hostContextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %v", models.ErrHostCancelled, err)
	}
	return nil
}

func openHostScope(t *testing.T, service models.Service) models.RuntimeScopeRef {
	t.Helper()
	opened, err := service.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	return opened.Scope
}

func TestScopedHostLease_ReadinessAcquireInspectReleaseAndStop(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	fake := newScopedHostLeasePeer(func() time.Time { return now })
	var service models.Service = fake
	scope := openHostScope(t, service)
	assertHostReadyAndDetached(t, service, scope)
	lease := assertLeaseAcquireInspectRelease(t, service, scope, now)
	if lease.IsZero() {
		t.Fatal("lease reference is zero after successful lifecycle")
	}
	assertHostStopped(t, service, scope)
}

func assertHostReadyAndDetached(t *testing.T, service models.Service, scope models.RuntimeScopeRef) {
	t.Helper()
	ensured, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: scope,
		Name:  "local-model",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	if ensured.Outcome != models.HostEnsureBecameReady ||
		ensured.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("EnsureModelHost = %#v, want newly ready host", ensured)
	}
	ensured.Host.Diagnostics["supervision"] = "mutated"
	inspected, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: scope,
		Name:  "local-model",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if inspected.Host.Diagnostics["supervision"] != "ready" {
		t.Fatal("host result mutation changed retained host state")
	}
}

func assertLeaseAcquireInspectRelease(
	t *testing.T,
	service models.Service,
	scope models.RuntimeScopeRef,
	now time.Time,
) models.ModelLeaseRef {
	t.Helper()
	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  scope,
		Name:   "local-model",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	if acquired.Lease.Lease.IsZero() || acquired.Lease.Scope != scope ||
		acquired.Lease.ModelName != "local-model" || acquired.Lease.Holder != "worker-a" ||
		acquired.Lease.Status != models.ModelLeaseStatusActive {
		t.Fatalf("AcquireModelLease = %#v, want opaque active lease facts", acquired)
	}
	got, err := service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if err != nil || got.Lease.ExpiresAt != now.Add(time.Minute) {
		t.Fatalf("GetModelLease = (%#v, %v), want active expiry facts", got, err)
	}

	released, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if err != nil || released.Outcome != models.ModelLeaseReleased ||
		released.Lease.Status != models.ModelLeaseStatusReleased {
		t.Fatalf("ReleaseModelLease = (%#v, %v), want released", released, err)
	}
	releasedAgain, err := service.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: scope,
		Lease: acquired.Lease.Lease,
	})
	if err != nil || releasedAgain.Outcome != models.ModelLeaseAlreadyReleased {
		t.Fatalf("second ReleaseModelLease = (%#v, %v), want safe already-released", releasedAgain, err)
	}
	return acquired.Lease.Lease
}

func assertHostStopped(t *testing.T, service models.Service, scope models.RuntimeScopeRef) {
	t.Helper()
	stopped, err := service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: scope,
		Name:  "local-model",
	})
	if err != nil || stopped.Outcome != models.HostStopStopped ||
		stopped.Host.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("StopModelHost = (%#v, %v), want unloaded host", stopped, err)
	}
}

func TestScopedHostLease_NormalizedFailuresRemainDistinct(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	fake := newScopedHostLeasePeer(func() time.Time { return now })
	var service models.Service = fake
	scope := openHostScope(t, service)
	fake.hostFailures["missing-assets"] = models.ErrHostMissingAssets
	fake.hostFailures["loading-timeout"] = models.ErrHostLoadingTimeout
	fake.acquireFailures["exhausted"] = models.ErrHostCapacityExhausted
	fake.acquireFailures["contended"] = models.ErrHostCapacityContended

	for name, want := range map[string]error{
		"missing-assets":  models.ErrHostMissingAssets,
		"loading-timeout": models.ErrHostLoadingTimeout,
	} {
		_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
			Scope: scope,
			Name:  name,
		})
		assertHostFailureIsOnly(t, "EnsureModelHost "+name, err, want)
	}
	_, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: scope, Name: "not-ready", Holder: "worker",
	})
	assertHostFailureIsOnly(t, "AcquireModelLease not-ready", err, models.ErrHostRuntimeNotReady)
	for name, want := range map[string]error{
		"exhausted": models.ErrHostCapacityExhausted,
		"contended": models.ErrHostCapacityContended,
	} {
		_, err = service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
			Scope: scope, Name: name, Holder: "worker",
		})
		assertHostFailureIsOnly(t, "AcquireModelLease "+name, err, want)
	}
	_, err = service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: scope, Name: "model", Holder: "",
	})
	assertHostFailureIsOnly(t, "AcquireModelLease invalid-holder", err, models.ErrHostInvalidHolder)

	unknown, parseErr := (models.ModelLeaseRef{}).Parse("host-peer:lease:unknown")
	if parseErr != nil {
		t.Fatalf("parse unknown lease: %v", parseErr)
	}
	_, err = service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope, Lease: unknown,
	})
	assertHostFailureIsOnly(t, "GetModelLease unknown", err, models.ErrHostLeaseNotFound)

	_, err = service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: scope, Name: "expiring",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost expiring: %v", err)
	}
	acquired, err := service.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope: scope, Name: "expiring", Holder: "worker",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease expiring: %v", err)
	}
	now = now.Add(time.Minute)
	expired, err := service.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: scope, Lease: acquired.Lease.Lease,
	})
	assertHostFailureIsOnly(t, "GetModelLease expired", err, models.ErrHostLeaseExpired)
	if expired.Lease.Status != models.ModelLeaseStatusExpired {
		t.Fatalf("expired lease status = %q, want EXPIRED", expired.Lease.Status)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.InspectModelHost(cancelled, models.InspectModelHostRequest{
		Scope: scope, Name: "expiring",
	})
	assertHostFailureIsOnly(t, "InspectModelHost cancelled", err, models.ErrHostCancelled)
}

func assertHostFailureIsOnly(t *testing.T, label string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("%s = %v, want %v", label, err, want)
	}
	for _, other := range []error{
		models.ErrHostMissingAssets,
		models.ErrHostLoadingTimeout,
		models.ErrHostRuntimeNotReady,
		models.ErrHostCapacityExhausted,
		models.ErrHostCapacityContended,
		models.ErrHostLeaseExpired,
		models.ErrHostLeaseNotFound,
		models.ErrHostInvalidHolder,
		models.ErrHostCancelled,
	} {
		if other != want && errors.Is(err, other) {
			t.Fatalf("%s must keep %v distinct from %v: %v", label, want, other, err)
		}
	}
}
