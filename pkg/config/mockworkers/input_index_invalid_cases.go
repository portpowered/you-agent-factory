package mockworkers

func parseInvalidInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "invalid-unknown-top-level",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/unknown-top-level.json",
			Description: "unknown top-level keys are rejected under strict decode",
			ErrorFragments: []string{
				"decode mock workers JSON",
				"unknown field",
			},
		},
		{
			ID:          "invalid-unknown-nested-mock-worker",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/unknown-nested-mock-worker.json",
			Description: "unknown nested mockWorkers[] keys are rejected under strict decode",
			ErrorFragments: []string{
				"decode mock workers JSON",
				"unknown field",
			},
		},
		{
			ID:          "invalid-trailing-json",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/trailing-json.json",
			Description: "trailing JSON values after the root object are rejected",
			ErrorFragments: []string{
				"unexpected trailing JSON",
			},
		},
		{
			ID:          "invalid-unknown-run-type",
			Category:    categoryParseAcceptEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/unknown-run-type.json",
			Description: "unknown runType values fail validation with actionable diagnostics",
			ErrorFragments: []string{
				`runType must be one of "accept", "script", or "reject"; got "maybe"`,
			},
		},
		{
			ID:          "invalid-unknown-unmatched-policy",
			Category:    categoryParseUnmatchedPolicy,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/unknown-unmatched-policy.json",
			Description: "unknown unmatchedDispatchPolicy values fail validation with actionable diagnostics",
			ErrorFragments: []string{
				`unmatchedDispatchPolicy must be one of "accept" or "passthrough"; got "maybe"`,
			},
		},
		{
			ID:          "invalid-script-without-script-config",
			Category:    categoryParseScriptEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/script-without-script-config.json",
			Description: "script runType without scriptConfig fails validation",
			ErrorFragments: []string{
				"scriptConfig is required",
			},
		},
		{
			ID:          "invalid-script-without-command",
			Category:    categoryParseScriptEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/script-without-command.json",
			Description: "script runType without scriptConfig.command fails validation",
			ErrorFragments: []string{
				"scriptConfig.command is required",
			},
		},
		{
			ID:          "invalid-reject-exit-code-out-of-range",
			Category:    categoryParseRejectEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/invalid/reject-exit-code-out-of-range.json",
			Description: "rejectConfig.exitCode outside 1-255 fails validation",
			ErrorFragments: []string{
				"rejectConfig.exitCode must be between 1 and 255",
			},
		},
	}
}
