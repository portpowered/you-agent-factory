package cli

import (
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"strings"
)

func presentationScopeRequestFromInvoke(cfg InvokeConfig) PresentationScopeRequest {
	return PresentationScopeRequest{
		FactoryDir:       cfg.FactoryDir,
		WorkingDirectory: cfg.WorkingDirectory,
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

func genericCLIStringPointer(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	copy := value
	return &copy
}

func catalogPresentationForOperation(catalog modelinference.Detail, operation string) (string, string) {
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return capability.Worker, string(capability.ProviderLocality)
			}
		}
	}
	return "", string(catalog.ProviderLocality)
}

func resolvedPresentationBindings(
	catalog modelinference.Detail,
	operation string,
	inputText string,
) []modelinference.ResolvedModelOperationBinding {
	operationDetail, ok := catalogOperationForName(catalog, operation)
	if !ok {
		return []modelinference.ResolvedModelOperationBinding{}
	}
	for _, input := range operationDetail.Inputs {
		slot := strings.TrimSpace(input.Name)
		if slot == "" {
			continue
		}
		return []modelinference.ResolvedModelOperationBinding{{
			Slot:   slot,
			Source: "INPUT",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: inputText,
			}},
		}}
	}
	return []modelinference.ResolvedModelOperationBinding{}
}

func catalogOperationForName(catalog modelinference.Detail, operation string) (modelinference.Operation, bool) {
	for _, catalogOperation := range catalog.Operations {
		if catalogOperation.Name == operation {
			return catalogOperation, true
		}
	}
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return catalogOperation, true
			}
		}
	}
	return modelinference.Operation{}, false
}

func catalogCapabilityOperationForName(catalog modelinference.Detail, operation string) (modelinference.Operation, bool) {
	for _, capability := range catalog.Capabilities {
		for _, catalogOperation := range capability.Operations {
			if catalogOperation.Name == operation {
				return catalogOperation, true
			}
		}
	}
	return modelinference.Operation{}, false
}
