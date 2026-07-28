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
		Workers: operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{
			Integrations: append([]operatorsettings.ACPIntegration(nil), config.Workers.ACP.Integrations...),
		}},
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

func configFromDocument(document operatorsettings.Document) operatorsettings.Config {
	config := operatorsettings.Config{
		BackendScopeID: document.BackendScopeID,
		Defaults: operatorsettings.Defaults{
			WorkerModelProvider: document.Defaults.WorkerModelProvider,
			WorkerModel:         document.Defaults.WorkerModel,
		},
		Runtime: operatorsettings.RuntimeSettings{
			Logging: operatorsettings.RuntimeArtifactSettings(document.Runtime.Logging),
			Metrics: operatorsettings.RuntimeArtifactSettings(document.Runtime.Metrics),
		},
		Workers: operatorsettings.WorkerSettings{ACP: operatorsettings.ACPSettings{
			Integrations: append([]operatorsettings.ACPIntegration(nil), document.Workers.ACP.Integrations...),
		}},
	}
	if document.WorkerPresets != nil {
		config.WorkerPresets = make([]operatorsettings.WorkerPreset, len(document.WorkerPresets))
		for i, preset := range document.WorkerPresets {
			config.WorkerPresets[i] = operatorsettings.WorkerPreset{
				ID:              preset.ID,
				ModelProvider:   preset.ModelProvider,
				Model:           preset.Model,
				ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return config
}
