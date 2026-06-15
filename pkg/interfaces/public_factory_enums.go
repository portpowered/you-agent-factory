package interfaces

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

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
	ModelProviderCursor: factoryapi.WorkerModelProviderCursor,
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

var publicFactoryWorkerTypeAliases = map[string]string{
	WorkerTypeModel:  WorkerTypeModel,
	WorkerTypeScript: WorkerTypeScript,
	WorkerTypeHosted: WorkerTypeHosted,
}

var publicFactoryWorkerModelProviderAliases = map[string]string{
	publicFactoryWorkerModelProviderClaude:   publicFactoryWorkerModelProviderClaude,
	publicFactoryWorkerModelProviderCodex:    publicFactoryWorkerModelProviderCodex,
	publicFactoryWorkerModelProviderCursor:   publicFactoryWorkerModelProviderCursor,
	publicFactoryWorkerModelProviderGemini:   publicFactoryWorkerModelProviderGemini,
	publicFactoryWorkerModelProviderKiro:     publicFactoryWorkerModelProviderKiro,
	publicFactoryWorkerModelProviderOpenCode: publicFactoryWorkerModelProviderOpenCode,
}

const publicFactoryModelProviderSelectionDefault = "DEFAULT"

var publicFactoryModelProviderSelectionAliases = map[string]string{
	publicFactoryModelProviderSelectionDefault: publicFactoryModelProviderSelectionDefault,
	publicFactoryWorkerModelProviderClaude:     publicFactoryWorkerModelProviderClaude,
	publicFactoryWorkerModelProviderCodex:      publicFactoryWorkerModelProviderCodex,
	publicFactoryWorkerModelProviderCursor:     publicFactoryWorkerModelProviderCursor,
	publicFactoryWorkerModelProviderGemini:     publicFactoryWorkerModelProviderGemini,
	publicFactoryWorkerModelProviderKiro:       publicFactoryWorkerModelProviderKiro,
	publicFactoryWorkerModelProviderOpenCode:   publicFactoryWorkerModelProviderOpenCode,
}

var publicFactoryWorkerProviderAliases = map[string]string{
	publicFactoryWorkerProviderScriptWrap: publicFactoryWorkerProviderScriptWrap,
}

var publicFactoryHostedWorkerProviderAliases = map[string]string{
	HostedWorkerProviderLinear: HostedWorkerProviderLinear,
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

var publicFactoryResourceTypeAliases = map[string]string{
	ResourceTypeModel:          ResourceTypeModel,
	ResourceTypeProviderQuota:  ResourceTypeProviderQuota,
	ResourceTypeInvocationSlot: ResourceTypeInvocationSlot,
}

var publicFactoryWorkstationTypeAliases = map[string]string{
	WorkstationTypeClassify: WorkstationTypeClassify,
	WorkstationTypeLogical:  WorkstationTypeLogical,
	WorkstationTypeInvoke:   WorkstationTypeInvoke,
	WorkstationTypeModel:    WorkstationTypeModel,
}

var publicFactoryModelProviderSelectionSourceAliases = map[string]string{
	string(ModelProviderSelectionSourceWorkstation):     string(ModelProviderSelectionSourceWorkstation),
	string(ModelProviderSelectionSourceFactory):         string(ModelProviderSelectionSourceFactory),
	string(ModelProviderSelectionSourceWorker):          string(ModelProviderSelectionSourceWorker),
	string(ModelProviderSelectionSourceOperatorDefault): string(ModelProviderSelectionSourceOperatorDefault),
	string(RunnerSelectionSourceLegacyProvider):         string(ModelProviderSelectionSourceWorker),
	string(RunnerSelectionSourceDefault):                string(ModelProviderSelectionSourceOperatorDefault),
}

var publicFactoryWorkTypeHandlingBehaviorAliases = map[string]string{
	WorkTypeHandlingBehaviorDefault: WorkTypeHandlingBehaviorDefault,
}

const (
	publicFactoryWorkerModelProviderClaude   = "CLAUDE"
	publicFactoryWorkerModelProviderCodex    = "CODEX"
	publicFactoryWorkerModelProviderCursor   = "CURSOR"
	publicFactoryWorkerModelProviderGemini   = "GEMINI"
	publicFactoryWorkerModelProviderKiro     = "KIRO"
	publicFactoryWorkerModelProviderOpenCode = "OPENCODE"
	publicFactoryWorkerProviderScriptWrap    = "SCRIPT_WRAP"
)

// WorkerModelProviderDefault is the symbolic operator-config model provider value
// that resolves through a lower-precedence concrete provider before dispatch.
const WorkerModelProviderDefault = "DEFAULT"

var internalFactoryWorkerModelProviderAliases = map[string]string{
	"ANTHROPIC":    publicFactoryWorkerModelProviderClaude,
	"CLAUDE":       publicFactoryWorkerModelProviderClaude,
	"CODEX":        publicFactoryWorkerModelProviderCodex,
	"CURSOR":       publicFactoryWorkerModelProviderCursor,
	"CURSOR_AGENT": publicFactoryWorkerModelProviderCursor,
	"GEMINI":       publicFactoryWorkerModelProviderGemini,
	"KIRO":         publicFactoryWorkerModelProviderKiro,
	"OPENCODE":     publicFactoryWorkerModelProviderOpenCode,
	"OPENAI":       publicFactoryWorkerModelProviderCodex,
	"agent":        publicFactoryWorkerModelProviderCursor,
	"anthropic":    publicFactoryWorkerModelProviderClaude,
	"claude":       publicFactoryWorkerModelProviderClaude,
	"codex":        publicFactoryWorkerModelProviderCodex,
	"cursor":       publicFactoryWorkerModelProviderCursor,
	"cursor-agent": publicFactoryWorkerModelProviderCursor,
	"gemini":       publicFactoryWorkerModelProviderGemini,
	"kiro":         publicFactoryWorkerModelProviderKiro,
	"kiro-cli":     publicFactoryWorkerModelProviderKiro,
	"openai":       publicFactoryWorkerModelProviderCodex,
	"opencode":     publicFactoryWorkerModelProviderOpenCode,
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

// StrictPublicFactoryModelProviderSelection canonicalizes supported public factory-level
// modelProvider values, including symbolic DEFAULT, and rejects unknown values.
func StrictPublicFactoryModelProviderSelection(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryModelProviderSelectionAliases, false)
}

// IsSymbolicWorkerModelProviderDefault reports whether value is the symbolic DEFAULT provider.
func IsSymbolicWorkerModelProviderDefault(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), WorkerModelProviderDefault)
}

// CanonicalizeOperatorWorkerModelProviderInput canonicalizes operator-config worker
// model provider inputs, including public aliases and symbolic DEFAULT.
func CanonicalizeOperatorWorkerModelProviderInput(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", true
	}
	if IsSymbolicWorkerModelProviderDefault(trimmed) {
		return WorkerModelProviderDefault, true
	}
	if canonical := normalizePublicFactoryEnumValue(trimmed, internalFactoryWorkerModelProviderAliases, false); canonical != "" {
		return canonical, true
	}
	if canonical := StrictPublicFactoryWorkerModelProvider(trimmed); canonical != "" {
		return canonical, true
	}
	return "", false
}

// AcceptedPublicWorkerModelProviderSummary returns canonical provider names and
// representative public aliases for operator-facing validation errors.
func AcceptedPublicWorkerModelProviderSummary() string {
	return "accepted canonical providers: CLAUDE, CODEX, CURSOR, GEMINI, KIRO, OPENCODE; " +
		"accepted aliases include codex, claude, gemini, kiro-cli, opencode, agent, cursor, and anthropic"
}

// PermissivePublicFactoryWorkerProvider canonicalizes supported public worker providers and preserves unknown values.
func PermissivePublicFactoryWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerProviderAliases, true)
}

// StrictPublicFactoryWorkerProvider canonicalizes supported public worker providers and rejects unknown values.
func StrictPublicFactoryWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerProviderAliases, false)
}

// PermissivePublicFactoryHostedWorkerProvider canonicalizes supported hosted worker providers and preserves unknown values.
func PermissivePublicFactoryHostedWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryHostedWorkerProviderAliases, true)
}

// StrictPublicFactoryHostedWorkerProvider canonicalizes supported hosted worker providers and rejects unknown values.
func StrictPublicFactoryHostedWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryHostedWorkerProviderAliases, false)
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

// PermissivePublicFactoryResourceType canonicalizes supported public resource types and preserves unknown values.
func PermissivePublicFactoryResourceType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryResourceTypeAliases, true)
}

// StrictPublicFactoryResourceType canonicalizes supported public resource types and rejects unknown values.
func StrictPublicFactoryResourceType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryResourceTypeAliases, false)
}

// PermissivePublicFactoryWorkstationType canonicalizes supported public workstation types and preserves unknown values.
func PermissivePublicFactoryWorkstationType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationTypeAliases, true)
}

// StrictPublicFactoryWorkstationType canonicalizes supported public workstation types and rejects unknown values.
func StrictPublicFactoryWorkstationType(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationTypeAliases, false)
}

// PermissivePublicFactoryModelProviderSelectionSource canonicalizes supported public
// model-provider selection sources and preserves unknown values.
func PermissivePublicFactoryModelProviderSelectionSource(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryModelProviderSelectionSourceAliases, true)
}

// StrictPublicFactoryModelProviderSelectionSource canonicalizes supported public
// model-provider selection sources and rejects unknown values.
func StrictPublicFactoryModelProviderSelectionSource(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryModelProviderSelectionSourceAliases, false)
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

// GeneratedPublicFactoryHostedWorkerProvider returns the generated hosted worker provider enum.
func GeneratedPublicFactoryHostedWorkerProvider(value string) factoryapi.HostedWorkerProvider {
	return factoryapi.HostedWorkerProvider(PermissivePublicFactoryHostedWorkerProvider(value))
}

// GeneratedPublicFactoryHostedWorkerProviderPtr returns the generated hosted worker provider enum when non-empty.
func GeneratedPublicFactoryHostedWorkerProviderPtr(value string) *factoryapi.HostedWorkerProvider {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryHostedWorkerProvider)
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

// GeneratedPublicFactoryModelProviderSelectionSource returns the generated model-provider selection source enum.
func GeneratedPublicFactoryModelProviderSelectionSource(value string) factoryapi.ModelProviderSelectionSource {
	return factoryapi.ModelProviderSelectionSource(PermissivePublicFactoryModelProviderSelectionSource(value))
}

// GeneratedPublicFactoryModelProviderSelectionSourcePtr returns the generated model-provider selection source enum when non-empty.
func GeneratedPublicFactoryModelProviderSelectionSourcePtr(value string) *factoryapi.ModelProviderSelectionSource {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryModelProviderSelectionSource)
}

// GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProvider maps a built-in runner ID
// or internal provider command to the generated public WorkerModelProvider enum.
func GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProvider(value string) (factoryapi.WorkerModelProvider, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if provider := ModelProvider(strings.ToLower(value)); IsSupportedModelProvider(provider) {
		return PublicWorkerModelProviderFromInternal(provider)
	}
	if provider, ok := ModelProviderFromRunnerID(value); ok {
		return PublicWorkerModelProviderFromInternal(provider)
	}
	if canonical := StrictPublicFactoryWorkerModelProvider(value); canonical != "" {
		return factoryapi.WorkerModelProvider(canonical), true
	}
	return "", false
}

// GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProviderPtr returns the generated
// public WorkerModelProvider enum when one can be derived from the input value.
func GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProviderPtr(value string) *factoryapi.WorkerModelProvider {
	if public, ok := GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProvider(value); ok {
		return &public
	}
	return nil
}

// GeneratedPublicFactoryWorkerModelProviderFromStoredSelection returns the generated
// public WorkerModelProvider enum preferring a canonical stored provider value over
// legacy runner identifiers.
func GeneratedPublicFactoryWorkerModelProviderFromStoredSelection(modelProvider, runnerID string) *factoryapi.WorkerModelProvider {
	if ptr := GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProviderPtr(modelProvider); ptr != nil {
		return ptr
	}
	return GeneratedPublicFactoryWorkerModelProviderFromRunnerOrProviderPtr(runnerID)
}

const (
	publicFactoryWorkstationKindStandard = "STANDARD"
	publicFactoryWorkstationKindRepeater = "REPEATER"
	publicFactoryWorkstationKindCron     = "CRON"
	publicFactoryWorkstationKindPoller   = "POLLER"
)

func normalizePublicWorkstationKind(value string, acceptInternal bool, preserveUnknown bool) string {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "":
		return ""
	case publicFactoryWorkstationKindStandard:
		return publicFactoryWorkstationKindStandard
	case publicFactoryWorkstationKindRepeater:
		return publicFactoryWorkstationKindRepeater
	case publicFactoryWorkstationKindCron:
		return publicFactoryWorkstationKindCron
	case publicFactoryWorkstationKindPoller:
		return publicFactoryWorkstationKindPoller
	default:
		if acceptInternal {
			switch trimmed {
			case string(WorkstationKindStandard):
				return publicFactoryWorkstationKindStandard
			case string(WorkstationKindRepeater):
				return publicFactoryWorkstationKindRepeater
			case string(WorkstationKindCron):
				return publicFactoryWorkstationKindCron
			case string(WorkstationKindPoller):
				return publicFactoryWorkstationKindPoller
			}
		}
		if preserveUnknown {
			return trimmed
		}
		return ""
	}
}

// CanonicalPublicWorkstationKind returns the public factory-config enum value
// for a runtime workstation kind while preserving unknown values verbatim.
func CanonicalPublicWorkstationKind(kind WorkstationKind) string {
	return normalizePublicWorkstationKind(string(kind), true, true)
}

// StrictPublicWorkstationKind canonicalizes supported public workstation kinds and rejects unknown values.
func StrictPublicWorkstationKind(value string) string {
	return normalizePublicWorkstationKind(value, false, false)
}

// PermissivePublicWorkTypeHandlingBehavior canonicalizes supported public work-type handling markers and preserves unknown values.
func PermissivePublicWorkTypeHandlingBehavior(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkTypeHandlingBehaviorAliases, true)
}

// StrictPublicWorkTypeHandlingBehavior canonicalizes supported public work-type handling markers and rejects unknown values.
func StrictPublicWorkTypeHandlingBehavior(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkTypeHandlingBehaviorAliases, false)
}

// GeneratedPublicWorkstationKind returns the generated workstation kind enum.
func GeneratedPublicWorkstationKind(kind WorkstationKind) factoryapi.WorkstationKind {
	return factoryapi.WorkstationKind(CanonicalPublicWorkstationKind(kind))
}

// GeneratedPublicWorkstationKindPtr returns the generated workstation kind enum when non-empty.
func GeneratedPublicWorkstationKindPtr(kind WorkstationKind) *factoryapi.WorkstationKind {
	return generatedPublicFactoryEnumPtr(string(kind), func(value string) factoryapi.WorkstationKind {
		return GeneratedPublicWorkstationKind(WorkstationKind(value))
	})
}
