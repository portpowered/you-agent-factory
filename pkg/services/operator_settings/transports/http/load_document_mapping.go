package http

import (
	"errors"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var (
	// ErrInvalidLoadPath reports a missing or blank document path at the
	// Operator Settings load HTTP adapter edge.
	ErrInvalidLoadPath = errors.New("operator settings http: invalid load path")
)

// LoadDocumentInput carries decoded HTTP inputs for one Operator Settings
// document-load operation owned by this adapter.
type LoadDocumentInput struct {
	Path            string
	RequireExisting bool
}

// LoadDocumentResponse is the adapter-owned HTTP success shape for one document
// load outcome.
type LoadDocumentResponse struct {
	Found    bool                    `json:"found"`
	Path     string                  `json:"path"`
	Document factoryapi.GlobalConfig `json:"document"`
}

// IsLoadDocumentBadRequest reports whether an error is a decode/validation
// failure that maps to a typed bad-request HTTP outcome before root invocation.
func IsLoadDocumentBadRequest(err error) bool {
	return errors.Is(err, ErrInvalidLoadPath)
}

// LoadDocumentRequestFromHTTP maps one load-document HTTP request into the
// accepted Operator Settings root request vocabulary.
func LoadDocumentRequestFromHTTP(input LoadDocumentInput) (operatorsettings.LoadDocumentRequest, error) {
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return operatorsettings.LoadDocumentRequest{}, ErrInvalidLoadPath
	}
	request := operatorsettings.LoadDocumentRequest{
		Path:            path,
		RequireExisting: input.RequireExisting,
	}
	if err := request.Validate(); err != nil {
		return operatorsettings.LoadDocumentRequest{}, err
	}
	return request, nil
}

// LoadDocumentResponseToHTTP encodes one fake-root load-document result into the
// adapter-owned HTTP success response shape.
func LoadDocumentResponseToHTTP(result operatorsettings.LoadDocumentResult) LoadDocumentResponse {
	return LoadDocumentResponse{
		Found:    result.Found,
		Path:     strings.TrimSpace(result.Path),
		Document: documentToGlobalConfig(result.Document),
	}
}

func documentToGlobalConfig(document operatorsettings.Document) factoryapi.GlobalConfig {
	generated := factoryapi.GlobalConfig{}
	if scopeID := strings.TrimSpace(document.BackendScopeID); scopeID != "" {
		generated.BackendScopeID = &scopeID
	}
	if document.Defaults != (operatorsettings.DocumentDefaults{}) {
		generated.Defaults = &factoryapi.GlobalConfigDefaults{
			WorkerModelProvider: optionalStringPointer(document.Defaults.WorkerModelProvider),
			WorkerModel:         optionalStringPointer(document.Defaults.WorkerModel),
		}
	}
	generated.Runtime = &factoryapi.GlobalConfigRuntime{
		Logging: runtimeArtifactSettingsToAPI(document.Runtime.Logging),
		Metrics: runtimeArtifactSettingsToAPI(document.Runtime.Metrics),
	}
	if document.WorkerPresets != nil {
		presets := make([]factoryapi.GlobalConfigWorkerPreset, len(document.WorkerPresets))
		for i, preset := range document.WorkerPresets {
			presets[i] = factoryapi.GlobalConfigWorkerPreset{
				Id:            preset.ID,
				ModelProvider: factoryapi.GlobalConfigWorkerPresetModelProvider(preset.ModelProvider),
				Model:         optionalStringPointer(preset.Model),
			}
			if preset.ReasoningEffort != "" {
				effort := factoryapi.GlobalConfigWorkerPresetReasoningEffort(preset.ReasoningEffort)
				presets[i].ReasoningEffort = &effort
			}
		}
		generated.WorkerPresets = &presets
	}
	return generated
}

func runtimeArtifactSettingsToAPI(
	settings operatorsettings.DocumentRuntimeArtifactSettings,
) *factoryapi.GlobalConfigRuntimeArtifactSettings {
	return &factoryapi.GlobalConfigRuntimeArtifactSettings{
		Directory:  optionalStringPointer(settings.Directory),
		MaxSizeMB:  &settings.MaxSizeMB,
		MaxBackups: &settings.MaxBackups,
		MaxAgeDays: &settings.MaxAgeDays,
		Compress:   &settings.Compress,
	}
}

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
