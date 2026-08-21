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
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) operatorsettings.DocumentOwner {
	return settingsdocumentwire.NewService(files, createTemp, decoder, encoder, providers, diagnosticDecoders...)
}

// NewConfigDocumentService constructs a root ConfigDocumentService whose load,
// update, and persist operations delegate to the nested document owner.
func NewConfigDocumentService(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
	persistenceLock sync.Locker,
	diagnosticDecoders ...operatorsettings.ConfigDiagnosticsDecoder,
) operatorsettings.ConfigDocumentService {
	return operatorsettings.ConfigDocumentService{
		Files:             files,
		CreateTemp:        createTemp,
		Providers:         providers,
		Decoder:           decoder,
		DiagnosticDecoder: firstDiagnosticDecoder(diagnosticDecoders),
		Encoder:           encoder,
		DocumentOwner:     NewDocumentOwner(files, createTemp, decoder, encoder, providers, diagnosticDecoders...),
		PersistenceLock:   persistenceLock,
	}
}

func firstDiagnosticDecoder(decoders []operatorsettings.ConfigDiagnosticsDecoder) operatorsettings.ConfigDiagnosticsDecoder {
	if len(decoders) == 0 {
		return nil
	}
	return decoders[0]
}
