package settingsdocument

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

func documentFromConfigDocument(document operatorsettings.ConfigDocument) operatorsettings.Document {
	return documentFromConfig(document.FileConfig())
}

func configDocumentFromDocument(document operatorsettings.Document) operatorsettings.ConfigDocument {
	return operatorsettings.ConfigDocumentFromConfig(configFromDocument(document))
}

func documentFromConfig(config operatorsettings.Config) operatorsettings.Document {
	document := operatorsettings.Document{
		BackendScopeID: config.BackendScopeID,
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		Runtime: operatorsettings.DocumentRuntimeSettings{
			Logging: operatorsettings.DocumentRuntimeArtifactSettings(config.Runtime.Logging),
			Metrics: operatorsettings.DocumentRuntimeArtifactSettings(config.Runtime.Metrics),
		},
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

func documentProviderModelUpdateFromProviderModelUpdate(
	update operatorsettings.ProviderModelUpdate,
) operatorsettings.DocumentProviderModelUpdate {
	return operatorsettings.DocumentProviderModelUpdate{
		Provider: update.Provider,
		Model:    update.Model,
	}
}
