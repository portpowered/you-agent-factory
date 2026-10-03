package cli

import (
	"fmt"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// ResolveOperatorDefaultsConfig carries observed environment and flag layers for
// one defaults-resolution invocation.
type ResolveOperatorDefaultsConfig struct {
	HomeDir     string
	Environment operatorsettings.Defaults
	Flags       operatorsettings.FlagOverrides
}

func (service *service) ResolveOperatorDefaults(
	cfg ResolveOperatorDefaultsConfig,
) (operatorsettings.ResolvedDefaults, error) {
	homeDir := strings.TrimSpace(cfg.HomeDir)
	if homeDir == "" {
		return operatorsettings.ResolvedDefaults{}, fmt.Errorf("home directory is required")
	}
	return service.root.ResolveFromHomeWithEnvironment(
		homeDir,
		operatorsettings.Defaults{
			WorkerModelProvider: strings.TrimSpace(cfg.Environment.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Environment.WorkerModel),
		},
		operatorsettings.FlagOverrides{
			WorkerModelProvider: strings.TrimSpace(cfg.Flags.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(cfg.Flags.WorkerModel),
		},
	)
}
