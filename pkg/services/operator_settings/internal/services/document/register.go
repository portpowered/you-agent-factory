package settingsdocument

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func init() {
	operatorsettings.ConfigureConfigDocumentOperations(operatorsettings.ConfigDocumentOperations{
		ConfigureOwnerConstructor:      ConfigureOwnerConstructor,
		Load:                           Load,
		Parse:                          Parse,
		MergeProviderModelDefaults:     MergeProviderModelDefaults,
		ConfigureProviderModel:         ConfigureProviderModel,
		ConfigureProviderModelPrompted: ConfigureProviderModelPrompted,
		Marshal:                        Marshal,
		Persist:                        Persist,
		EmptyConfigDocument:            EmptyConfigDocument,
	})
}
