package operatordefaults

import (
	"fmt"
	"regexp"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

var exactInvocationInterpolationPattern = regexp.MustCompile(`^\$\{([A-Za-z0-9_.-]+)\}$`)

// ApplyToLoadedConfig fills omitted model-provider worker modelProvider and model
// fields from operator defaults in the opened in-memory runtime config. Authored
// worker values and non-model workers are left unchanged.
func ApplyToLoadedConfig(loaded interfaces.MutableLoadedFactorySource, defaults operatorconfig.ResolvedDefaults) error {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil
	}

	defaultProvider, err := operatorDefaultProviderInternal(defaults.WorkerModelProvider)
	if err != nil {
		return err
	}
	defaultModel := strings.TrimSpace(defaults.WorkerModel)

	return loaded.MutateWorkers(func(worker *interfaces.FactoryWorkerConfig) error {
		return applyOperatorDefaultsToWorker(worker, defaultProvider, defaultModel)
	})
}

// ResolveConcreteProviderSelections resolves every concrete provider selection
// through the process registry after operator defaults have been applied.
// Invocation expressions stay unresolved until invocation input is available.
func ResolveConcreteProviderSelections(
	loaded interfaces.MutableLoadedFactorySource,
	providers factorysessions.ProviderIdentityResolver,
) error {
	if loaded == nil || loaded.FactoryConfig() == nil {
		return nil
	}
	if providers == nil {
		return fmt.Errorf("provider identity resolver is required")
	}
	factoryCfg := loaded.FactoryConfig()
	for index := range factoryCfg.Workers {
		worker := &factoryCfg.Workers[index]
		if !isModelWorkerType(worker.Type) {
			continue
		}
		canonical, err := resolveConcreteProvider(
			factoryCfg.InvocationSignature,
			worker.ModelProvider,
			providers,
		)
		if err != nil {
			return fmt.Errorf("workers[%d].modelProvider: %w", index, err)
		}
		worker.ModelProvider = canonical
	}
	for index := range factoryCfg.Guards {
		guard := &factoryCfg.Guards[index]
		canonical, err := resolveConcreteProvider(nil, guard.ModelProvider, providers)
		if err != nil {
			return fmt.Errorf("guards[%d].modelProvider: %w", index, err)
		}
		guard.ModelProvider = canonical
	}
	return loaded.MutateWorkers(func(worker *interfaces.FactoryWorkerConfig) error {
		if worker == nil || !isModelWorkerType(worker.Type) {
			return nil
		}
		canonical, err := resolveConcreteProvider(
			factoryCfg.InvocationSignature,
			worker.ModelProvider,
			providers,
		)
		if err != nil {
			return fmt.Errorf("model worker %q modelProvider: %w", worker.Name, err)
		}
		worker.ModelProvider = canonical
		return nil
	})
}

func applyOperatorDefaultsToWorker(worker *interfaces.FactoryWorkerConfig, defaultProvider, defaultModel string) error {
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
	if ok {
		return string(internal), nil
	}
	if interfaces.StrictPublicFactoryWorkerModelProvider(trimmed) == "" {
		return "", fmt.Errorf(
			"unsupported worker model provider %q: %s",
			trimmed,
			interfaces.AcceptedPublicWorkerModelProviderSummary(),
		)
	}
	return trimmed, nil
}

func resolveConcreteProvider(
	signature *interfaces.InvocationSignatureConfig,
	modelProvider string,
	providers factorysessions.ProviderIdentityResolver,
) (string, error) {
	trimmed := strings.TrimSpace(modelProvider)
	if trimmed == "" {
		return "", nil
	}
	if invocationInterpolationParameterName(signature, trimmed) != "" {
		return modelProvider, nil
	}
	if interfaces.IsSymbolicWorkerModelProviderDefault(trimmed) {
		return "", fmt.Errorf(
			"unresolved DEFAULT model provider; set modelProvider or configure an operator default worker model provider",
		)
	}
	canonical, err := providers(trimmed)
	if err != nil {
		return "", fmt.Errorf("select provider %q: %w", trimmed, err)
	}
	return canonical, nil
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
