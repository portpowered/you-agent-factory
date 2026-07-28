package wire

import (
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingscli "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/cli"
)

func init() {
	RegisterDefaultsResolutionFromHome()
}

// RegisterDefaultsResolutionFromHome wires defaults resolution through the
// Settings-owned CLI adapter ownership path. Tests may call this after clearing
// the hook to restore production wiring.
func RegisterDefaultsResolutionFromHome() {
	operatorsettings.ConfigureDefaultsResolutionFromHome(resolveFromHomeViaSettingsCLI)
}

func resolveFromHomeViaSettingsCLI(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	homeDir string,
	environment operatorsettings.Defaults,
	flags operatorsettings.FlagOverrides,
) (operatorsettings.ResolvedDefaults, error) {
	root, err := NewServiceFromHomePorts(files, decode)
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, fmt.Errorf("resolve operator defaults: %w", err)
	}
	return operatorsettingscli.ResolveOperatorDefaults(operatorsettingscli.ResolveOperatorDefaultsConfig{
		HomeDir:     homeDir,
		Environment: environment,
		Flags:       flags,
	}, root)
}
