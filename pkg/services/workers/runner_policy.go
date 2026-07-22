package workers

import (
	"fmt"
	"strings"
)

var baselineV1 = []RunnerBaselineCapability{
	RunnerBaselineCapabilityPromptSubmission,
	RunnerBaselineCapabilityToolExecution,
}

func V1BaselineCapabilities() []RunnerBaselineCapability {
	return append([]RunnerBaselineCapability(nil), baselineV1...)
}

func NewCapabilities(optional ...RunnerOptionalCapabilitySupport) RunnerCapabilities {
	return RunnerCapabilities{
		Baseline: V1BaselineCapabilities(),
		Optional: append([]RunnerOptionalCapabilitySupport(nil), optional...),
	}
}

// TODO: we should convert this into a config file that we can just export out to customers, so that they can know, :
// What is the agent interfaces that we support
// We should make the interfaces for implementing a new worker more obvious.
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
	RunnerIDGemini: {
		ID:          RunnerIDGemini,
		DisplayName: "Gemini",
		Capabilities: NewCapabilities(
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
		Capabilities: NewCapabilities(
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
		Capabilities: NewCapabilities(
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
		Capabilities: NewCapabilities(
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
		Capabilities: NewCapabilities(
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
