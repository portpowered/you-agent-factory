package interfaces

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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

var internalModelProviderToPublicWorkerModelProvider = map[ModelProvider]factoryapi.WorkerModelProvider{
	ModelProviderClaude:   factoryapi.WorkerModelProviderClaude,
	ModelProviderCodex:    factoryapi.WorkerModelProviderCodex,
	ModelProviderCursor:   factoryapi.WorkerModelProviderCursor,
	ModelProviderGemini:   factoryapi.WorkerModelProviderGemini,
	ModelProviderKiro:     factoryapi.WorkerModelProviderKiro,
	ModelProviderOpenCode: factoryapi.WorkerModelProviderOpenCode,
	ModelProviderPi:       factoryapi.WorkerModelProviderPi,
	ModelProviderAgy:      factoryapi.WorkerModelProviderAgy,
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
	return IsProviderBackedWorkerType(workerType) && locality == ModelLocalityLocal
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
	return StrictPublicFactoryWorkstationType(workstationType) == WorkstationTypeScript
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
	RunnerIDCodex:     RunnerIDCodex,
	RunnerIDGemini:    RunnerIDGemini,
	RunnerIDKiro:      RunnerIDKiro,
	RunnerIDCursorCLI: RunnerIDCursorCLI,
	RunnerIDOpenCode:  RunnerIDOpenCode,
	RunnerIDPi:        RunnerIDPi,
}

// WorkstationOutcomeFormatDecisionEnvelope routes agent output through the
// reviewer/checker JSON decision envelope instead of stop-token parsing.
const WorkstationOutcomeFormatDecisionEnvelope = "decision-envelope"

var publicFactoryWorkstationOutcomeFormatAliases = map[string]string{
	WorkstationOutcomeFormatDecisionEnvelope: WorkstationOutcomeFormatDecisionEnvelope,
}

var publicFactoryRunnerSelectionSourceAliases = map[string]string{
	string(RunnerSelectionSourceWorkstation):    string(RunnerSelectionSourceWorkstation),
	string(RunnerSelectionSourceFactory):        string(RunnerSelectionSourceFactory),
	string(RunnerSelectionSourceLegacyProvider): string(RunnerSelectionSourceLegacyProvider),
	string(RunnerSelectionSourceDefault):        string(RunnerSelectionSourceDefault),
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

func generatedPublicFactoryEnumPtr[T ~string](value string, convert func(string) T) *T {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := convert(value)
	return &enumValue
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

// StrictPublicFactoryWorkerModelProvider canonicalizes supported public worker model providers and rejects unknown values.
func StrictPublicFactoryWorkerModelProvider(value string) string {
	return normalizePublicFactoryEnumValue(value, publicFactoryWorkerModelProviderAliases, false)
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

// AcceptedPublicWorkerModelProviderSummary returns canonical provider names and
// representative public aliases for operator-facing validation errors.
func AcceptedPublicWorkerModelProviderSummary() string {
	return "accepted canonical providers: CLAUDE, CODEX, CURSOR, GEMINI, KIRO, OPENCODE, PI, AGY; " +
		"accepted aliases include codex, claude, gemini, kiro-cli, opencode, pi, agy, agent, cursor, anthropic, and openai"
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

// IsPollerRunPublicWorkstationType reports whether a public workstation type value
// denotes poller-run behavior, including empty type on poller-kind workstations.
func IsPollerRunPublicWorkstationType(value string, kind WorkstationKind) bool {
	if PermissivePublicFactoryWorkstationType(value) == WorkstationTypePoller {
		return true
	}
	return strings.TrimSpace(value) == "" && kind == WorkstationKindPoller
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

// GeneratedPublicFactoryWorkerType returns the generated worker type enum.
func GeneratedPublicFactoryWorkerType(value string) factoryapi.WorkerType {
	return factoryapi.WorkerType(PublicWorkerTypeFromInternalRuntime(value))
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
	return factoryapi.WorkstationType(PublicWorkstationTypeFromInternalRuntime(value, "", ""))
}

// GeneratedPublicFactoryWorkstationTypeFromWorkstation returns the generated workstation
// type enum using worker and scheduling context for behavior-aware projection.
func GeneratedPublicFactoryWorkstationTypeFromWorkstation(workstation FactoryWorkstationConfig, workerType string) factoryapi.WorkstationType {
	return factoryapi.WorkstationType(PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind))
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

// GeneratedPublicFactoryWorkstationOutcomeFormat returns the generated workstation outcome format enum.
func GeneratedPublicFactoryWorkstationOutcomeFormat(value string) factoryapi.WorkstationOutcomeFormat {
	return factoryapi.WorkstationOutcomeFormat(PermissivePublicFactoryWorkstationOutcomeFormat(value))
}

// GeneratedPublicFactoryWorkstationOutcomeFormatPtr returns the generated workstation outcome format enum when non-empty.
func GeneratedPublicFactoryWorkstationOutcomeFormatPtr(value string) *factoryapi.WorkstationOutcomeFormat {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryWorkstationOutcomeFormat)
}

// GeneratedPublicFactoryRunnerSelectionSource returns the generated runner selection source enum.
func GeneratedPublicFactoryRunnerSelectionSource(value string) factoryapi.RunnerSelectionSource {
	return factoryapi.RunnerSelectionSource(PermissivePublicFactoryRunnerSelectionSource(value))
}

// GeneratedPublicFactoryRunnerSelectionSourcePtr returns the generated runner selection source enum when non-empty.
func GeneratedPublicFactoryRunnerSelectionSourcePtr(value string) *factoryapi.RunnerSelectionSource {
	return generatedPublicFactoryEnumPtr(value, GeneratedPublicFactoryRunnerSelectionSource)
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
