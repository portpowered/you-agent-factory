package operatorsettings

// DefaultsResolver is the exact application operation for resolving operator
// defaults from already-observed process edges and the service-owned file
// configuration.
type DefaultsResolver func(homeDir string, environment Defaults, flags FlagOverrides) (ResolvedDefaults, error)

// DefaultsResolutionFromHome resolves operator defaults from observed environment
// and flag layers using the composition-selected Settings root ports.
type DefaultsResolutionFromHome func(
	files FileSystem,
	decode ConfigDecoder,
	homeDir string,
	environment Defaults,
	flags FlagOverrides,
) (ResolvedDefaults, error)

// DefaultsResolutionOperations wires private defaults-resolution behavior into
// the published Operator Settings root without importing the resolution
// subservice from the peer root package.
type DefaultsResolutionOperations struct {
	DefaultConfigPath              func(string) string
	LoadFileDefaults               func(FileSystem, ConfigDecoder, string) (Defaults, error)
	LoadFileConfig                 func(FileSystem, ConfigDecoder, string) (Config, error)
	Resolve                        func(ResolveInput, string) (ResolvedDefaults, error)
	ResolveFromHomeWithEnvironment func(FileSystem, ConfigDecoder, string, Defaults, FlagOverrides) (ResolvedDefaults, error)
	DeriveProviderBackendScopeID   func(provider, kind, boundary string) string
}

var defaultsResolutionOperations DefaultsResolutionOperations

var defaultsResolutionFromHome DefaultsResolutionFromHome

// ConfigureDefaultsResolutionOperations registers private defaults-resolution
// behavior for the published Operator Settings root surface.
func ConfigureDefaultsResolutionOperations(operations DefaultsResolutionOperations) {
	defaultsResolutionOperations = operations
}

// ConfigureDefaultsResolutionFromHome registers the composition-facing defaults
// resolver that dispatches through the Settings-owned CLI adapter ownership path.
func ConfigureDefaultsResolutionFromHome(resolver DefaultsResolutionFromHome) {
	defaultsResolutionFromHome = resolver
}

// DefaultConfigPath returns the default operator config file path for homeDir.
func DefaultConfigPath(homeDir string) string {
	if defaultsResolutionOperations.DefaultConfigPath == nil {
		panic("operator settings defaults resolution operations are required")
	}
	return defaultsResolutionOperations.DefaultConfigPath(homeDir)
}

// LoadFileDefaults reads operator defaults from path. A missing file returns
// empty defaults without error. Malformed JSON fails with an error naming path.
func LoadFileDefaults(files FileSystem, decode ConfigDecoder, path string) (Defaults, error) {
	if defaultsResolutionOperations.LoadFileDefaults == nil {
		panic("operator settings defaults resolution operations are required")
	}
	return defaultsResolutionOperations.LoadFileDefaults(files, decode, path)
}

// LoadFileConfig reads and validates the operator-owned configuration. A
// missing file returns an empty configuration without error.
func LoadFileConfig(files FileSystem, decode ConfigDecoder, path string) (Config, error) {
	if defaultsResolutionOperations.LoadFileConfig == nil {
		panic("operator settings defaults resolution operations are required")
	}
	return defaultsResolutionOperations.LoadFileConfig(files, decode, path)
}

// Resolve applies file, environment, and flag precedence independently per
// field, resolves symbolic DEFAULT providers, and validates supported providers.
func Resolve(input ResolveInput, configPath string) (ResolvedDefaults, error) {
	if defaultsResolutionOperations.Resolve == nil {
		panic("operator settings defaults resolution operations are required")
	}
	return defaultsResolutionOperations.Resolve(input, configPath)
}

// ResolveFromHomeWithEnvironment loads file defaults from homeDir and applies
// the supplied environment and flag layers. Callers at a process boundary can
// use this form without changing or reading the process environment.
func ResolveFromHomeWithEnvironment(
	files FileSystem,
	decode ConfigDecoder,
	homeDir string,
	environment Defaults,
	flags FlagOverrides,
) (ResolvedDefaults, error) {
	if defaultsResolutionFromHome != nil {
		return defaultsResolutionFromHome(files, decode, homeDir, environment, flags)
	}
	if defaultsResolutionOperations.ResolveFromHomeWithEnvironment == nil {
		panic("operator settings defaults resolution operations are required")
	}
	return defaultsResolutionOperations.ResolveFromHomeWithEnvironment(files, decode, homeDir, environment, flags)
}

// DeriveProviderBackendScopeID returns a stable backend scope identifier for one
// provider-backed runtime boundary without embedding secret material.
func DeriveProviderBackendScopeID(provider, kind, boundary string) string {
	if defaultsResolutionOperations.DeriveProviderBackendScopeID == nil {
		panic("operator settings defaults resolution operations are required")
	}
	return defaultsResolutionOperations.DeriveProviderBackendScopeID(provider, kind, boundary)
}
