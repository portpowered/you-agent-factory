package cli

import (
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func presentationScopeRequestFromInvoke(cfg InvokeConfig) PresentationScopeRequest {
	return PresentationScopeRequest{
		FactoryDir:       cfg.FactoryDir,
		HomeDir:          cfg.HomeDir,
		OperatorDefaults: presentationOperatorDefaultsFromResolved(cfg.OperatorDefaults),
		Logger:           cfg.Logger,
		Verbose:          cfg.Verbose,
	}
}

func presentationOperatorDefaultsFromResolved(
	defaults operatorconfig.ResolvedDefaults,
) modelinference.PresentationOperatorDefaults {
	return modelinference.PresentationOperatorDefaults{
		WorkerModelProvider: defaults.WorkerModelProvider,
		WorkerModel:         defaults.WorkerModel,
	}
}
