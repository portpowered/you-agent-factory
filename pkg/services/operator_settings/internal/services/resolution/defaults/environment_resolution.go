package settingsresolution

import (
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// ResolveFromHomeWithEnvironment loads file defaults from homeDir and applies
// the supplied environment and flag layers. Callers at a process boundary can
// use this form without changing or reading the process environment.
func ResolveFromHomeWithEnvironment(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	homeDir string,
	environment operatorsettings.Defaults,
	flags operatorsettings.FlagOverrides,
) (operatorsettings.ResolvedDefaults, error) {
	configPath := DefaultConfigPath(homeDir)
	fileDefaults, err := LoadFileDefaults(files, decode, configPath)
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, err
	}
	return Resolve(operatorsettings.ResolveInput{
		File: fileDefaults,
		Env: operatorsettings.Defaults{
			WorkerModelProvider: strings.TrimSpace(environment.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(environment.WorkerModel),
		},
		Flag: operatorsettings.Defaults{
			WorkerModelProvider: strings.TrimSpace(flags.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(flags.WorkerModel),
		},
	}, configPath)
}
