package internal_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

var testFiles platformfilesystem.Local
var testIDGenerator operatorsettings.IDGenerator = uuid.NewString
var testCreateTemp operatorsettings.CreateTemporaryFile = func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
	return os.CreateTemp(dir, pattern)
}

func decodeTestConfig(data []byte) (operatorsettings.Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var generated *factoryapi.GlobalConfig
	if err := decoder.Decode(&generated); err != nil {
		return operatorsettings.Config{}, err
	}
	if generated == nil {
		return operatorsettings.Config{}, fmt.Errorf("expected a JSON object")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return operatorsettings.Config{}, fmt.Errorf("unexpected trailing JSON")
	}
	config := operatorsettings.Config{}
	if generated.BackendScopeID != nil {
		config.BackendScopeID = *generated.BackendScopeID
	}
	if generated.Defaults != nil {
		if generated.Defaults.WorkerModelProvider != nil {
			config.Defaults.WorkerModelProvider = *generated.Defaults.WorkerModelProvider
		}
		if generated.Defaults.WorkerModel != nil {
			config.Defaults.WorkerModel = *generated.Defaults.WorkerModel
		}
	}
	if generated.WorkerPresets != nil {
		for _, preset := range *generated.WorkerPresets {
			mapped := operatorsettings.WorkerPreset{ID: preset.Id, ModelProvider: string(preset.ModelProvider)}
			if preset.Model != nil {
				mapped.Model = *preset.Model
			}
			if preset.ReasoningEffort != nil {
				mapped.ReasoningEffort = string(*preset.ReasoningEffort)
			}
			config.WorkerPresets = append(config.WorkerPresets, mapped)
		}
	}
	return config.Normalize()
}

func encodeTestConfig(config operatorsettings.Config) ([]byte, error) {
	generated := factoryapi.GlobalConfig{}
	if config.BackendScopeID != "" {
		generated.BackendScopeID = &config.BackendScopeID
	}
	if config.Defaults != (operatorsettings.Defaults{}) {
		generated.Defaults = &factoryapi.GlobalConfigDefaults{}
		if config.Defaults.WorkerModelProvider != "" {
			generated.Defaults.WorkerModelProvider = &config.Defaults.WorkerModelProvider
		}
		if config.Defaults.WorkerModel != "" {
			generated.Defaults.WorkerModel = &config.Defaults.WorkerModel
		}
	}
	if config.WorkerPresets != nil {
		presets := make([]factoryapi.GlobalConfigWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			presets[i] = factoryapi.GlobalConfigWorkerPreset{
				Id:            preset.ID,
				ModelProvider: factoryapi.GlobalConfigWorkerPresetModelProvider(preset.ModelProvider),
			}
			if preset.Model != "" {
				presets[i].Model = &preset.Model
			}
			if preset.ReasoningEffort != "" {
				effort := factoryapi.GlobalConfigWorkerPresetReasoningEffort(preset.ReasoningEffort)
				presets[i].ReasoningEffort = &effort
			}
		}
		generated.WorkerPresets = &presets
	}
	payload, err := json.MarshalIndent(generated, "", "  ")
	return append(payload, '\n'), err
}

func ensureTestBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	return operatorsettings.EnsureLocalBackendScope(
		testFiles,
		testCreateTemp,
		testIDGenerator,
		decodeTestConfig,
		encodeTestConfig,
		path,
	)
}
