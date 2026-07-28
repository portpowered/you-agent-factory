package construct

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
)

func init() {
	operatorsettings.ConfigureDocumentOwnerConstructor(NewDocumentOwner)
}
