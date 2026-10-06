package factorycontracts

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// CloneFactoryConfig returns a detached copy of a canonical Factory
// definition.
func CloneFactoryConfig(cfg *FactoryConfig) (*FactoryConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	// Worker cloning is already typed; immutable portable file and workstation
	// prompt text stays out of the remaining JSON representation copy.
	serialized := *cfg
	serialized.Workers = nil
	serialized.ResourceManifest = nil
	if cfg.Workstations != nil {
		serialized.Workstations = make([]FactoryWorkstationConfig, len(cfg.Workstations))
		for index, workstation := range cfg.Workstations {
			serialized.Workstations[index] = workstationWithoutText(workstation)
		}
	}
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
			restoreWorkstationText(&cloned.Workstations[index], cfg.Workstations[index], serialized.Workstations[index])
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
	serialized := workstationWithoutText(def)
	cloned := cloneValue(serialized)
	restoreWorkstationText(&cloned, def, serialized)
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

// Valid UTF-8 prompt strings are immutable. Invalid text stays in the JSON
// copy so the existing replacement-character normalization is preserved.
func workstationWithoutText(def FactoryWorkstationConfig) FactoryWorkstationConfig {
	if utf8.ValidString(def.Body) {
		def.Body = ""
	}
	if utf8.ValidString(def.PromptTemplate) {
		def.PromptTemplate = ""
	}
	return def
}

func restoreWorkstationText(cloned *FactoryWorkstationConfig, source, serialized FactoryWorkstationConfig) {
	if serialized.Body == "" {
		cloned.Body = source.Body
	}
	if serialized.PromptTemplate == "" {
		cloned.PromptTemplate = source.PromptTemplate
	}
}
