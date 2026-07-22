package identityinventory

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func ensureScopeInputCases() []InputCase {
	cases := make([]InputCase, 0, 9)
	cases = append(cases, ensureScopeAcceptInputCases()...)
	cases = append(cases, ensureScopeRejectInputCases()...)
	return cases
}

func ensureScopeAcceptInputCases() []InputCase {
	cases := make([]InputCase, 0, 7)
	cases = append(cases, ensureScopeGeneratedInputCases()...)
	cases = append(cases, ensureScopeReuseAndSiblingInputCases()...)
	return cases
}

func ensureScopeGeneratedInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "valid-missing-file",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Description: "missing config file generates local-<uuid> and persists it before returning",
			ExpectedScope: &ScopeExpectation{
				Outcome:          operatorsettings.BackendScopeOutcomeGenerated,
				RequireLocalUUID: true,
			},
			PersistedFileExpectation: &PersistedFileExpectation{
				BackendScopeIDMatchesResolved: true,
			},
		},
		{
			ID:          "valid-missing-scope",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Fixture:     "valid/missing-scope.json",
			Description: "empty config object without backendScopeID generates and persists local-<uuid>",
			ExpectedScope: &ScopeExpectation{
				Outcome:          operatorsettings.BackendScopeOutcomeGenerated,
				RequireLocalUUID: true,
			},
			PersistedFileExpectation: &PersistedFileExpectation{
				BackendScopeIDMatchesResolved: true,
			},
		},
		{
			ID:          "valid-empty-scope-field",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Fixture:     "valid/empty-scope-field.json",
			Description: "whitespace-only backendScopeID value is treated as missing and regenerated",
			ExpectedScope: &ScopeExpectation{
				Outcome:          operatorsettings.BackendScopeOutcomeGenerated,
				RequireLocalUUID: true,
			},
			PersistedFileExpectation: &PersistedFileExpectation{
				BackendScopeIDMatchesResolved: true,
			},
		},
		{
			ID:          "valid-whitespace-config",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Fixture:     "valid/whitespace-only.json",
			Description: "whitespace-only config file content is treated as missing scope and regenerated",
			ExpectedScope: &ScopeExpectation{
				Outcome:          operatorsettings.BackendScopeOutcomeGenerated,
				RequireLocalUUID: true,
			},
			PersistedFileExpectation: &PersistedFileExpectation{
				BackendScopeIDMatchesResolved: true,
			},
		},
	}
}

func ensureScopeReuseAndSiblingInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "valid-reuse-persisted-scope",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Fixture:     "valid/existing-scope.json",
			Description: "existing local backendScopeID is reused without rewrite",
			ExpectedScope: &ScopeExpectation{
				BackendScopeID: "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee",
				Outcome:        operatorsettings.BackendScopeOutcomeReused,
			},
		},
		{
			ID:          "valid-preserves-defaults-siblings",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Fixture:     "valid/defaults-sibling.json",
			Description: "generating backendScopeID preserves unrelated defaults sibling keys on persist",
			ExpectedScope: &ScopeExpectation{
				Outcome:          operatorsettings.BackendScopeOutcomeGenerated,
				RequireLocalUUID: true,
			},
			PersistedFileExpectation: &PersistedFileExpectation{
				BackendScopeIDMatchesResolved: true,
				PreservesDefaults:             true,
			},
		},
		{
			ID:          "valid-tolerates-unknown-sibling",
			Category:    categoryTolerantSibling,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeAccept,
			Fixture:     "valid/unknown-sibling.json",
			Description: "tolerant load ignores unknown top-level siblings on read and preserves them on persist",
			ExpectedScope: &ScopeExpectation{
				Outcome:          operatorsettings.BackendScopeOutcomeGenerated,
				RequireLocalUUID: true,
			},
			PersistedFileExpectation: &PersistedFileExpectation{
				BackendScopeIDMatchesResolved: true,
				PreservesDefaults:             true,
				PreservesSiblingKeys:          []string{"unknownTopLevel"},
			},
		},
	}
}

func ensureScopeRejectInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "invalid-empty-config-path",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeReject,
			Description: "empty config path is rejected before file access",
			ErrorFragments: []string{
				"system config path is required",
			},
		},
		{
			ID:          "invalid-malformed-json",
			Category:    categoryEnsureScope,
			Entrypoint:  entrypointEnsureLocalBackendScope,
			Outcome:     outcomeReject,
			Fixture:     "invalid/malformed-json.json",
			Description: "malformed JSON fails parse during load",
			ErrorFragments: []string{
				"parse system config",
			},
		},
	}
}

func persistScopeInputCases() []InputCase {
	return []InputCase{
		{
			ID:             "invalid-persist-empty-scope",
			Category:       categoryPersistScope,
			Entrypoint:     entrypointPersistBackendScopeID,
			Outcome:        outcomeReject,
			Description:    "persistBackendScopeID rejects empty backend scope IDs",
			PersistScopeID: "",
			ErrorFragments: []string{
				"backend scope ID is required",
			},
		},
		{
			ID:             "invalid-persist-non-local-scope",
			Category:       categoryPersistScope,
			Entrypoint:     entrypointPersistBackendScopeID,
			Outcome:        outcomeReject,
			Description:    "persistBackendScopeID rejects non-local backend scope IDs",
			PersistScopeID: "not-a-local-scope",
			ErrorFragments: []string{
				"not a valid local backend scope",
			},
		},
		{
			ID:             "invalid-persist-provider-scope",
			Category:       categoryPersistScope,
			Entrypoint:     entrypointPersistBackendScopeID,
			Outcome:        outcomeReject,
			Description:    "provider-derived scope IDs cannot be persisted as local backend scopes",
			PersistScopeID: "provider-codex-account-workspace",
			ErrorFragments: []string{
				"not a valid local backend scope",
			},
		},
	}
}
