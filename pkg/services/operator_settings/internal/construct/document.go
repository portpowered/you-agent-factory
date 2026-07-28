// Package construct assembles Operator Settings roots and ConfigDocument
// services from owner-private document and resolution capabilities.
package construct

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
) operatorsettings.DocumentOwner {
	return settingsdocumentwire.NewService(files, createTemp, decoder, encoder, providers)
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
	return operatorsettings.ConfigDocumentService{
		Files:           files,
		CreateTemp:      createTemp,
		Providers:       providers,
		Decoder:         decoder,
		Encoder:         encoder,
		DocumentOwner:   NewDocumentOwner(files, createTemp, decoder, encoder, providers),
		PersistenceLock: persistenceLock,
	}
}
