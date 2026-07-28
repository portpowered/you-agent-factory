package operatorsettings

import (
	"context"
	"errors"
	"fmt"
	"io"
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

func (service ConfigDocumentService) resolvedDocumentOwner() (DocumentOwner, error) {
	if service.DocumentOwner != nil {
		return service.DocumentOwner, nil
	}
	return newDocumentOwnerFromConstructor(service)
}

// ErrProviderModelInputCanceled is returned by a prompt when the operator
// cancels or interrupts provider/model input. Prompt EOF is mapped to this
// outcome as well.
var ErrProviderModelInputCanceled = errors.New("provider/model input canceled")

// Load reads and validates a complete operator configuration. A
// missing destination is represented by an empty, valid document.
func (service ConfigDocumentService) Load(path string) (ConfigDocument, error) {
	if service.PersistenceLock != nil {
		service.PersistenceLock.Lock()
		defer service.PersistenceLock.Unlock()
	}
	return service.load(path)
}

func (service ConfigDocumentService) load(path string) (ConfigDocument, error) {
	owner, err := service.resolvedDocumentOwner()
	if err != nil {
		return ConfigDocument{}, err
	}
	result, err := owner.LoadDocument(LoadDocumentRequest{Path: path})
	if err != nil {
		return ConfigDocument{}, err
	}
	return configDocumentFromDocument(result.Document), nil
}

// Parse validates bytes through the injected canonical global-config codec.
func (service ConfigDocumentService) Parse(data []byte) (ConfigDocument, error) {
	if service.Decoder == nil {
		return ConfigDocument{}, fmt.Errorf("global config decoder is required")
	}
	config, err := service.Decoder(data)
	if err != nil {
		return ConfigDocument{}, err
	}
	return ConfigDocument{config: config}, nil
}

// FileConfig returns the validated semantic view of the document.
func (document ConfigDocument) FileConfig() Config {
	config := document.config
	if document.config.WorkerPresets != nil {
		config.WorkerPresets = append([]WorkerPreset{}, document.config.WorkerPresets...)
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
	owner, err := service.resolvedDocumentOwner()
	if err != nil {
		return ConfigDocument{}, err
	}
	merged, err := owner.MergeDocumentProviderModel(
		documentFromConfigDocument(document),
		documentProviderModelUpdateFromProviderModelUpdate(update),
	)
	if err != nil {
		return ConfigDocument{}, err
	}
	return configDocumentFromDocument(merged), nil
}

// ConfigureProviderModel applies pre-supplied values through the complete
// transport-neutral load, merge, validation, and atomic persistence operation.
func (service ConfigDocumentService) ConfigureProviderModel(
	ctx context.Context,
	path string,
	update ProviderModelUpdate,
) (ConfigDocument, error) {
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	if service.PersistenceLock == nil {
		return ConfigDocument{}, fmt.Errorf("operator config persistence lock is required")
	}
	service.PersistenceLock.Lock()
	defer service.PersistenceLock.Unlock()

	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	owner, err := service.resolvedDocumentOwner()
	if err != nil {
		return ConfigDocument{}, err
	}
	result, err := owner.ApplyDocumentUpdate(ApplyDocumentUpdateRequest{
		Path:          path,
		ProviderModel: documentProviderModelUpdateFromProviderModelUpdate(update),
	})
	if err != nil {
		return ConfigDocument{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	return configDocumentFromDocument(result.Document), nil
}

// ConfigureProviderModelPrompted acquires values through a write-free prompt,
// then delegates successful input to ConfigureProviderModel. EOF, cancellation,
// interrupt, and prompt failures return before persistence.
func (service ConfigDocumentService) ConfigureProviderModelPrompted(
	ctx context.Context,
	path string,
	prompt ProviderModelPrompt,
) (ConfigDocument, error) {
	if prompt == nil {
		return ConfigDocument{}, fmt.Errorf("provider/model prompt is required")
	}
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	document, err := service.Load(path)
	if err != nil {
		return ConfigDocument{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	update, err := prompt(ctx, document.FileConfig().Defaults)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, ErrProviderModelInputCanceled) {
			return ConfigDocument{}, fmt.Errorf("acquire provider/model input: %w", ErrProviderModelInputCanceled)
		}
		return ConfigDocument{}, fmt.Errorf("acquire provider/model input: %w", err)
	}
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	return service.ConfigureProviderModel(ctx, path, update)
}

func operationContextError(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("operator config context is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("configure provider/model defaults: %w", err)
	}
	return nil
}

// Marshal encodes the complete validated document as JSON.
func (service ConfigDocumentService) Marshal(document ConfigDocument) ([]byte, error) {
	if service.Encoder == nil {
		return nil, fmt.Errorf("global config encoder is required")
	}
	config, err := document.FileConfig().Normalize()
	if err != nil {
		return nil, err
	}
	return service.Encoder(config)
}

// Persist atomically publishes one complete, validated operator configuration.
// Rename is the commit boundary: cancellation observed before it prevents the
// replacement, while a successful rename is always reported as committed.
func (service ConfigDocumentService) Persist(ctx context.Context, path string, document ConfigDocument) error {
	if ctx == nil {
		return fmt.Errorf("operator config context is required")
	}
	if service.Files == nil {
		return fmt.Errorf("operator config filesystem is required")
	}
	if service.CreateTemp == nil {
		return fmt.Errorf("operator config temporary-file creator is required")
	}
	if service.PersistenceLock == nil {
		return fmt.Errorf("operator config persistence lock is required")
	}
	service.PersistenceLock.Lock()
	defer service.PersistenceLock.Unlock()
	owner, err := service.resolvedDocumentOwner()
	if err != nil {
		return err
	}
	return owner.PersistDocument(ctx, PersistDocumentRequest{
		Path:     path,
		Document: documentFromConfigDocument(document),
	})
}

func emptyConfigDocument() ConfigDocument {
	return ConfigDocument{config: Config{Runtime: defaultRuntimeSettings()}}
}
