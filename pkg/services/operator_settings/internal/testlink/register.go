// Package testlink registers the nested document owner constructor for
// Operator Settings unit tests without creating an import cycle.
package testlink

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"

	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/identityinputinventory"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/defaults"
)

// RegisterDocumentOwner wires the nested document owner constructor into Operator Settings
// unit tests.
func RegisterDocumentOwner() {
	operatorsettings.ConfigureDocumentOwnerConstructor(settingsconstruct.NewDocumentOwner)
}

// RegisterProvidersRoot wires the Providers root constructor used by transitional
// servicewire composition in tests that do not load pkg/wire.
func RegisterProvidersRoot() {
	operatorsettings.ConfigureProvidersRootConstructor(func() (providers.Service, error) {
		return internaltestproviders.StandardCatalog(), nil
	})
}

// RegisterComposition wires transitional Settings composition hooks for tests.
func RegisterComposition() {
	RegisterDocumentOwner()
	RegisterProvidersRoot()
}
