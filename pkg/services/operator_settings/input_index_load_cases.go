package operatorsettings

func loadInputCases() []InputCase {
	return []InputCase{
		{
			ID:             "valid-missing-file",
			Category:       categoryLoadFile,
			Entrypoint:     entrypointLoadFileConfig,
			Outcome:        outcomeAccept,
			Description:    "missing config file returns empty Config without error",
			ExpectedConfig: &ConfigExpectation{},
		},
		{
			ID:          "valid-load-defaults",
			Category:    categoryLoadFile,
			Entrypoint:  entrypointLoadFileConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/load-defaults.json",
			Description: "LoadFileConfig reads and validates defaults from disk",
			ExpectedConfig: &ConfigExpectation{
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
	}
}
