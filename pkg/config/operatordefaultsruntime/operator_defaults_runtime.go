package operatordefaultsruntime

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

var exactInvocationInterpolationPattern = regexp.MustCompile(`^\$\{([A-Za-z0-9_.-]+)\}$`)

// ApplyToLoadedConfig fills omitted model-provider worker modelProvider and model
// fields from operator defaults in the in-memory effective runtime config. Authored
// worker values and non-model workers are left unchanged.
func ApplyToLoadedConfig(loaded *config.LoadedFactoryConfig, defaults operatorconfig.ResolvedDefaults) error {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil
	}

	defaultProvider, err := operatorDefaultProviderInternal(defaults.WorkerModelProvider)
	if err != nil {
		return err
	}
	defaultModel := strings.TrimSpace(defaults.WorkerModel)

	return loaded.MutateWorkers(func(worker *workerconfig.Config) error {
		return applyOperatorDefaultsToWorker(worker, defaultProvider, defaultModel)
	})
}

// ValidateModelWorkerRuntimeProviders rejects unresolved DEFAULT and unsupported
// model providers on MODEL_WORKER definitions before runtime dispatch.
func ValidateModelWorkerRuntimeProviders(loaded *config.LoadedFactoryConfig) error {
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

func applyOperatorDefaultsToWorker(worker *workerconfig.Config, defaultProvider, defaultModel string) error {
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
	switch strings.TrimSpace(workerType) {
	case interfaces.WorkerTypeModel, interfaces.WorkerTypeInference, interfaces.WorkerTypeAgent:
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
	internal, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(trimmed)
	if !ok {
		return "", fmt.Errorf(
			"unsupported worker model provider %q: %s",
			trimmed,
			interfaces.AcceptedPublicWorkerModelProviderSummary(),
		)
	}
	return string(internal), nil
}

func validateModelWorkerProvider(signature *interfaces.InvocationSignatureConfig, workerName, modelProvider string) error {
	trimmed := strings.TrimSpace(modelProvider)
	if trimmed == "" {
		return nil
	}
	if invocationInterpolationParameterName(signature, trimmed) != "" {
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
	for _, provider := range modelprovider.Supported() {
		if string(provider) == trimmed {
			return true
		}
	}
	if canonical := interfaces.StrictPublicFactoryWorkerModelProvider(trimmed); canonical != "" {
		if _, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(canonical); ok {
			return true
		}
	}
	if canonical, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(trimmed); ok && !interfaces.IsSymbolicWorkerModelProviderDefault(canonical) {
		if _, ok := interfaces.InternalModelProviderFromPublicWorkerModelProvider(canonical); ok {
			return true
		}
	}
	return false
}

func invocationInterpolationParameterName(signature *interfaces.InvocationSignatureConfig, value string) string {
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
