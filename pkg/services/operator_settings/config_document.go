package operatorsettings

import (
	"context"
	"errors"
	"strings"
	"sync"
)

// ConfigDocument is one complete, validated operator configuration. It keeps
// the encoded fields private so callers can only change it through semantic
// operator-settings operations.
type ConfigDocument struct {
	config Config
}

// ProviderModelUpdate distinguishes omitted defaults from explicitly supplied
// values. A nil field preserves the current value; a non-nil field replaces it
// after trimming, including clearing it when the supplied value is empty.
type ProviderModelUpdate struct {
	Provider *string
	Model    *string
}

// ConfigDocumentService owns complete operator-config loading, semantic merge,
// and encoding. Files is required only by Load; pure operations remain usable
// without a filesystem dependency.
type ConfigDocumentService struct {
	Files           FileSystem
	CreateTemp      CreateTemporaryFile
	Providers       ProviderCatalog
	Decoder         ConfigDecoder
	Encoder         ConfigEncoder
	DocumentOwner   DocumentOwner
	PersistenceLock sync.Locker
}

// DocumentOwnerConstructor builds the parent-private document owner from
// injected Operator Settings ports.
type DocumentOwnerConstructor func(
	FileSystem,
	CreateTemporaryFile,
	ConfigDecoder,
	ConfigEncoder,
	ProviderCatalog,
) DocumentOwner

// ConfigDocumentOperations wires private document construction behavior into
// the published ConfigDocumentService surface without importing the document
// subservice from the peer root package.
type ConfigDocumentOperations struct {
	ConfigureOwnerConstructor  func(DocumentOwnerConstructor)
	Load                       func(ConfigDocumentService, string) (ConfigDocument, error)
	Parse                      func(ConfigDocumentService, []byte) (ConfigDocument, error)
	MergeProviderModelDefaults func(
		ConfigDocumentService,
		ConfigDocument,
		ProviderModelUpdate,
	) (ConfigDocument, error)
	ConfigureProviderModel func(
		ConfigDocumentService,
		context.Context,
		string,
		ProviderModelUpdate,
	) (ConfigDocument, error)
	ConfigureProviderModelPrompted func(
		ConfigDocumentService,
		context.Context,
		string,
		ProviderModelPrompt,
	) (ConfigDocument, error)
	Marshal func(ConfigDocumentService, ConfigDocument) ([]byte, error)
	Persist func(
		ConfigDocumentService,
		context.Context,
		string,
		ConfigDocument,
	) error
	EmptyConfigDocument func(func() RuntimeSettings) ConfigDocument
}

var configDocumentOperations ConfigDocumentOperations

// ErrProviderModelInputCanceled is returned by a prompt when the operator
// cancels or interrupts provider/model input. Prompt EOF is mapped to this
// outcome as well.
var ErrProviderModelInputCanceled = errors.New("provider/model input canceled")

// ConfigureConfigDocumentOperations registers private document construction
// behavior for the published ConfigDocumentService surface.
func ConfigureConfigDocumentOperations(operations ConfigDocumentOperations) {
	configDocumentOperations = operations
}

// ConfigureDocumentOwnerConstructor registers the nested document owner
// constructor used when ConfigDocumentService.DocumentOwner is unset.
// Wire and servicewire call this during process composition.
func ConfigureDocumentOwnerConstructor(constructor DocumentOwnerConstructor) {
	if configDocumentOperations.ConfigureOwnerConstructor == nil {
		panic("operator settings config document operations are required")
	}
	configDocumentOperations.ConfigureOwnerConstructor(constructor)
}

// ConfigDocumentFromConfig wraps validated configuration in a ConfigDocument.
func ConfigDocumentFromConfig(config Config) ConfigDocument {
	return ConfigDocument{config: config}
}

// Load reads and validates a complete operator configuration. A
// missing destination is represented by an empty, valid document.
func (service ConfigDocumentService) Load(path string) (ConfigDocument, error) {
	return configDocumentOperations.Load(service, path)
}

// Parse validates bytes through the injected canonical global-config codec.
func (service ConfigDocumentService) Parse(data []byte) (ConfigDocument, error) {
	return configDocumentOperations.Parse(service, data)
}

// FileConfig returns the validated semantic view of the document.
func (document ConfigDocument) FileConfig() Config {
	config := document.config
	if document.config.WorkerPresets != nil {
		config.WorkerPresets = append([]WorkerPreset{}, document.config.WorkerPresets...)
	}
	if document.config.Workers.ACP.Integrations != nil {
		config.Workers.ACP.Integrations = append([]ACPIntegration{}, document.config.Workers.ACP.Integrations...)
	}
	return config
}

// BackendScopeID returns the operator identity stored beside the defaults.
func (document ConfigDocument) BackendScopeID() string {
	return strings.TrimSpace(document.config.BackendScopeID)
}

// MergeProviderModelDefaults returns a new validated document with only the
// explicitly supplied provider/model defaults changed.
func (service ConfigDocumentService) MergeProviderModelDefaults(
	document ConfigDocument,
	update ProviderModelUpdate,
) (ConfigDocument, error) {
	return configDocumentOperations.MergeProviderModelDefaults(service, document, update)
}

// ConfigureProviderModel applies pre-supplied values through the complete
// transport-neutral load, merge, validation, and atomic persistence operation.
func (service ConfigDocumentService) ConfigureProviderModel(
	ctx context.Context,
	path string,
	update ProviderModelUpdate,
) (ConfigDocument, error) {
	return configDocumentOperations.ConfigureProviderModel(service, ctx, path, update)
}

// ConfigureProviderModelPrompted acquires values through a write-free prompt,
// then delegates successful input to ConfigureProviderModel.
func (service ConfigDocumentService) ConfigureProviderModelPrompted(
	ctx context.Context,
	path string,
	prompt ProviderModelPrompt,
) (ConfigDocument, error) {
	return configDocumentOperations.ConfigureProviderModelPrompted(service, ctx, path, prompt)
}

// Marshal encodes the complete validated document as JSON.
func (service ConfigDocumentService) Marshal(document ConfigDocument) ([]byte, error) {
	return configDocumentOperations.Marshal(service, document)
}

// Persist atomically publishes one complete, validated operator configuration.
func (service ConfigDocumentService) Persist(ctx context.Context, path string, document ConfigDocument) error {
	return configDocumentOperations.Persist(service, ctx, path, document)
}

func emptyConfigDocument() ConfigDocument {
	return configDocumentOperations.EmptyConfigDocument(defaultRuntimeSettings)
}
