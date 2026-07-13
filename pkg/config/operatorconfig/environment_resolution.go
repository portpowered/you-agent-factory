package operatorconfig

import "strings"

// ResolveFromHomeWithEnvironment loads file defaults from homeDir and applies
// the supplied environment and flag layers. Callers at a process boundary can
// use this form without changing or reading the process environment.
func ResolveFromHomeWithEnvironment(homeDir string, environment Defaults, flags FlagOverrides) (ResolvedDefaults, error) {
	configPath := DefaultConfigPath(homeDir)
	fileDefaults, err := LoadFileDefaults(configPath)
	if err != nil {
		return ResolvedDefaults{}, err
	}
	return Resolve(ResolveInput{
		File: fileDefaults,
		Env: Defaults{
			WorkerModelProvider: strings.TrimSpace(environment.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(environment.WorkerModel),
		},
		Flag: Defaults{
			WorkerModelProvider: strings.TrimSpace(flags.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(flags.WorkerModel),
		},
	}, configPath)
}
