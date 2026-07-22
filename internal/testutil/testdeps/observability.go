package testdeps

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"go.uber.org/zap"
)

// Observability holds injectable logging and metrics dependencies for tests.
type Observability struct {
	Logger         logging.Logger
	MetricsEmitter factoryruntime.MetricsEmitter
	ZapLogger      *zap.Logger
}

// Default returns quiet noop observability for ordinary tests.
func Default() Observability {
	return Observability{
		Logger:         logging.NoopLogger{},
		MetricsEmitter: factoryruntime.NoopEmitter{},
		ZapLogger:      zap.NewNop(),
	}
}

// QuietFactoryServiceConfig applies quiet observability defaults to cfg and
// returns it for narrowly owned runtime-component tests.
type FactoryServiceObservabilityConfig struct {
	Logger                   *zap.Logger
	RuntimeFileLoggingPolicy factoryruntime.RuntimeFileLoggingPolicy
	RuntimeMetricsPolicy     factoryruntime.RuntimeMetricsPolicy
}

func QuietFactoryServiceConfig(cfg *FactoryServiceObservabilityConfig) *FactoryServiceObservabilityConfig {
	if cfg == nil {
		cfg = &FactoryServiceObservabilityConfig{}
	}
	Default().ApplyFactoryServiceConfig(cfg)
	return cfg
}

// ApplyFactoryServiceConfig applies quiet observability defaults to cfg when
// callers have not set explicit observability dependencies.
func (o Observability) ApplyFactoryServiceConfig(cfg *FactoryServiceObservabilityConfig) {
	if cfg == nil {
		return
	}
	if cfg.Logger == nil {
		cfg.Logger = o.ZapLogger
	}
	if cfg.RuntimeFileLoggingPolicy == "" {
		cfg.RuntimeFileLoggingPolicy = factoryruntime.RuntimeFileLoggingPolicyDisabled
	}
	if cfg.RuntimeMetricsPolicy == "" {
		cfg.RuntimeMetricsPolicy = factoryruntime.RuntimeMetricsPolicyDisabled
	}
}
