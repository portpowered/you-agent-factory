package interfaces

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/work"
)

type WorkPayloadLineageProjection = work.WorkPayloadLineageProjection
type WorkPayloadSnapshot = work.WorkPayloadSnapshot
type WorkPayloadSnapshotKind = work.WorkPayloadSnapshotKind
type WorkPayloadContinuity = work.WorkPayloadContinuity
type WorkPayloadResolutionStatus = work.WorkPayloadResolutionStatus
type WorkPayloadRef = work.WorkPayloadRef
type WorkPayloadResolution = work.WorkPayloadResolution

const (
	WorkPayloadSnapshotKindWorkRequest     = work.WorkPayloadSnapshotKindWorkRequest
	WorkPayloadSnapshotKindDispatchOutput  = work.WorkPayloadSnapshotKindDispatchOutput
	WorkPayloadContinuityInitial           = work.WorkPayloadContinuityInitial
	WorkPayloadContinuitySameWorkID        = work.WorkPayloadContinuitySameWorkID
	WorkPayloadContinuityNewDownstreamWork = work.WorkPayloadContinuityNewDownstreamWork
	WorkPayloadResolutionResolved          = work.WorkPayloadResolutionResolved
	WorkPayloadResolutionUnavailable       = work.WorkPayloadResolutionUnavailable
)

func CanonicalChainingTraceIDs(traceIDs []string) []string {
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func PreviousChainingTraceIDsFromTokens(tokens []Token) []string {
	traceIDs := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID))
	}
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func PreviousChainingTraceIDsFromTokenColors(colors []TokenColor) []string {
	traceIDs := make([]string, 0, len(colors))
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		traceIDs = append(traceIDs, firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID))
	}
	return work.CanonicalChainingTraceIDs(traceIDs)
}

func PreviousChainingTraceIDsFromWorkItems(items []FactoryWorkItem) []string {
	return work.PreviousChainingTraceIDsFromWorkItems(items)
}

func CurrentChainingTraceIDFromWorkItems(items []FactoryWorkItem) string {
	return work.CurrentChainingTraceIDFromWorkItems(items)
}

func CurrentChainingTraceIDFromTokens(tokens []Token) string {
	for _, token := range tokens {
		if token.Color.DataType == DataTypeResource || token.Color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
	}
	for _, token := range tokens {
		if token.Color.DataType != DataTypeResource {
			return firstNonEmptyString(token.Color.CurrentChainingTraceID, token.Color.TraceID)
		}
	}
	return ""
}

func ChainingTraceDepthForTokenColor(color TokenColor) int {
	if color.ChainingTraceDepth > 0 {
		return color.ChainingTraceDepth
	}
	if firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID) != "" {
		return 1
	}
	return 0
}

func ChainingTraceDepthForWorkItem(item FactoryWorkItem) int {
	return work.ChainingTraceDepthForWorkItem(item)
}

func ChainingTraceDepthFromTokenColors(colors []TokenColor) int {
	depth := 0
	for _, color := range colors {
		if color.DataType == DataTypeResource {
			continue
		}
		if candidate := ChainingTraceDepthForTokenColor(color); candidate > depth {
			depth = candidate
		}
	}
	if depth > 0 {
		return depth + 1
	}
	if CurrentChainingTraceIDFromTokenColors(colors) != "" {
		return 1
	}
	return 0
}

func CurrentChainingTraceIDFromTokenColors(colors []TokenColor) string {
	for _, color := range colors {
		if color.DataType == DataTypeResource || color.WorkTypeID == SystemTimeWorkTypeID {
			continue
		}
		return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
	}
	for _, color := range colors {
		if color.DataType != DataTypeResource {
			return firstNonEmptyString(color.CurrentChainingTraceID, color.TraceID)
		}
	}
	return ""
}

func sortedStringKeys(values map[string]WorkPayloadRef) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// RunnerExecutionRequest is the shared runner-owned execution request contract
// used by standard orchestration flows.
type RunnerExecutionRequest = ProviderInferenceRequest

// RunnerExecutionResult is the shared runner-owned execution result contract
// used by standard orchestration flows.
type RunnerExecutionResult = InferenceResponse

// RunnerToolExecutionMode describes whether a runner invocation is expected to
// permit tool execution during the request lifecycle.
type RunnerToolExecutionMode string

const (
	// RunnerToolExecutionModeRequired means the orchestration path expects the
	// selected runner to support tool execution for this request.
	RunnerToolExecutionModeRequired RunnerToolExecutionMode = "required"
	// RunnerToolExecutionModeDisabled means the request does not require tool
	// execution support.
	RunnerToolExecutionModeDisabled RunnerToolExecutionMode = "disabled"
)

// RunnerBaselineCapability is a v1 baseline runner behavior that every built-in
// runner must support for standard orchestration participation.
type RunnerBaselineCapability string

const (
	RunnerBaselineCapabilityPromptSubmission RunnerBaselineCapability = "prompt_submission"
	RunnerBaselineCapabilityToolExecution    RunnerBaselineCapability = "tool_execution"
)

var runnerBaselineCapabilitiesV1 = []RunnerBaselineCapability{
	RunnerBaselineCapabilityPromptSubmission,
	RunnerBaselineCapabilityToolExecution,
}

// V1RunnerBaselineCapabilities returns the explicit v1 baseline capability set.
func V1RunnerBaselineCapabilities() []RunnerBaselineCapability {
	return append([]RunnerBaselineCapability(nil), runnerBaselineCapabilitiesV1...)
}

// RunnerOptionalCapability identifies an execution behavior that may be
// supported by some runners without being required for baseline participation.
type RunnerOptionalCapability string

const (
	RunnerOptionalCapabilityImageInput       RunnerOptionalCapability = "image_input"
	RunnerOptionalCapabilitySessionResume    RunnerOptionalCapability = "session_resume"
	RunnerOptionalCapabilityStructuredOutput RunnerOptionalCapability = "structured_output"
	RunnerOptionalCapabilityWorkingDirectory RunnerOptionalCapability = "working_directory"
	RunnerOptionalCapabilityWorktree         RunnerOptionalCapability = "worktree"
)

// RunnerOptionalCapabilityStatus reports whether a runner can satisfy one
// optional capability.
type RunnerOptionalCapabilityStatus string

const (
	RunnerOptionalCapabilityStatusSupported   RunnerOptionalCapabilityStatus = "supported"
	RunnerOptionalCapabilityStatusUnsupported RunnerOptionalCapabilityStatus = "unsupported"
)

// RunnerOptionalCapabilitySupport is the machine-readable capability status
// shape intended for backend, CLI, API, and UI consumption.
type RunnerOptionalCapabilitySupport struct {
	Capability RunnerOptionalCapability       `json:"capability"`
	Status     RunnerOptionalCapabilityStatus `json:"status"`
	Detail     string                         `json:"detail,omitempty"`
}

// RunnerCapabilities describes one runner's baseline and optional capability
// support in a product-surface-friendly shape.
type RunnerCapabilities struct {
	Baseline []RunnerBaselineCapability        `json:"baseline"`
	Optional []RunnerOptionalCapabilitySupport `json:"optional,omitempty"`
}

// NewRunnerCapabilities creates a capability payload with the explicit v1
// baseline and detached optional capability support entries.
func NewRunnerCapabilities(optional ...RunnerOptionalCapabilitySupport) RunnerCapabilities {
	return RunnerCapabilities{
		Baseline: V1RunnerBaselineCapabilities(),
		Optional: cloneRunnerOptionalCapabilitySupport(optional),
	}
}

// RunnerMetadata is the canonical metadata shape that product surfaces can use
// to inspect one runner's identity and capability support.
type RunnerMetadata struct {
	ID           string             `json:"id"`
	DisplayName  string             `json:"display_name,omitempty"`
	Capabilities RunnerCapabilities `json:"capabilities"`
}

func cloneRunnerOptionalCapabilitySupport(values []RunnerOptionalCapabilitySupport) []RunnerOptionalCapabilitySupport {
	if len(values) == 0 {
		return nil
	}
	clone := make([]RunnerOptionalCapabilitySupport, len(values))
	copy(clone, values)
	return clone
}

const (
	RunnerIDCodex     = "codex"
	RunnerIDGemini    = "gemini"
	RunnerIDKiro      = "kiro"
	RunnerIDCursorCLI = "cursor-cli"
	RunnerIDOpenCode  = "opencode"
	RunnerIDPi        = "pi"
	RunnerIDAgy       = "agy"
)

// RunnerSelectionSource reports which configuration layer supplied the runner
// choice that execution should use.
type RunnerSelectionSource string

const (
	RunnerSelectionSourceWorkstation    RunnerSelectionSource = "workstation"
	RunnerSelectionSourceFactory        RunnerSelectionSource = "factory"
	RunnerSelectionSourceLegacyProvider RunnerSelectionSource = "legacy_provider"
	RunnerSelectionSourceDefault        RunnerSelectionSource = "default"
)

// ResolvedRunnerSelection is the canonical runner-selection result used by
// backend runtime code before dispatch starts.
type ResolvedRunnerSelection struct {
	RunnerID string                `json:"runner_id,omitempty"`
	Source   RunnerSelectionSource `json:"source,omitempty"`
}

var builtInRunnerMetadata = map[string]RunnerMetadata{
	RunnerIDCodex: {
		ID:          RunnerIDCodex,
		DisplayName: "Codex",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusSupported, Detail: "factory-managed git worktree preparation under the factory root"},
		),
	},
	RunnerIDGemini: {
		ID:          RunnerIDGemini,
		DisplayName: "Gemini",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDKiro: {
		ID:          RunnerIDKiro,
		DisplayName: "Kiro",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDCursorCLI: {
		ID:          RunnerIDCursorCLI,
		DisplayName: "Cursor CLI",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDOpenCode: {
		ID:          RunnerIDOpenCode,
		DisplayName: "OpenCode",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDPi: {
		ID:          RunnerIDPi,
		DisplayName: "Pi",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDAgy: {
		ID:          RunnerIDAgy,
		DisplayName: "Agy",
		Capabilities: NewRunnerCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
}

// BuiltInRunnerMetadata returns the metadata for one stable built-in runner ID.
func BuiltInRunnerMetadata(id string) (RunnerMetadata, bool) {
	metadata, ok := builtInRunnerMetadata[NormalizeRunnerID(id)]
	if !ok {
		return RunnerMetadata{}, false
	}
	return metadata, true
}

// IsBuiltInRunnerID reports whether a runner ID is one of the stable built-ins.
func IsBuiltInRunnerID(id string) bool {
	_, ok := BuiltInRunnerMetadata(id)
	return ok
}

// ResolveOpenCodeAgent returns the configured OpenCode agent profile for one
// dispatch using workstation override precedence over the worker default.
func ResolveOpenCodeAgent(workstationAgent, workerAgent string) string {
	if agent := strings.TrimSpace(workstationAgent); agent != "" {
		return agent
	}
	return strings.TrimSpace(workerAgent)
}

// ValidateOpenCodeAgentForRunnerSelection reports a configuration error when a
// non-empty OpenCode agent profile is configured for a dispatch that will not
// use the OpenCode runner.
func ValidateOpenCodeAgentForRunnerSelection(workstationAgent, workerAgent string, selection ResolvedRunnerSelection) error {
	agent := ResolveOpenCodeAgent(workstationAgent, workerAgent)
	if agent == "" {
		return nil
	}
	runnerID := NormalizeRunnerID(selection.RunnerID)
	if runnerID == RunnerIDOpenCode {
		return nil
	}
	return fmt.Errorf(
		"openCodeAgent %q requires runner %q, resolved runner %q",
		agent,
		RunnerIDOpenCode,
		runnerID,
	)
}

// ResolveRunnerSelection applies the v1 precedence rules for backend runtime
// runner choice: workstation override, then factory override, then legacy
// worker modelProvider compatibility, then the codex default.
func ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider string) ResolvedRunnerSelection {
	if runner := NormalizeRunnerID(workstationRunner); runner != "" {
		return ResolvedRunnerSelection{RunnerID: runner, Source: RunnerSelectionSourceWorkstation}
	}
	if runner := NormalizeRunnerID(factoryRunner); runner != "" {
		return ResolvedRunnerSelection{RunnerID: runner, Source: RunnerSelectionSourceFactory}
	}
	if runner := NormalizeRunnerID(workerModelProvider); IsBuiltInRunnerID(runner) {
		return ResolvedRunnerSelection{RunnerID: runner, Source: RunnerSelectionSourceLegacyProvider}
	}
	return ResolvedRunnerSelection{RunnerID: RunnerIDCodex, Source: RunnerSelectionSourceDefault}
}

// NormalizeRunnerID trims operator-supplied runner IDs into the canonical
// lowercase form used by the registry and runtime selection logic.
func NormalizeRunnerID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
