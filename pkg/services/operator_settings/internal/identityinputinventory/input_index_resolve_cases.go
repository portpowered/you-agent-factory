package identityinputinventory

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func resolveInputCases() []operatorsettings.InputCase {
	cases := make([]operatorsettings.InputCase, 0, 10)
	cases = append(cases, resolvePrecedencePrimaryInputCases()...)
	cases = append(cases, resolvePrecedenceSecondaryInputCases()...)
	cases = append(cases, resolveSymbolicInputCases()...)
	return cases
}

func resolvePrecedencePrimaryInputCases() []operatorsettings.InputCase {
	return []operatorsettings.InputCase{
		{
			ID:          "resolve-flag-wins-both",
			Category:    categoryResolvePrecedence,
			Entrypoint:  entrypointResolve,
			Outcome:     outcomeAccept,
			Description: "flag layer wins independently for provider and model when both are set",
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{
					WorkerModelProvider: "claude",
					WorkerModel:         "file-model",
				},
				Env: map[string]string{
					operatorsettings.EnvDefaultWorkerModelProvider: "codex",
					operatorsettings.EnvDefaultWorkerModel:         "env-model",
				},
				Flag: operatorsettings.FlagSnapshot{
					WorkerModelProvider: "gemini",
					WorkerModel:         "flag-model",
				},
			},
			PrecedenceWinners: &operatorsettings.PrecedenceWinners{
				WorkerModelProviderSource: string(operatorsettings.SourceFlag),
				WorkerModelSource:         string(operatorsettings.SourceFlag),
			},
			ExpectedResolved: &operatorsettings.ResolvedExpectation{
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
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{
					WorkerModelProvider: "claude",
					WorkerModel:         "file-model",
				},
				Env: map[string]string{
					operatorsettings.EnvDefaultWorkerModelProvider: "codex",
					operatorsettings.EnvDefaultWorkerModel:         "env-model",
				},
			},
			PrecedenceWinners: &operatorsettings.PrecedenceWinners{
				WorkerModelProviderSource: string(operatorsettings.SourceEnv),
				WorkerModelSource:         string(operatorsettings.SourceEnv),
			},
			ExpectedResolved: &operatorsettings.ResolvedExpectation{
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
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{
					WorkerModelProvider: "claude",
					WorkerModel:         "file-model",
				},
				Env: map[string]string{
					operatorsettings.EnvDefaultWorkerModelProvider: "codex",
					operatorsettings.EnvDefaultWorkerModel:         "env-model",
				},
				Flag: operatorsettings.FlagSnapshot{WorkerModelProvider: "gemini"},
			},
			PrecedenceWinners: &operatorsettings.PrecedenceWinners{
				WorkerModelProviderSource: string(operatorsettings.SourceFlag),
				WorkerModelSource:         string(operatorsettings.SourceEnv),
			},
			ExpectedResolved: &operatorsettings.ResolvedExpectation{
				WorkerModelProvider: "GEMINI",
				WorkerModel:         "env-model",
			},
		},
	}
}

func resolvePrecedenceSecondaryInputCases() []operatorsettings.InputCase {
	return []operatorsettings.InputCase{
		{
			ID:          "resolve-file-only",
			Category:    categoryResolvePrecedence,
			Entrypoint:  entrypointResolve,
			Outcome:     outcomeAccept,
			Description: "file layer supplies both fields when env and flag are unset",
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{
					WorkerModelProvider: "codex",
					WorkerModel:         "file-model",
				},
			},
			PrecedenceWinners: &operatorsettings.PrecedenceWinners{
				WorkerModelProviderSource: string(operatorsettings.SourceFile),
				WorkerModelSource:         string(operatorsettings.SourceFile),
			},
			ExpectedResolved: &operatorsettings.ResolvedExpectation{
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
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{WorkerModelProvider: "kiro-cli"},
			},
			PrecedenceWinners: &operatorsettings.PrecedenceWinners{
				WorkerModelProviderSource: string(operatorsettings.SourceFile),
			},
			ExpectedResolved: &operatorsettings.ResolvedExpectation{
				WorkerModelProvider: "KIRO",
			},
		},
		{
			ID:          "resolve-unsupported-provider",
			Category:    categoryResolvePrecedence,
			Entrypoint:  entrypointResolve,
			Outcome:     outcomeReject,
			Description: "malformed provider values fail resolve with provider identity syntax",
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{WorkerModelProvider: "Not_A_Provider"},
			},
			ErrorFragments: []string{
				"unsupported worker model provider",
				"canonical lowercase provider identity",
			},
		},
	}
}

func resolveSymbolicInputCases() []operatorsettings.InputCase {
	return []operatorsettings.InputCase{
		{
			ID:          "resolve-symbolic-default-from-file",
			Category:    categoryResolveSymbolic,
			Entrypoint:  entrypointResolve,
			Outcome:     outcomeAccept,
			Description: "symbolic DEFAULT provider resolves through lower-precedence concrete provider",
			ResolveLayers: &operatorsettings.ResolveLayers{
				FileDefaults: operatorsettings.DefaultsSnapshot{WorkerModelProvider: "codex"},
				Flag:         operatorsettings.FlagSnapshot{WorkerModelProvider: "DEFAULT"},
			},
			PrecedenceWinners: &operatorsettings.PrecedenceWinners{
				WorkerModelProviderSource: string(operatorsettings.SourceFlag),
			},
			ExpectedResolved: &operatorsettings.ResolvedExpectation{
				WorkerModelProvider: "CODEX",
			},
		},
		{
			ID:          "resolve-symbolic-default-unresolved",
			Category:    categoryResolveSymbolic,
			Entrypoint:  entrypointResolve,
			Outcome:     outcomeReject,
			Description: "symbolic DEFAULT without a concrete lower-precedence provider fails resolve",
			ResolveLayers: &operatorsettings.ResolveLayers{
				Flag: operatorsettings.FlagSnapshot{WorkerModelProvider: "DEFAULT"},
			},
			ErrorFragments: []string{
				"concrete provider",
			},
		},
	}
}
