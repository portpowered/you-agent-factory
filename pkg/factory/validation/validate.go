package validation

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const validationRoot = "factory"

// ValidateStructural runs shared structural validation without work-type outcome
// invariants that legacy mapper fixtures may omit until they are migrated.
func ValidateStructural(cfg *interfaces.FactoryConfig) Result {
	if cfg == nil {
		return Result{}
	}
	result := Result{Targets: OrchestratorTargets(cfg)}
	if !IsPetriOrchestratorValidationScope(cfg) {
		return result
	}
	var targets []Target
	targets = append(targets, duplicateIdentifierTargets(cfg)...)
	targets = append(targets, duplicateWorkStateTargets(cfg)...)
	targets = append(targets, danglingReferenceTargets(cfg)...)
	targets = append(targets, invalidPlaceReferenceTargets(cfg)...)
	targets = append(targets, conflictingWorkstationOutputTargets(cfg)...)
	targets = append(targets, missingOutcomeRouteTargets(cfg)...)
	targets = append(targets, ManagedRuntimeDependencyTargets(cfg)...)
	result.Targets = append(result.Targets, targets...)
	return result
}

// Validate runs structural factory validation for a complete factory definition and
// returns aggregated canonical targets.
func Validate(cfg *interfaces.FactoryConfig) Result {
	result := ValidateStructural(cfg)
	if cfg == nil {
		return result
	}
	if !IsPetriOrchestratorValidationScope(cfg) {
		return result
	}
	result.Targets = append(result.Targets, WorkTypeHandlingBehaviorTargets(cfg, WorkTypeHandlingBehaviorOptions{})...)
	result.Targets = append(result.Targets, InvocationReturnTargets(cfg)...)
	result.Targets = append(result.Targets, missingWorkTypeOutcomeStateTargets(cfg)...)
	result.Targets = append(result.Targets, missingTerminalCompletionPathTargets(cfg)...)

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result.Targets = append(result.Targets, ValidateLayout(cfg, topology).Targets...)
	return result
}

// InvocationReturnTargets validates the authored invocation primary-result
// policy against the declared factory work types and terminal states.
func InvocationReturnTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || cfg.InvocationReturn == nil {
		return nil
	}

	policy := strings.TrimSpace(cfg.InvocationReturn.Policy)
	switch factoryapi.InvocationReturnPolicy(policy) {
	case factoryapi.InvocationReturnPolicySubmittedWorkTerminal:
		return nil
	case factoryapi.InvocationReturnPolicyExplicit:
		return explicitInvocationReturnTargets(cfg)
	default:
		return []Target{invocationReturnTarget(
			CodeInvocationReturnUnsupportedPolicy,
			"policy",
			fmt.Sprintf("unsupported invocationReturn.policy %q", cfg.InvocationReturn.Policy),
		)}
	}
}

func explicitInvocationReturnTargets(cfg *interfaces.FactoryConfig) []Target {
	workTypeName := strings.TrimSpace(cfg.InvocationReturn.WorkTypeName)
	if workTypeName == "" {
		return []Target{invocationReturnTarget(
			CodeInvocationReturnMissingWorkTypeName,
			"workTypeName",
			"invocationReturn.workTypeName is required when policy is EXPLICIT",
		)}
	}

	workType, ok := findWorkType(cfg, workTypeName)
	if !ok {
		return []Target{invocationReturnTarget(
			CodeInvocationReturnUnknownWorkTypeName,
			"workTypeName",
			fmt.Sprintf("invocationReturn.workTypeName %q does not match a declared work type", workTypeName),
		)}
	}

	terminalState := strings.TrimSpace(cfg.InvocationReturn.TerminalState)
	if terminalState == "" {
		return []Target{invocationReturnTarget(
			CodeInvocationReturnMissingTerminalState,
			"terminalState",
			"invocationReturn.terminalState is required when policy is EXPLICIT",
		)}
	}

	for _, state := range workType.States {
		if state.Name != terminalState {
			continue
		}
		if state.Type == interfaces.StateTypeTerminal {
			return nil
		}
		break
	}

	return []Target{invocationReturnTarget(
		CodeInvocationReturnInvalidTerminalState,
		"terminalState",
		fmt.Sprintf("invocationReturn.terminalState %q must name a TERMINAL state on work type %q", terminalState, workTypeName),
	)}
}

func findWorkType(cfg *interfaces.FactoryConfig, name string) (interfaces.WorkTypeConfig, bool) {
	for _, workType := range cfg.WorkTypes {
		if workType.Name == name {
			return workType, true
		}
	}
	return interfaces.WorkTypeConfig{}, false
}

func invocationReturnTarget(code, field, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "invocationReturn",
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.invocationReturn.%s", validationRoot, field),
	}
}
