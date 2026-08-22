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
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) settingsdocument.Service {
	return internalservice.New(files, createTemp, decoder, encoder, providers, diagnosticDecoders...)
}

// NewServiceWithPreserver constructs the private document owner with the
// optional preservation port used when an existing tolerant document is
// atomically updated.
func NewServiceWithPreserver(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) settingsdocument.Service {
	return internalservice.NewWithPreserver(files, createTemp, decoder, encoder, providers, preserveUnknown, diagnosticDecoders...)
}
