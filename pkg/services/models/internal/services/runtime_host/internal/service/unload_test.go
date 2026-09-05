package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
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

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "idle-unload")
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
		internalservice.HostPolicyTestConfig{IdleUnloadAfter: 40 * time.Millisecond},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	internalservice.AcquireSlotCapacity(service, ref, "OMNIVOICE_Q4_K_M")
	cfg := supervisedRuntimeConfig()
	internalservice.ReleaseSlotCapacity(service, ref, "OMNIVOICE_Q4_K_M", &cfg)

	deadline := time.Now().Add(2 * time.Second)
	for stopCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for idle unload")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestIdleUnloadDoesNotStopActiveCapacityHolder(t *testing.T) {
	t.Parallel()

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "idle-active-holder")
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
		internalservice.HostPolicyTestConfig{IdleUnloadAfter: 40 * time.Millisecond},
	)

	_, err := service.EnsureModelHost(context.Background(), models.EnsureModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	internalservice.AcquireSlotCapacity(service, ref, "OMNIVOICE_Q4_K_M")

	time.Sleep(120 * time.Millisecond)
	if stopCount.Load() != 0 {
		t.Fatalf("stop count = %d, want 0 while holder active", stopCount.Load())
	}

	inspected, err := service.InspectModelHost(context.Background(), models.InspectModelHostRequest{
		Scope: ref,
		Name:  "OMNIVOICE_Q4_K_M",
	})
	if err != nil {
		t.Fatalf("InspectModelHost: %v", err)
	}
	if inspected.Host.ReadinessState != models.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY", inspected.Host.ReadinessState)
	}
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

	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	cacheDirectory := t.TempDir()
	writeCacheFixture(t, cacheDirectory, true)
	scopes := newScopes(t, "idle-unload-lease-release")
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
		leaseReadySlotFacts{capacity: 1},
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
	acquired, err := leases.AcquireModelLease(context.Background(), models.AcquireModelLeaseRequest{
		Scope:  ref,
		Name:   "OMNIVOICE_Q4_K_M",
		Holder: "worker-a",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	if _, err := leases.ReleaseModelLease(context.Background(), models.ReleaseModelLeaseRequest{
		Scope: ref,
		Lease: acquired.Lease.Lease,
	}); err != nil {
		t.Fatalf("ReleaseModelLease: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for stopCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for idle unload after lease release")
		}
		time.Sleep(10 * time.Millisecond)
	}
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
