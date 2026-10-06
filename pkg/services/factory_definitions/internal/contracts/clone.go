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
	// Worker cloning is already typed, and portable file bodies are immutable
	// strings. Keep those bytes out of the remaining JSON representation copy.
	serialized := *cfg
	serialized.Workers = nil
	serialized.ResourceManifest = nil
	data, err := json.Marshal(&serialized)
	if err != nil {
		return nil, fmt.Errorf("encode Factory definition clone: %w", err)
	}
	var cloned FactoryConfig
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, fmt.Errorf("decode Factory definition clone: %w", err)
	}
	cloned.SetIgnoredJSONPaths(cfg.IgnoredJSONPaths())
	if cfg.Workers != nil {
		cloned.Workers = make([]FactoryWorkerConfig, len(cfg.Workers))
		for index, worker := range cfg.Workers {
			cloned.Workers[index] = CloneWorkerConfig(worker)
			// Preserve the existing Factory clone's runtime metadata boundary.
			cloned.Workers[index].SessionID = ""
			cloned.Workers[index].Concurrency = 0
			cloned.Workers[index].RuntimeDefaultModelProvider = ""
			cloned.Workers[index].RuntimeDefaultModel = ""
		}
	}
	cloned.ResourceManifest = cloneFactoryResourceManifest(cfg.ResourceManifest)
	for index := range cloned.Workstations {
		if index < len(cfg.Workstations) {
			cloned.Workstations[index].PromptSourcePath = cfg.Workstations[index].PromptSourcePath
			cloned.Workstations[index].PromptSourceIsTemplate = cfg.Workstations[index].PromptSourceIsTemplate
		}
	}
	return &cloned, nil
}

func cloneFactoryResourceManifest(source *PortableResourceManifestConfig) *PortableResourceManifestConfig {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.BundledFiles = append([]BundledFileConfig(nil), source.BundledFiles...)
	cloned.RequiredTools = append([]RequiredToolConfig(nil), source.RequiredTools...)
	for index := range cloned.RequiredTools {
		cloned.RequiredTools[index].VersionArgs = append([]string(nil), source.RequiredTools[index].VersionArgs...)
	}
	return &cloned
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
	cloned := cloneValue(def)
	cloned.PromptSourcePath = def.PromptSourcePath
	cloned.PromptSourceIsTemplate = def.PromptSourceIsTemplate
	return cloned
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
