package wire

import (
	"sync"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocumentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
)

// NewDocumentOwner constructs the nested document owner from injected ports.
func NewDocumentOwner(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder,
) operatorsettings.DocumentOwner {
	return settingsdocumentwire.NewService(files, createTemp, decoder, encoder, providers, diagnosticDecoder)
}

// NewDocumentOwnerWithPreserver constructs the nested document owner with
// preservation of ignored forward-compatible fields during atomic updates.
func NewDocumentOwnerWithPreserver(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder,
) operatorsettings.DocumentOwner {
	return settingsdocumentwire.NewServiceWithPreserver(files, createTemp, decoder, encoder, providers, preserveUnknown, diagnosticDecoder)
}

// NewConfigDocumentService binds a completed document owner to the legacy
// config representation. Preservation and diagnostics belong to that owner.
func NewConfigDocumentService(
	document operatorsettings.DocumentOwner,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	persistenceLock sync.Locker,
) operatorsettings.ConfigDocumentService {
	return operatorsettings.ConfigDocumentService{
		Decoder:         decoder,
		Encoder:         encoder,
		DocumentOwner:   document,
		PersistenceLock: persistenceLock,
	}
}

func firstDiagnosticDecoder(decoders []operatorsettings.ConfigDiagnosticsDecoder) operatorsettings.ConfigDiagnosticsDecoder {
	if len(decoders) == 0 {
		return nil
	}
	return decoders[0]
}

// NewDocumentService constructs the completed document owner with the selected
// preservation and diagnostics policy, without reading or writing settings.
func NewDocumentService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	preserveUnknown operatorsettings.ConfigDocumentPreserver,
	diagnostics operatorsettings.ConfigDiagnosticsDecoder,
) DocumentService {
	return settingsdocumentwire.NewServiceWithPreserver(files, createTemp, decoder, encoder,
		providers, preserveUnknown, diagnostics)
}
