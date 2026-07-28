package settingsdocument

import (
	"context"
	"errors"
	"fmt"
	"io"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// Load reads and validates a complete operator configuration. A missing
// destination is represented by an empty, valid document.
func Load(service operatorsettings.ConfigDocumentService, path string) (operatorsettings.ConfigDocument, error) {
	if service.PersistenceLock != nil {
		service.PersistenceLock.Lock()
		defer service.PersistenceLock.Unlock()
	}
	return load(service, path)
}

func load(service operatorsettings.ConfigDocumentService, path string) (operatorsettings.ConfigDocument, error) {
	owner, err := ResolvedDocumentOwner(service)
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	result, err := owner.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	return configDocumentFromDocument(result.Document), nil
}

// Parse validates bytes through the injected canonical global-config codec.
func Parse(service operatorsettings.ConfigDocumentService, data []byte) (operatorsettings.ConfigDocument, error) {
	if service.Decoder == nil {
		return operatorsettings.ConfigDocument{}, fmt.Errorf("global config decoder is required")
	}
	config, err := service.Decoder(data)
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	return operatorsettings.ConfigDocumentFromConfig(config), nil
}

// MergeProviderModelDefaults returns a new validated document with only the
// explicitly supplied provider/model defaults changed.
func MergeProviderModelDefaults(
	service operatorsettings.ConfigDocumentService,
	document operatorsettings.ConfigDocument,
	update operatorsettings.ProviderModelUpdate,
) (operatorsettings.ConfigDocument, error) {
	owner, err := ResolvedDocumentOwner(service)
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	merged, err := owner.MergeDocumentProviderModel(
		documentFromConfigDocument(document),
		documentProviderModelUpdateFromProviderModelUpdate(update),
	)
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	return configDocumentFromDocument(merged), nil
}

// ConfigureProviderModel applies pre-supplied values through the complete
// transport-neutral load, merge, validation, and atomic persistence operation.
func ConfigureProviderModel(
	service operatorsettings.ConfigDocumentService,
	ctx context.Context,
	path string,
	update operatorsettings.ProviderModelUpdate,
) (operatorsettings.ConfigDocument, error) {
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	if service.PersistenceLock == nil {
		return operatorsettings.ConfigDocument{}, fmt.Errorf("operator config persistence lock is required")
	}
	service.PersistenceLock.Lock()
	defer service.PersistenceLock.Unlock()

	if err := operationContextError(ctx); err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	owner, err := ResolvedDocumentOwner(service)
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	result, err := owner.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path:          path,
		ProviderModel: documentProviderModelUpdateFromProviderModelUpdate(update),
	})
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	return configDocumentFromDocument(result.Document), nil
}

// ConfigureProviderModelPrompted acquires values through a write-free prompt,
// then delegates successful input to ConfigureProviderModel. EOF, cancellation,
// interrupt, and prompt failures return before persistence.
func ConfigureProviderModelPrompted(
	service operatorsettings.ConfigDocumentService,
	ctx context.Context,
	path string,
	prompt operatorsettings.ProviderModelPrompt,
) (operatorsettings.ConfigDocument, error) {
	if prompt == nil {
		return operatorsettings.ConfigDocument{}, fmt.Errorf("provider/model prompt is required")
	}
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	document, err := Load(service, path)
	if err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	update, err := prompt(ctx, document.FileConfig().Defaults)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, operatorsettings.ErrProviderModelInputCanceled) {
			return operatorsettings.ConfigDocument{}, fmt.Errorf("acquire provider/model input: %w", operatorsettings.ErrProviderModelInputCanceled)
		}
		return operatorsettings.ConfigDocument{}, fmt.Errorf("acquire provider/model input: %w", err)
	}
	if err := operationContextError(ctx); err != nil {
		return operatorsettings.ConfigDocument{}, err
	}
	return ConfigureProviderModel(service, ctx, path, update)
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
func Marshal(
	service operatorsettings.ConfigDocumentService,
	document operatorsettings.ConfigDocument,
) ([]byte, error) {
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
func Persist(
	service operatorsettings.ConfigDocumentService,
	ctx context.Context,
	path string,
	document operatorsettings.ConfigDocument,
) error {
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
	owner, err := ResolvedDocumentOwner(service)
	if err != nil {
		return err
	}
	return owner.PersistDocument(ctx, operatorsettings.PersistDocumentRequest{
		Path:     path,
		Document: documentFromConfigDocument(document),
	})
}

// EmptyConfigDocument returns a valid empty operator configuration document.
func EmptyConfigDocument(defaultRuntime func() operatorsettings.RuntimeSettings) operatorsettings.ConfigDocument {
	return operatorsettings.ConfigDocumentFromConfig(operatorsettings.Config{Runtime: defaultRuntime()})
}
