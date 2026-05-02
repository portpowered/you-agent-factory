// Package defaultcmd defines the no-argument agent-factory default flow.
package defaultcmd

import (
	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
	"github.com/portpowered/infinite-you/pkg/logging"
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
		Dir:              FactoryDir,
		Port:             FactoryPort,
		AutoPort:         true,
		RuntimeLogConfig: logging.DefaultRuntimeLogConfig(),
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
