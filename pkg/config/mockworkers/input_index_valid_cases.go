package mockworkers

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
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/valid/empty-accept.json",
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
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/valid/accept-entry-selectors.json",
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
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/valid/reject-without-reject-config.json",
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
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/valid/script-minimal-command.json",
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
			Fixture:     "pkg/config/mockworkers/testdata/fixtures/valid/unmatched-policy-explicit-accept.json",
			Description: "explicit unmatchedDispatchPolicy accept matches omitted-policy default behavior",
			ExpectedConfig: &MockWorkersConfigExpectation{
				UnmatchedDispatchPolicy: string(MockWorkerUnmatchedDispatchPolicyAccept),
				MockWorkerCount:         0,
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
