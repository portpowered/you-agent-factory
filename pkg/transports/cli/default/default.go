// Package defaultcmd defines the no-argument agent-factory default flow.
package defaultcmd

import runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"

const (
	// FactoryDir is the base directory used by the default local factory.
	FactoryDir = "factory"
	// FactoryPort is the dashboard/API port used by the default local factory.
	FactoryPort = 7437
)

// ExplicitRunConfig returns the baseline configuration for the explicit run command.
func ExplicitRunConfig(defaults runcli.RunConfig) runcli.RunConfig {
	defaults.Dir = FactoryDir
	defaults.Port = FactoryPort
	defaults.AutoPort = true
	return defaults
}

// OOTBRunConfig returns the no-argument out-of-the-box run configuration.
func OOTBRunConfig(defaults runcli.RunConfig) runcli.RunConfig {
	cfg := ExplicitRunConfig(defaults)
	cfg.Continuously = true
	cfg.Bootstrap = true
	cfg.OpenDashboard = true
	return cfg
}
