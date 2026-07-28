package http

import (
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// EffectiveOverrideFactsInput carries decoded HTTP override facts for one
// effective-resolution request.
type EffectiveOverrideFactsInput struct {
	WorkerModelProvider string
	WorkerModel         string
	WorkerPresetID      string
}

// ResolveEffectiveInput carries decoded HTTP inputs for one Operator Settings
// effective-resolution operation owned by this adapter.
type ResolveEffectiveInput struct {
	DocumentBaseline         operatorsettings.DocumentDefaults
	BackendScopeID           string
	WorkerPresets            []operatorsettings.DocumentWorkerPreset
	ExpectedDocumentBaseline *operatorsettings.DocumentDefaults
	EnvironmentOverrides     EffectiveOverrideFactsInput
	InvocationOverrides      EffectiveOverrideFactsInput
	ConfigPath               string
}

// EffectiveSelectionResponse is the adapter-owned HTTP success shape for one
// effective operator default selection.
type EffectiveSelectionResponse struct {
	BackendScopeID            string                              `json:"backendScopeId,omitempty"`
	WorkerPresets             []factoryapi.GlobalConfigWorkerPreset `json:"workerPresets,omitempty"`
	WorkerModelProvider       string                              `json:"workerModelProvider,omitempty"`
	WorkerModel               string                              `json:"workerModel,omitempty"`
	WorkerModelProviderSource string                              `json:"workerModelProviderSource,omitempty"`
	WorkerModelSource         string                              `json:"workerModelSource,omitempty"`
	ConfigPath                string                              `json:"configPath,omitempty"`
}

// ResolveEffectiveResponse is the adapter-owned HTTP success shape for one
// effective-resolution outcome.
type ResolveEffectiveResponse struct {
	Selection EffectiveSelectionResponse `json:"selection"`
}

// ResolveEffectiveRequestFromHTTP maps one resolve-effective HTTP request into
// the accepted Operator Settings root request vocabulary.
func ResolveEffectiveRequestFromHTTP(
	input ResolveEffectiveInput,
) (operatorsettings.ResolveEffectiveRequest, error) {
	request := operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline:         input.DocumentBaseline,
		BackendScopeID:           strings.TrimSpace(input.BackendScopeID),
		WorkerPresets:            cloneWorkerPresets(input.WorkerPresets),
		ExpectedDocumentBaseline: input.ExpectedDocumentBaseline,
		EnvironmentOverrides:     overrideFactsFromHTTP(input.EnvironmentOverrides),
		InvocationOverrides:      overrideFactsFromHTTP(input.InvocationOverrides),
		ConfigPath:               strings.TrimSpace(input.ConfigPath),
	}
	if err := request.Validate(); err != nil {
		return operatorsettings.ResolveEffectiveRequest{}, err
	}
	return request, nil
}

// ResolveEffectiveResponseToHTTP encodes one fake-root effective-resolution
// result into the adapter-owned HTTP success response shape.
func ResolveEffectiveResponseToHTTP(
	result operatorsettings.ResolveEffectiveResult,
) ResolveEffectiveResponse {
	return ResolveEffectiveResponse{
		Selection: effectiveSelectionToHTTP(result.Selection),
	}
}

func overrideFactsFromHTTP(
	input EffectiveOverrideFactsInput,
) operatorsettings.EffectiveOverrideFacts {
	return operatorsettings.EffectiveOverrideFacts{
		WorkerModelProvider: strings.TrimSpace(input.WorkerModelProvider),
		WorkerModel:         strings.TrimSpace(input.WorkerModel),
		WorkerPresetID:      strings.TrimSpace(input.WorkerPresetID),
	}
}

func effectiveSelectionToHTTP(
	selection operatorsettings.EffectiveSelection,
) EffectiveSelectionResponse {
	response := EffectiveSelectionResponse{
		BackendScopeID:            strings.TrimSpace(selection.BackendScopeID),
		WorkerModelProvider:       strings.TrimSpace(selection.WorkerModelProvider),
		WorkerModel:               strings.TrimSpace(selection.WorkerModel),
		WorkerModelProviderSource: string(selection.WorkerModelProviderSource),
		WorkerModelSource:         string(selection.WorkerModelSource),
		ConfigPath:                strings.TrimSpace(selection.ConfigPath),
	}
	if selection.WorkerPresets != nil {
		presets := make([]factoryapi.GlobalConfigWorkerPreset, len(selection.WorkerPresets))
		for i, preset := range selection.WorkerPresets {
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
		response.WorkerPresets = presets
	}
	return response
}

func cloneWorkerPresets(
	presets []operatorsettings.DocumentWorkerPreset,
) []operatorsettings.DocumentWorkerPreset {
	if presets == nil {
		return nil
	}
	cloned := make([]operatorsettings.DocumentWorkerPreset, len(presets))
	copy(cloned, presets)
	return cloned
}
