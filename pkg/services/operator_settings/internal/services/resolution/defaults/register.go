package settingsresolution

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func init() {
	operatorsettings.ConfigureDefaultsResolutionOperations(operatorsettings.DefaultsResolutionOperations{
		DefaultConfigPath:              DefaultConfigPath,
		LoadFileDefaults:               LoadFileDefaults,
		LoadFileConfig:                 LoadFileConfig,
		Resolve:                        Resolve,
		ResolveFromHomeWithEnvironment: ResolveFromHomeWithEnvironment,
		DeriveProviderBackendScopeID:   DeriveProviderBackendScopeID,
	})
}
