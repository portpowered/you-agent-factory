package config

// LoadFromCanonicalJSON normalizes canonical factory JSON, expands it to a
// FactoryConfig, and runs the same blocking load validation used for inline
// factory directories. It does not require an on-disk factory directory; split
// worker/workstation AGENTS.md layouts still need LoadFromFactoryDir.
func LoadFromCanonicalJSON(payload []byte, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error) {
	factoryCfg, _, err := normalizeNamedFactoryPayload("factory", payload)
	if err != nil {
		return nil, err
	}
	if err := validatePortableBundledFilesForExpandOnPath("", factoryCfg); err != nil {
		return nil, err
	}

	inlineDefinitionsRequired := hasInlineRuntimeDefinitions(factoryCfg)
	runtimeDefs, err := loadRuntimeDefinitionLookupMapsFromFactoryConfig("", factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: inlineDefinitionsRequired,
		WorkstationLoader:       workstationLoader,
	})
	if err != nil {
		return nil, err
	}

	return NewLoadedFactoryConfig("", factoryCfg, runtimeDefs)
}
