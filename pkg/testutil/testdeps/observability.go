package testdeps

import (
	"github.com/portpowered/infinite-you/pkg/internal/metrics"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"go.uber.org/zap"
)

// Observability holds injectable logging and metrics dependencies for tests.
type Observability struct {
	Logger         logging.Logger
	MetricsEmitter metrics.MetricsEmitter
	ZapLogger      *zap.Logger
}

// Default returns quiet noop observability for ordinary tests.
func Default() Observability {
	return Observability{
		Logger:         logging.NoopLogger{},
		MetricsEmitter: metrics.NoopEmitter{},
		ZapLogger:      zap.NewNop(),
	}
}

// ApplyFactoryServiceConfig applies quiet observability defaults to cfg when
// callers have not set explicit observability dependencies.
func (o Observability) ApplyFactoryServiceConfig(cfg *service.FactoryServiceConfig) {
	if cfg == nil {
		return
	}
	if cfg.Logger == nil {
		cfg.Logger = o.ZapLogger
	}
	if cfg.RuntimeFileLoggingPolicy == "" {
		cfg.RuntimeFileLoggingPolicy = service.RuntimeFileLoggingPolicyDisabled
	}
	if cfg.RuntimeMetricsPolicy == "" {
		cfg.RuntimeMetricsPolicy = service.RuntimeMetricsPolicyDisabled
	}
}
