package runtimebuild

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func applyOperatorDefaultsToLoadedConfig(
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	loaded factorydefinitions.MutableLoadedFactorySource,
) error {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil
	}
	defaultProvider, err := operatorDefaultProviderInternal(defaultWorkerModelProvider)
	if err != nil {
		return fmt.Errorf("apply operator defaults: %w", err)
	}
	defaultModel := strings.TrimSpace(defaultWorkerModel)
	if err := loaded.MutateWorkers(func(worker *factorydefinitions.FactoryWorkerConfig) error {
		if worker == nil || !isModelWorkerType(worker.Type) {
			return nil
		}
		if strings.TrimSpace(worker.ModelProvider) == "" && defaultProvider != "" {
			worker.ModelProvider = defaultProvider
		}
		if strings.TrimSpace(worker.Model) == "" && defaultModel != "" {
			worker.Model = defaultModel
		}
		return nil
	}); err != nil {
		return fmt.Errorf("apply operator defaults: %w", err)
	}
	return nil
}

func isModelWorkerType(workerType string) bool {
	switch strings.TrimSpace(workerType) {
	case factorydefinitions.WorkerTypeModel, factorydefinitions.WorkerTypeInference, factorydefinitions.WorkerTypeAgent:
		return true
	default:
		return false
	}
}

func operatorDefaultProviderInternal(canonicalPublic string) (string, error) {
	trimmed := strings.TrimSpace(canonicalPublic)
	if trimmed == "" {
		return "", nil
	}
	internal, ok := factorydefinitions.InternalModelProviderFromPublicWorkerModelProvider(trimmed)
	if ok {
		return string(internal), nil
	}
	if factorydefinitions.StrictPublicFactoryWorkerModelProvider(trimmed) == "" {
		return "", fmt.Errorf(
			"unsupported worker model provider %q: %s",
			trimmed,
			factorydefinitions.AcceptedPublicWorkerModelProviderSummary(),
		)
	}
	return trimmed, nil
}
