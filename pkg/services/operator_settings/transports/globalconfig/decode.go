// Package globalconfig owns the generated GlobalConfig codec for Operator
// Settings. It keeps generated-contract decoding, representation mapping, and
// canonical document encoding at the Operator Settings transport boundary.
package globalconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Decode strictly decodes one generated GlobalConfig document and maps it to
// normalized Operator Settings values.
func Decode(data []byte) (operatorsettings.Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var generated *factoryapi.GlobalConfig
	if err := decoder.Decode(&generated); err != nil {
		return operatorsettings.Config{}, fmt.Errorf("decode generated global config: %w", err)
	}
	if generated == nil {
		return operatorsettings.Config{}, fmt.Errorf("decode generated global config: expected a JSON object")
	}
	if err := requireEOF(decoder); err != nil {
		return operatorsettings.Config{}, err
	}

	config, err := mapConfig(*generated)
	if err != nil {
		return operatorsettings.Config{}, err
	}
	return config.Normalize()
}

func requireEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("decode generated global config: %w", err)
	}
	return fmt.Errorf("decode generated global config: unexpected trailing JSON")
}

func mapConfig(generated factoryapi.GlobalConfig) (operatorsettings.Config, error) {
	config := operatorsettings.Config{
		BackendScopeID: optionalString(generated.BackendScopeID),
		Runtime:        defaultRuntimeSettings(),
	}
	if generated.Defaults != nil {
		config.Defaults = operatorsettings.Defaults{
			WorkerModelProvider: optionalString(generated.Defaults.WorkerModelProvider),
			WorkerModel:         optionalString(generated.Defaults.WorkerModel),
		}
	}
	if generated.Runtime != nil {
		var err error
		config.Runtime.Logging, err = mapRuntimeArtifactSettings(
			"runtime.logging",
			generated.Runtime.Logging,
			config.Runtime.Logging,
		)
		if err != nil {
			return operatorsettings.Config{}, err
		}
		config.Runtime.Metrics, err = mapRuntimeArtifactSettings(
			"runtime.metrics",
			generated.Runtime.Metrics,
			config.Runtime.Metrics,
		)
		if err != nil {
			return operatorsettings.Config{}, err
		}
	}
	if generated.Workers != nil && generated.Workers.Acp != nil && generated.Workers.Acp.Integrations != nil {
		config.Workers.ACP.Integrations = make([]operatorsettings.ACPIntegration, len(*generated.Workers.Acp.Integrations))
		for index, integration := range *generated.Workers.Acp.Integrations {
			config.Workers.ACP.Integrations[index] = operatorsettings.ACPIntegration{
				ID: integration.Id, Name: integration.Name,
				Transport: string(integration.Transport), Command: integration.Command,
			}
		}
	}
	if generated.WorkerPresets == nil {
		return config, nil
	}

	config.WorkerPresets = make([]operatorsettings.WorkerPreset, len(*generated.WorkerPresets))
	for i, preset := range *generated.WorkerPresets {
		config.WorkerPresets[i] = operatorsettings.WorkerPreset{
			ID:              preset.Id,
			ModelProvider:   string(preset.ModelProvider),
			Model:           optionalString(preset.Model),
			ReasoningEffort: optionalEnum(preset.ReasoningEffort),
		}
	}
	return config, nil
}

func defaultRuntimeSettings() operatorsettings.RuntimeSettings {
	defaults := operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
		MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
	}
	return operatorsettings.RuntimeSettings{Logging: defaults, Metrics: defaults}
}

func mapRuntimeArtifactSettings(
	fieldPath string,
	generated *factoryapi.GlobalConfigRuntimeArtifactSettings,
	defaults operatorsettings.RuntimeArtifactSettings,
) (operatorsettings.RuntimeArtifactSettings, error) {
	if generated == nil {
		return defaults, nil
	}
	settings := defaults
	if generated.Directory != nil {
		settings.Directory = strings.TrimSpace(*generated.Directory)
		if settings.Directory == "" {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.directory must be non-empty", fieldPath)
		}
	}
	if generated.MaxSizeMB != nil {
		if *generated.MaxSizeMB < 1 {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.maxSizeMB must be at least 1", fieldPath)
		}
		settings.MaxSizeMB = *generated.MaxSizeMB
	}
	if generated.MaxBackups != nil {
		if *generated.MaxBackups < 1 {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.maxBackups must be at least 1", fieldPath)
		}
		settings.MaxBackups = *generated.MaxBackups
	}
	if generated.MaxAgeDays != nil {
		if *generated.MaxAgeDays < 1 {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.maxAgeDays must be at least 1", fieldPath)
		}
		settings.MaxAgeDays = *generated.MaxAgeDays
	}
	if generated.Compress != nil {
		settings.Compress = *generated.Compress
	}
	return settings, nil
}

// Encode maps normalized Operator Settings values into the generated
// GlobalConfig model and serializes one canonical filesystem document.
func Encode(config operatorsettings.Config) ([]byte, error) {
	normalized, err := config.Normalize()
	if err != nil {
		return nil, fmt.Errorf("encode generated global config: %w", err)
	}
	config = normalized
	generated := factoryapi.GlobalConfig{}
	if scopeID := strings.TrimSpace(config.BackendScopeID); scopeID != "" {
		generated.BackendScopeID = &scopeID
	}
	if config.Defaults != (operatorsettings.Defaults{}) {
		generated.Defaults = &factoryapi.GlobalConfigDefaults{
			WorkerModelProvider: optionalStringPointer(config.Defaults.WorkerModelProvider),
			WorkerModel:         optionalStringPointer(config.Defaults.WorkerModel),
		}
	}
	generated.Runtime = &factoryapi.GlobalConfigRuntime{
		Logging: mapRuntimeArtifactSettingsToAPI(config.Runtime.Logging),
		Metrics: mapRuntimeArtifactSettingsToAPI(config.Runtime.Metrics),
	}
	if config.Workers.ACP.Integrations != nil {
		integrations := make([]factoryapi.GlobalConfigACPIntegration, len(config.Workers.ACP.Integrations))
		for index, integration := range config.Workers.ACP.Integrations {
			integrations[index] = factoryapi.GlobalConfigACPIntegration{
				Id: integration.ID, Name: integration.Name, Command: integration.Command,
				Transport: factoryapi.GlobalConfigACPIntegrationTransport(integration.Transport),
			}
		}
		generated.Workers = &factoryapi.GlobalConfigWorkers{
			Acp: &factoryapi.GlobalConfigACPSettings{Integrations: &integrations},
		}
	}
	if config.WorkerPresets != nil {
		presets := make([]factoryapi.GlobalConfigWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
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

	payload, err := json.MarshalIndent(generated, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated global config: %w", err)
	}
	return append(payload, '\n'), nil
}

func mapRuntimeArtifactSettingsToAPI(
	settings operatorsettings.RuntimeArtifactSettings,
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

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalEnum(value *factoryapi.GlobalConfigWorkerPresetReasoningEffort) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
