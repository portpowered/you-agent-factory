package service

import (
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func documentFromConfig(config operatorsettings.Config) operatorsettings.Document {
	document := operatorsettings.Document{
		BackendScopeID: strings.TrimSpace(config.BackendScopeID),
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		Runtime: documentRuntimeFromConfig(config.Runtime),
	}
	if config.WorkerPresets != nil {
		document.WorkerPresets = make([]operatorsettings.DocumentWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			document.WorkerPresets[i] = operatorsettings.DocumentWorkerPreset{
				ID:              preset.ID,
				ModelProvider:   preset.ModelProvider,
				Model:           preset.Model,
				ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return document
}

func documentRuntimeFromConfig(runtime operatorsettings.RuntimeSettings) operatorsettings.DocumentRuntimeSettings {
	return operatorsettings.DocumentRuntimeSettings{
		Logging: operatorsettings.DocumentRuntimeArtifactSettings(runtime.Logging),
		Metrics: operatorsettings.DocumentRuntimeArtifactSettings(runtime.Metrics),
	}
}
