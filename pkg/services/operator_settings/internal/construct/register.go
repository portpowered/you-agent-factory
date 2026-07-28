package construct

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func init() {
	operatorsettings.ConfigureDocumentOwnerConstructor(NewDocumentOwner)
}
