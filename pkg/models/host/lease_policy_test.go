package modelhost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCatalogHost_AllowsConcurrentLeasesWhenCapacityAllows(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfigWithLeaseCapacity(2))
	host := mustNewCatalogHost(t, stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, Options{SourceResolver: DefaultManagedRuntimeSourceResolverAdapter()})

	ctx := context.Background()
	leaseOne, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{Holder: "caller-1"})
	if err != nil {
		t.Fatalf("first AcquireLease: %v", err)
	}
	leaseTwo, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{Holder: "caller-2"})
	if err != nil {
		t.Fatalf("second AcquireLease: %v", err)
	}
	if leaseOne.ID == leaseTwo.ID {
		t.Fatalf("lease ids must be unique: %q", leaseOne.ID)
	}
	if err := host.ReleaseLease(ctx, leaseOne.ID); err != nil {
		t.Fatalf("ReleaseLease leaseOne: %v", err)
	}
	if err := host.ReleaseLease(ctx, leaseTwo.ID); err != nil {
		t.Fatalf("ReleaseLease leaseTwo: %v", err)
	}
}

func TestCatalogHost_RejectsLeaseWhenCapacityExhausted(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := mustNewCatalogHost(t, stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, Options{SourceResolver: DefaultManagedRuntimeSourceResolverAdapter()})

	ctx := context.Background()
	lease, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	_, err = host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("second AcquireLease = %v, want ErrCapacityExhausted", err)
	}
	if err := host.ReleaseLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func TestCatalogHost_SupervisedBackend_SharesEndpointAcrossConcurrentLeases(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var startCount atomic.Int32
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfigWithLeaseCapacity(2))
	cfg := supervisedCatalogFactoryConfig()
	cfg.Resources[0].Capacity = 2
	loaded = mustLoadedCatalogConfig(t, cfg)

	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			startCount.Add(1)
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})

	ctx := context.Background()
	leaseOne, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("first AcquireLease: %v", err)
	}
	leaseTwo, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("second AcquireLease: %v", err)
	}
	if leaseOne.Endpoint != leaseTwo.Endpoint {
		t.Fatalf("endpoints differ: %q vs %q", leaseOne.Endpoint, leaseTwo.Endpoint)
	}
	if startCount.Load() != 1 {
		t.Fatalf("process start count = %d, want 1 shared runtime", startCount.Load())
	}
	if err := host.ReleaseLease(ctx, leaseOne.ID); err != nil {
		t.Fatalf("ReleaseLease leaseOne: %v", err)
	}
	if err := host.ReleaseLease(ctx, leaseTwo.ID); err != nil {
		t.Fatalf("ReleaseLease leaseTwo: %v", err)
	}
}

func TestCatalogHost_IdleUnloadStopsRuntimeAfterLastLeaseReleased(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	})
	host.idleUnloadAfter = 40 * time.Millisecond

	ctx := context.Background()
	lease, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if err := host.ReleaseLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for stopCount.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for idle unload")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCatalogHost_IdleUnloadDoesNotEvictActiveLease(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	})
	host.idleUnloadAfter = 40 * time.Millisecond

	ctx := context.Background()
	lease, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	if stopCount.Load() != 0 {
		t.Fatalf("stop count = %d, want 0 while lease active", stopCount.Load())
	}
	ready, err := host.InspectReadiness(ctx, loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectReadiness: %v", err)
	}
	if ready.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %s, want READY", ready.ReadinessState)
	}
	if err := host.ReleaseLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func TestCatalogHost_ResourcePressureEvictsIdleRuntime(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	var stopCount atomic.Int32
	loadedA := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	loadedBDir := t.TempDir()
	cfgB := supervisedCatalogFactoryConfig()
	cfgB.Workers[0].Model = "OTHER_MODEL"
	cfgB.Resources[0].Model = "OTHER_MODEL"
	loadedB, err := factoryconfig.NewLoadedFactoryConfig(loadedBDir, cfgB, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			process := newFakeManagedProcess(healthServer.URL, nil)
			process.stopFn = func() error {
				stopCount.Add(1)
				return process.defaultStop()
			}
			return process
		},
	})
	host.maxLoadedRuntimes = 1
	host.cacheInspector = stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {Supported: true, Installed: true, InstalledFileCount: 2, CachePath: t.TempDir()},
			"OTHER_MODEL":      {Supported: true, Installed: true, InstalledFileCount: 2, CachePath: t.TempDir()},
		},
	}

	ctx := context.Background()
	lease, err := host.AcquireLease(ctx, loadedA, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease A: %v", err)
	}
	if err := host.ReleaseLease(ctx, lease.ID); err != nil {
		t.Fatalf("ReleaseLease A: %v", err)
	}

	_, err = host.AcquireLease(ctx, loadedB, "OTHER_MODEL", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease B: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want eviction of idle prior runtime", stopCount.Load())
	}
}

func TestCatalogHost_ConcurrentLeaseAndUnloadStress(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	cfg := supervisedCatalogFactoryConfig()
	cfg.Resources[0].Capacity = 4
	loaded := mustLoadedCatalogConfig(t, cfg)
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})

	const (
		workers    = 8
		iterations = 25
	)
	var wg sync.WaitGroup
	errCh := make(chan error, workers*iterations)
	ctx := context.Background()

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				lease, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
				if err != nil {
					if errors.Is(err, ErrCapacityExhausted) || errors.Is(err, ErrRuntimeNotReady) {
						continue
					}
					var readinessErr *ReadinessError
					if errors.As(err, &readinessErr) {
						switch readinessErr.Snapshot.FailureClass {
						case FailureClassLoadingTimeout, FailureClassProcessCrash, FailureClassCancelled:
							continue
						}
					}
					errCh <- err
					return
				}
				if lease.Endpoint == "" {
					errCh <- errors.New("lease endpoint is empty")
					return
				}
				if err := host.ReleaseLease(ctx, lease.ID); err != nil {
					errCh <- err
					return
				}
				if i%7 == 0 {
					_ = host.Unload(ctx, loaded, "OMNIVOICE_Q4_K_M")
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("stress error: %v", err)
		}
	}
}

func catalogFactoryConfigWithLeaseCapacity(capacity int) *interfaces.FactoryConfig {
	cfg := catalogFactoryConfig(true)
	cfg.Resources[0].Capacity = capacity
	return cfg
}
