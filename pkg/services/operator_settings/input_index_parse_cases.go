package operatorsettings

func parseValidInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "valid-defaults-only",
			Category:    categoryParseDefaults,
			Entrypoint:  entrypointParseFileConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/defaults-only.json",
			Description: "defaults.workerModelProvider and defaults.workerModel parse from file",
			ExpectedFileConfig: &FileConfigExpectation{
				Defaults: DefaultsSnapshot{
					WorkerModelProvider: "codex",
					WorkerModel:         "gpt-5-codex",
				},
			},
		},
		{
			ID:          "valid-backend-scope-sibling",
			Category:    categoryParseDefaults,
			Entrypoint:  entrypointParseFileConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/backend-scope-sibling.json",
			Description: "operatorconfig tolerates sibling backendScopeID without returning it on FileConfig",
			ExpectedFileConfig: &FileConfigExpectation{
				Defaults: DefaultsSnapshot{
					WorkerModelProvider: "claude",
					WorkerModel:         "claude-sonnet",
				},
			},
		},
		{
			ID:          "valid-worker-presets-canonicalized",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointParseFileConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/worker-presets-canonicalized.json",
			Description: "workerPresets ids, providers, models, and reasoningEffort are trimmed and canonicalized",
			ExpectedFileConfig: &FileConfigExpectation{
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
			Entrypoint:  entrypointParseFileConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/worker-presets-missing.json",
			Description: "missing workerPresets array is backward compatible",
			ExpectedFileConfig: &FileConfigExpectation{
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
			Entrypoint:  entrypointParseFileConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/malformed-json.json",
			Description: "malformed JSON fails parse",
			ErrorFragments: []string{
				"decode operator config JSON",
			},
		},
		{
			ID:          "invalid-unknown-top-level",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
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
			Entrypoint:  entrypointParseFileConfig,
			Outcome:     outcomeReject,
			Fixture:     "invalid/preset-unsupported-provider.json",
			Description: "unsupported workerPresets[].modelProvider values are rejected",
			ErrorFragments: []string{
				`"build"`,
				`"other"`,
				"unsupported modelProvider",
			},
		},
		{
			ID:          "invalid-preset-unsupported-reasoning",
			Category:    categoryParseWorkerPreset,
			Entrypoint:  entrypointParseFileConfig,
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
