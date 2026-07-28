package identityinputinventory

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func init() {
	operatorsettings.ConfigureIdentityInputInventoryOperations(operatorsettings.IdentityInputInventoryOperations{
		EnsureLocalBackendScope: EnsureLocalBackendScope,
		ProjectInputInventory:   ProjectInputInventory,
	})
}
