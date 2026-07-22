package runtimebuild

import (
	"fmt"
	"regexp"
	"strings"

	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

var exactInvocationInterpolationPattern = regexp.MustCompile(`^\$\{([A-Za-z0-9_.-]+)\}$`)

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
	return validateModelWorkerRuntimeProviders(loaded)
}

func validateModelWorkerRuntimeProviders(loaded factorydefinitions.MutableLoadedFactorySource) error {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil
	}
	factoryCfg := loaded.FactoryConfig()
	for _, worker := range factoryCfg.Workers {
		if !isModelWorkerType(worker.Type) {
			continue
		}
		if err := validateModelWorkerProvider(factoryCfg.InvocationSignature, worker.Name, worker.ModelProvider); err != nil {
			return err
		}
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
	if !ok {
		return "", fmt.Errorf(
			"unsupported worker model provider %q: %s",
			trimmed,
			factorydefinitions.AcceptedPublicWorkerModelProviderSummary(),
		)
	}
	return string(internal), nil
}

func validateModelWorkerProvider(
	signature *factorydefinitions.InvocationSignatureConfig,
	workerName string,
	modelProvider string,
) error {
	trimmed := strings.TrimSpace(modelProvider)
	if trimmed == "" || invocationInterpolationParameterName(signature, trimmed) != "" {
		return nil
	}
	if factorydefinitions.IsSymbolicWorkerModelProviderDefault(trimmed) {
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
		factorydefinitions.AcceptedPublicWorkerModelProviderSummary(),
	)
}

func isSupportedRuntimeModelProvider(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return true
	}
	for _, provider := range factorycontracts.SupportedModelProviders() {
		if string(provider) == trimmed {
			return true
		}
	}
	if canonical := factorydefinitions.StrictPublicFactoryWorkerModelProvider(trimmed); canonical != "" {
		if _, ok := factorydefinitions.InternalModelProviderFromPublicWorkerModelProvider(canonical); ok {
			return true
		}
	}
	if canonical, ok := factorydefinitions.CanonicalizeOperatorWorkerModelProviderInput(trimmed); ok &&
		!factorydefinitions.IsSymbolicWorkerModelProviderDefault(canonical) {
		if _, ok := factorydefinitions.InternalModelProviderFromPublicWorkerModelProvider(canonical); ok {
			return true
		}
	}
	return false
}

func invocationInterpolationParameterName(
	signature *factorydefinitions.InvocationSignatureConfig,
	value string,
) string {
	if signature == nil {
		return ""
	}
	match := exactInvocationInterpolationPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(match) != 2 {
		return ""
	}
	name := strings.TrimSpace(match[1])
	for _, parameter := range signature.Parameters {
		if strings.TrimSpace(parameter.Name) == name {
			return name
		}
	}
	return ""
}
