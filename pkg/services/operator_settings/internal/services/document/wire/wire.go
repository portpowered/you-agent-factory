// Package wire constructs the Operator Settings document subservice.
package wire

import (
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	internalservice "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/internal/service"
)

// NewService constructs the private document owner from injected filesystem,
// temporary-file, decoder, encoder, and provider-catalog ports selected by the
// Operator Settings root. Construction performs no filesystem, temp-file, or
// codec work.
func NewService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
) settingsdocument.Service {
	return internalservice.New(files, createTemp, decoder, encoder, providers)
}
