package wire

import (
	"sync"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
)

// NewDocumentOwner constructs the nested document owner from injected ports.
func NewDocumentOwner(
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	providers operatorsettings.ProviderCatalog,
) operatorsettings.DocumentOwner {
	return settingsconstruct.NewDocumentOwner(files, createTemp, decoder, encoder, providers)
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
) operatorsettings.ConfigDocumentService {
	return settingsconstruct.NewConfigDocumentService(
		files,
		createTemp,
		decoder,
		encoder,
		providers,
		persistenceLock,
	)
}
