package service

import (
	"context"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
)

// SupervisorTestConfig overrides supervisor timing and health probing for tests.
type SupervisorTestConfig struct {
	ReadinessTimeout    time.Duration
	HealthCheckInterval time.Duration
	HealthChecker       healthChecker
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
	processLauncher models.HostProcessLauncher,
	hostHTTP models.HostHTTPDoer,
	hostClock models.HostClock,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
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
	processLauncher models.HostProcessLauncher,
	hostHTTP models.HostHTTPDoer,
	hostClock models.HostClock,
	hostLogger models.HostDiagnosticLogger,
	hostMetrics models.HostMetricsRecorder,
	supervisorCfg SupervisorTestConfig,
	policyCfg HostPolicyTestConfig,
) runtimehost.Service {
	s := New(
		scopes,
		assets,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
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
	s.idleUnloadAfter, s.maxLoadedRuntimes = normalizeHostPolicy(
		policyCfg.IdleUnloadAfter,
		policyCfg.MaxLoadedRuntimes,
	)
	return s
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
