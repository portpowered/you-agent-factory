// Package runner owns built-in runner capabilities and selection policy.
package runner

import (
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

var baselineV1 = []workerexecution.RunnerBaselineCapability{
	workerexecution.RunnerBaselineCapabilityPromptSubmission,
	workerexecution.RunnerBaselineCapabilityToolExecution,
}

func V1BaselineCapabilities() []workerexecution.RunnerBaselineCapability {
	return append([]workerexecution.RunnerBaselineCapability(nil), baselineV1...)
}

func NewCapabilities(optional ...workerexecution.RunnerOptionalCapabilitySupport) workerexecution.RunnerCapabilities {
	return workerexecution.RunnerCapabilities{
		Baseline: V1BaselineCapabilities(),
		Optional: append([]workerexecution.RunnerOptionalCapabilitySupport(nil), optional...),
	}
}

var builtInRunnerMetadata = map[string]workerexecution.RunnerMetadata{
	workerexecution.RunnerIDCodex: {
		ID:          workerexecution.RunnerIDCodex,
		DisplayName: "Codex",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusSupported, Detail: "factory-managed git worktree preparation under the factory root"},
		),
	},
	workerexecution.RunnerIDGemini: {
		ID:          workerexecution.RunnerIDGemini,
		DisplayName: "Gemini",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	workerexecution.RunnerIDKiro: {
		ID:          workerexecution.RunnerIDKiro,
		DisplayName: "Kiro",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	workerexecution.RunnerIDCursorCLI: {
		ID:          workerexecution.RunnerIDCursorCLI,
		DisplayName: "Cursor CLI",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	workerexecution.RunnerIDOpenCode: {
		ID:          workerexecution.RunnerIDOpenCode,
		DisplayName: "OpenCode",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	workerexecution.RunnerIDPi: {
		ID:          workerexecution.RunnerIDPi,
		DisplayName: "Pi",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
		),
	},
	workerexecution.RunnerIDAgy: {
		ID:          workerexecution.RunnerIDAgy,
		DisplayName: "Agy",
		Capabilities: NewCapabilities(
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityImageInput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilitySessionResume, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityStructuredOutput, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorkingDirectory, Status: workerexecution.RunnerOptionalCapabilityStatusSupported},
			workerexecution.RunnerOptionalCapabilitySupport{Capability: workerexecution.RunnerOptionalCapabilityWorktree, Status: workerexecution.RunnerOptionalCapabilityStatusUnsupported},
		),
	},
}

// BuiltInRunnerMetadata returns the metadata for one stable built-in runner ID.
func BuiltInRunnerMetadata(id string) (workerexecution.RunnerMetadata, bool) {
	metadata, ok := builtInRunnerMetadata[NormalizeRunnerID(id)]
	if !ok {
		return workerexecution.RunnerMetadata{}, false
	}
	metadata.Capabilities.Baseline = append([]workerexecution.RunnerBaselineCapability(nil), metadata.Capabilities.Baseline...)
	metadata.Capabilities.Optional = append([]workerexecution.RunnerOptionalCapabilitySupport(nil), metadata.Capabilities.Optional...)
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
func ValidateOpenCodeAgentForRunnerSelection(workstationAgent, workerAgent string, selection workerexecution.ResolvedRunnerSelection) error {
	agent := ResolveOpenCodeAgent(workstationAgent, workerAgent)
	if agent == "" {
		return nil
	}
	runnerID := NormalizeRunnerID(selection.RunnerID)
	if runnerID == workerexecution.RunnerIDOpenCode {
		return nil
	}
	return fmt.Errorf(
		"openCodeAgent %q requires runner %q, resolved runner %q",
		agent,
		workerexecution.RunnerIDOpenCode,
		runnerID,
	)
}

// ResolveRunnerSelection applies the v1 precedence rules for backend runtime
// runner choice: workstation override, then factory override, then legacy
// worker modelProvider compatibility, then the codex default.
func ResolveRunnerSelection(workstationRunner, factoryRunner, workerModelProvider string) workerexecution.ResolvedRunnerSelection {
	if runner := NormalizeRunnerID(workstationRunner); runner != "" {
		return workerexecution.ResolvedRunnerSelection{RunnerID: runner, Source: workerexecution.RunnerSelectionSourceWorkstation}
	}
	if runner := NormalizeRunnerID(factoryRunner); runner != "" {
		return workerexecution.ResolvedRunnerSelection{RunnerID: runner, Source: workerexecution.RunnerSelectionSourceFactory}
	}
	if runner := NormalizeRunnerID(workerModelProvider); IsBuiltInRunnerID(runner) {
		return workerexecution.ResolvedRunnerSelection{RunnerID: runner, Source: workerexecution.RunnerSelectionSourceLegacyProvider}
	}
	return workerexecution.ResolvedRunnerSelection{RunnerID: workerexecution.RunnerIDCodex, Source: workerexecution.RunnerSelectionSourceDefault}
}

// NormalizeRunnerID trims operator-supplied runner IDs into the canonical
// lowercase form used by the registry and runtime selection logic.
func NormalizeRunnerID(id string) string {
	return strings.ToLower(strings.TrimSpace(id))
}
