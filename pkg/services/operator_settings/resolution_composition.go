package operatorsettings

// DefaultsResolutionFromHome resolves operator defaults from observed environment
// and flag layers using the composition-selected Settings root ports.
type DefaultsResolutionFromHome func(
	files FileSystem,
	decode ConfigDecoder,
	homeDir string,
	environment Defaults,
	flags FlagOverrides,
) (ResolvedDefaults, error)

var defaultsResolutionFromHome DefaultsResolutionFromHome

// ConfigureDefaultsResolutionFromHome registers the composition-facing defaults
// resolver that dispatches through the Settings-owned CLI adapter ownership path.
func ConfigureDefaultsResolutionFromHome(resolver DefaultsResolutionFromHome) {
	defaultsResolutionFromHome = resolver
}
