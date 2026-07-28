package identityinputinventory

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func loadInputCases() []operatorsettings.InputCase {
	return []operatorsettings.InputCase{
		{
			ID:             "valid-missing-file",
			Category:       categoryLoadFile,
			Entrypoint:     entrypointLoadFileConfig,
			Outcome:        outcomeAccept,
			Description:    "missing config file returns empty Config without error",
			ExpectedConfig: &operatorsettings.ConfigExpectation{},
		},
		{
			ID:          "valid-load-defaults",
			Category:    categoryLoadFile,
			Entrypoint:  entrypointLoadFileConfig,
			Outcome:     outcomeAccept,
			Fixture:     "valid/load-defaults.json",
			Description: "LoadFileConfig reads and validates defaults from disk",
			ExpectedConfig: &operatorsettings.ConfigExpectation{
				Defaults: operatorsettings.DefaultsSnapshot{
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
