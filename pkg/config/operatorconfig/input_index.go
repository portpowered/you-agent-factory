package operatorconfig

// ProjectInputInventory builds the deterministic operator-config input inventory
// from committed fixtures and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	return InputInventory{
		FormatVersion: InputInventoryFormatVersion,
		UnknownFieldPolicy: "ParseFileConfig uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, " +
			"unknown nested keys, and trailing JSON values",
		PrecedenceChain: PrecedenceChain,
		Cases: []InputCase{
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
			{
				ID:          "valid-missing-file",
				Category:    categoryLoadFile,
				Entrypoint:  entrypointLoadFileConfig,
				Outcome:     outcomeAccept,
				Description: "missing config file returns empty FileConfig without error",
				ExpectedFileConfig: &FileConfigExpectation{},
			},
			{
				ID:          "valid-load-defaults",
				Category:    categoryLoadFile,
				Entrypoint:  entrypointLoadFileConfig,
				Outcome:     outcomeAccept,
				Fixture:     "valid/load-defaults.json",
				Description: "LoadFileConfig reads and validates defaults from disk",
				ExpectedFileConfig: &FileConfigExpectation{
					Defaults: DefaultsSnapshot{
						WorkerModelProvider: "claude",
						WorkerModel:         "claude-sonnet",
					},
				},
			},
			{
				ID:          "invalid-load-malformed",
				Category:    categoryLoadFile,
				Entrypoint:  entrypointLoadFileConfig,
				Outcome:     outcomeReject,
				Fixture:     "invalid/load-malformed.json",
				Description: "malformed on-disk config fails with path in error",
				ErrorFragments: []string{
					"parse operator config",
				},
			},
			{
				ID:          "resolve-flag-wins-both",
				Category:    categoryResolvePrecedence,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeAccept,
				Description: "flag layer wins independently for provider and model when both are set",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{
						WorkerModelProvider: "claude",
						WorkerModel:         "file-model",
					},
					Env: map[string]string{
						EnvDefaultWorkerModelProvider: "codex",
						EnvDefaultWorkerModel:         "env-model",
					},
					Flag: FlagSnapshot{
						WorkerModelProvider: "gemini",
						WorkerModel:         "flag-model",
					},
				},
				PrecedenceWinners: &PrecedenceWinners{
					WorkerModelProviderSource: string(SourceFlag),
					WorkerModelSource:         string(SourceFlag),
				},
				ExpectedResolved: &ResolvedExpectation{
					WorkerModelProvider: "GEMINI",
					WorkerModel:         "flag-model",
				},
			},
			{
				ID:          "resolve-env-wins-both",
				Category:    categoryResolvePrecedence,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeAccept,
				Description: "env layer wins for both fields when flags are unset",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{
						WorkerModelProvider: "claude",
						WorkerModel:         "file-model",
					},
					Env: map[string]string{
						EnvDefaultWorkerModelProvider: "codex",
						EnvDefaultWorkerModel:         "env-model",
					},
				},
				PrecedenceWinners: &PrecedenceWinners{
					WorkerModelProviderSource: string(SourceEnv),
					WorkerModelSource:         string(SourceEnv),
				},
				ExpectedResolved: &ResolvedExpectation{
					WorkerModelProvider: "CODEX",
					WorkerModel:         "env-model",
				},
			},
			{
				ID:          "resolve-mixed-independent",
				Category:    categoryResolvePrecedence,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeAccept,
				Description: "provider and model precedence are independent per field",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{
						WorkerModelProvider: "claude",
						WorkerModel:         "file-model",
					},
					Env: map[string]string{
						EnvDefaultWorkerModelProvider: "codex",
						EnvDefaultWorkerModel:         "env-model",
					},
					Flag: FlagSnapshot{WorkerModelProvider: "gemini"},
				},
				PrecedenceWinners: &PrecedenceWinners{
					WorkerModelProviderSource: string(SourceFlag),
					WorkerModelSource:         string(SourceEnv),
				},
				ExpectedResolved: &ResolvedExpectation{
					WorkerModelProvider: "GEMINI",
					WorkerModel:         "env-model",
				},
			},
			{
				ID:          "resolve-file-only",
				Category:    categoryResolvePrecedence,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeAccept,
				Description: "file layer supplies both fields when env and flag are unset",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{
						WorkerModelProvider: "codex",
						WorkerModel:         "file-model",
					},
				},
				PrecedenceWinners: &PrecedenceWinners{
					WorkerModelProviderSource: string(SourceFile),
					WorkerModelSource:         string(SourceFile),
				},
				ExpectedResolved: &ResolvedExpectation{
					WorkerModelProvider: "CODEX",
					WorkerModel:         "file-model",
				},
			},
			{
				ID:          "resolve-symbolic-default-from-file",
				Category:    categoryResolveSymbolic,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeAccept,
				Description: "symbolic DEFAULT provider resolves through lower-precedence concrete provider",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{WorkerModelProvider: "codex"},
					Flag:         FlagSnapshot{WorkerModelProvider: "DEFAULT"},
				},
				PrecedenceWinners: &PrecedenceWinners{
					WorkerModelProviderSource: string(SourceFlag),
				},
				ExpectedResolved: &ResolvedExpectation{
					WorkerModelProvider: "CODEX",
				},
			},
			{
				ID:          "resolve-symbolic-default-unresolved",
				Category:    categoryResolveSymbolic,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeReject,
				Description: "symbolic DEFAULT without a concrete lower-precedence provider fails resolve",
				ResolveLayers: &ResolveLayers{
					Flag: FlagSnapshot{WorkerModelProvider: "DEFAULT"},
				},
				ErrorFragments: []string{
					"concrete provider",
				},
			},
			{
				ID:          "resolve-provider-alias",
				Category:    categoryResolvePrecedence,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeAccept,
				Description: "provider aliases are canonicalized during resolve",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{WorkerModelProvider: "kiro-cli"},
				},
				PrecedenceWinners: &PrecedenceWinners{
					WorkerModelProviderSource: string(SourceFile),
				},
				ExpectedResolved: &ResolvedExpectation{
					WorkerModelProvider: "KIRO",
				},
			},
			{
				ID:          "resolve-unsupported-provider",
				Category:    categoryResolvePrecedence,
				Entrypoint:  entrypointResolve,
				Outcome:     outcomeReject,
				Description: "unsupported provider values fail resolve with accepted provider summary",
				ResolveLayers: &ResolveLayers{
					FileDefaults: DefaultsSnapshot{WorkerModelProvider: "not-a-provider"},
				},
				ErrorFragments: []string{
					"unsupported worker model provider",
					"accepted canonical providers",
				},
			},
		},
	}
}
