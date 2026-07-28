// Package testlink registers the nested document owner constructor for
// Operator Settings unit tests without creating an import cycle.
package testlink

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	operatorsettingsservicewire "github.com/portpowered/infinite-you/pkg/services/operator_settings/servicewire"
	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// RegisterDocumentOwner wires the nested document owner into Operator Settings
// unit tests.
func RegisterDocumentOwner() {
	operatorsettings.ConfigureDocumentOwnerConstructor(operatorsettingsservicewire.NewDocumentOwner)
}

// RegisterProvidersRoot wires the Providers root constructor used by transitional
// servicewire composition in tests that do not load pkg/wire.
func RegisterProvidersRoot() {
	operatorsettings.ConfigureProvidersRootConstructor(func() (providers.Service, error) {
		return testproviders.StandardCatalog(), nil
	})
}

// RegisterComposition wires transitional Settings composition hooks for tests.
func RegisterComposition() {
	RegisterDocumentOwner()
	RegisterProvidersRoot()
}
