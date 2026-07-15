package workflowpolicy

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workerrunner "github.com/portpowered/infinite-you/pkg/workers/runner"
)

var (
	knownRouteProfiles = map[string]struct{}{
		"scout":       {},
		"reviewer":    {},
		"security":    {},
		"synthesizer": {},
	}
	knownSandboxModes = map[string]struct{}{
		"read-only":       {},
		"workspace-write": {},
	}
)

func validatePolicyMap(document map[string]any, deploymentCap int) []Issue {
	var issues []Issue
	issues = append(issues, validatePolicyModeOverride(document)...)
	issues = append(issues, validatePolicyDeniedFlagOverrides(document)...)
	issues = append(issues, validatePolicyWritableRootOverride(document)...)
	issues = append(issues, validatePolicyConcurrencyOverride(document)...)
	issues = append(issues, validatePolicyMaxAgentsOverride(document, deploymentCap)...)
	return issues
}

func validatePolicyModeOverride(document map[string]any) []Issue {
	value, ok := document["mode"]
	if !ok {
		return nil
	}
	mode, ok := value.(string)
	if ok && strings.TrimSpace(mode) == ModeReadOnly {
		return nil
	}
	return []Issue{{
		Code:    CodeUnsupportedPolicyMode,
		Message: fmt.Sprintf("policy.mode must be %q for the read-only MVP default", ModeReadOnly),
		Path:    "policy.mode",
	}}
}

func validatePolicyDeniedFlagOverrides(document map[string]any) []Issue {
	var issues []Issue
	for field, capability := range map[string]string{
		"allowNetwork":          "network access",
		"allowConnectors":       "connectors",
		"allowDangerFullAccess": "danger-full-access",
	} {
		value, ok := document[field]
		if !ok {
			continue
		}
		allowed, ok := value.(bool)
		if ok && allowed {
			issues = append(issues, validateDeniedFlag(field, capability))
		}
	}
	return issues
}

func validatePolicyWritableRootOverride(document map[string]any) []Issue {
	value, ok := document["writableRoots"]
	if !ok {
		return nil
	}
	roots, ok := value.([]any)
	if !ok || len(roots) == 0 {
		return nil
	}
	return []Issue{{
		Code:    CodeWritableRootsReadOnly,
		Message: "writableRoots are not allowed when policy.mode is READ_ONLY",
		Path:    "policy.writableRoots",
	}}
}

func validatePolicyConcurrencyOverride(document map[string]any) []Issue {
	value, ok := document["concurrency"]
	if !ok {
		return nil
	}
	concurrency, ok := asInt(value)
	if ok && concurrency > 0 {
		return nil
	}
	return []Issue{{
		Code:    CodeInvalidConcurrency,
		Message: "concurrency must be greater than zero",
		Path:    "policy.concurrency",
	}}
}

func validatePolicyMaxAgentsOverride(document map[string]any, deploymentCap int) []Issue {
	value, ok := document["maxAgents"]
	if !ok {
		return nil
	}
	maxAgents, ok := asInt(value)
	if !ok || maxAgents <= 0 {
		return []Issue{{
			Code:    CodeInvalidMaxAgents,
			Message: "maxAgents must be greater than zero",
			Path:    "policy.maxAgents",
		}}
	}
	if maxAgents > deploymentCap {
		return []Issue{{
			Code:    CodeExcessiveMaxAgents,
			Message: fmt.Sprintf("maxAgents %d exceeds deployment cap %d", maxAgents, deploymentCap),
			Path:    "policy.maxAgents",
		}}
	}
	return nil
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	default:
		return 0, false
	}
}

// Validate checks one effective policy against deployment limits.
func Validate(policy EffectivePolicy, deploymentCap int) []Issue {
	if deploymentCap <= 0 {
		deploymentCap = DefaultDeploymentCap
	}
	var issues []Issue

	if policy.Concurrency > policy.MaxAgents {
		issues = append(issues, Issue{
			Code:    CodeConcurrencyAboveMaxAgents,
			Message: fmt.Sprintf("concurrency %d exceeds maxAgents %d", policy.Concurrency, policy.MaxAgents),
			Path:    "policy.concurrency",
		})
	}

	if policy.Mode == ModeReadOnly {
		if len(policy.WritableRoots) > 0 {
			issues = append(issues, Issue{
				Code:    CodeWritableRootsReadOnly,
				Message: "writableRoots are not allowed when policy.mode is READ_ONLY",
				Path:    "policy.writableRoots",
			})
		}
		if policy.AllowNetwork {
			issues = append(issues, validateDeniedFlag("allowNetwork", "network access"))
		}
		if policy.AllowConnectors {
			issues = append(issues, validateDeniedFlag("allowConnectors", "connectors"))
		}
		if policy.AllowDangerFullAccess {
			issues = append(issues, validateDeniedFlag("allowDangerFullAccess", "danger-full-access"))
		}
		if sandbox := strings.TrimSpace(policy.SandboxMode); sandbox == "workspace-write" {
			issues = append(issues, Issue{
				Code:    CodeUnsupportedSandboxMode,
				Message: `sandboxMode "workspace-write" is not allowed when policy.mode is READ_ONLY`,
				Path:    "policy.sandboxMode",
			})
		}
	}

	issues = append(issues, validateStringAllowlist("allowedRunners", policy.AllowedRunners, validateRunner)...)
	issues = append(issues, validateStringAllowlist("allowedModels", policy.AllowedModels, validateModel)...)
	issues = append(issues, validateStringAllowlist("allowedReasoningEfforts", policy.AllowedReasoningEfforts, validateReasoningEffort)...)
	issues = append(issues, validateStringAllowlist("allowedRouteProfiles", policy.AllowedRouteProfiles, validateRouteProfile)...)
	issues = append(issues, validateStringAllowlist("allowedCommands", policy.AllowedCommands, validateCommand)...)
	if sandbox := strings.TrimSpace(policy.SandboxMode); sandbox != "" {
		if _, ok := knownSandboxModes[sandbox]; !ok {
			issues = append(issues, Issue{
				Code:    CodeUnsupportedSandboxMode,
				Message: fmt.Sprintf("unsupported sandboxMode %q", sandbox),
				Path:    "policy.sandboxMode",
			})
		}
	}

	return issues
}

func validateDeniedFlag(field, capability string) Issue {
	return Issue{
		Code:    CodeDeniedCapability,
		Message: fmt.Sprintf("%s is denied for policy.mode READ_ONLY (%s)", field, capability),
		Path:    "policy." + field,
	}
}

type allowlistValidator func(string) *Issue

func validateStringAllowlist(field string, values []string, validate allowlistValidator) []Issue {
	if len(values) == 0 {
		return nil
	}
	var issues []Issue
	for index, value := range values {
		if issue := validate(value); issue != nil {
			issue.Path = fmt.Sprintf("policy.%s[%d]", field, index)
			issues = append(issues, *issue)
		}
	}
	return issues
}

func validateRunner(value string) *Issue {
	runner := workerrunner.NormalizeRunnerID(value)
	if runner == "" || !workerrunner.IsBuiltInRunnerID(runner) {
		return &Issue{
			Code:    CodeUnsupportedRunner,
			Message: fmt.Sprintf("unsupported runner %q", value),
		}
	}
	return nil
}

func validateModel(value string) *Issue {
	if strings.TrimSpace(value) == "" {
		return &Issue{
			Code:    CodeUnsupportedModel,
			Message: "model identifiers must be non-empty strings",
		}
	}
	return nil
}

func validateReasoningEffort(value string) *Issue {
	if _, ok := interfaces.CanonicalizeReasoningEffort(value); !ok || strings.TrimSpace(value) == "" {
		return &Issue{
			Code:    CodeUnsupportedReasoning,
			Message: fmt.Sprintf("unsupported reasoning effort %q", value),
		}
	}
	return nil
}

func validateRouteProfile(value string) *Issue {
	profile := strings.ToLower(strings.TrimSpace(value))
	if _, ok := knownRouteProfiles[profile]; !ok {
		return &Issue{
			Code:    CodeUnsupportedRouteProfile,
			Message: fmt.Sprintf("unsupported route profile %q", value),
		}
	}
	return nil
}

func validateCommand(value string) *Issue {
	if strings.TrimSpace(value) == "" {
		return &Issue{
			Code:    CodeUnsupportedCommand,
			Message: "command identifiers must be non-empty strings",
		}
	}
	return nil
}
