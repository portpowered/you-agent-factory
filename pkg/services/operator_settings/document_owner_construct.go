package operatorsettings

import "fmt"

// DocumentOwnerConstructor builds the parent-private document owner from
// injected Operator Settings ports.
type DocumentOwnerConstructor func(
	FileSystem,
	CreateTemporaryFile,
	ConfigDecoder,
	ConfigEncoder,
	ProviderCatalog,
) DocumentOwner

var documentOwnerConstructor DocumentOwnerConstructor

// ConfigureDocumentOwnerConstructor registers the nested document owner
// constructor used when ConfigDocumentService.DocumentOwner is unset.
// Wire and servicewire call this during process composition.
func ConfigureDocumentOwnerConstructor(constructor DocumentOwnerConstructor) {
	documentOwnerConstructor = constructor
}

func newDocumentOwnerFromConstructor(service ConfigDocumentService) (DocumentOwner, error) {
	if documentOwnerConstructor == nil {
		return nil, fmt.Errorf("operator settings document owner is required")
	}
	return documentOwnerConstructor(
		service.Files,
		service.CreateTemp,
		service.Decoder,
		service.Encoder,
		service.Providers,
	), nil
}
