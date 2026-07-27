package service

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type service struct {
	providers providerQuery
}

var _ resolution.Service = (*service)(nil)

// New constructs an inert effective-resolution owner.
func New(providersRoot providers.Service) (resolution.Service, error) {
	providerQuery, err := newProvidersRootQuery(providersRoot)
	if err != nil {
		return nil, err
	}
	return &service{providers: providerQuery}, nil
}

func (s *service) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if err := request.Validate(); err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	invocationOverrides, err := expandInvocationOverrides(request)
	if err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	providerRaw, providerSource := winningLayerValue(
		request.DocumentBaseline.WorkerModelProvider,
		request.EnvironmentOverrides.WorkerModelProvider,
		invocationOverrides.WorkerModelProvider,
	)
	modelRaw, modelSource := winningLayerValue(
		request.DocumentBaseline.WorkerModel,
		request.EnvironmentOverrides.WorkerModel,
		invocationOverrides.WorkerModel,
	)

	resolvedProvider, err := s.resolveWorkerModelProvider(providerRaw, providerSource, request, invocationOverrides)
	if err != nil {
		return operatorsettings.ResolveEffectiveResult{}, err
	}

	return operatorsettings.ResolveEffectiveResult{
		Selection: operatorsettings.EffectiveSelection{
			BackendScopeID:            strings.TrimSpace(request.BackendScopeID),
			WorkerPresets:             cloneWorkerPresets(request.WorkerPresets),
			WorkerModelProvider:       resolvedProvider,
			WorkerModel:               strings.TrimSpace(modelRaw),
			WorkerModelProviderSource: providerSource,
			WorkerModelSource:         modelSource,
			ConfigPath:                strings.TrimSpace(request.ConfigPath),
		},
	}, nil
}

func expandInvocationOverrides(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.EffectiveOverrideFacts, error) {
	overrides := request.InvocationOverrides.Clone()
	presetID := strings.TrimSpace(overrides.WorkerPresetID)
	if presetID == "" {
		return overrides, nil
	}

	preset, ok := findWorkerPreset(request.WorkerPresets, presetID)
	if !ok {
		return operatorsettings.EffectiveOverrideFacts{}, operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: presetID,
			Field:   "workerPresetID",
		}
	}

	if strings.TrimSpace(overrides.WorkerModelProvider) == "" {
		overrides.WorkerModelProvider = preset.ModelProvider
	}
	if strings.TrimSpace(overrides.WorkerModel) == "" {
		overrides.WorkerModel = preset.Model
	}
	return overrides, nil
}

func findWorkerPreset(
	presets []operatorsettings.DocumentWorkerPreset,
	id string,
) (operatorsettings.DocumentWorkerPreset, bool) {
	for _, preset := range presets {
		if strings.TrimSpace(preset.ID) == id {
			return preset, true
		}
	}
	return operatorsettings.DocumentWorkerPreset{}, false
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

func winningLayerValue(
	baselineValue, envValue, flagValue string,
) (string, operatorsettings.EffectiveLayerSource) {
	switch {
	case strings.TrimSpace(flagValue) != "":
		return strings.TrimSpace(flagValue), operatorsettings.EffectiveLayerSourceFlag
	case strings.TrimSpace(envValue) != "":
		return strings.TrimSpace(envValue), operatorsettings.EffectiveLayerSourceEnv
	case strings.TrimSpace(baselineValue) != "":
		return strings.TrimSpace(baselineValue), operatorsettings.EffectiveLayerSourceFile
	default:
		return "", ""
	}
}

func (s *service) resolveWorkerModelProvider(
	raw string,
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
	invocationOverrides operatorsettings.EffectiveOverrideFacts,
) (string, error) {
	if raw == "" {
		return "", nil
	}

	if interfaces.IsSymbolicWorkerModelProviderDefault(raw) {
		concreteRaw := concreteProviderBelowSource(winningSource, request, invocationOverrides)
		if concreteRaw == "" {
			return "", operatorsettings.ResolutionFailure{
				Kind:    operatorsettings.ResolutionFailureKindInvalidInput,
				Message: "symbolic DEFAULT requires a concrete provider from file or environment",
				Field:   "workerModelProvider",
			}
		}
		return s.providers.CanonicalizeConcreteProvider(concreteRaw)
	}

	return s.providers.CanonicalizeConcreteProvider(raw)
}

func concreteProviderBelowSource(
	winningSource operatorsettings.EffectiveLayerSource,
	request operatorsettings.ResolveEffectiveRequest,
	invocationOverrides operatorsettings.EffectiveOverrideFacts,
) string {
	type layer struct {
		source operatorsettings.EffectiveLayerSource
		value  string
	}
	layers := []layer{
		{source: operatorsettings.EffectiveLayerSourceFile, value: request.DocumentBaseline.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceEnv, value: request.EnvironmentOverrides.WorkerModelProvider},
		{source: operatorsettings.EffectiveLayerSourceFlag, value: invocationOverrides.WorkerModelProvider},
	}

	below := make([]layer, 0, 2)
	for _, layer := range layers {
		if layer.source == winningSource {
			break
		}
		below = append(below, layer)
	}
	for i := len(below) - 1; i >= 0; i-- {
		value := strings.TrimSpace(below[i].value)
		if value == "" || interfaces.IsSymbolicWorkerModelProviderDefault(value) {
			continue
		}
		return value
	}
	return ""
}
