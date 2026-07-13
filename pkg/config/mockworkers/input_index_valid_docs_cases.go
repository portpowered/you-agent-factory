package mockworkers

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
