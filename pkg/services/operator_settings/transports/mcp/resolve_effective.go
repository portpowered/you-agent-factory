package operatorsettingsmcp

import (
	"context"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// DocumentDefaultsInput is the MCP request shape for document baseline facts.
type DocumentDefaultsInput struct {
	WorkerModelProvider string `json:"workerModelProvider"`
	WorkerModel         string `json:"workerModel"`
}

// DocumentWorkerPresetInput is the MCP request shape for worker preset facts.
type DocumentWorkerPresetInput struct {
	ID              string `json:"id"`
	ModelProvider   string `json:"modelProvider"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoningEffort"`
}

// EffectiveOverrideInput is the MCP request shape for override facts.
type EffectiveOverrideInput struct {
	WorkerModelProvider string `json:"workerModelProvider"`
	WorkerModel         string `json:"workerModel"`
	WorkerPresetID      string `json:"workerPresetId"`
}

// ResolveEffectiveInput is the MCP request shape for
// you.operator_settings.resolve_effective.
type ResolveEffectiveInput struct {
	DocumentBaseline         DocumentDefaultsInput       `json:"documentBaseline"`
	BackendScopeID           string                      `json:"backendScopeId"`
	WorkerPresets            []DocumentWorkerPresetInput `json:"workerPresets,omitempty"`
	ExpectedDocumentBaseline *DocumentDefaultsInput      `json:"expectedDocumentBaseline,omitempty"`
	EnvironmentOverrides     EffectiveOverrideInput      `json:"environmentOverrides,omitempty"`
	InvocationOverrides      EffectiveOverrideInput      `json:"invocationOverrides,omitempty"`
	ConfigPath               string                      `json:"configPath"`
}

// ResolveEffective returns detached effective selection facts through the
// you.operator_settings.resolve_effective MCP tool.
func ResolveEffective(
	ctx context.Context,
	service operatorsettings.Service,
	input ResolveEffectiveInput,
) ToolResponse[operatorsettings.ResolveEffectiveResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[operatorsettings.ResolveEffectiveResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[operatorsettings.ResolveEffectiveResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[operatorsettings.ResolveEffectiveResult]{Error: &envelope}
	}

	result, err := service.ResolveEffective(mapResolveEffectiveRequest(input))
	if err != nil {
		envelope := resolveEffectiveErrorEnvelope(input.ConfigPath, err)
		return ToolResponse[operatorsettings.ResolveEffectiveResult]{Error: &envelope}
	}
	return ToolResponse[operatorsettings.ResolveEffectiveResult]{Result: &result}
}

func mapResolveEffectiveRequest(input ResolveEffectiveInput) operatorsettings.ResolveEffectiveRequest {
	request := operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: input.DocumentBaseline.WorkerModelProvider,
			WorkerModel:         input.DocumentBaseline.WorkerModel,
		},
		BackendScopeID: input.BackendScopeID,
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: input.EnvironmentOverrides.WorkerModelProvider,
			WorkerModel:         input.EnvironmentOverrides.WorkerModel,
			WorkerPresetID:      input.EnvironmentOverrides.WorkerPresetID,
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: input.InvocationOverrides.WorkerModelProvider,
			WorkerModel:         input.InvocationOverrides.WorkerModel,
			WorkerPresetID:      input.InvocationOverrides.WorkerPresetID,
		},
		ConfigPath: input.ConfigPath,
	}
	if input.ExpectedDocumentBaseline != nil {
		expected := operatorsettings.DocumentDefaults{
			WorkerModelProvider: input.ExpectedDocumentBaseline.WorkerModelProvider,
			WorkerModel:         input.ExpectedDocumentBaseline.WorkerModel,
		}
		request.ExpectedDocumentBaseline = &expected
	}
	if len(input.WorkerPresets) > 0 {
		request.WorkerPresets = make([]operatorsettings.DocumentWorkerPreset, len(input.WorkerPresets))
		for i, preset := range input.WorkerPresets {
			request.WorkerPresets[i] = operatorsettings.DocumentWorkerPreset{
				ID:              preset.ID,
				ModelProvider:   preset.ModelProvider,
				Model:           preset.Model,
				ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return request
}
