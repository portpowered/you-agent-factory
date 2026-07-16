// Package defaultcmd defines the no-argument agent-factory default flow.
package defaultcmd

import (
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

const (
	// FactoryDir is the base directory used by the default local factory.
	FactoryDir = "factory"
	// FactoryPort is the dashboard/API port used by the default local factory.
	FactoryPort = 7437
)

// ExplicitRunConfig returns the baseline configuration for the explicit run command.
func ExplicitRunConfig() runcli.RunConfig {
	return runcli.RunConfig{
		Dir:                  FactoryDir,
		Port:                 FactoryPort,
		AutoPort:             true,
		RuntimeLogConfig:     logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig: platformmetrics.DefaultRuntimeMetricsConfig(),
	}
}

// OOTBRunConfig returns the no-argument out-of-the-box run configuration.
func OOTBRunConfig() runcli.RunConfig {
	cfg := ExplicitRunConfig()
	cfg.Continuously = true
	cfg.Bootstrap = true
	cfg.OpenDashboard = true
	return cfg
}
