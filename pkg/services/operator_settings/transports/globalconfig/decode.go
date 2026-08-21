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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func mapConfig(generated factoryapi.GlobalConfig) (operatorsettings.Config, error) {
	config := operatorsettings.Config{
		BackendScopeID: optionalString(generated.BackendScopeID),
		PriceTable:     operatorsettings.DefaultPriceTable(),
		Runtime:        defaultRuntimeSettings(),
	}
	if generated.Defaults != nil {
		config.Defaults = operatorsettings.Defaults{
			WorkerModelProvider: optionalString(generated.Defaults.WorkerModelProvider),
			WorkerModel:         optionalString(generated.Defaults.WorkerModel),
		}
	}
	if generated.Models != nil {
		config.Models = mapModels(*generated.Models)
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
	if generated.PriceTable != nil {
		priceTable, err := mapPriceTable(*generated.PriceTable)
		if err != nil {
			return operatorsettings.Config{}, err
		}
		config.PriceTable = priceTable
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
	if generated.Workers != nil && generated.Workers.Acp != nil && generated.Workers.Acp.AgentProfile != nil {
		profile := operatorsettings.ACPAgentProfile{
			DefaultTarget: generated.Workers.Acp.AgentProfile.DefaultTarget,
		}
		// An omitted allowedTargets means unrestricted, so it stays nil here.
		// An authored-but-empty array must be rejected rather than folded into
		// that same nil: silently widening "allow nothing" into "allow every
		// installed Factory" is the opposite of what the operator wrote. This
		// boundary is the only place the two shapes are still
		// distinguishable, because appending an empty slice onto a nil slice
		// yields nil.
		if allowed := generated.Workers.Acp.AgentProfile.AllowedTargets; allowed != nil {
			if len(*allowed) == 0 {
				return operatorsettings.Config{}, fmt.Errorf(
					"workers.acp.agentProfile.allowedTargets must not be empty; omit it to leave the profile unrestricted",
				)
			}
			profile.AllowedTargets = append([]string(nil), *allowed...)
		}
		config.Workers.ACP.AgentProfile = &profile
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

func mapPriceTable(generated factoryapi.GlobalConfigPriceTable) (operatorsettings.PriceTable, error) {
	if string(generated.Currency) == "" {
		return operatorsettings.PriceTable{}, fmt.Errorf("priceTable.currency is required and must be USD")
	}
	if generated.Models == nil {
		return operatorsettings.PriceTable{}, fmt.Errorf(
			"priceTable.models is required; use an empty array when no prices are configured",
		)
	}
	models := make([]operatorsettings.PriceTableModel, len(generated.Models))
	for index, model := range generated.Models {
		models[index] = operatorsettings.PriceTableModel{
			Provider:                        model.Provider,
			Model:                           model.Model,
			InputPerMillionTokens:           model.InputPerMillionTokens,
			OutputPerMillionTokens:          model.OutputPerMillionTokens,
			CachedInputPerMillionTokens:     model.CachedInputPerMillionTokens,
			ReasoningOutputPerMillionTokens: model.ReasoningOutputPerMillionTokens,
		}
	}
	table, err := (operatorsettings.PriceTable{Currency: string(generated.Currency), Models: models}).Normalize()
	if err != nil {
		return operatorsettings.PriceTable{}, err
	}
	return table, nil
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
	priceModels := make([]factoryapi.GlobalConfigPriceTableModel, len(config.PriceTable.Models))
	for index, model := range config.PriceTable.Models {
		priceModels[index] = factoryapi.GlobalConfigPriceTableModel{
			Provider:                        model.Provider,
			Model:                           model.Model,
			InputPerMillionTokens:           model.InputPerMillionTokens,
			OutputPerMillionTokens:          model.OutputPerMillionTokens,
			CachedInputPerMillionTokens:     model.CachedInputPerMillionTokens,
			ReasoningOutputPerMillionTokens: model.ReasoningOutputPerMillionTokens,
		}
	}
	generated.PriceTable = &factoryapi.GlobalConfigPriceTable{
		Currency: factoryapi.GlobalConfigPriceTableCurrency(config.PriceTable.Currency),
		Models:   priceModels,
	}
	if config.Defaults != (operatorsettings.Defaults{}) {
		generated.Defaults = &factoryapi.GlobalConfigDefaults{
			WorkerModelProvider: optionalStringPointer(config.Defaults.WorkerModelProvider),
			WorkerModel:         optionalStringPointer(config.Defaults.WorkerModel),
		}
	}
	if config.Models != nil {
		models := modelsToGenerated(config.Models)
		generated.Models = &models
	}
	generated.Runtime = &factoryapi.GlobalConfigRuntime{
		Logging: mapRuntimeArtifactSettingsToAPI(config.Runtime.Logging),
		Metrics: mapRuntimeArtifactSettingsToAPI(config.Runtime.Metrics),
	}
	if config.Workers.ACP.Integrations != nil || config.Workers.ACP.AgentProfile != nil {
		acp := &factoryapi.GlobalConfigACPSettings{}
		if config.Workers.ACP.Integrations != nil {
			integrations := make([]factoryapi.GlobalConfigACPIntegration, len(config.Workers.ACP.Integrations))
			for index, integration := range config.Workers.ACP.Integrations {
				integrations[index] = factoryapi.GlobalConfigACPIntegration{
					Id: integration.ID, Name: integration.Name, Command: integration.Command,
					Transport: factoryapi.GlobalConfigACPIntegrationTransport(integration.Transport),
				}
			}
			acp.Integrations = &integrations
		}
		if config.Workers.ACP.AgentProfile != nil {
			acp.AgentProfile = &factoryapi.GlobalConfigACPAgentProfile{
				DefaultTarget: config.Workers.ACP.AgentProfile.DefaultTarget,
			}
			// Omit allowedTargets entirely for an unrestricted profile, so the
			// encoded document round-trips back to unrestricted rather than to
			// an empty array the schema would reject.
			if !config.Workers.ACP.AgentProfile.IsUnrestricted() {
				allowed := append([]string(nil), config.Workers.ACP.AgentProfile.AllowedTargets...)
				acp.AgentProfile.AllowedTargets = &allowed
			}
		}
		generated.Workers = &factoryapi.GlobalConfigWorkers{Acp: acp}
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

func mapModels(generated factoryapi.GlobalConfigModels) map[string]operatorsettings.ModelConfig {
	models := make(map[string]operatorsettings.ModelConfig, len(generated))
	for name, model := range generated {
		entry := operatorsettings.ModelConfig{}
		if model.Source != nil {
			source := *model.Source
			entry.Source = &source
		}
		if model.Backend != nil {
			backend := *model.Backend
			entry.Backend = &backend
		}
		if model.LoadPolicy != nil {
			policy := operatorsettings.ModelLoadPolicy(*model.LoadPolicy)
			entry.LoadPolicy = &policy
		}
		if model.Operations != nil {
			operations := make([]string, len(*model.Operations))
			for index, operation := range *model.Operations {
				operations[index] = string(operation)
			}
			entry.Operations = operations
		}
		models[name] = entry
	}
	return models
}

func modelsToGenerated(values map[string]operatorsettings.ModelConfig) factoryapi.GlobalConfigModels {
	models := make(factoryapi.GlobalConfigModels, len(values))
	for name, config := range values {
		entry := factoryapi.GlobalConfigModel{}
		if config.Source != nil {
			source := *config.Source
			entry.Source = &source
		}
		if config.Backend != nil {
			backend := *config.Backend
			entry.Backend = &backend
		}
		if config.LoadPolicy != nil {
			policy := factoryapi.GlobalConfigModelLoadPolicy(*config.LoadPolicy)
			entry.LoadPolicy = &policy
		}
		if config.Operations != nil {
			operations := make([]factoryapi.GlobalConfigModelOperation, len(config.Operations))
			for index, operation := range config.Operations {
				operations[index] = factoryapi.GlobalConfigModelOperation(operation)
			}
			entry.Operations = &operations
		}
		models[name] = entry
	}
	return models
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
