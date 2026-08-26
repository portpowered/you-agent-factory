package service

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	hostleases "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases"
	leaseswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/internal/services/leases/wire"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// SupervisorTestConfig overrides supervisor timing and health probing for tests.
type SupervisorTestConfig struct {
	ReadinessTimeout          time.Duration
	HealthCheckInterval       time.Duration
	HealthChecker             healthChecker
	AfterLoadStateObservation func()
	OnProcessFailure          func()
}

// HostPolicyTestConfig overrides idle unload and loaded-runtime pressure policy for tests.
type HostPolicyTestConfig struct {
	IdleUnloadAfter   time.Duration
	MaxLoadedRuntimes int
}

// NewWithSupervisorTestConfig constructs a Runtime Host with test-only supervisor overrides.
func NewWithSupervisorTestConfig(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	cfg SupervisorTestConfig,
) runtimehost.Service {
	return NewWithHostTestConfig(
		scopes,
		assets,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
		cfg,
		HostPolicyTestConfig{},
	)
}

// NewWithHostTestConfig constructs a Runtime Host with test-only supervisor and policy overrides.
func NewWithHostTestConfig(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	supervisorCfg SupervisorTestConfig,
	policyCfg HostPolicyTestConfig,
	options ...runtimehost.Options,
) runtimehost.Service {
	s := New(
		scopes,
		assets,
		mustLeasesService(hostClock),
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
		options...,
	).(*service)
	if supervisorCfg.ReadinessTimeout > 0 {
		s.supervisor.ReadinessTimeout = supervisorCfg.ReadinessTimeout
	}
	if supervisorCfg.HealthCheckInterval > 0 {
		s.supervisor.HealthCheckInterval = supervisorCfg.HealthCheckInterval
	}
	if supervisorCfg.HealthChecker != nil {
		s.supervisor.HealthChecker = supervisorCfg.HealthChecker
	}
	s.supervisor.afterLoadStateObservation = supervisorCfg.AfterLoadStateObservation
	s.supervisor.onProcessFailure = supervisorCfg.OnProcessFailure
	s.idleUnloadAfter, s.maxLoadedRuntimes = normalizeHostPolicy(
		policyCfg.IdleUnloadAfter,
		policyCfg.MaxLoadedRuntimes,
	)
	return s
}

// LeasesService returns the nested leases owner for focused integration tests.
func LeasesService(s runtimehost.Service) hostleases.Service {
	return s.(*service).leases
}

// NewWithLeasesFacts constructs a Runtime Host whose nested leases owner uses
// the supplied slot facts and binds holder-aware cleanup to the host.
func NewWithLeasesFacts(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	slotFacts modelseffects.SlotFactsProvider,
	policyCfg HostPolicyTestConfig,
) runtimehost.Service {
	leases, err := leaseswire.NewService(hostClock, slotFacts)
	if err != nil {
		panic(err)
	}
	s := New(
		scopes,
		assets,
		leases,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
	).(*service)
	s.idleUnloadAfter, s.maxLoadedRuntimes = normalizeHostPolicy(
		policyCfg.IdleUnloadAfter,
		policyCfg.MaxLoadedRuntimes,
	)
	return s
}

// NewWiredWithSupervisorConfig constructs the host-backed lease adapter with
// deterministic supervisor overrides for focused integration tests.
func NewWiredWithSupervisorConfig(
	scopes runtimescopes.Service,
	assets scopedassets.Service,
	processLauncher modelseffects.HostProcessLauncher,
	hostHTTP modelseffects.HostHTTPDoer,
	hostClock modelseffects.HostClock,
	hostLogger modelseffects.HostDiagnosticLogger,
	hostMetrics modelseffects.HostMetricsRecorder,
	supervisorCfg SupervisorTestConfig,
	policyCfg HostPolicyTestConfig,
	options ...runtimehost.Options,
) (runtimehost.Service, error) {
	host, err := NewWired(
		scopes,
		assets,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
		options...,
	)
	if err != nil {
		return nil, err
	}
	s := host.(*service)
	if supervisorCfg.ReadinessTimeout > 0 {
		s.supervisor.ReadinessTimeout = supervisorCfg.ReadinessTimeout
	}
	if supervisorCfg.HealthCheckInterval > 0 {
		s.supervisor.HealthCheckInterval = supervisorCfg.HealthCheckInterval
	}
	if supervisorCfg.HealthChecker != nil {
		s.supervisor.HealthChecker = supervisorCfg.HealthChecker
	}
	s.supervisor.afterLoadStateObservation = supervisorCfg.AfterLoadStateObservation
	s.supervisor.onProcessFailure = supervisorCfg.OnProcessFailure
	s.idleUnloadAfter, s.maxLoadedRuntimes = normalizeHostPolicy(
		policyCfg.IdleUnloadAfter,
		policyCfg.MaxLoadedRuntimes,
	)
	return host, nil
}

// AcquireSlotCapacity simulates one active capacity holder for idle-unload tests.
func AcquireSlotCapacity(s runtimehost.Service, scope models.RuntimeScopeRef, modelName string) {
	host := s.(*service)
	slotKey := runtimeSlotKey(scope, modelName)
	host.mu.Lock()
	host.acquireSlotCapacityLocked(slotKey)
	host.mu.Unlock()
}

// ReleaseSlotCapacity releases one simulated capacity holder and may schedule idle unload.
func ReleaseSlotCapacity(
	s runtimehost.Service,
	scope models.RuntimeScopeRef,
	modelName string,
	runtimeCfg *models.RuntimeConfig,
) {
	host := s.(*service)
	host.releaseSlotCapacity(scope, modelName, runtimeCfg)
}

// ShutdownHost stops all supervised runtimes owned by the host.
func ShutdownHost(ctx context.Context, s runtimehost.Service) error {
	return s.(*service).Shutdown(ctx)
}

func mustLeasesService(hostClock modelseffects.HostClock) hostleases.Service {
	leases, err := leaseswire.NewService(hostClock, modelseffects.UnconfiguredSlotFacts{})
	if err != nil {
		panic(err)
	}
	return leases
}

func TestInvocationEndpointExposesOnlyReadyRuntimeEndpoints(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:endpoint-test")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	host := &service{runtimeSlots: map[string]*supervisedRuntime{
		runtimeSlotKey(scope, "llm"): {
			state:    supervisedStateReady,
			endpoint: "  grpc://127.0.0.1:45731  ",
		},
	}}

	endpoint, err := host.InvocationEndpoint(context.Background(), scope, "llm")
	if err != nil {
		t.Fatalf("ready InvocationEndpoint: %v", err)
	}
	if endpoint != "grpc://127.0.0.1:45731" {
		t.Fatalf("ready endpoint = %q, want trimmed endpoint", endpoint)
	}

	if _, err := host.InvocationEndpoint(context.Background(), scope, "missing"); !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("missing endpoint error = %v, want ErrHostRuntimeNotReady", err)
	}

	for _, state := range []supervisedState{supervisedStateAbsent, supervisedStateLoading, supervisedStateFailed} {
		host.runtimeSlots[runtimeSlotKey(scope, "llm")] = &supervisedRuntime{
			state: state, endpoint: "grpc://127.0.0.1:45731",
		}
		if _, err := host.InvocationEndpoint(context.Background(), scope, "llm"); !errors.Is(err, models.ErrHostRuntimeNotReady) {
			t.Fatalf("state %q endpoint error = %v, want ErrHostRuntimeNotReady", state, err)
		}
	}

	var nilHost *service
	if _, err := nilHost.InvocationEndpoint(context.Background(), scope, "llm"); !errors.Is(err, models.ErrUnavailable) {
		t.Fatalf("nil host endpoint error = %v, want ErrUnavailable", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := host.InvocationEndpoint(cancelled, scope, "llm"); !errors.Is(err, models.ErrHostCancelled) {
		t.Fatalf("cancelled endpoint error = %v, want ErrHostCancelled", err)
	}
}
