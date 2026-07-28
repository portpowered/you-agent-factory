// Package testlink registers the nested document owner constructor for
// Operator Settings unit tests without creating an import cycle.
package testlink

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingsservicewire "github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire"
)

// RegisterDocumentOwner wires the nested document owner into Operator Settings
// unit tests.
func RegisterDocumentOwner() {
	operatorsettings.ConfigureDocumentOwnerConstructor(operatorsettingsservicewire.NewDocumentOwner)
}
