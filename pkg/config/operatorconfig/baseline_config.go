package operatorconfig

import (
	"encoding/json"
	"fmt"
	"os"
)

// WriteBaselineClassifierWorkerPresets adds the packaged classifier presets to
// a newly generated operator config. Callers must use it only while creating a
// new config; existing operator config is intentionally never rewritten.
func WriteBaselineClassifierWorkerPresets(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read generated operator config %q: %w", path, err)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse generated operator config %q: %w", path, err)
	}
	if _, exists := config["workerPresets"]; exists {
		return fmt.Errorf("generated operator config %q already has workerPresets", path)
	}

	presets, err := json.Marshal(BaselineClassifierWorkerPresets())
	if err != nil {
		return fmt.Errorf("marshal baseline classifier worker presets: %w", err)
	}
	config["workerPresets"] = presets

	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal generated operator config %q: %w", path, err)
	}
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write generated operator config %q: %w", path, err)
	}
	return nil
}
