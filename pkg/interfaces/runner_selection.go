package interfaces

import "strings"

const (
	RunnerIDCodex     = "codex"
	RunnerIDGemini    = "gemini"
	RunnerIDKiro      = "kiro"
	RunnerIDCursorCLI = "cursor-cli"
	RunnerIDOpenCode  = "opencode"
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
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported, Detail: "codex ignores workstation worktree selection in v1"},
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
		ID:           RunnerIDKiro,
		DisplayName:  "Kiro",
		Capabilities: NewRunnerCapabilities(),
	},
	RunnerIDCursorCLI: {
		ID:           RunnerIDCursorCLI,
		DisplayName:  "Cursor CLI",
		Capabilities: NewRunnerCapabilities(),
	},
	RunnerIDOpenCode: {
		ID:           RunnerIDOpenCode,
		DisplayName:  "OpenCode",
		Capabilities: NewRunnerCapabilities(),
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
