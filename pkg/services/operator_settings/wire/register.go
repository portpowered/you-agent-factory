package wire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/identityinputinventory"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/defaults"
)

func init() {
	operatorsettings.ConfigureDocumentOwnerConstructor(NewDocumentOwner)
}
