package validation

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
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
	result.Targets = append(result.Targets, PollerRunWorkstationKindTargets(cfg)...)
	result.Targets = append(result.Targets, WorkerWorkstationBehaviorCompatibilityTargets(cfg)...)
	result.Targets = append(result.Targets, workerModelProviderTargets(cfg)...)
	result.Targets = append(result.Targets, InvocationReturnTargets(cfg)...)
	result.Targets = append(result.Targets, InvocationSignatureTargets(cfg)...)
	result.Targets = append(result.Targets, WorkPropagationTargets(cfg)...)
	result.Targets = append(result.Targets, missingWorkTypeOutcomeStateTargets(cfg)...)
	result.Targets = append(result.Targets, missingTerminalCompletionPathTargets(cfg)...)

	topology := interfaces.BuildPendingFactoryGraphTopology(cfg)
	result.Targets = append(result.Targets, ValidateLayout(cfg, topology).Targets...)
	return result
}

func workerModelProviderTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	var targets []Target
	for workerIndex, worker := range cfg.Workers {
		switch worker.Type {
		case interfaces.WorkerTypeModel, interfaces.WorkerTypeInference, interfaces.WorkerTypeAgent:
		default:
			continue
		}
		provider := strings.TrimSpace(worker.ModelProvider)
		if provider == "" || interfaces.IsSymbolicWorkerModelProviderDefault(provider) || invocationParameterInterpolation(cfg.InvocationSignature, provider) {
			continue
		}
		if _, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(provider); ok {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkerUnsupportedModelProvider,
			Severity: SeverityError,
			Message:  fmt.Sprintf("worker modelProvider %q is unsupported; supported values: %s", worker.ModelProvider, interfaces.AcceptedPublicWorkerModelProviderSummary()),
			Subject:  Subject{Type: SubjectTypeWorker, ID: worker.Name, Location: SubjectLocationDefinition},
			Path:     fmt.Sprintf("%s.workers[%d](%s).modelProvider", validationRoot, workerIndex, worker.Name),
		})
	}
	return targets
}

func invocationParameterInterpolation(signature *interfaces.InvocationSignatureConfig, value string) bool {
	if signature == nil || len(value) < 4 || !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return false
	}
	name := strings.TrimSpace(value[2 : len(value)-1])
	for _, parameter := range signature.Parameters {
		if parameter.Name == name {
			return true
		}
	}
	return false
}

// InvocationReturnTargets validates the authored invocation primary-result
// policy against the declared factory work types and terminal states.
func InvocationReturnTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || cfg.InvocationReturn == nil {
		return nil
	}

	policy := strings.TrimSpace(cfg.InvocationReturn.Policy)
	switch policy {
	case interfaces.InvocationReturnPolicySubmittedWorkTerminal:
		return nil
	case interfaces.InvocationReturnPolicyExplicit:
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

// WorkPropagationTargets validates authored workstation payload propagation modes.
func WorkPropagationTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if workstation.WorkPropagation == nil {
			continue
		}

		mode := strings.TrimSpace(string(workstation.WorkPropagation.Mode))
		switch interfaces.WorkPropagationMode(mode) {
		case interfaces.WorkPropagationModeOutputAsPayload,
			interfaces.WorkPropagationModePreserveInput:
			continue
		default:
			basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
			targets = append(targets, Target{
				Code:     CodeWorkstationUnsupportedWorkPropagationMode,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"unsupported workPropagation.mode %q (supported: %q, %q)",
					workstation.WorkPropagation.Mode,
					interfaces.WorkPropagationModeOutputAsPayload,
					interfaces.WorkPropagationModePreserveInput,
				),
				Subject: Subject{
					Type:     SubjectTypeWorkstation,
					ID:       workstation.Name,
					Location: SubjectLocationDefinition,
				},
				Path: basePath + ".workPropagation.mode",
			})
		}
	}

	return targets
}

func OrchestratorTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	if cfg.Orchestrator == nil {
		return nil
	}

	var targets []Target
	kind := strings.TrimSpace(cfg.Orchestrator.Kind)
	if kind == "" {
		return nil
	}

	canonicalKind := interfaces.StrictPublicFactoryOrchestratorKind(kind)
	if canonicalKind == "" {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorUnsupportedKind,
			"kind",
			fmt.Sprintf("unsupported orchestrator.kind %q (supported: %q, %q)", kind, interfaces.OrchestratorKindPetri, interfaces.OrchestratorKindJavaScript),
		))
		return targets
	}

	switch canonicalKind {
	case interfaces.OrchestratorKindPetri:
		targets = append(targets, incompatibleJavaScriptOrchestratorTargets(cfg)...)
	case interfaces.OrchestratorKindJavaScript:
		targets = append(targets, incompatiblePetriOrchestratorTargets(cfg)...)
		targets = append(targets, javascriptOrchestratorConfigTargets(cfg)...)
	}
	return targets
}

func incompatiblePetriOrchestratorTargets(cfg *interfaces.FactoryConfig) []Target {
	var targets []Target
	if cfg.Orchestrator != nil && cfg.Orchestrator.Petri != nil {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriConfig,
			"petri",
			"orchestrator.petri is only valid when orchestrator.kind = PETRI",
		))
	}
	if len(cfg.WorkTypes) > 0 {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriField,
			"workTypes",
			"workTypes are only valid for orchestrator.kind = PETRI",
		))
	}
	if len(cfg.Workers) > 0 {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriField,
			"workers",
			"workers are only valid for orchestrator.kind = PETRI",
		))
	}
	if len(cfg.Workstations) > 0 {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorIncompatiblePetriField,
			"workstations",
			"workstations are only valid for orchestrator.kind = PETRI",
		))
	}
	return targets
}

func incompatibleJavaScriptOrchestratorTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return nil
	}
	return []Target{orchestratorTarget(
		CodeOrchestratorIncompatibleJavaScriptConfig,
		"javascript",
		"orchestrator.javascript is only valid when orchestrator.kind = JAVASCRIPT",
	)}
}

func javascriptOrchestratorConfigTargets(cfg *interfaces.FactoryConfig) []Target {
	jsCfg := cfg.Orchestrator.JavaScript
	if jsCfg == nil {
		return []Target{orchestratorTarget(
			CodeOrchestratorJavaScriptMissingConfig,
			"javascript",
			"orchestrator.javascript is required when orchestrator.kind = JAVASCRIPT",
		)}
	}

	var targets []Target
	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	hasInline := jsCfg.InlineSource != nil && strings.TrimSpace(jsCfg.InlineSource.Inline) != ""
	switch {
	case sourceRef == "" && !hasInline:
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorJavaScriptMissingSource,
			"javascript.sourceRef",
			"JavaScript factories require orchestrator.javascript.sourceRef or orchestrator.javascript.inlineSource",
		))
	case sourceRef != "" && hasInline:
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorJavaScriptConflictingSource,
			"javascript.sourceRef",
			"JavaScript factories must declare either orchestrator.javascript.sourceRef or orchestrator.javascript.inlineSource, not both",
		))
	}
	if jsCfg.InlineSource != nil {
		encoding := strings.TrimSpace(jsCfg.InlineSource.Encoding)
		if encoding != "" && encoding != interfaces.OrchestratorInlineEncoding {
			targets = append(targets, orchestratorTarget(
				CodeOrchestratorJavaScriptInvalidInlineEncoding,
				"javascript.inlineSource.encoding",
				fmt.Sprintf("orchestrator.javascript.inlineSource.encoding must be %q when provided", interfaces.OrchestratorInlineEncoding),
			))
		}
	}
	for id, agent := range jsCfg.Agents {
		trimmedID := strings.TrimSpace(id)
		trimmedPreset := strings.TrimSpace(agent.Preset)
		if trimmedID == "" || trimmedPreset == "" {
			targets = append(targets, orchestratorTarget(
				CodeOrchestratorJavaScriptInvalidAgent,
				"javascript.agents."+id,
				fmt.Sprintf("orchestrator.javascript.agents agent id %q and preset %q must be non-empty", id, agent.Preset),
			))
		}
	}
	return targets
}

func orchestratorTarget(code, path, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "factory",
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.orchestrator.%s", validationRoot, path),
	}
}

// IsPetriOrchestratorValidationScope reports whether Petri graph validation should run.
func IsPetriOrchestratorValidationScope(cfg *interfaces.FactoryConfig) bool {
	return interfaces.IsPetriOrchestratorFactory(cfg)
}
