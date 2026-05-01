package interfaces

import (
	"strings"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
)

var publicFactoryWorkerTypeAliases = map[string]string{
	WorkerTypeModel:  WorkerTypeModel,
	WorkerTypeScript: WorkerTypeScript,
}

var publicFactoryWorkerModelProviderAliases = map[string]string{
	publicFactoryWorkerModelProviderClaude: publicFactoryWorkerModelProviderClaude,
	publicFactoryWorkerModelProviderCodex:  publicFactoryWorkerModelProviderCodex,
}

var publicFactoryWorkerProviderAliases = map[string]string{
	publicFactoryWorkerProviderScriptWrap: publicFactoryWorkerProviderScriptWrap,
}

var publicFactoryWorkstationTypeAliases = map[string]string{
	WorkstationTypeLogical: WorkstationTypeLogical,
	WorkstationTypeModel:   WorkstationTypeModel,
}

const (
	publicFactoryWorkerModelProviderClaude = "CLAUDE"
	publicFactoryWorkerModelProviderCodex  = "CODEX"
	publicFactoryWorkerProviderScriptWrap  = "SCRIPT_WRAP"
)

var internalFactoryWorkerModelProviderAliases = map[string]string{
	"ANTHROPIC": publicFactoryWorkerModelProviderClaude,
	"CLAUDE":    publicFactoryWorkerModelProviderClaude,
	"CODEX":     publicFactoryWorkerModelProviderCodex,
	"OPENAI":    publicFactoryWorkerModelProviderCodex,
	"anthropic": publicFactoryWorkerModelProviderClaude,
	"claude":    publicFactoryWorkerModelProviderClaude,
	"codex":     publicFactoryWorkerModelProviderCodex,
	"openai":    publicFactoryWorkerModelProviderCodex,
}

var internalFactoryWorkerProviderAliases = map[string]string{
	"ANTHROPIC":    publicFactoryWorkerProviderScriptWrap,
	"CLAUDE":       publicFactoryWorkerProviderScriptWrap,
	"CLAUDE_CLI":   publicFactoryWorkerProviderScriptWrap,
	"CODEX_CLI":    publicFactoryWorkerProviderScriptWrap,
	"LOCAL":        publicFactoryWorkerProviderScriptWrap,
	"LOCAL_CLAUDE": publicFactoryWorkerProviderScriptWrap,
	"SCRIPT":       publicFactoryWorkerProviderScriptWrap,
	"SCRIPTWRAP":   publicFactoryWorkerProviderScriptWrap,
	"SCRIPT_WRAP":  publicFactoryWorkerProviderScriptWrap,
	"anthropic":    publicFactoryWorkerProviderScriptWrap,
	"claude":       publicFactoryWorkerProviderScriptWrap,
	"claude_cli":   publicFactoryWorkerProviderScriptWrap,
	"claude-cli":   publicFactoryWorkerProviderScriptWrap,
	"codex_cli":    publicFactoryWorkerProviderScriptWrap,
	"codex-cli":    publicFactoryWorkerProviderScriptWrap,
	"local":        publicFactoryWorkerProviderScriptWrap,
	"local_claude": publicFactoryWorkerProviderScriptWrap,
	"local-claude": publicFactoryWorkerProviderScriptWrap,
	"script":       publicFactoryWorkerProviderScriptWrap,
	"scriptwrap":   publicFactoryWorkerProviderScriptWrap,
	"script_wrap":  publicFactoryWorkerProviderScriptWrap,
	"script-wrap":  publicFactoryWorkerProviderScriptWrap,
}

func normalizePublicFactoryEnumValue(value string, aliases map[string]string, preserveUnknown bool) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if canonical, ok := aliases[trimmed]; ok {
		return canonical
	}
	if preserveUnknown {
		return trimmed
	}
	return ""
}

// PermissivePublicFactoryWorkerType canonicalizes supported public worker types and preserves unknown values.
func PermissivePublicFactoryWorkerType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerTypeAliases, true)
}

// StrictPublicFactoryWorkerType canonicalizes supported public worker types and rejects unknown values.
func StrictPublicFactoryWorkerType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerTypeAliases, false)
}

// PermissivePublicFactoryWorkerModelProvider canonicalizes supported public worker model providers and preserves unknown values.
func PermissivePublicFactoryWorkerModelProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelProviderAliases, true)
}

// StrictPublicFactoryWorkerModelProvider canonicalizes supported public worker model providers and rejects unknown values.
func StrictPublicFactoryWorkerModelProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelProviderAliases, false)
}

// PermissivePublicFactoryWorkerProvider canonicalizes supported public worker providers and preserves unknown values.
func PermissivePublicFactoryWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerProviderAliases, true)
}

// StrictPublicFactoryWorkerProvider canonicalizes supported public worker providers and rejects unknown values.
func StrictPublicFactoryWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerProviderAliases, false)
}

// PermissivePublicFactoryWorkstationType canonicalizes supported public workstation types and preserves unknown values.
func PermissivePublicFactoryWorkstationType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationTypeAliases, true)
}

// StrictPublicFactoryWorkstationType canonicalizes supported public workstation types and rejects unknown values.
func StrictPublicFactoryWorkstationType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationTypeAliases, false)
}

// GeneratedPublicFactoryWorkerType returns the generated worker type enum.
func GeneratedPublicFactoryWorkerType(value string) factoryapi.WorkerType {
	return factoryapi.WorkerType(PermissivePublicFactoryWorkerType(value))
}

// GeneratedPublicFactoryWorkerTypePtr returns the generated worker type enum when non-empty.
func GeneratedPublicFactoryWorkerTypePtr(value string) *factoryapi.WorkerType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := GeneratedPublicFactoryWorkerType(value)
	return &enumValue
}

// GeneratedPublicFactoryWorkerModelProvider returns the generated worker model provider enum.
func GeneratedPublicFactoryWorkerModelProvider(value string) factoryapi.WorkerModelProvider {
	return factoryapi.WorkerModelProvider(normalizePublicFactoryEnumValue(value, internalFactoryWorkerModelProviderAliases, true))
}

// GeneratedPublicFactoryWorkerModelProviderPtr returns the generated worker model provider enum when non-empty.
func GeneratedPublicFactoryWorkerModelProviderPtr(value string) *factoryapi.WorkerModelProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := GeneratedPublicFactoryWorkerModelProvider(value)
	return &enumValue
}

// GeneratedPublicFactoryWorkerProvider returns the generated worker provider enum.
func GeneratedPublicFactoryWorkerProvider(value string) factoryapi.WorkerProvider {
	return factoryapi.WorkerProvider(normalizePublicFactoryEnumValue(value, internalFactoryWorkerProviderAliases, true))
}

// GeneratedPublicFactoryWorkerProviderPtr returns the generated worker provider enum when non-empty.
func GeneratedPublicFactoryWorkerProviderPtr(value string) *factoryapi.WorkerProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := GeneratedPublicFactoryWorkerProvider(value)
	return &enumValue
}

// GeneratedPublicFactoryWorkstationType returns the generated workstation type enum.
func GeneratedPublicFactoryWorkstationType(value string) factoryapi.WorkstationType {
	return factoryapi.WorkstationType(PermissivePublicFactoryWorkstationType(value))
}

// GeneratedPublicFactoryWorkstationTypePtr returns the generated workstation type enum when non-empty.
func GeneratedPublicFactoryWorkstationTypePtr(value string) *factoryapi.WorkstationType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := GeneratedPublicFactoryWorkstationType(value)
	return &enumValue
}
