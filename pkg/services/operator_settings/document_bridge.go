package operatorsettings

func documentFromConfigDocument(document ConfigDocument) Document {
	return documentFromConfig(document.FileConfig())
}

func configDocumentFromDocument(document Document) ConfigDocument {
	return ConfigDocument{config: configFromDocument(document)}
}

func documentFromConfig(config Config) Document {
	document := Document{
		BackendScopeID: config.BackendScopeID,
		Defaults: DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		Runtime: DocumentRuntimeSettings{
			Logging: DocumentRuntimeArtifactSettings(config.Runtime.Logging),
			Metrics: DocumentRuntimeArtifactSettings(config.Runtime.Metrics),
		},
	}
	if config.WorkerPresets != nil {
		document.WorkerPresets = make([]DocumentWorkerPreset, len(config.WorkerPresets))
		for i, preset := range config.WorkerPresets {
			document.WorkerPresets[i] = DocumentWorkerPreset{
				ID:              preset.ID,
				ModelProvider:   preset.ModelProvider,
				Model:           preset.Model,
				ReasoningEffort: preset.ReasoningEffort,
			}
		}
	}
	return document
}

func configFromDocument(document Document) Config {
	config := Config{
		BackendScopeID: document.BackendScopeID,
		Defaults: Defaults{
			WorkerModelProvider: document.Defaults.WorkerModelProvider,
			WorkerModel:         document.Defaults.WorkerModel,
		},
		Runtime: RuntimeSettings{
			Logging: RuntimeArtifactSettings(document.Runtime.Logging),
			Metrics: RuntimeArtifactSettings(document.Runtime.Metrics),
		},
	}
	if document.WorkerPresets != nil {
		config.WorkerPresets = make([]WorkerPreset, len(document.WorkerPresets))
		for i, preset := range document.WorkerPresets {
			config.WorkerPresets[i] = WorkerPreset{
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
	update ProviderModelUpdate,
) DocumentProviderModelUpdate {
	return DocumentProviderModelUpdate{
		Provider: update.Provider,
		Model:    update.Model,
	}
}
