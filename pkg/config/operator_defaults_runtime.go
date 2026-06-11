package config

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ApplyOperatorDefaultsToLoadedConfig fills omitted MODEL_WORKER modelProvider and
// model fields from operator defaults in the in-memory effective runtime config.
// Authored worker values and non-model workers are left unchanged.
func ApplyOperatorDefaultsToLoadedConfig(loaded *LoadedFactoryConfig, defaults operatorconfig.ResolvedDefaults) error {
	if loaded == nil || loaded.factory == nil {
		return nil
	}

	defaultProvider, err := operatorDefaultProviderInternal(defaults.WorkerModelProvider)
	if err != nil {
		return err
	}
	defaultModel := strings.TrimSpace(defaults.WorkerModel)

	for i := range loaded.factory.Workers {
		if err := applyOperatorDefaultsToWorker(&loaded.factory.Workers[i], defaultProvider, defaultModel); err != nil {
			return err
		}
	}
	if loaded.lookup != nil {
		for name, worker := range loaded.lookup.workers {
			if worker == nil {
				continue
			}
			if err := applyOperatorDefaultsToWorker(worker, defaultProvider, defaultModel); err != nil {
				return fmt.Errorf("worker %q: %w", name, err)
			}
		}
	}
	return nil
}

// ValidateModelWorkerRuntimeProviders rejects unresolved DEFAULT and unsupported
// model providers on MODEL_WORKER definitions before runtime dispatch.
func ValidateModelWorkerRuntimeProviders(loaded *LoadedFactoryConfig) error {
	if loaded == nil || loaded.factory == nil {
		return nil
	}
	for _, worker := range loaded.factory.Workers {
		if !isModelWorkerType(worker.Type) {
			continue
		}
		if err := validateModelWorkerProvider(worker.Name, worker.ModelProvider); err != nil {
			return err
		}
	}
	return nil
}

func applyOperatorDefaultsToWorker(worker *interfaces.WorkerConfig, defaultProvider, defaultModel string) error {
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
}

func isModelWorkerType(workerType string) bool {
	return interfaces.StrictPublicFactoryWorkerType(workerType) == interfaces.WorkerTypeModel
}

func operatorDefaultProviderInternal(canonicalPublic string) (string, error) {
	trimmed := strings.TrimSpace(canonicalPublic)
	if trimmed == "" {
		return "", nil
	}
	public := factoryapi.WorkerModelProvider(trimmed)
	internal, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(public)
	if !ok {
		return "", fmt.Errorf(
			"unsupported worker model provider %q: %s",
			trimmed,
			interfaces.AcceptedPublicWorkerModelProviderSummary(),
		)
	}
	return string(internal), nil
}

func validateModelWorkerProvider(workerName, modelProvider string) error {
	trimmed := strings.TrimSpace(modelProvider)
	if trimmed == "" {
		return nil
	}
	if interfaces.IsSymbolicWorkerModelProviderDefault(trimmed) {
		return fmt.Errorf(
			"model worker %q has unresolved DEFAULT model provider; set modelProvider or configure an operator default worker model provider",
			workerName,
		)
	}
	if isSupportedRuntimeModelProvider(trimmed) {
		return nil
	}
	return fmt.Errorf(
		"model worker %q has unsupported model provider %q: %s",
		workerName,
		trimmed,
		interfaces.AcceptedPublicWorkerModelProviderSummary(),
	)
}

func isSupportedRuntimeModelProvider(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	for _, provider := range interfaces.SupportedModelProviders() {
		if string(provider) == trimmed {
			return true
		}
	}
	if canonical := interfaces.StrictPublicFactoryWorkerModelProvider(trimmed); canonical != "" {
		if _, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(factoryapi.WorkerModelProvider(canonical)); ok {
			return true
		}
	}
	if canonical, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(trimmed); ok && !interfaces.IsSymbolicWorkerModelProviderDefault(canonical) {
		if _, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(factoryapi.WorkerModelProvider(canonical)); ok {
			return true
		}
	}
	return false
}
