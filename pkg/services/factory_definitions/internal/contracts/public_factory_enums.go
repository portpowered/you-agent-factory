package factorycontracts

import (
	"strings"

	catalogresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/taxonomy"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ModelProvider is the Factory Definitions-owned authored provider command.
// Runtime services map this value into their own provider contracts.
type ModelProvider string

const (
	ModelProviderClaude   ModelProvider = "claude"
	ModelProviderCodex    ModelProvider = "codex"
	ModelProviderGemini   ModelProvider = "gemini"
	ModelProviderKiro     ModelProvider = "kiro-cli"
	ModelProviderCursor   ModelProvider = "agent"
	ModelProviderOpenCode ModelProvider = "opencode"
	ModelProviderPi       ModelProvider = "pi"
	ModelProviderAgy      ModelProvider = "agy"
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
		ModelProviderPi,
		ModelProviderAgy,
	}
}

var internalModelProviderToPublicWorkerModelProvider = map[ModelProvider]string{
	ModelProviderClaude:   publicFactoryWorkerModelProviderClaude,
	ModelProviderCodex:    publicFactoryWorkerModelProviderCodex,
	ModelProviderCursor:   publicFactoryWorkerModelProviderCursor,
	ModelProviderGemini:   publicFactoryWorkerModelProviderGemini,
	ModelProviderKiro:     publicFactoryWorkerModelProviderKiro,
	ModelProviderOpenCode: publicFactoryWorkerModelProviderOpenCode,
	ModelProviderPi:       publicFactoryWorkerModelProviderPi,
	ModelProviderAgy:      publicFactoryWorkerModelProviderAgy,
}

// PublicWorkerModelProviderFromInternal maps a canonical provider command to
// the authored public Factory vocabulary without depending on a transport type.
func PublicWorkerModelProviderFromInternal(provider ModelProvider) (string, bool) {
	public, ok := internalModelProviderToPublicWorkerModelProvider[provider]
	return public, ok
}

// InternalModelProviderFromPublicWorkerModelProvider maps authored public
// Factory vocabulary to the canonical provider command.
func InternalModelProviderFromPublicWorkerModelProvider(value string) (ModelProvider, bool) {
	switch StrictPublicFactoryWorkerModelProvider(value) {
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
	case publicFactoryWorkerModelProviderPi:
		return ModelProviderPi, true
	case publicFactoryWorkerModelProviderAgy:
		return ModelProviderAgy, true
	default:
		return "", false
	}
}

var publicFactoryWorkerTypeAliases = map[string]string{
	WorkerTypeInference: WorkerTypeInference,
	WorkerTypeAgent:     WorkerTypeAgent,
	WorkerTypeScript:    WorkerTypeScript,
	WorkerTypePoller:    WorkerTypePoller,
	WorkerTypeModel:     WorkerTypeInference,
	WorkerTypeHosted:    WorkerTypePoller,
}

// IsInferenceWorkerType reports whether workerType is an accepted inference-worker taxonomy value.
func IsInferenceWorkerType(workerType string) bool {
	switch StrictPublicFactoryWorkerType(workerType) {
	case WorkerTypeInference, WorkerTypeModel:
		return true
	default:
		return false
	}
}

// IsAgentWorkerType reports whether workerType is an accepted agent-worker taxonomy value.
func IsAgentWorkerType(workerType string) bool {
	return StrictPublicFactoryWorkerType(workerType) == WorkerTypeAgent
}

// IsProviderBackedWorkerType reports whether workerType dispatches through the
// provider-backed agent executor path, including inference-worker aliases.
func IsProviderBackedWorkerType(workerType string) bool {
	return IsInferenceWorkerType(workerType) || IsAgentWorkerType(workerType)
}

// UsesModelhostLease reports whether local model calls for this worker should
// acquire and release modelhost inferencer leases.
func UsesModelhostLease(workerType string, locality string) bool {
	return IsProviderBackedWorkerType(workerType) && locality == workerconfig.ModelLocalityLocal
}

// IsScriptWorkerType reports whether workerType is an accepted script-worker taxonomy value.
func IsScriptWorkerType(workerType string) bool {
	return StrictPublicFactoryWorkerType(workerType) == WorkerTypeScript
}

// IsPollerWorkerType reports whether workerType is an accepted poller-worker taxonomy
// value, including the legacy HOSTED_WORKER compatibility alias.
func IsPollerWorkerType(workerType string) bool {
	switch StrictPublicFactoryWorkerType(workerType) {
	case WorkerTypePoller, WorkerTypeHosted:
		return true
	default:
		return false
	}
}

// ProjectWorkerBehaviorClass maps accepted worker taxonomy values to the runtime
// behavior class used for compatibility projection during the migration window.
func ProjectWorkerBehaviorClass(workerType string) string {
	switch StrictPublicFactoryWorkerType(workerType) {
	case WorkerTypeInference, WorkerTypeModel:
		return WorkerTypeInference
	case WorkerTypeAgent:
		return WorkerTypeAgent
	case WorkerTypeScript:
		return WorkerTypeScript
	case WorkerTypePoller, WorkerTypeHosted:
		return WorkerTypePoller
	default:
		return ""
	}
}

var publicFactoryWorkerModelProviderAliases = map[string]string{
	publicFactoryWorkerModelProviderClaude:   publicFactoryWorkerModelProviderClaude,
	publicFactoryWorkerModelProviderCodex:    publicFactoryWorkerModelProviderCodex,
	publicFactoryWorkerModelProviderCursor:   publicFactoryWorkerModelProviderCursor,
	publicFactoryWorkerModelProviderGemini:   publicFactoryWorkerModelProviderGemini,
	publicFactoryWorkerModelProviderKiro:     publicFactoryWorkerModelProviderKiro,
	publicFactoryWorkerModelProviderOpenCode: publicFactoryWorkerModelProviderOpenCode,
	publicFactoryWorkerModelProviderPi:       publicFactoryWorkerModelProviderPi,
	publicFactoryWorkerModelProviderAgy:      publicFactoryWorkerModelProviderAgy,
}

var publicFactoryWorkerProviderAliases = map[string]string{
	publicFactoryWorkerProviderScriptWrap: publicFactoryWorkerProviderScriptWrap,
}

var publicFactoryHostedWorkerProviderAliases = map[string]string{
	HostedWorkerProviderLinear: HostedWorkerProviderLinear,
}

var publicFactoryWorkerModelLocalityAliases = map[string]string{
	workerconfig.ModelLocalityLocal: workerconfig.ModelLocalityLocal,
	workerconfig.ModelLocalityCloud: workerconfig.ModelLocalityCloud,
}

var publicFactoryWorkerModelOperationContentTypeAliases = map[string]string{
	workerconfig.ModelOperationContentTypeText:   workerconfig.ModelOperationContentTypeText,
	workerconfig.ModelOperationContentTypeImage:  workerconfig.ModelOperationContentTypeImage,
	workerconfig.ModelOperationContentTypeAudio:  workerconfig.ModelOperationContentTypeAudio,
	workerconfig.ModelOperationContentTypeJSON:   workerconfig.ModelOperationContentTypeJSON,
	workerconfig.ModelOperationContentTypeBinary: workerconfig.ModelOperationContentTypeBinary,
}

var publicFactoryResourceTypeAliases = map[string]string{
	catalogresource.TypeModel:          catalogresource.TypeModel,
	catalogresource.TypeProviderQuota:  catalogresource.TypeProviderQuota,
	catalogresource.TypeInvocationSlot: catalogresource.TypeInvocationSlot,
}

var publicFactoryWorkstationTypeAliases = map[string]string{
	WorkstationTypeInference: WorkstationTypeInference,
	WorkstationTypeAgent:     WorkstationTypeAgent,
	WorkstationTypeScript:    WorkstationTypeScript,
	WorkstationTypePoller:    WorkstationTypePoller,
	WorkstationTypeInvoke:    WorkstationTypeInference,
	WorkstationTypeModel:     WorkstationTypeAgent,
	WorkstationTypeClassify:  WorkstationTypeClassify,
	WorkstationTypeLogical:   WorkstationTypeLogical,
}

// IsInferenceRunWorkstationType reports whether workstationType is an accepted
// inference-run taxonomy value, including the legacy MODEL_INVOKE compatibility alias.
func IsInferenceRunWorkstationType(workstationType string) bool {
	switch StrictPublicFactoryWorkstationType(workstationType) {
	case WorkstationTypeInference, WorkstationTypeInvoke:
		return true
	default:
		return false
	}
}

// IsAgentRunWorkstationType reports whether workstationType is an accepted agent-run
// taxonomy value, including the legacy MODEL_WORKSTATION compatibility alias.
func IsAgentRunWorkstationType(workstationType string) bool {
	switch StrictPublicFactoryWorkstationType(workstationType) {
	case WorkstationTypeAgent, WorkstationTypeModel:
		return true
	default:
		return false
	}
}

// IsScriptRunWorkstationType reports whether workstationType is an accepted script-run taxonomy value.
func IsScriptRunWorkstationType(workstationType string) bool {
	return workertaxonomy.IsScriptRunWorkstationType(workstationType)
}

// IsPollerRunWorkstationType reports whether workstationType and workstation kind
// denote poller-run behavior, including explicit POLLER_RUN values and legacy
// poller-kind workstations without an explicit type.
func IsPollerRunWorkstationType(workstationType string, kind WorkstationKind) bool {
	switch StrictPublicFactoryWorkstationType(workstationType) {
	case WorkstationTypePoller:
		return true
	default:
		return strings.TrimSpace(workstationType) == "" && kind == WorkstationKindPoller
	}
}

// ProjectWorkstationBehaviorClass maps accepted workstation taxonomy values to the
// runtime behavior class used for compatibility projection during the migration window.
func ProjectWorkstationBehaviorClass(workstationType string, kind WorkstationKind) string {
	switch StrictPublicFactoryWorkstationType(workstationType) {
	case WorkstationTypeInference, WorkstationTypeInvoke:
		return WorkstationTypeInference
	case WorkstationTypeAgent, WorkstationTypeModel:
		return WorkstationTypeAgent
	case WorkstationTypeScript:
		return WorkstationTypeScript
	case WorkstationTypePoller:
		return WorkstationTypePoller
	default:
		if strings.TrimSpace(workstationType) == "" && kind == WorkstationKindPoller {
			return WorkstationTypePoller
		}
		return ""
	}
}

var publicFactoryRunnerIDAliases = map[string]string{
	workerexecution.RunnerIDCodex:     workerexecution.RunnerIDCodex,
	workerexecution.RunnerIDGemini:    workerexecution.RunnerIDGemini,
	workerexecution.RunnerIDKiro:      workerexecution.RunnerIDKiro,
	workerexecution.RunnerIDCursorCLI: workerexecution.RunnerIDCursorCLI,
	workerexecution.RunnerIDOpenCode:  workerexecution.RunnerIDOpenCode,
	workerexecution.RunnerIDPi:        workerexecution.RunnerIDPi,
}

// WorkstationOutcomeFormatDecisionEnvelope routes agent output through the
// reviewer/checker JSON decision envelope instead of stop-token parsing.
const WorkstationOutcomeFormatDecisionEnvelope = "decision-envelope"

var publicFactoryWorkstationOutcomeFormatAliases = map[string]string{
	WorkstationOutcomeFormatDecisionEnvelope: WorkstationOutcomeFormatDecisionEnvelope,
}

var publicFactoryRunnerSelectionSourceAliases = map[string]string{
	string(workerexecution.RunnerSelectionSourceWorkstation):    string(workerexecution.RunnerSelectionSourceWorkstation),
	string(workerexecution.RunnerSelectionSourceFactory):        string(workerexecution.RunnerSelectionSourceFactory),
	string(workerexecution.RunnerSelectionSourceLegacyProvider): string(workerexecution.RunnerSelectionSourceLegacyProvider),
	string(workerexecution.RunnerSelectionSourceDefault):        string(workerexecution.RunnerSelectionSourceDefault),
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
	publicFactoryWorkerModelProviderPi       = "PI"
	publicFactoryWorkerModelProviderAgy      = "AGY"
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
	"PI":           publicFactoryWorkerModelProviderPi,
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
	"pi":           publicFactoryWorkerModelProviderPi,
	"agy":          publicFactoryWorkerModelProviderAgy,
	"AGY":          publicFactoryWorkerModelProviderAgy,
	"antigravity":  publicFactoryWorkerModelProviderAgy,
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

// InternalRuntimeWorkerTypeFromPublic maps canonical public worker types to the
// internal runtime identifiers used by validation and execution.
func InternalRuntimeWorkerTypeFromPublic(value string) string {
	switch PermissivePublicFactoryWorkerType(value) {
	case WorkerTypeInference:
		return WorkerTypeModel
	case WorkerTypePoller:
		return WorkerTypeHosted
	case WorkerTypeAgent, WorkerTypeScript:
		return PermissivePublicFactoryWorkerType(value)
	default:
		return PermissivePublicFactoryWorkerType(value)
	}
}

// PublicWorkerTypeFromInternalRuntime maps internal runtime worker identifiers
// to the preferred public taxonomy names.
func PublicWorkerTypeFromInternalRuntime(value string) string {
	switch strings.TrimSpace(value) {
	case WorkerTypeModel:
		return WorkerTypeInference
	case WorkerTypeHosted:
		return WorkerTypePoller
	case WorkerTypeInference, WorkerTypeAgent, WorkerTypeScript, WorkerTypePoller:
		return strings.TrimSpace(value)
	default:
		return PermissivePublicFactoryWorkerType(value)
	}
}

// IsPollerWorkerPublicType reports whether a public worker type value denotes
// poller-worker behavior, including the legacy HOSTED_WORKER alias.
func IsPollerWorkerPublicType(value string) bool {
	return PermissivePublicFactoryWorkerType(value) == WorkerTypePoller
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

// PublicWorkerModelProviderFromInternalRuntime maps runtime provider aliases to
// authored Factory vocabulary while preserving unknown values.
func PublicWorkerModelProviderFromInternalRuntime(value string) string {
	return normalizePublicFactoryEnumValue(value, internalFactoryWorkerModelProviderAliases, true)
}

// StrictPublicFactoryWorkerModelProvider canonicalizes built-in aliases and
// preserves extension identities that use the provider contract's canonical
// syntax. Registry membership is deliberately outside this authored boundary.
func StrictPublicFactoryWorkerModelProvider(value string) string {
	trimmed := strings.TrimSpace(value)
	if canonical := normalizePublicFactoryEnumValue(trimmed, internalFactoryWorkerModelProviderAliases, false); canonical != "" {
		return canonical
	}
	if trimmed != value {
		return ""
	}
	if err := workers.ProviderIdentity(value).Validate(); err != nil {
		return ""
	}
	return value
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
	if canonical := StrictPublicFactoryWorkerModelProvider(trimmed); canonical != "" {
		return canonical, true
	}
	return "", false
}

// CanonicalizeReasoningEffort trims and lowercases a supported worker
// reasoning effort. The empty value is valid when the setting is optional.
func CanonicalizeReasoningEffort(value string) (string, bool) {
	canonical := strings.ToLower(strings.TrimSpace(value))
	switch canonical {
	case "", "minimal", "low", "medium", "high":
		return canonical, true
	default:
		return "", false
	}
}

// AcceptedPublicWorkerModelProviderSummary describes the open provider identity
// syntax and representative compatibility aliases for validation errors.
func AcceptedPublicWorkerModelProviderSummary() string {
	return "use a canonical lowercase provider identity (1-128 bytes; lowercase letters, digits, dots, or hyphens), " +
		"or a supported built-in identity or legacy alias"
}

// PermissivePublicFactoryWorkerProvider canonicalizes supported public worker providers and preserves unknown values.
func PermissivePublicFactoryWorkerProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerProviderAliases, true)
}

// PublicWorkerProviderFromInternalRuntime maps runtime executor aliases to
// authored Factory vocabulary while preserving unknown values.
func PublicWorkerProviderFromInternalRuntime(value string) string {
	return normalizePublicFactoryEnumValue(value, internalFactoryWorkerProviderAliases, true)
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

// InternalRuntimeWorkstationTypeFromPublic maps canonical public workstation types to
// the internal runtime identifiers used by validation and execution.
func InternalRuntimeWorkstationTypeFromPublic(value string) string {
	switch PermissivePublicFactoryWorkstationType(value) {
	case WorkstationTypeInference:
		return WorkstationTypeInvoke
	case WorkstationTypeAgent, WorkstationTypeScript:
		return WorkstationTypeModel
	case WorkstationTypePoller:
		return ""
	case WorkstationTypeLogical, WorkstationTypeClassify:
		return PermissivePublicFactoryWorkstationType(value)
	default:
		return PermissivePublicFactoryWorkstationType(value)
	}
}

// PublicWorkstationTypeFromInternalRuntime maps internal runtime workstation identifiers
// to the preferred public taxonomy names.
func PublicWorkstationTypeFromInternalRuntime(workstationType, workerType string, kind WorkstationKind) string {
	switch strings.TrimSpace(workstationType) {
	case WorkstationTypeInvoke:
		return WorkstationTypeInference
	case WorkstationTypeModel:
		if PermissivePublicFactoryWorkerType(workerType) == WorkerTypeScript {
			return WorkstationTypeScript
		}
		return WorkstationTypeAgent
	case WorkstationTypeLogical, WorkstationTypeClassify:
		return strings.TrimSpace(workstationType)
	case "":
		if kind == WorkstationKindPoller {
			return WorkstationTypePoller
		}
		return ""
	case WorkstationTypeInference, WorkstationTypeAgent, WorkstationTypeScript, WorkstationTypePoller:
		return strings.TrimSpace(workstationType)
	default:
		return PermissivePublicFactoryWorkstationType(workstationType)
	}
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

// PermissivePublicFactoryWorkstationOutcomeFormat canonicalizes supported public workstation outcome formats and preserves unknown values.
func PermissivePublicFactoryWorkstationOutcomeFormat(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationOutcomeFormatAliases, true)
}

// StrictPublicFactoryWorkstationOutcomeFormat canonicalizes supported public workstation outcome formats and rejects unknown values.
func StrictPublicFactoryWorkstationOutcomeFormat(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkstationOutcomeFormatAliases, false)
}

// PermissivePublicFactoryRunnerSelectionSource canonicalizes supported public runner selection sources and preserves unknown values.
func PermissivePublicFactoryRunnerSelectionSource(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryRunnerSelectionSourceAliases, true)
}

// StrictPublicFactoryRunnerSelectionSource canonicalizes supported public runner selection sources and rejects unknown values.
func StrictPublicFactoryRunnerSelectionSource(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryRunnerSelectionSourceAliases, false)
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
