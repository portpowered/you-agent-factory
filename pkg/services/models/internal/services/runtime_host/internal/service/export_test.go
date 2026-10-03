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
	options ...HostOptions,
) runtimehost.Service {
	host, err := newHostFixture(scopes, assets, processLauncher, hostHTTP, hostClock, hostLogger, hostMetrics, modelseffects.UnconfiguredSlotFacts{}, policyCfg, options...)
	if err != nil {
		panic(err)
	}
	s := hostFixture(host)
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
	return host
}

// LeasesService returns the nested leases owner for focused integration tests.
func LeasesService(s runtimehost.Service) hostleases.Service {
	return hostFixture(s).leases
}

// NewWithLeasesFacts constructs a Runtime Host whose nested leases owner uses
// the supplied slot facts and independently constructed holder-aware cleanup.
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
	s, err := newHostFixture(scopes, assets, processLauncher, hostHTTP, hostClock, hostLogger, hostMetrics, slotFacts, policyCfg)
	if err != nil {
		panic(err)
	}
	return s
}

// NewWiredWithSupervisorConfig constructs shared slot facts and coordinator with
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
	options ...HostOptions,
) (runtimehost.Service, error) {
	host, err := newHostFixture(
		scopes,
		assets,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
		nil, policyCfg, options...,
	)
	if err != nil {
		return nil, err
	}
	s := hostFixture(host)
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
	return host, nil
}

// AcquireSlotCapacity simulates one active capacity holder for idle-unload tests.
func AcquireSlotCapacity(s runtimehost.Service, scope models.RuntimeScopeRef, modelName string) {
	host := hostFixture(s)
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
	s.(*hostTestFixture).coordinator.releaseSlotCapacity(scope, modelName, runtimeCfg)
}

// ShutdownHost stops all supervised runtimes owned by the host.
func ShutdownHost(ctx context.Context, s runtimehost.Service) error {
	return hostFixture(s).Shutdown(ctx)
}

func TestInvocationEndpointExposesOnlyReadyRuntimeEndpoints(t *testing.T) {
	t.Parallel()

	scope, err := (models.RuntimeScopeRef{}).Parse("scope:endpoint-test")
	if err != nil {
		t.Fatalf("scope.Parse: %v", err)
	}
	host := &service{SlotState: &SlotState{runtimeSlots: map[string]*supervisedRuntime{
		runtimeSlotKey(scope, "llm"): {
			state:    supervisedStateReady,
			endpoint: "  grpc://127.0.0.1:45731  ",
		},
	}}}

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

// HostOptions configures only test fixtures; production constructors use direct arguments.
type HostOptions struct {
	Platform             models.AssetHostPlatform
	ProtocolNegotiator   modelseffects.HostProtocolNegotiator
	CompatibilityChecker modelseffects.HostCompatibilityChecker
	ResolveSymlinks      modelseffects.HostResolveSymlinks
	RuntimeEvidence      modelseffects.RuntimeEvidenceRecorder
	IdleUnloadAfter      time.Duration
	MaxLoadedRuntimes    int
}

func newHostFixture(scopes runtimescopes.Service, assets scopedassets.Service, processLauncher modelseffects.HostProcessLauncher, hostHTTP modelseffects.HostHTTPDoer, clock modelseffects.HostClock, logger modelseffects.HostDiagnosticLogger, metrics modelseffects.HostMetricsRecorder, facts modelseffects.SlotFactsProvider, policy HostPolicyTestConfig, options ...HostOptions) (runtimehost.Service, error) {
	opt := HostOptions{}
	if len(options) > 0 {
		opt = options[0]
	}
	state := NewSlotState()
	if facts == nil {
		facts = NewSlotFacts(scopes, assets, state)
	}
	coordinator := NewSlotCoordinator(state, scopes, clock, logger, metrics, policy.IdleUnloadAfter)
	leases, err := leaseswire.NewService(clock, facts, coordinator)
	if err != nil {
		return nil, err
	}
	host := New(scopes, assets, leases, state, processLauncher, hostHTTP, clock, logger, metrics, opt.Platform, opt.ProtocolNegotiator, opt.CompatibilityChecker, opt.ResolveSymlinks, opt.RuntimeEvidence, policy.IdleUnloadAfter, policy.MaxLoadedRuntimes).(*service)
	return &hostTestFixture{service: host, coordinator: coordinator.(*slotCoordinator)}, nil
}

type hostTestFixture struct {
	*service
	coordinator *slotCoordinator
}

func hostFixture(host runtimehost.Service) *service {
	if fixture, ok := host.(*hostTestFixture); ok {
		return fixture.service
	}
	return host.(*service)
}

func TestIdleCoordinatorIgnoresStaleTimerAndProtectsPeer(t *testing.T) {
	t.Parallel()
	state := NewSlotState()
	coordinator := &slotCoordinator{SlotState: state}
	selected := &supervisedRuntime{state: supervisedStateReady}
	peer := &supervisedRuntime{state: supervisedStateReady}
	state.runtimeSlots["selected"] = selected
	state.runtimeSlots["peer"] = peer
	stale, current := &idleUnload{}, &idleUnload{}
	state.idleUnloadTimers["selected"] = current
	coordinator.runIdleUnload(supervisedIdentity{}, "selected", stale)
	if !selected.isReady() || !peer.isReady() {
		t.Fatal("stale timer stopped a runtime")
	}
	state.capacityHolders["selected"] = 1
	coordinator.runIdleUnload(supervisedIdentity{}, "selected", current)
	if !selected.isReady() || !peer.isReady() {
		t.Fatal("timer stopped active holder or peer")
	}
	delete(state.capacityHolders, "selected")
	state.idleUnloadTimers["selected"] = current
	coordinator.runIdleUnload(supervisedIdentity{}, "selected", current)
	if selected.isReady() || !peer.isReady() {
		t.Fatal("eligible timer did not stop only its selected runtime")
	}
}

func (s *slotCoordinator) releaseSlotCapacity(
	scope models.RuntimeScopeRef,
	modelName string,
	runtimeCfg *models.RuntimeConfig,
) {
	s.releaseSlotCapacityWithOverlays(scope, modelName, runtimeCfg, nil)
}
