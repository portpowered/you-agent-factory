package workers

import "strings"

var baselineV1RunnerCapabilities = []RunnerBaselineCapability{
	RunnerBaselineCapabilityPromptSubmission,
	RunnerBaselineCapabilityToolExecution,
}

// V1BaselineCapabilities returns the v1 baseline runner capability set.
func V1BaselineCapabilities() []RunnerBaselineCapability {
	return append([]RunnerBaselineCapability(nil), baselineV1RunnerCapabilities...)
}

// NewCapabilities assembles one runner capability view from optional supports.
func NewCapabilities(optional ...RunnerOptionalCapabilitySupport) RunnerCapabilities {
	return RunnerCapabilities{
		Baseline: V1BaselineCapabilities(),
		Optional: append([]RunnerOptionalCapabilitySupport(nil), optional...),
	}
}

var builtInRunnerMetadata = map[string]RunnerMetadata{
	RunnerIDCodex: {
		ID:          RunnerIDCodex,
		DisplayName: "Codex",
		Capabilities: NewCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusSupported, Detail: "factory-managed git worktree preparation under the factory root"},
		),
	},
	RunnerIDClaude: {
		ID:          RunnerIDClaude,
		DisplayName: "Claude Code",
		Capabilities: NewCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusSupported},
		),
	},
	RunnerIDCursorCLI: {
		ID:          RunnerIDCursorCLI,
		DisplayName: "Cursor CLI",
		Capabilities: NewCapabilities(
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityImageInput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilitySessionResume, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityStructuredOutput, Status: RunnerOptionalCapabilityStatusUnsupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorkingDirectory, Status: RunnerOptionalCapabilityStatusSupported},
			RunnerOptionalCapabilitySupport{Capability: RunnerOptionalCapabilityWorktree, Status: RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	RunnerIDAntigravity: {
		ID:          RunnerIDAntigravity,
		DisplayName: "Antigravity",
		Capabilities: NewCapabilities(
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
	metadata.Capabilities.Baseline = append([]RunnerBaselineCapability(nil), metadata.Capabilities.Baseline...)
	metadata.Capabilities.Optional = append([]RunnerOptionalCapabilitySupport(nil), metadata.Capabilities.Optional...)
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
