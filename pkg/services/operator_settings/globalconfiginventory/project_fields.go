package globalconfiginventory

import (
	operator_settings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func topologyFieldRecords() []FieldRecord {
	fields := make([]FieldRecord, 0, 9)
	fields = append(fields, topologyBackendScopeField())
	fields = append(fields, topologyDefaultsFields()...)
	fields = append(fields, topologyWorkerPresetFields()...)
	return fields
}

func topologyBackendScopeField() FieldRecord {
	return FieldRecord{
		ID:                   "backendScopeID",
		JSONPath:             "backendScopeID",
		JSONName:             "backendScopeID",
		ValueType:            "string",
		DefaultEmptyBehavior: "missing document or missing, empty, or whitespace-only value triggers EnsureLocalBackendScope to generate local-<uuid> and persist it; a present whitespace-only document is invalid",
		Strictness:           strictnessOperatorSettingsStrictDecode,
		PersistenceOwner:     ownerOperatorSettings,
		ParseOwner:           ownerOperatorSettings,
		Notes:                "operator_settings loads identity from the canonical generated GlobalConfig document and persists it by re-encoding modeled fields",
	}
}

func topologyDefaultsFields() []FieldRecord {
	return []FieldRecord{
		{
			ID:                   "defaults",
			JSONPath:             "defaults",
			JSONName:             "defaults",
			ValueType:            "object",
			DefaultEmptyBehavior: "missing defaults object yields empty operator defaults",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file", "env", "flag"},
			Notes:                "defaults are parsed before env/flag resolution and preserved when operator_settings persists a generated backend scope",
		},
		{
			ID:                   "defaults.workerModelProvider",
			JSONPath:             "defaults.workerModelProvider",
			JSONName:             "workerModelProvider",
			ValueType:            "string",
			ParentField:          "defaults",
			DefaultEmptyBehavior: "missing or whitespace-only value is unset until env/flag layers apply; symbolic DEFAULT resolves through lower-precedence concrete provider",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file", "env", "flag"},
			EnvironmentVariable:  operator_settings.EnvDefaultWorkerModelProvider,
			FlagName:             "--default-worker-model-provider",
			Notes:                "values are trimmed and canonicalized; unsupported providers fail resolve",
		},
		{
			ID:                   "defaults.workerModel",
			JSONPath:             "defaults.workerModel",
			JSONName:             "workerModel",
			ValueType:            "string",
			ParentField:          "defaults",
			DefaultEmptyBehavior: "missing or whitespace-only value is unset until env/flag layers apply",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file", "env", "flag"},
			EnvironmentVariable:  operator_settings.EnvDefaultWorkerModel,
			FlagName:             "--default-worker-model",
		},
	}
}

func topologyWorkerPresetFields() []FieldRecord {
	return []FieldRecord{
		{
			ID:                   "workerPresets",
			JSONPath:             "workerPresets",
			JSONName:             "workerPresets",
			ValueType:            "array",
			DefaultEmptyBehavior: "missing workerPresets array is backward compatible and yields an empty preset list",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file"},
			Notes:                strictnessFileOnly,
		},
		{
			ID:                   "workerPresets[].id",
			JSONPath:             "workerPresets[].id",
			JSONName:             "id",
			ValueType:            "string",
			ParentField:          "workerPresets[]",
			DefaultEmptyBehavior: "required non-empty after trim; duplicate ids are rejected",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file"},
		},
		{
			ID:                   "workerPresets[].modelProvider",
			JSONPath:             "workerPresets[].modelProvider",
			JSONName:             "modelProvider",
			ValueType:            "string",
			ParentField:          "workerPresets[]",
			DefaultEmptyBehavior: "required non-empty; canonicalized to supported providers; symbolic DEFAULT is rejected in presets",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file"},
		},
		{
			ID:                   "workerPresets[].model",
			JSONPath:             "workerPresets[].model",
			JSONName:             "model",
			ValueType:            "string",
			ParentField:          "workerPresets[]",
			DefaultEmptyBehavior: "optional; trimmed when present",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file"},
		},
		{
			ID:                   "workerPresets[].reasoningEffort",
			JSONPath:             "workerPresets[].reasoningEffort",
			JSONName:             "reasoningEffort",
			ValueType:            "string",
			ParentField:          "workerPresets[]",
			DefaultEmptyBehavior: "optional; empty value is accepted; supported values are minimal, low, medium, high",
			Strictness:           strictnessOperatorSettingsStrictDecode,
			PersistenceOwner:     ownerOperatorSettings,
			ParseOwner:           ownerOperatorSettings,
			PrecedenceLayers:     []string{"file"},
		},
	}
}
