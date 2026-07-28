package operatorsettings

import "strings"

// DefaultsResolver is the exact application operation for resolving operator
// defaults from already-observed process edges and the service-owned file
// configuration.
type DefaultsResolver func(homeDir string, environment Defaults, flags FlagOverrides) (ResolvedDefaults, error)

// ResolveFromHomeWithEnvironment loads file defaults from homeDir and applies
// the supplied environment and flag layers. Callers at a process boundary can
// use this form without changing or reading the process environment.
func ResolveFromHomeWithEnvironment(files FileSystem, decode ConfigDecoder, homeDir string, environment Defaults, flags FlagOverrides) (ResolvedDefaults, error) {
	if defaultsResolutionFromHome != nil {
		return defaultsResolutionFromHome(files, decode, homeDir, environment, flags)
	}
	configPath := DefaultConfigPath(homeDir)
	fileDefaults, err := LoadFileDefaults(files, decode, configPath)
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
