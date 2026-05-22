package interfaces

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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

var publicFactoryWorkerModelLocalityAliases = map[string]string{
	ModelLocalityLocal: ModelLocalityLocal,
	ModelLocalityCloud: ModelLocalityCloud,
}

var publicFactoryWorkerModelOperationContentTypeAliases = map[string]string{
	ModelOperationContentTypeText:   ModelOperationContentTypeText,
	ModelOperationContentTypeImage:  ModelOperationContentTypeImage,
	ModelOperationContentTypeAudio:  ModelOperationContentTypeAudio,
	ModelOperationContentTypeJSON:   ModelOperationContentTypeJSON,
	ModelOperationContentTypeBinary: ModelOperationContentTypeBinary,
}

var publicFactoryWorkstationTypeAliases = map[string]string{
	WorkstationTypeLogical: WorkstationTypeLogical,
	WorkstationTypeModel:   WorkstationTypeModel,
}

var publicFactoryRunnerIDAliases = map[string]string{
	RunnerIDCodex:     RunnerIDCodex,
	RunnerIDGemini:    RunnerIDGemini,
	RunnerIDKiro:      RunnerIDKiro,
	RunnerIDCursorCLI: RunnerIDCursorCLI,
	RunnerIDOpenCode:  RunnerIDOpenCode,
}

var publicFactoryRunnerSelectionSourceAliases = map[string]string{
	string(RunnerSelectionSourceWorkstation):    string(RunnerSelectionSourceWorkstation),
	string(RunnerSelectionSourceFactory):        string(RunnerSelectionSourceFactory),
	string(RunnerSelectionSourceLegacyProvider): string(RunnerSelectionSourceLegacyProvider),
	string(RunnerSelectionSourceDefault):        string(RunnerSelectionSourceDefault),
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

func generatedPublicFactoryEnumPtr[T ~string](value string, convert func(string) T) *T {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := convert(value)
	return &enumValue
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

// PermissivePublicFactoryWorkerModelLocality canonicalizes supported public worker model localities and preserves unknown values.
func PermissivePublicFactoryWorkerModelLocality(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelLocalityAliases, true)
}

// StrictPublicFactoryWorkerModelLocality canonicalizes supported public worker model localities and rejects unknown values.
func StrictPublicFactoryWorkerModelLocality(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelLocalityAliases, false)
}

// PermissivePublicFactoryWorkerModelOperationContentType canonicalizes supported public capability content types and preserves unknown values.
func PermissivePublicFactoryWorkerModelOperationContentType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelOperationContentTypeAliases, true)
}

// StrictPublicFactoryWorkerModelOperationContentType canonicalizes supported public capability content types and rejects unknown values.
func StrictPublicFactoryWorkerModelOperationContentType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelOperationContentTypeAliases, false)
}

// PermissivePublicFactoryWorkstationType canonicalizes supported public workstation types and preserves unknown values.
func PermissivePublicFactoryWorkstationType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationTypeAliases, true)
}

// StrictPublicFactoryWorkstationType canonicalizes supported public workstation types and rejects unknown values.
func StrictPublicFactoryWorkstationType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationTypeAliases, false)
}

// PermissivePublicFactoryRunnerID canonicalizes supported public runner IDs and preserves unknown values.
func PermissivePublicFactoryRunnerID(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryRunnerIDAliases, true)
}

// StrictPublicFactoryRunnerID canonicalizes supported public runner IDs and rejects unknown values.
func StrictPublicFactoryRunnerID(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryRunnerIDAliases, false)
}

// PermissivePublicFactoryRunnerSelectionSource canonicalizes supported public runner selection sources and preserves unknown values.
func PermissivePublicFactoryRunnerSelectionSource(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryRunnerSelectionSourceAliases, true)
}

// StrictPublicFactoryRunnerSelectionSource canonicalizes supported public runner selection sources and rejects unknown values.
func StrictPublicFactoryRunnerSelectionSource(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryRunnerSelectionSourceAliases, false)
}

// GeneratedPublicFactoryWorkerType returns the generated worker type enum.
func GeneratedPublicFactoryWorkerType(value string) factoryapi.WorkerType {
	return factoryapi.WorkerType(PermissivePublicFactoryWorkerType(value))
}

// GeneratedPublicFactoryWorkerTypePtr returns the generated worker type enum when non-empty.
func GeneratedPublicFactoryWorkerTypePtr(value string) *factoryapi.WorkerType {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkerType)
}

// GeneratedPublicFactoryWorkerModelProvider returns the generated worker model provider enum.
func GeneratedPublicFactoryWorkerModelProvider(value string) factoryapi.WorkerModelProvider {
	return factoryapi.WorkerModelProvider(normalizePublicFactoryEnumValue(value, internalFactoryWorkerModelProviderAliases, true))
}

// GeneratedPublicFactoryWorkerModelProviderPtr returns the generated worker model provider enum when non-empty.
func GeneratedPublicFactoryWorkerModelProviderPtr(value string) *factoryapi.WorkerModelProvider {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkerModelProvider)
}

// GeneratedPublicFactoryWorkerProvider returns the generated worker provider enum.
func GeneratedPublicFactoryWorkerProvider(value string) factoryapi.WorkerProvider {
	return factoryapi.WorkerProvider(normalizePublicFactoryEnumValue(value, internalFactoryWorkerProviderAliases, true))
}

// GeneratedPublicFactoryWorkerProviderPtr returns the generated worker provider enum when non-empty.
func GeneratedPublicFactoryWorkerProviderPtr(value string) *factoryapi.WorkerProvider {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkerProvider)
}

// GeneratedPublicFactoryWorkerModelLocality returns the generated worker model locality enum.
func GeneratedPublicFactoryWorkerModelLocality(value string) factoryapi.WorkerModelLocality {
	return factoryapi.WorkerModelLocality(PermissivePublicFactoryWorkerModelLocality(value))
}

// GeneratedPublicFactoryWorkerModelLocalityPtr returns the generated worker model locality enum when non-empty.
func GeneratedPublicFactoryWorkerModelLocalityPtr(value string) *factoryapi.WorkerModelLocality {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkerModelLocality)
}

// GeneratedPublicFactoryWorkerModelOperationContentType returns the generated worker capability content type enum.
func GeneratedPublicFactoryWorkerModelOperationContentType(value string) factoryapi.ModelOperationContentType {
	return factoryapi.ModelOperationContentType(PermissivePublicFactoryWorkerModelOperationContentType(value))
}

// GeneratedPublicFactoryWorkerModelOperationContentTypePtr returns the generated worker capability content type enum when non-empty.
func GeneratedPublicFactoryWorkerModelOperationContentTypePtr(value string) *factoryapi.ModelOperationContentType {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkerModelOperationContentType)
}

// GeneratedPublicFactoryWorkstationType returns the generated workstation type enum.
func GeneratedPublicFactoryWorkstationType(value string) factoryapi.WorkstationType {
	return factoryapi.WorkstationType(PermissivePublicFactoryWorkstationType(value))
}

// GeneratedPublicFactoryWorkstationTypePtr returns the generated workstation type enum when non-empty.
func GeneratedPublicFactoryWorkstationTypePtr(value string) *factoryapi.WorkstationType {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkstationType)
}

// GeneratedPublicFactoryRunnerID returns the generated runner ID enum.
func GeneratedPublicFactoryRunnerID(value string) factoryapi.RunnerID {
	return factoryapi.RunnerID(PermissivePublicFactoryRunnerID(NormalizeRunnerID(value)))
}

// GeneratedPublicFactoryRunnerIDPtr returns the generated runner ID enum when non-empty.
func GeneratedPublicFactoryRunnerIDPtr(value string) *factoryapi.RunnerID {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryRunnerID)
}

// GeneratedPublicFactoryRunnerSelectionSource returns the generated runner selection source enum.
func GeneratedPublicFactoryRunnerSelectionSource(value string) factoryapi.RunnerSelectionSource {
	return factoryapi.RunnerSelectionSource(PermissivePublicFactoryRunnerSelectionSource(value))
}

// GeneratedPublicFactoryRunnerSelectionSourcePtr returns the generated runner selection source enum when non-empty.
func GeneratedPublicFactoryRunnerSelectionSourcePtr(value string) *factoryapi.RunnerSelectionSource {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryRunnerSelectionSource)
}
