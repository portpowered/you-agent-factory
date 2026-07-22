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

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

func TestCatalogHost_SupervisedBackend_InspectPreservesInstalledAssetReadinessWithoutLiveSlot(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, nil)

	ready, err := host.InspectReadiness(context.Background(), loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectReadiness: %v", err)
	}
	if ready.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY for installed assets without a live supervised slot", ready.ReadinessState)
	}
	if ready.LifecycleState != managedruntime.LifecycleStateInstalled {
		t.Fatalf("lifecycle = %s, want INSTALLED", ready.LifecycleState)
	}
}

func TestCatalogHost_SupervisedBackend_HealthyStartupIssuesLeaseWithEndpoint(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{Holder: "test"})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if lease.Endpoint != healthServer.URL {
		t.Fatalf("endpoint = %q, want %q", lease.Endpoint, healthServer.URL)
	}
	if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func TestCatalogHost_SupervisedBackend_ReadinessTimeoutReturnsTypedFailure(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(healthServer.Close)

	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, nil)
		},
	})
	host.supervisor.ReadinessTimeout = 75 * time.Millisecond
	host.supervisor.HealthCheckInterval = 10 * time.Millisecond

	_, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	var readinessErr *ReadinessError
	if !errors.As(err, &readinessErr) {
		t.Fatalf("error = %v, want *ReadinessError", err)
	}
	if !errors.Is(err, ErrLoadingTimeout) {
		t.Fatalf("error = %v, want ErrLoadingTimeout", err)
	}
	if readinessErr.Snapshot.FailureClass != FailureClassLoadingTimeout {
		t.Fatalf("failure class = %s, want %s", readinessErr.Snapshot.FailureClass, FailureClassLoadingTimeout)
	}
}

func TestCatalogHost_SupervisedBackend_PostStartCrashSurfacesFailedReadiness(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	exitCh := make(chan error, 1)
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, exitCh)
		},
	})

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	exitCh <- errors.New("unexpected exit")

	deadline := time.Now().Add(2 * time.Second)
	for {
		ready, inspectErr := host.InspectReadiness(context.Background(), loaded, "OMNIVOICE_Q4_K_M")
		if inspectErr != nil {
			t.Fatalf("InspectReadiness: %v", inspectErr)
		}
		if ready.ReadinessState == managedruntime.ReadinessStateFailed {
			if ready.FailureClass != FailureClassProcessCrash {
				t.Fatalf("failure class = %s, want %s", ready.FailureClass, FailureClassProcessCrash)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for crash readiness; last state = %s", ready.ReadinessState)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
}

func TestCatalogHost_SupervisedBackend_UnloadStopsManagedProcess(t *testing.T) {
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

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := host.Unload(context.Background(), loaded, "OMNIVOICE_Q4_K_M"); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want 1", stopCount.Load())
	}
}

func TestCatalogHost_SupervisedBackend_CancellationStopsManagedProcess(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
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
	host.supervisor.ReadinessTimeout = time.Second
	host.supervisor.HealthCheckInterval = 25 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := host.AcquireLease(ctx, loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
		errCh <- err
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-errCh
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("error = %v, want ErrCancelled", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want 1", stopCount.Load())
	}
}

func TestCatalogHost_ShutdownStopsSupervisedRuntimes(t *testing.T) {
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

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
		t.Fatalf("ReleaseLease: %v", err)
	}
	if err := host.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if stopCount.Load() != 1 {
		t.Fatalf("stop count = %d, want 1", stopCount.Load())
	}
}

func TestHTTPHealthChecker_AcceptsSuccessfulResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	checker := HTTPHealthChecker{Client: http.DefaultClient, Path: "/health"}
	if err := checker.Check(context.Background(), server.URL); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func supervisedCatalogFactoryConfig() *testFactoryConfig {
	cfg := catalogFactoryConfig(true)
	cfg.Resources[0].Backend = "LLAMACPP"
	cfg.Workers[0].Command = "fake-llamacpp-server"
	cfg.Workers[0].Args = []string{supervisedHealthEndpointFlag, "http://127.0.0.1:1"}
	return cfg
}

func newSupervisedTestHost(t *testing.T, launcher *fakeProcessLauncher) *CatalogHost {
	t.Helper()
	if launcher == nil {
		launcher = &fakeProcessLauncher{}
	}
	host := mustNewCatalogHost(t, stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
				CachePath:          t.TempDir(),
			},
		},
	}, testHostOptions{
		SourceResolver: DefaultManagedRuntimeSourceResolverAdapter(),
		Supervisor: SupervisorConfig{
			ReadinessTimeout:    500 * time.Millisecond,
			HealthCheckInterval: 10 * time.Millisecond,
			ProcessLauncher:     launcher,
			HealthChecker:       HTTPHealthChecker{Client: http.DefaultClient, Path: "/health"},
			ServerStartBuilder:  DefaultServerStartBuilder,
		},
	})
	return host
}

type fakeProcessLauncher struct {
	mu         sync.Mutex
	starts     []ProcessStartSpec
	newProcess func(spec ProcessStartSpec) *fakeManagedProcess
}

func (f *fakeProcessLauncher) Start(_ context.Context, spec ProcessStartSpec) (ManagedProcess, error) {
	f.mu.Lock()
	f.starts = append(f.starts, spec)
	newProcess := f.newProcess
	f.mu.Unlock()
	if newProcess == nil {
		return nil, errors.New("fake process launcher is not configured")
	}
	return newProcess(spec), nil
}

type fakeManagedProcess struct {
	endpoint string
	exitCh   chan error
	stopCh   chan struct{}
	stopOnce sync.Once
	stopFn   func() error
}

func newFakeManagedProcess(endpoint string, exitCh chan error) *fakeManagedProcess {
	if exitCh == nil {
		exitCh = make(chan error, 1)
	}
	return &fakeManagedProcess{
		endpoint: endpoint,
		exitCh:   exitCh,
		stopCh:   make(chan struct{}),
	}
}

func (p *fakeManagedProcess) HealthEndpoint() string {
	return p.endpoint
}

func (p *fakeManagedProcess) Wait() error {
	return <-p.exitCh
}

func (p *fakeManagedProcess) Stop(context.Context) error {
	if p.stopFn != nil {
		return p.stopFn()
	}
	return p.defaultStop()
}

func (p *fakeManagedProcess) defaultStop() error {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		p.exitCh <- errors.New("stopped")
	})
	return nil
}

func TestDefaultLlamaCppServerStartBuilder_RequiresHealthEndpointArg(t *testing.T) {
	_, err := DefaultServerStartBuilder(
		Identity{Name: "OMNIVOICE_Q4_K_M", Backend: "LLAMACPP"},
		CacheInspection{CachePath: t.TempDir(), Installed: true},
		&modelRuntimeWorker{Command: "fake-server"},
	)
	if err == nil {
		t.Fatal("expected missing health endpoint error")
	}
}

func TestLoadedFactoryConfigForSupervisedTests(t *testing.T) {
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), supervisedCatalogFactoryConfig())
	if loaded == nil {
		t.Fatal("loaded config is nil")
	}
}
