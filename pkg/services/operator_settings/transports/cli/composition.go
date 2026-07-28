package cli

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// ConfigureOperation is the composition-facing configure role with full CLI
// presentation inputs when composition owns writers or diagnostics.
type ConfigureOperation func(ConfigureConfig) error

// ResolveOperatorDefaultsOperation is the composition-facing defaults-resolution
// role over observed environment and flag layers.
type ResolveOperatorDefaultsOperation = operatorsettings.DefaultsResolver

// BindConfigure returns the composition-facing operation closure that delegates
// configure presentation to the Settings-owned CLI adapter Service.
func BindConfigure(root operatorsettings.Service) ConfigureOperation {
	if root == nil {
		return nil
	}
	return func(cfg ConfigureConfig) error {
		return Configure(cfg, root)
	}
}

// BindResolveOperatorDefaults returns the composition-facing operation closure
// that delegates defaults resolution to the Settings-owned CLI adapter Service.
func BindResolveOperatorDefaults(root operatorsettings.Service) ResolveOperatorDefaultsOperation {
	if root == nil {
		return nil
	}
	return func(
		homeDir string,
		environment operatorsettings.Defaults,
		flags operatorsettings.FlagOverrides,
	) (operatorsettings.ResolvedDefaults, error) {
		return ResolveOperatorDefaults(ResolveOperatorDefaultsConfig{
			HomeDir:     homeDir,
			Environment: environment,
			Flags:       flags,
		}, root)
	}
}
