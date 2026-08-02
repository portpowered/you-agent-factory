package operatorsettings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ConfigDocument is a detached compatibility value for older configuration
// callers. New cross-service callers use Document through Service.
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

// ConfigDocumentService is a service-local compatibility adapter. It contains
// injected ports and a private DocumentOwner; it is not the peer-facing
// Operator Settings authority. New code should depend on Service.
type ConfigDocumentService struct {
	Files           FileSystem
	CreateTemp      CreateTemporaryFile
	Providers       ProviderCatalog
	Decoder         ConfigDecoder
	Encoder         ConfigEncoder
	DocumentOwner   DocumentOwner
	PersistenceLock sync.Locker
}

// ErrProviderModelInputCanceled is returned by a prompt when the operator
// cancels or interrupts provider/model input. Prompt EOF is mapped to this
// outcome as well.
var ErrProviderModelInputCanceled = errors.New("provider/model input canceled")

func (service ConfigDocumentService) owner() (DocumentOwner, error) {
	if service.DocumentOwner == nil {
		return nil, fmt.Errorf("operator settings document owner is required")
	}
	if rebindable, ok := service.DocumentOwner.(interface {
		RebindDocumentOwner(FileSystem, CreateTemporaryFile, ConfigDecoder, ConfigEncoder, ProviderCatalog) DocumentOwner
	}); ok {
		return rebindable.RebindDocumentOwner(
			service.Files,
			service.CreateTemp,
			service.Decoder,
			service.Encoder,
			service.Providers,
		), nil
	}
	return service.DocumentOwner, nil
}

// Load reads and validates a complete operator configuration. A missing
// destination is represented by an empty, valid document.
func (service ConfigDocumentService) Load(path string) (ConfigDocument, error) {
	if service.Files == nil {
		return ConfigDocument{}, fmt.Errorf("operator settings filesystem is required")
	}
	if service.PersistenceLock != nil {
		service.PersistenceLock.Lock()
		defer service.PersistenceLock.Unlock()
	}
	owner, err := service.owner()
	if err != nil {
		return ConfigDocument{}, err
	}
	result, err := owner.LoadDocument(LoadDocumentRequest{Path: path})
	if err != nil {
		return ConfigDocument{}, err
	}
	return ConfigDocument{config: configFromDocument(result.Document)}, nil
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
	if document.config.Workers.ACP.Integrations != nil {
		config.Workers.ACP.Integrations = append([]ACPIntegration{}, document.config.Workers.ACP.Integrations...)
	}
	config.Workers.ACP.AgentProfile = cloneACPAgentProfilePointer(document.config.Workers.ACP.AgentProfile)
	return config
}

// cloneACPAgentProfilePointer returns a detached copy of an optional ACP
// Agent profile pointer, preserving nil for an absent profile.
func cloneACPAgentProfilePointer(profile *ACPAgentProfile) *ACPAgentProfile {
	if profile == nil {
		return nil
	}
	cloned := profile.Clone()
	return &cloned
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
	owner, err := service.owner()
	if err != nil {
		return ConfigDocument{}, err
	}
	merged, err := owner.MergeDocumentProviderModel(
		documentFromConfigDocument(document),
		DocumentProviderModelUpdate{Provider: update.Provider, Model: update.Model},
	)
	if err != nil {
		return ConfigDocument{}, err
	}
	return ConfigDocument{config: configFromDocument(merged)}, nil
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
	owner, err := service.owner()
	if err != nil {
		return ConfigDocument{}, err
	}
	result, err := owner.ApplyDocumentUpdate(ApplyDocumentUpdateRequest{
		Path: path,
		ProviderModel: DocumentProviderModelUpdate{
			Provider: update.Provider,
			Model:    update.Model,
		},
	})
	if err != nil {
		return ConfigDocument{}, err
	}
	if err := operationContextError(ctx); err != nil {
		return ConfigDocument{}, err
	}
	return ConfigDocument{config: configFromDocument(result.Document)}, nil
}

// ConfigureProviderModelPrompted acquires values through a write-free prompt,
// then delegates successful input to ConfigureProviderModel.
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
func (service ConfigDocumentService) Persist(ctx context.Context, path string, document ConfigDocument) error {
	if ctx == nil {
		return fmt.Errorf("operator config context is required")
	}
	if err := service.validatePersistencePorts(path); err != nil {
		return err
	}
	if service.PersistenceLock == nil {
		return fmt.Errorf("operator config persistence lock is required")
	}
	service.PersistenceLock.Lock()
	defer service.PersistenceLock.Unlock()
	owner, err := service.owner()
	if err != nil {
		return err
	}
	return owner.PersistDocument(ctx, PersistDocumentRequest{
		Path:     path,
		Document: documentFromConfigDocument(document),
	})
}

func (service ConfigDocumentService) validatePersistencePorts(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("operator config path is required")
	}
	if service.Files == nil {
		return fmt.Errorf("operator settings filesystem is required")
	}
	if service.CreateTemp == nil {
		return fmt.Errorf("operator settings temporary-file creator is required")
	}
	return nil
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

func documentFromConfigDocument(document ConfigDocument) Document {
	return documentFromConfig(document.FileConfig())
}

func configFromDocument(document Document) Config {
	config := Config{
		BackendScopeID: document.BackendScopeID,
		Defaults: Defaults{
			WorkerModelProvider: document.Defaults.WorkerModelProvider,
			WorkerModel:         document.Defaults.WorkerModel,
		},
		Runtime: RuntimeSettings{
			Logging: RuntimeArtifactSettings(document.Runtime.Logging),
			Metrics: RuntimeArtifactSettings(document.Runtime.Metrics),
		},
		Workers: WorkerSettings{ACP: ACPSettings{
			Integrations: append([]ACPIntegration(nil), document.Workers.ACP.Integrations...),
			AgentProfile: cloneACPAgentProfilePointer(document.Workers.ACP.AgentProfile),
		}},
	}
	if document.WorkerPresets != nil {
		config.WorkerPresets = make([]WorkerPreset, len(document.WorkerPresets))
		for i, preset := range document.WorkerPresets {
			config.WorkerPresets[i] = WorkerPreset{
				ID: preset.ID, ModelProvider: preset.ModelProvider,
				Model: preset.Model, ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return config
}

func documentFromConfig(config Config) Document {
	document := Document{
		BackendScopeID: config.BackendScopeID,
		Defaults: DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		Runtime: DocumentRuntimeSettings{
			Logging: DocumentRuntimeArtifactSettings(config.Runtime.Logging),
			Metrics: DocumentRuntimeArtifactSettings(config.Runtime.Metrics),
		},
		Workers: DocumentWorkerSettings{ACP: DocumentACPSettings{
			Integrations: append([]ACPIntegration(nil), config.Workers.ACP.Integrations...),
			AgentProfile: cloneACPAgentProfilePointer(config.Workers.ACP.AgentProfile),
		}},
	}
	if config.WorkerPresets != nil {
		document.WorkerPresets = make([]DocumentWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			document.WorkerPresets[i] = DocumentWorkerPreset{
				ID: preset.ID, ModelProvider: preset.ModelProvider,
				Model: preset.Model, ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return document
}

func emptyConfigDocument() ConfigDocument {
	return ConfigDocument{config: Config{Runtime: defaultRuntimeSettings()}}
}
