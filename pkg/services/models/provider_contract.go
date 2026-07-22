package models

// Provider identifies the model provider used for inference dispatch.
type Provider string

const (
	ProviderClaude   Provider = "claude"
	ProviderCodex    Provider = "codex"
	ProviderGemini   Provider = "gemini"
	ProviderKiro     Provider = "kiro-cli"
	ProviderCursor   Provider = "agent"
	ProviderOpenCode Provider = "opencode"
	ProviderPi       Provider = "pi"
	ProviderAgy      Provider = "agy"
)
