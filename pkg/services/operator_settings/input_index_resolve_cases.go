package operatorsettings

func resolveInputCases() []InputCase {
	cases := make([]InputCase, 0, 10)
	cases = append(cases, resolvePrecedencePrimaryInputCases()...)
	cases = append(cases, resolvePrecedenceSecondaryInputCases()...)
	cases = append(cases, resolveSymbolicInputCases()...)
	return cases
}

func resolvePrecedencePrimaryInputCases() []InputCase {
	return []InputCase{
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
	}
}

func resolvePrecedenceSecondaryInputCases() []InputCase {
	return []InputCase{
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
			Description: "malformed provider values fail resolve with provider identity syntax",
			ResolveLayers: &ResolveLayers{
				FileDefaults: DefaultsSnapshot{WorkerModelProvider: "Not_A_Provider"},
			},
			ErrorFragments: []string{
				"unsupported worker model provider",
				"canonical lowercase provider identity",
			},
		},
	}
}

func resolveSymbolicInputCases() []InputCase {
	return []InputCase{
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
	}
}
