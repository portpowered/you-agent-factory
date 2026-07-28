package factorycontracts

import (
	"encoding/json"
	"fmt"
)

// CloneFactoryConfig returns a detached copy of a canonical Factory
// definition.
func CloneFactoryConfig(cfg *FactoryConfig) (*FactoryConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("encode Factory definition clone: %w", err)
	}
	var cloned FactoryConfig
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("decode Factory definition clone: %w", err)
	}
	return &cloned, nil
}

// CloneGuardMatchConfig returns a detached guard match definition.
func CloneGuardMatchConfig(config *GuardMatchConfig) *GuardMatchConfig {
	if config == nil {
		return nil
	}
	cloned := *config
	return &cloned
}

func cloneValue[T any](value T) T {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var clone T
	if err := json.Unmarshal(data, &clone); err != nil {
		return value
	}
	return clone
}

func CloneWorkstationConfig(def FactoryWorkstationConfig) FactoryWorkstationConfig {
	return cloneValue(def)
}

func CloneModelOperations(operations []ModelOperation) []ModelOperation {
	return cloneValue(operations)
}

func CloneIOConfigs(configs []IOConfig) []IOConfig {
	return cloneValue(configs)
}

func CloneModelOperationBindings(
	bindings []ModelOperationBinding,
) []ModelOperationBinding {
	return cloneValue(bindings)
}
