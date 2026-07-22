// Package globalconfig maps the generated global-config contract into
// Operator Settings domain values.
package globalconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Decode strictly decodes one generated GlobalConfig document and maps it to
// normalized Operator Settings values.
func Decode(data []byte) (operatorsettings.Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var generated factoryapi.GlobalConfig
	if err := decoder.Decode(&generated); err != nil {
		return operatorsettings.Config{}, fmt.Errorf("decode generated global config: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return operatorsettings.Config{}, err
	}

	return mapConfig(generated).Normalize()
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

func mapConfig(generated factoryapi.GlobalConfig) operatorsettings.Config {
	config := operatorsettings.Config{}
	if generated.Defaults != nil {
		config.Defaults = operatorsettings.Defaults{
			WorkerModelProvider: optionalString(generated.Defaults.WorkerModelProvider),
			WorkerModel:         optionalString(generated.Defaults.WorkerModel),
		}
	}
	if generated.WorkerPresets == nil {
		return config
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
	return config
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
