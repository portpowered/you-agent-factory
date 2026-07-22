package mockworkers

// ProjectInputInventory builds the deterministic mock-worker input inventory
// from committed fixtures, docs examples, and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	cases := make([]InputCase, 0, 24)
	cases = append(cases, parseValidInputCases()...)
	cases = append(cases, parseInvalidInputCases()...)
	cases = append(cases, loadValidInputCases()...)

	return InputInventory{
		FormatVersion: InputInventoryFormatVersion,
		UnknownFieldPolicy: "ParseMockWorkersConfig uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, " +
			"unknown nested keys, and trailing JSON values",
		LoaderEntrypoints: []string{
			entrypointParseMockWorkersConfig,
			entrypointLoadMockWorkersConfig,
		},
		Cases: cases,
	}
}

func parseValidInputCases() []InputCase {
	cases := make([]InputCase, 0, 9)
	cases = append(cases, parseValidFixtureInputCases()...)
	cases = append(cases, parseValidDocsExampleInputCases()...)
	return cases
}

func parseValidFixtureInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "valid-empty-default",
			Category:    categoryParseEmptyDefault,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/valid/empty-accept.json",
			Description: "empty mockWorkers array is the default accept config when unmatchedDispatchPolicy is omitted",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 0,
			},
		},
		{
			ID:          "valid-accept-entry-selectors",
			Category:    categoryParseAcceptEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/valid/accept-entry-selectors.json",
			Description: "accept runType preserves selector-bearing workInputs entries",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:              "accept-reviewer",
					WorkerName:      "reviewer",
					WorkstationName: "review",
					RunType:         string(MockWorkerRunTypeAccept),
				}},
			},
		},
		{
			ID:          "valid-reject-without-reject-config",
			Category:    categoryParseRejectEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/valid/reject-without-reject-config.json",
			Description: "reject runType accepts omitted rejectConfig",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:      "reject-minimal",
					RunType: string(MockWorkerRunTypeReject),
				}},
			},
		},
		{
			ID:          "valid-script-minimal-command",
			Category:    categoryParseScriptEntry,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/valid/script-minimal-command.json",
			Description: "script runType requires only scriptConfig.command; optional script fields may be omitted",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:            "script-minimal",
					RunType:       string(MockWorkerRunTypeScript),
					ScriptCommand: "echo",
				}},
			},
		},
		{
			ID:          "valid-unmatched-policy-explicit-accept",
			Category:    categoryParseUnmatchedPolicy,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/valid/unmatched-policy-explicit-accept.json",
			Description: "explicit unmatchedDispatchPolicy accept matches omitted-policy default behavior",
			ExpectedConfig: &MockWorkersConfigExpectation{
				UnmatchedDispatchPolicy: string(MockWorkerUnmatchedDispatchPolicyAccept),
				MockWorkerCount:         0,
			},
		},
	}
}

func parseValidDocsExampleInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "docs-example-mock-workers",
			Category:    categoryParseDocsExample,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "docs/examples/mock-workers.json",
			Description: "checked-in reject-with-rejectConfig docs example parses via production loader",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:              "reviewer-rejects-first-pass",
					WorkerName:      "reviewer",
					WorkstationName: "review-story",
					RunType:         string(MockWorkerRunTypeReject),
					RejectExitCode:  intPtr(42),
				}},
			},
		},
		{
			ID:          "docs-example-mock-workers-script",
			Category:    categoryParseDocsExample,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "docs/examples/mock-workers-script.json",
			Description: "checked-in script docs example parses with required command and optional script fields",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:              "executor-script-side-effect",
					WorkerName:      "executor",
					WorkstationName: "execute-story",
					RunType:         string(MockWorkerRunTypeScript),
					ScriptCommand:   "printf",
				}},
			},
		},
		{
			ID:          "docs-example-mock-workers-mixed",
			Category:    categoryParseDocsExample,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "docs/examples/mock-workers-mixed.json",
			Description: "checked-in mixed docs example parses with unmatchedDispatchPolicy passthrough",
			ExpectedConfig: &MockWorkersConfigExpectation{
				UnmatchedDispatchPolicy: string(MockWorkerUnmatchedDispatchPolicyPassthrough),
				MockWorkerCount:         1,
				MockWorkers: []MockWorkerExpectation{{
					ID:              "reviewer-rejects-first-pass",
					WorkerName:      "reviewer",
					WorkstationName: "review-story",
					RunType:         string(MockWorkerRunTypeReject),
					RejectExitCode:  intPtr(42),
				}},
			},
		},
	}
}

func parseInvalidInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "invalid-unknown-top-level",
			Category:    categoryParseUnknownField,
			Entrypoint:  entrypointParseMockWorkersConfig,
			Outcome:     outcomeReject,
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/unknown-top-level.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/unknown-nested-mock-worker.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/trailing-json.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/unknown-run-type.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/unknown-unmatched-policy.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/script-without-script-config.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/script-without-command.json",
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
			Fixture:     "pkg/services/workers/interface/testdata/fixtures/invalid/reject-exit-code-out-of-range.json",
			Description: "rejectConfig.exitCode outside 1-255 fails validation",
			ErrorFragments: []string{
				"rejectConfig.exitCode must be between 1 and 255",
			},
		},
	}
}

func loadValidInputCases() []InputCase {
	return []InputCase{
		{
			ID:          "valid-load-empty-path",
			Category:    categoryLoadEmptyPath,
			Entrypoint:  entrypointLoadMockWorkersConfig,
			Outcome:     outcomeAccept,
			Description: "LoadMockWorkersConfig with empty path returns the default empty accept config",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 0,
			},
		},
		{
			ID:          "load-docs-example-mock-workers",
			Category:    categoryLoadFile,
			Entrypoint:  entrypointLoadMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "docs/examples/mock-workers.json",
			Description: "LoadMockWorkersConfig loads the checked-in reject docs example from disk",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:      "reviewer-rejects-first-pass",
					RunType: string(MockWorkerRunTypeReject),
				}},
			},
		},
		{
			ID:          "load-docs-example-mock-workers-script",
			Category:    categoryLoadFile,
			Entrypoint:  entrypointLoadMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "docs/examples/mock-workers-script.json",
			Description: "LoadMockWorkersConfig loads the checked-in script docs example from disk",
			ExpectedConfig: &MockWorkersConfigExpectation{
				MockWorkerCount: 1,
				MockWorkers: []MockWorkerExpectation{{
					ID:            "executor-script-side-effect",
					RunType:       string(MockWorkerRunTypeScript),
					ScriptCommand: "printf",
				}},
			},
		},
		{
			ID:          "load-docs-example-mock-workers-mixed",
			Category:    categoryLoadFile,
			Entrypoint:  entrypointLoadMockWorkersConfig,
			Outcome:     outcomeAccept,
			Fixture:     "docs/examples/mock-workers-mixed.json",
			Description: "LoadMockWorkersConfig loads the checked-in passthrough-policy docs example from disk",
			ExpectedConfig: &MockWorkersConfigExpectation{
				UnmatchedDispatchPolicy: string(MockWorkerUnmatchedDispatchPolicyPassthrough),
				MockWorkerCount:         1,
			},
		},
	}
}

func intPtr(value int) *int {
	return &value
}
