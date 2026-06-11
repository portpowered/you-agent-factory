package interfaces

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ModelProviderSelectionSource reports which configuration layer supplied the
// concrete model provider used for one dispatch.
type ModelProviderSelectionSource string

const (
	ModelProviderSelectionSourceWorkstation     ModelProviderSelectionSource = "workstation"
	ModelProviderSelectionSourceFactory         ModelProviderSelectionSource = "factory"
	ModelProviderSelectionSourceWorker          ModelProviderSelectionSource = "worker"
	ModelProviderSelectionSourceOperatorDefault ModelProviderSelectionSource = "operator_default"
)

// OperatorDefaultModelProvider is the concrete provider command used when no
// configuration layer supplies one.
const OperatorDefaultModelProvider = ModelProviderCodex

// ResolvedModelProviderSelection is the canonical provider-selection result used
// by backend runtime code before dispatch starts.
type ResolvedModelProviderSelection struct {
	Provider ModelProvider                `json:"provider,omitempty"`
	Source   ModelProviderSelectionSource `json:"source,omitempty"`
}

// ModelProviderMetadata is the canonical metadata shape for one built-in model
// provider's identity and capability support.
type ModelProviderMetadata struct {
	Provider     ModelProvider      `json:"provider,omitempty"`
	DisplayName  string             `json:"display_name,omitempty"`
	Capabilities RunnerCapabilities `json:"capabilities"`
}

// IsSupportedModelProvider reports whether one value is a canonical internal
// model provider command used for runtime dispatch.
func IsSupportedModelProvider(provider ModelProvider) bool {
	switch provider {
	case ModelProviderClaude, ModelProviderCodex, ModelProviderGemini, ModelProviderKiro, ModelProviderCursor, ModelProviderOpenCode:
		return true
	default:
		return false
	}
}

// ModelProviderFromRunnerID maps one built-in runner identifier to the canonical
// internal model provider command when one exists.
func ModelProviderFromRunnerID(id string) (ModelProvider, bool) {
	switch NormalizeRunnerID(id) {
	case RunnerIDCodex:
		return ModelProviderCodex, true
	case RunnerIDGemini:
		return ModelProviderGemini, true
	case RunnerIDKiro:
		return ModelProviderKiro, true
	case RunnerIDCursorCLI:
		return ModelProviderCursor, true
	case RunnerIDOpenCode:
		return ModelProviderOpenCode, true
	default:
		return "", false
	}
}

var builtInModelProviderOnlyMetadata = map[ModelProvider]ModelProviderMetadata{
	ModelProviderClaude: {
		Provider:    ModelProviderClaude,
		DisplayName: "Claude",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusSupported, Detail: "factory-managed git worktree preparation under the factory root"},
		),
	},
}

// BuiltInModelProviderMetadata returns metadata for one built-in model provider
// command backed by the stable runner registry.
func BuiltInModelProviderMetadata(provider ModelProvider) (ModelProviderMetadata, bool) {
	if metadata, ok := builtInModelProviderOnlyMetadata[provider]; ok {
		return metadata, true
	}
	runnerID, ok := RunnerIDFromInternalModelProvider(string(provider))
	if !ok {
		return ModelProviderMetadata{}, false
	}
	runnerMeta, ok := BuiltInRunnerMetadata(runnerID)
	if !ok {
		return ModelProviderMetadata{}, false
	}
	return ModelProviderMetadata{
		Provider:     provider,
		DisplayName:  runnerMeta.DisplayName,
		Capabilities: runnerMeta.Capabilities,
	}, true
}

// ResolveModelProviderSelection applies the finalized precedence rules for
// backend runtime provider choice: workstation override, factory override,
// worker provider, then the operator default.
func ResolveModelProviderSelection(workstationModelProvider, factoryModelProvider, workerModelProvider string) ResolvedModelProviderSelection {
	if provider := resolveConcreteModelProviderFromSelection(workstationModelProvider); provider != "" {
		return ResolvedModelProviderSelection{Provider: provider, Source: ModelProviderSelectionSourceWorkstation}
	}
	if provider := resolveConcreteModelProviderFromSelection(factoryModelProvider); provider != "" {
		return ResolvedModelProviderSelection{Provider: provider, Source: ModelProviderSelectionSourceFactory}
	}
	if provider := resolveConcreteModelProviderFromSelection(workerModelProvider); provider != "" {
		return ResolvedModelProviderSelection{Provider: provider, Source: ModelProviderSelectionSourceWorker}
	}
	return ResolvedModelProviderSelection{
		Provider: OperatorDefaultModelProvider,
		Source:   ModelProviderSelectionSourceOperatorDefault,
	}
}

func resolveConcreteModelProviderFromSelection(selection string) ModelProvider {
	selection = strings.TrimSpace(selection)
	if selection == "" || selection == FactoryModelProviderDefault {
		return ""
	}
	if provider := ModelProvider(strings.ToLower(selection)); IsSupportedModelProvider(provider) {
		return provider
	}
	if canonical := StrictPublicFactoryModelProviderSelection(selection); canonical != "" && canonical != FactoryModelProviderDefault {
		if provider, ok := modelProviderFromPublicSelection(canonical); ok {
			return provider
		}
	}
	if runner := NormalizeRunnerID(selection); IsBuiltInRunnerID(runner) {
		if provider, ok := ModelProviderFromRunnerID(runner); ok {
			return provider
		}
	}
	return ""
}

func modelProviderFromPublicSelection(canonical string) (ModelProvider, bool) {
	public := GeneratedPublicFactoryWorkerModelProvider(canonical)
	return InternalModelProviderFromPublicWorkerModelProvider(public)
}

// InternalRunnerIDFromPublicWorkerModelProvider maps one public WorkerModelProvider
// value to the legacy built-in runner identifier used by internal projections.
func InternalRunnerIDFromPublicWorkerModelProvider(value factoryapi.WorkerModelProvider) (string, bool) {
	provider, ok := InternalModelProviderFromPublicWorkerModelProvider(value)
	if !ok {
		return "", false
	}
	return RunnerIDFromInternalModelProvider(string(provider))
}

// ValidateOpenCodeAgentForModelProviderSelection reports a configuration error
// when a non-empty OpenCode agent profile is configured for a dispatch that
// will not use the OpenCode provider.
func ValidateOpenCodeAgentForModelProviderSelection(workstationAgent, workerAgent string, selection ResolvedModelProviderSelection) error {
	agent := ResolveOpenCodeAgent(workstationAgent, workerAgent)
	if agent == "" {
		return nil
	}
	if selection.Provider == ModelProviderOpenCode {
		return nil
	}
	return fmt.Errorf(
		"openCodeAgent %q requires modelProvider %q, resolved modelProvider %q",
		agent,
		ModelProviderOpenCode,
		selection.Provider,
	)
}

func modelProviderSelectionSourceToRunnerSelectionSource(source ModelProviderSelectionSource) RunnerSelectionSource {
	switch source {
	case ModelProviderSelectionSourceWorkstation:
		return RunnerSelectionSourceWorkstation
	case ModelProviderSelectionSourceFactory:
		return RunnerSelectionSourceFactory
	case ModelProviderSelectionSourceWorker:
		return RunnerSelectionSourceLegacyProvider
	default:
		return RunnerSelectionSourceDefault
	}
}

func resolvedRunnerSelectionFromModelProviderSelection(selection ResolvedModelProviderSelection) ResolvedRunnerSelection {
	runnerID := ""
	if mapped, ok := RunnerIDFromInternalModelProvider(string(selection.Provider)); ok {
		runnerID = mapped
	}
	return ResolvedRunnerSelection{
		RunnerID: runnerID,
		Source:   modelProviderSelectionSourceToRunnerSelectionSource(selection.Source),
	}
}
