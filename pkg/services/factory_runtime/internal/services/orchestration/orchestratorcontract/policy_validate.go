package orchestratorcontract

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	knownPermissionModes = map[string]struct{}{
		PermissionModeDefault:         {},
		PermissionModeSkipPermissions: {},
	}
)

func validatePolicyMap(document map[string]any, deploymentCap int) []Issue {
	var issues []Issue
	issues = append(issues, validateRetiredPolicyFields(document)...)
	issues = append(issues, validatePolicyConcurrencyOverride(document)...)
	issues = append(issues, validatePolicyMaxAgentsOverride(document, deploymentCap)...)
	issues = append(issues, validatePolicyAllowedPermissionsShape(document)...)
	return issues
}

func validateRetiredPolicyFields(document map[string]any) []Issue {
	const replacement = "allowedPermissions"
	fields := []string{"mode", "allowNetwork", "allowConnectors", "allowDangerFullAccess", "writableRoots"}
	var issues []Issue
	for _, field := range fields {
		if _, ok := document[field]; !ok {
			continue
		}
		issues = append(issues, Issue{
			Code:    CodeUnsupportedPolicyField,
			Message: fmt.Sprintf("policy.%s is no longer supported; use policy.%s to authorize DEFAULT or SKIP_PERMISSIONS", field, replacement),
			Path:    "policy." + field,
		})
	}
	return issues
}

func validatePolicyAllowedPermissionsShape(document map[string]any) []Issue {
	value, ok := document["allowedPermissions"]
	if !ok {
		return nil
	}

	values, ok := policyAllowlistValues(value)
	if !ok {
		return []Issue{{
			Code:    CodeUnsupportedPermission,
			Message: fmt.Sprintf("allowedPermissions must be an array containing only %q or %q", PermissionModeDefault, PermissionModeSkipPermissions),
			Path:    "policy.allowedPermissions",
		}}
	}

	var issues []Issue
	for index, value := range values {
		if _, ok := value.(string); ok {
			continue
		}
		issues = append(issues, Issue{
			Code:    CodeUnsupportedPermission,
			Message: fmt.Sprintf("allowedPermissions[%d] must be a string containing %q or %q", index, PermissionModeDefault, PermissionModeSkipPermissions),
			Path:    fmt.Sprintf("policy.allowedPermissions[%d]", index),
		})
	}
	return issues
}

func policyAllowlistValues(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []string:
		values := make([]any, len(typed))
		for index, value := range typed {
			values[index] = value
		}
		return values, true
	default:
		return nil, false
	}
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

	issues = append(issues, validateStringAllowlist("allowedPermissions", policy.AllowedPermissions, validatePermission)...)
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
	runner := workers.NormalizeRunnerID(value)
	if runner == "" || !workers.IsBuiltInRunnerID(runner) {
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

func validatePermission(value string) *Issue {
	if _, ok := knownPermissionModes[value]; ok {
		return nil
	}
	return &Issue{
		Code:    CodeUnsupportedPermission,
		Message: fmt.Sprintf("unsupported permission %q; allowedPermissions accepts only %q or %q", value, PermissionModeDefault, PermissionModeSkipPermissions),
	}
}
