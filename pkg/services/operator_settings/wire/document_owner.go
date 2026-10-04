package wire

import (
	"sync"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsdocumentwire "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/wire"
)

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
