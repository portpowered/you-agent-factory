package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
)

func TestStopModelHostStopsReadySupervisedProcess(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "explicit-unload")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	}
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}

	stopped, err := service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("StopModelHost: %v", err)
	}
	if stopped.Outcome != models.HostStopStopped ||
		stopped.Host.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("stop result = %#v, want stopped/installed", stopped)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want 1", stopCount.Load())
	}
	inspected, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if inspected.Host.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("inspect after stop = %#v, want installed without live slot", inspected.Host)
	}
}

func TestStopModelHostRejectsLoadingRuntime(t *testing.T) {
	t.Parallel()

	healthChecker := &blockingHealthChecker{started: make(chan struct{})}
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "unload-loading")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	service := internalservice.NewWithSupervisorTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		&fakeProcessLauncher{
			newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
				return newFakeManagedProcess(spec.HealthEndpoint, nil)
			},
		},
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{
			ReadinessTimeout:    time.Second,
			HealthCheckInterval: 25 * time.Millisecond,
			HealthChecker:       healthChecker,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := service.EnsureModelHost(ctx, models.EnsureModelHostRequest{
			Scope: ref,
			Name:  "OMNIVOICE_Q4_K_M",
		})
		errCh <- err
	}()
	<-healthChecker.started

	_, err := service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	cancel()
	<-errCh

	var readinessErr *models.HostReadinessError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("StopModelHost error = %v, want *HostReadinessError", err)
	}
	if !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("StopModelHost error = %v, want ErrHostCapacityExhausted", err)
	}
	if readinessErr.Snapshot.ReadinessState != models.ReadinessStateLoading {
		t.Fatalf("readiness = %s, want LOADING", readinessErr.Snapshot.ReadinessState)
	}
}

func TestCloseRuntimeScopeCancelsAndJoinsInFlightHostLoad(t *testing.T) {
	t.Parallel()

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "close-in-flight-load")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := newBlockedProcessLauncher()
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})
	closer, ok := service.(interface {
		CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error
	})
	if !ok {
		t.Fatal("runtime host does not expose CloseRuntimeScope")
	}

	ensureErrCh := make(chan error, 1)
	go func() {
		_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
			Scope: ref,
			Name:  "OMNIVOICE_Q4_K_M",
		})
		ensureErrCh <- err
	}()
	awaitSignal(t, launcher.started, "managed process launcher did not enter the in-flight load")

	closeErrCh := make(chan error, 1)
	go func() {
		closeErrCh <- closer.CloseRuntimeScope(context.Background(), ref)
	}()
	awaitLoadCancellation(t, launcher, ensureErrCh, closeErrCh)
	launcher.releaseLoad()

	ensureErr := awaitError(t, ensureErrCh, "in-flight EnsureModelHost did not return after scope close")
	if !errors.Is(ensureErr, models.ErrHostCancelled) || !errors.Is(ensureErr, context.Canceled) {
		t.Fatalf("EnsureModelHost error = %v, want host cancellation with context.Canceled", ensureErr)
	}

	if err := awaitError(t, closeErrCh, "CloseRuntimeScope did not join the in-flight load"); err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}

	process := launcher.loadedProcess()
	if process == nil {
		t.Fatal("launcher did not return the late-created managed process")
	}
	awaitSignal(t, process.stopCh, "late-created managed process survived scope close")
}

func TestStopModelHostRejectsActiveCapacityHolder(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "unload-active-holder")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	service := newTestRuntimeHostWithScopesAndClock(t, scopes, &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	}, realHostClock{})

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	internalservice.AcquireSlotCapacity(service, ref, "OMNIVOICE_Q4_K_M")

	_, err = service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("StopModelHost with active holder = %v, want capacity exhausted", err)
	}

	cfg := supervisedRuntimeConfig()
	internalservice.ReleaseSlotCapacity(service, ref, "OMNIVOICE_Q4_K_M", &cfg)
	_, err = service.StopModelHost(context.Background(), models.StopModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("StopModelHost after release: %v", err)
	}
}

func TestIdleUnloadStopsRuntimeAfterLastCapacityReleased(t *testing.T) {
	t.Parallel()
	host, clock, process, ref := newControlledIdleHost(t)
	acquired, err := host.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M", Holder: "worker-a"})
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := host.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{Scope: ref, Lease: acquired.Lease.Lease}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if clock.timerCount() != 1 {
		t.Fatalf("idle timers = %d, want 1", clock.timerCount())
	}
	clock.fireAll()
	awaitSignal(t, process.stopped, "idle timer did not stop selected process")
	inspected, err := host.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
	if err != nil || inspected.Host.LifecycleState != models.LifecycleStateInstalled {
		t.Fatalf("after idle unload = %#v, %v", inspected, err)
	}
}

func TestIdleUnloadDoesNotStopActiveCapacityHolder(t *testing.T) {
	t.Parallel()
	host, clock, process, ref := newControlledIdleHost(t)
	acquire := models.AcquireModelLeaseRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M", Holder: "worker-a"}
	acquired, err := host.AcquireModelLease(context.Background(), acquire)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if _, err := host.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{Scope: ref, Lease: acquired.Lease.Lease}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if clock.timerCount() != 1 {
		t.Fatalf("idle timers = %d, want 1", clock.timerCount())
	}
	if _, err := host.AcquireModelLease(context.Background(), acquire); err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	if clock.timerCount() != 0 {
		t.Fatalf("reacquire did not cancel idle timer")
	}
	clock.fireAll()
	select {
	case <-process.stopped:
		t.Fatal("active process stopped")
	default:
	}
	inspected, err := host.InspectModelHost(context.Background(), models.InspectModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
	if err != nil || inspected.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("active host = %#v, %v", inspected, err)
	}
}

func newControlledIdleHost(t *testing.T) (runtimehost.Service, *deterministicHostClock, *controlledManagedProcess, models.RuntimeScopeRef) {
	t.Helper()
	clock := newDeterministicHostClock()
	cache := t.TempDir()
	writeCacheFixture(t, cache, true)
	scopes := newScopes(t, t.Name())
	ref := openScope(t, scopes, cache, supervisedRuntimeConfig())
	launcher := &controlledProcessLauncher{}
	host, err := internalservice.NewWiredWithSupervisorConfig(scopes, mustAssetsService(t, scopes), launcher, http.DefaultClient, clock, nil, nil,
		internalservice.SupervisorTestConfig{HealthChecker: alwaysHealthyChecker{}}, internalservice.HostPolicyTestConfig{IdleUnloadAfter: time.Hour})
	if err != nil {
		t.Fatalf("construct host: %v", err)
	}
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })
	if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	return host, clock, launcher.process(0), ref
}

func TestResourcePressureEvictsIdleRuntime(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectoryA := t.TempDir()
	writeCacheFixture(t, cacheDirectoryA, true)
	cacheDirectoryB := t.TempDir()
	writeCacheFixture(t, cacheDirectoryB, true)
	scopes := newScopes(t, "pressure-eviction")
	refA := openScope(t, scopes, cacheDirectoryA, supervisedRuntimeConfig())
	refB := openScope(t, scopes, cacheDirectoryB, supervisedRuntimeConfig())
	service := newRuntimeHostWithPolicy(
		t,
		scopes,
		&fakeProcessLauncher{
			newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
				process := newFakeManagedProcess(healthServer.URL, nil)
				process.stopFn = func() error {
					stopCount.Add(1)
					return process.defaultStop()
				}
				return process
			},
		},
		internalservice.HostPolicyTestConfig{MaxLoadedRuntimes: 1},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: refA,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost A: %v", err)
	}

	_, err = service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: refB,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost B: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want eviction of idle prior runtime", stopCount.Load())
	}
}

func TestResourcePressureDoesNotEvictPeerScopeActiveLeaseHolder(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)
	cacheDirectoryA, cacheDirectoryB := t.TempDir(), t.TempDir()
	writeCacheFixture(t, cacheDirectoryA, true)
	writeCacheFixture(t, cacheDirectoryB, true)
	scopes := newScopes(t, "pressure-active-peer")
	refA := openScope(t, scopes, cacheDirectoryA, supervisedRuntimeConfig())
	refB := openScope(t, scopes, cacheDirectoryB, supervisedRuntimeConfig())
	var stopCount atomic.Int32
	launcher := &fakeProcessLauncher{
		newProcess: func(modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	}
	host, err := internalservice.NewWiredWithSupervisorConfig(
		scopes, mustAssetsService(t, scopes), launcher, http.DefaultClient,
		realHostClock{}, nil, nil, internalservice.SupervisorTestConfig{},
		internalservice.HostPolicyTestConfig{MaxLoadedRuntimes: 1},
	)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })
	ctx := context.Background()
	requestA := models.EnsureModelHostRequest{Scope: refA, Name: "OMNIVOICE_Q4_K_M"}
	requestB := models.EnsureModelHostRequest{Scope: refB, Name: "OMNIVOICE_Q4_K_M"}
	if _, err := host.EnsureModelHost(ctx, requestA); err != nil {
		t.Fatalf("EnsureModelHost A: %v", err)
	}
	leases := internalservice.LeasesService(host)
	acquired, err := leases.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope: refA, Name: requestA.Name, Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease A: %v", err)
	}
	if _, err := host.EnsureModelHost(ctx, requestB); !errors.Is(err, models.ErrHostCapacityExhausted) {
		t.Fatalf("EnsureModelHost B under pressure = %v, want ErrHostCapacityExhausted", err)
	}
	if stopCount.Load() != 0 || launcher.startCount() != 1 {
		t.Fatalf("pressure stops/starts = %d/%d, want 0/1", stopCount.Load(), launcher.startCount())
	}
	retained, err := host.EnsureModelHost(ctx, requestA)
	if err != nil || retained.Outcome != models.HostEnsureAlreadyReady {
		t.Fatalf("retained A = %#v, %v, want already ready", retained, err)
	}
	if _, err := leases.GetModelLease(ctx, models.GetModelLeaseRequest{
		Scope: refA, Lease: acquired.Lease.Lease,
	}); err != nil {
		t.Fatalf("peer pressure invalidated A lease: %v", err)
	}
	if _, err := leases.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{
		Scope: refA, Lease: acquired.Lease.Lease,
	}); err != nil {
		t.Fatalf("ReleaseModelLease A: %v", err)
	}
	ready, err := host.EnsureModelHost(ctx, requestB)
	if err != nil || ready.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("EnsureModelHost B after release = %#v, %v, want ready", ready, err)
	}
	if stopCount.Load() != 1 || launcher.startCount() != 2 {
		t.Fatalf("release/retry stops/starts = %d/%d, want 1/2", stopCount.Load(), launcher.startCount())
	}
}

func TestShutdownStopsSupervisedRuntimesAndCancelsIdleTimers(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "shutdown-cleanup")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	service := newRuntimeHostWithPolicy(
		t,
		scopes,
		&fakeProcessLauncher{
			newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
				process := newFakeManagedProcess(healthServer.URL, nil)
				process.stopFn = func() error {
					stopCount.Add(1)
					return process.defaultStop()
				}
				return process
			},
		},
		internalservice.HostPolicyTestConfig{IdleUnloadAfter: time.Minute},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}

	if err := internalservice.ShutdownHost(context.Background(), service); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want 1", stopCount.Load())
	}
}

func TestCloseRuntimeScopeStopsOnlyThatScopesSupervisedRuntimes(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "scoped-unload")
	left := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	right := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	launcher := &fakeProcessLauncher{
		newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	}
	host := newTestRuntimeHostWithScopesAndClock(t, scopes, launcher, realHostClock{})
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	for _, scope := range []models.RuntimeScopeRef{left, right} {
		if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
			Scope: scope,
			Name:  "OMNIVOICE_Q4_K_M",
		}); err != nil {
			t.Fatalf("EnsureModelHost(%s): %v", scope, err)
		}
	}

	closer, ok := host.(interface {
		CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error
	})
	if !ok {
		t.Fatal("Runtime Host does not expose optional scope cleanup")
	}
	if err := closer.CloseRuntimeScope(context.Background(), left); err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("scope close stop count = %d, want 1", stopCount.Load())
	}

	retained, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: right,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost(retained scope): %v", err)
	}
	if retained.Outcome != models.HostEnsureAlreadyReady {
		t.Fatalf("retained scope outcome = %q, want already ready", retained.Outcome)
	}

	if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: left,
		Name:  "OMNIVOICE_Q4_K_M",
	}); err != nil {
		t.Fatalf("EnsureModelHost(reopened scope slot): %v", err)
	}
	if launcher.startCount() != 3 {
		t.Fatalf("host starts after scoped cleanup = %d, want 3", launcher.startCount())
	}
}

func TestCloseRuntimeScopeRevokesLeasesOwnedByStoppedHost(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "scoped-lease-cleanup")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	host := internalservice.NewWithLeasesFacts(
		scopes,
		mustAssetsService(t, scopes),
		&fakeProcessLauncher{
			newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
				process := newFakeManagedProcess(healthServer.URL, nil)
				process.stopFn = func() error {
					stopCount.Add(1)
					return process.defaultStop()
				}
				return process
			},
		},
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		leaseReadySlotFacts{capacity: 1},
		internalservice.HostPolicyTestConfig{},
	)
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })

	if _, err := host.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	}); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	leases := internalservice.LeasesService(host)
	acquired, err := leases.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  ref,
		Name:   "OMNIVOICE_Q4_K_M",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}

	closer, ok := host.(interface {
		CloseRuntimeScope(context.Context, models.RuntimeScopeRef) error
	})
	if !ok {
		t.Fatal("Runtime Host does not expose optional scope cleanup")
	}
	if err := closer.CloseRuntimeScope(context.Background(), ref); err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("scope close stop count = %d, want 1", stopCount.Load())
	}
	got, err := leases.GetModelLease(context.Background(), models.GetModelLeaseRequest{
		Scope: ref,
		Lease: acquired.Lease.Lease,
	})
	if !errors.Is(err, models.ErrHostLeaseExpired) || got.Lease.Status != models.ModelLeaseStatusExpired {
		t.Fatalf("GetModelLease after scope close = %#v, %v, want expired lease", got, err)
	}
}

func TestIdleUnloadStopsRuntimeAfterLeaseRelease(t *testing.T) {
	t.Parallel()
	const idleDuration = 40 * time.Millisecond
	clock := &deadlineHostClock{now: time.Unix(0, 0).UTC()}
	process := &observedIdleProcess{
		controlledManagedProcess: newControlledManagedProcess("http://controlled.invalid"),
		stopCompleted:            make(chan struct{}),
	}
	cache := t.TempDir()
	writeCacheFixture(t, cache, true)
	scopes := newScopes(t, t.Name())
	ref := openScope(t, scopes, cache, supervisedRuntimeConfig())
	host, err := internalservice.NewWiredWithSupervisorConfig(
		scopes, mustAssetsService(t, scopes), idleProcessLauncher{process}, http.DefaultClient,
		clock, nil, nil, internalservice.SupervisorTestConfig{HealthChecker: alwaysHealthyChecker{}},
		internalservice.HostPolicyTestConfig{IdleUnloadAfter: idleDuration},
	)
	if err != nil {
		t.Fatalf("construct host: %v", err)
	}
	t.Cleanup(func() { _ = internalservice.ShutdownHost(context.Background(), host) })
	ctx := context.Background()
	if _, err := host.EnsureModelHost(ctx, models.EnsureModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"}); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	acquired, err := host.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M", Holder: "worker-a"})
	if err != nil || acquired.Lease.Status != models.ModelLeaseStatusActive {
		t.Fatalf("accepted lease = %#v, %v", acquired, err)
	}
	releasedAt := clock.Now()
	if _, err := host.ReleaseModelLease(ctx, models.ReleaseModelLeaseRequest{Scope: ref, Lease: acquired.Lease.Lease}); err != nil {
		t.Fatalf("release: %v", err)
	}
	timer := clock.onlyTimer(t)
	if timer.duration != idleDuration || !timer.due.Equal(releasedAt.Add(idleDuration)) {
		t.Fatalf("supplied scheduler duration/deadline = %s/%s, want %s/%s", timer.duration, timer.due, idleDuration, releasedAt.Add(idleDuration))
	}
	clock.advanceTo(timer.due.Add(-time.Nanosecond))
	before, err := host.InspectModelHost(ctx, models.InspectModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
	if err != nil || before.Host.ReadinessState != models.ReadinessStateReady || before.Host.LifecycleState != models.LifecycleStateLoaded || process.stopCalls.Load() != 0 {
		t.Fatalf("before deadline %s: host=%#v err=%v stops=%d", timer.due, before, err, process.stopCalls.Load())
	}
	clock.advanceTo(timer.due)
	awaitSignal(t, process.stopCompleted, "selected deadline did not complete process Stop")
	awaitSignal(t, process.waited, "selected deadline did not complete process Wait")
	after, err := host.InspectModelHost(ctx, models.InspectModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
	if err != nil || after.Host.LifecycleState != models.LifecycleStateInstalled || after.Host.ReadinessState != models.ReadinessStateReady || process.stopCalls.Load() != 1 {
		t.Fatalf("at deadline %s: host=%#v err=%v stops=%d", timer.due, after, err, process.stopCalls.Load())
	}
	clock.advanceTo(timer.due.Add(idleDuration))
	stopped, err := host.StopModelHost(ctx, models.StopModelHostRequest{Scope: ref, Name: "OMNIVOICE_Q4_K_M"})
	if err != nil || stopped.Outcome != models.HostStopAlreadyStopped || stopped.Host.LifecycleState != models.LifecycleStateInstalled || process.stopCalls.Load() != 1 {
		t.Fatalf("already unloaded: result=%#v err=%v stops=%d", stopped, err, process.stopCalls.Load())
	}
	t.Logf("C02/C03: supplied scheduler release=%s duration=%s due=%s; pre-deadline READY/LOADED, deadline INSTALLED (asset readiness remains READY), ALREADY_STOPPED, actual Stop calls=1", releasedAt, timer.duration, timer.due)
}

// All clock and timer state uses one lock; delivery is buffered and happens once.
// Virtual advancement triggers behavior. awaitSignal uses real time only as a failure ceiling.
type deadlineHostClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*deadlineHostTimer
}

type deadlineHostTimer struct {
	clock    *deadlineHostClock
	duration time.Duration
	due      time.Time
	channel  chan time.Time
	stopped  bool
	fired    bool
}

func (clock *deadlineHostClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *deadlineHostClock) NewTimer(duration time.Duration) modelseffects.HostTimer {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	timer := &deadlineHostTimer{clock: clock, duration: duration, due: clock.now.Add(duration), channel: make(chan time.Time, 1)}
	clock.timers = append(clock.timers, timer)
	return timer
}

func (clock *deadlineHostClock) onlyTimer(t *testing.T) *deadlineHostTimer {
	t.Helper()
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if len(clock.timers) != 1 {
		t.Fatalf("supplied scheduler timers = %d, want one idle timer", len(clock.timers))
	}
	return clock.timers[0]
}

func (clock *deadlineHostClock) advanceTo(now time.Time) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	if now.Before(clock.now) {
		panic("controlled scheduler cannot move backwards")
	}
	clock.now = now
	for _, timer := range clock.timers {
		if !timer.stopped && !timer.fired && !now.Before(timer.due) {
			timer.fired = true
			timer.channel <- timer.due
		}
	}
}

func (timer *deadlineHostTimer) C() <-chan time.Time { return timer.channel }

func (timer *deadlineHostTimer) Stop() bool {
	timer.clock.mu.Lock()
	defer timer.clock.mu.Unlock()
	if timer.stopped || timer.fired {
		return false
	}
	timer.stopped = true
	return true
}

type observedIdleProcess struct {
	*controlledManagedProcess
	stopCalls     atomic.Int32
	stopCompleted chan struct{}
	completedOnce sync.Once
}

func (process *observedIdleProcess) Stop(ctx context.Context) error {
	// Count entry before the underlying fake's sync.Once can mask duplicate effects.
	process.stopCalls.Add(1)
	err := process.controlledManagedProcess.Stop(ctx)
	process.completedOnce.Do(func() { close(process.stopCompleted) })
	return err
}

type idleProcessLauncher struct{ process *observedIdleProcess }

func (launcher idleProcessLauncher) Start(context.Context, modelseffects.HostProcessStartSpec) (modelseffects.HostManagedProcess, error) {
	return launcher.process, nil
}

func TestIdleUnloadDoesNotStopActiveLeaseHolder(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "idle-active-lease-holder")
	ref := openScope(t, scopes, cacheDirectory, supervisedRuntimeConfig())
	service := internalservice.NewWithLeasesFacts(
		scopes,
		mustAssetsService(t, scopes),
		&fakeProcessLauncher{
			newProcess: func(spec modelseffects.HostProcessStartSpec) *fakeManagedProcess {
				process := newFakeManagedProcess(healthServer.URL, nil)
				process.stopFn = func() error {
					stopCount.Add(1)
					return process.defaultStop()
				}
				return process
			},
		},
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		leaseReadySlotFacts{capacity: 2},
		internalservice.HostPolicyTestConfig{IdleUnloadAfter: 40 * time.Millisecond},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}

	leases := internalservice.LeasesService(service)
	if _, err := leases.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  ref,
		Name:   "OMNIVOICE_Q4_K_M",
		Holder: "worker-a",
	}); err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if stopCount.Load() != 0 {
		t.Fatalf("stop count = %d, want 0 while lease holder active", stopCount.Load())
	}
}

type leaseReadySlotFacts struct {
	capacity int
}

func (facts leaseReadySlotFacts) SlotFacts(
	context.Context,
	models.RuntimeScopeRef,
	string,
) (modelseffects.SlotFacts, error) {
	return modelseffects.SlotFacts{
		Readiness: models.ReadinessStateReady,
		Capacity:  facts.capacity,
	}, nil
}

func newRuntimeHostWithPolicy(
	t *testing.T,
	scopes runtimescopes.Service,
	launcher modelseffects.HostProcessLauncher,
	policy internalservice.HostPolicyTestConfig,
) runtimehost.Service {
	t.Helper()
	return internalservice.NewWithHostTestConfig(
		scopes,
		mustAssetsService(t, scopes),
		launcher,
		http.DefaultClient,
		realHostClock{},
		nil,
		nil,
		internalservice.SupervisorTestConfig{},
		policy,
	)
}

type blockedProcessLauncher struct {
	started        chan struct{}
	cancelObserved chan struct{}
	release        chan struct{}
	startedOnce    sync.Once
	cancelOnce     sync.Once
	releaseOnce    sync.Once
	mu             sync.Mutex
	process        *fakeManagedProcess
}

func newBlockedProcessLauncher() *blockedProcessLauncher {
	return &blockedProcessLauncher{
		started:        make(chan struct{}),
		cancelObserved: make(chan struct{}),
		release:        make(chan struct{}),
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatal(message)
	}
}

func awaitError(t *testing.T, result <-chan error, message string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal(message)
		return nil
	}
}

func awaitLoadCancellation(
	t *testing.T,
	launcher *blockedProcessLauncher,
	ensureErrCh <-chan error,
	closeErrCh <-chan error,
) {
	t.Helper()
	select {
	case <-launcher.cancelObserved:
	case <-time.After(5 * time.Second):
		launcher.releaseLoad()
		_ = awaitError(t, ensureErrCh, "in-flight EnsureModelHost did not unwind after cancellation")
		_ = awaitError(t, closeErrCh, "CloseRuntimeScope did not unwind after cancellation")
		t.Fatal("runtime-scope close did not cancel the in-flight load")
	}
}

func (launcher *blockedProcessLauncher) Start(
	ctx context.Context,
	spec modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.startedOnce.Do(func() { close(launcher.started) })
	<-ctx.Done()
	launcher.cancelOnce.Do(func() { close(launcher.cancelObserved) })
	<-launcher.release
	process := newFakeManagedProcess(spec.HealthEndpoint, nil)
	launcher.mu.Lock()
	launcher.process = process
	launcher.mu.Unlock()
	return process, nil
}

func (launcher *blockedProcessLauncher) releaseLoad() {
	launcher.releaseOnce.Do(func() { close(launcher.release) })
}

func (launcher *blockedProcessLauncher) loadedProcess() *fakeManagedProcess {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.process
}

type blockingHealthChecker struct {
	started chan struct{}
	once    sync.Once
}

func (checker *blockingHealthChecker) Check(ctx context.Context, _ string) error {
	checker.once.Do(func() { close(checker.started) })
	<-ctx.Done()
	return ctx.Err()
}
