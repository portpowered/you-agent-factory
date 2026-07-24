package operatorsettings

func parseValidInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "valid-defaults-only",
			Category:    categoryParseDefaults,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/defaults-only.json",
			Description: "defaults.workerModelProvider and defaults.workerModel parse from file",
			ExpectedConfig: &ConfigExpectation{
				Defaults: DefaultsSnapshot{
					WorkerModelProvider: "codex",
					WorkerModel:         "gpt-5-codex",
				},
			},
		},
		{
			ID:          "valid-backend-scope-sibling",
			Category:    categoryParseDefaults,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/backend-scope-sibling.json",
			Description: "generated global config decode returns backendScopeID with normalized defaults",
			ExpectedConfig: &ConfigExpectation{
				BackendScopeID: "local-11111111-1111-4111-8111-111111111111",
				Defaults: DefaultsSnapshot{
					WorkerModelProvider: "claude",
					WorkerModel:         "claude-sonnet",
				},
			},
		},
		{
			ID:          "valid-worker-presets-canonicalized",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/worker-presets-canonicalized.json",
			Description: "workerPresets ids, providers, models, and reasoningEffort are trimmed and canonicalized",
			ExpectedConfig: &ConfigExpectation{
				WorkerPresets: []WorkerPreset{{
					ID:              "research",
					ModelProvider:   "CODEX",
					Model:           "gpt-5.4",
					ReasoningEffort: "high",
				}},
			},
		},
		{
			ID:          "valid-worker-presets-missing",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/worker-presets-missing.json",
			Description: "missing workerPresets array is backward compatible",
			ExpectedConfig: &ConfigExpectation{
				Defaults: DefaultsSnapshot{WorkerModel: "existing-model"},
			},
		},
	}
}

func parseInvalidDefaultsInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "invalid-malformed-json",
			Category:    categoryParseDefaults,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/malformed-json.json",
			Description: "malformed JSON fails parse",
			ErrorFragments: []string{
				"decode generated global config",
			},
		},
		{
			ID:          "invalid-unknown-top-level",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/unknown-top-level.json",
			Description: "unknown top-level keys are rejected under strict decode",
			ErrorFragments: []string{
				"unknown field",
			},
		},
		{
			ID:          "invalid-unknown-nested-defaults",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/unknown-nested-defaults.json",
			Description: "unknown nested defaults keys are rejected under strict decode",
			ErrorFragments: []string{
				"unknown field",
			},
		},
		{
			ID:          "invalid-trailing-json",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/trailing-json.json",
			Description: "trailing JSON values after the root object are rejected",
			ErrorFragments: []string{
				"unexpected trailing JSON",
			},
		},
	}
}

func parseInvalidWorkerPresetInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "invalid-preset-empty-id",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-empty-id.json",
			Description: "workerPresets[].id must be non-empty after trim",
			ErrorFragments: []string{
				`workerPresets[0].id`,
				"non-empty",
			},
		},
		{
			ID:          "invalid-preset-duplicate-id",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-duplicate-id.json",
			Description: "duplicate workerPresets[].id values are rejected",
			ErrorFragments: []string{
				`workerPresets[1].id`,
				"duplicated",
			},
		},
		{
			ID:          "invalid-preset-missing-provider",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-missing-provider.json",
			Description: "workerPresets[].modelProvider is required",
			ErrorFragments: []string{
				`workerPresets[0]`,
				"modelProvider",
			},
		},
		{
			ID:          "invalid-preset-symbolic-provider",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-symbolic-provider.json",
			Description: "symbolic DEFAULT provider is rejected in worker presets",
			ErrorFragments: []string{
				`"build"`,
				`"DEFAULT"`,
				"unsupported modelProvider",
			},
		},
		{
			ID:          "invalid-preset-unsupported-provider",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-unsupported-provider.json",
			Description: "malformed workerPresets[].modelProvider values are rejected",
			ErrorFragments: []string{
				`"build"`,
				`"Other_Provider"`,
				"unsupported modelProvider",
			},
		},
		{
			ID:          "invalid-preset-unsupported-reasoning",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointDecodeGlobalConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-unsupported-reasoning.json",
			Description: "unsupported workerPresets[].reasoningEffort values are rejected",
			ErrorFragments: []string{
				`"build"`,
				`"extreme"`,
				"unsupported reasoningEffort",
			},
		},
	}
}
