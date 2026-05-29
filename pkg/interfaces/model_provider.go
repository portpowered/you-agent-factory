package interfaces

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

// ModelProvider identifies the CLI command used for model inference dispatch.
type ModelProvider string

const (
	ModelProviderClaude   ModelProvider = "claude"
	ModelProviderCodex    ModelProvider = "codex"
	ModelProviderGemini   ModelProvider = "gemini"
	ModelProviderKiro     ModelProvider = "kiro-cli"
	ModelProviderCursor   ModelProvider = "agent"
	ModelProviderOpenCode ModelProvider = "opencode"
)

// SupportedModelProviders returns the canonical internal model provider commands
// used for runtime dispatch and validation.
func SupportedModelProviders() []ModelProvider {
	return []ModelProvider{
		ModelProviderClaude,
		ModelProviderCodex,
		ModelProviderGemini,
		ModelProviderKiro,
		ModelProviderCursor,
		ModelProviderOpenCode,
	}
}

var internalModelProviderToPublicWorkerModelProvider = map[ModelProvider]factoryapi.WorkerModelProvider{
	ModelProviderClaude:   factoryapi.WorkerModelProviderClaude,
	ModelProviderCodex:    factoryapi.WorkerModelProviderCodex,
	ModelProviderCursor:   factoryapi.WorkerModelProviderCursor,
	ModelProviderGemini:   factoryapi.WorkerModelProviderGemini,
	ModelProviderKiro:     factoryapi.WorkerModelProviderKiro,
	ModelProviderOpenCode: factoryapi.WorkerModelProviderOpenCode,
}

// PublicWorkerModelProviderFromInternal maps a canonical internal provider command to the generated public enum.
func PublicWorkerModelProviderFromInternal(provider ModelProvider) (factoryapi.WorkerModelProvider, bool) {
	public, ok := internalModelProviderToPublicWorkerModelProvider[provider]
	return public, ok
}

// InternalModelProviderFromPublicWorkerModelProvider maps a canonical public WorkerModelProvider to the internal command.
func InternalModelProviderFromPublicWorkerModelProvider(value factoryapi.WorkerModelProvider) (ModelProvider, bool) {
	switch StrictPublicFactoryWorkerModelProvider(string(value)) {
	case publicFactoryWorkerModelProviderClaude:
		return ModelProviderClaude, true
	case publicFactoryWorkerModelProviderCodex:
		return ModelProviderCodex, true
	case publicFactoryWorkerModelProviderCursor:
		return ModelProviderCursor, true
	case publicFactoryWorkerModelProviderGemini:
		return ModelProviderGemini, true
	case publicFactoryWorkerModelProviderKiro:
		return ModelProviderKiro, true
	case publicFactoryWorkerModelProviderOpenCode:
		return ModelProviderOpenCode, true
	default:
		return "", false
	}
}
