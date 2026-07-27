package service

import (
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
	s := New(
		scopes,
		assets,
		processLauncher,
		hostHTTP,
		hostClock,
		hostLogger,
		hostMetrics,
	).(*service)
	if cfg.ReadinessTimeout > 0 {
		s.supervisor.ReadinessTimeout = cfg.ReadinessTimeout
	}
	if cfg.HealthCheckInterval > 0 {
		s.supervisor.HealthCheckInterval = cfg.HealthCheckInterval
	}
	if cfg.HealthChecker != nil {
		s.supervisor.HealthChecker = cfg.HealthChecker
	}
	return s
}
