package impl

import (
	"bytes"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const validationRoot = "factory"

// ValidateStructural runs shared structural validation without work-type outcome
// invariants that legacy mapper fixtures may omit until they are migrated.
func ValidateStructural(cfg *factorydefinitions.FactoryConfig) Result {
	if cfg == nil {
		return Result{}
	}
	result := Result{
		Targets: append(WebhookTargets(cfg), OrchestratorTargets(cfg)...),
	}
	result.Targets = append(result.Targets, ExpectedArtifactTargets(cfg)...)
	if !IsPetriOrchestratorValidationScope(cfg) {
		return result
	}
	var targets []Target
	targets = append(targets, duplicateIdentifierTargets(cfg)...)
	targets = append(targets, duplicateWorkStateTargets(cfg)...)
	targets = append(targets, ValidateGraphTopology(cfg).Targets...)
	targets = append(targets, unsupportedSameNameAllChildrenCompleteJoinArityTargets(cfg)...)
	targets = append(targets, conflictingWorkstationOutputTargets(cfg)...)
	targets = append(targets, missingOutcomeRouteTargets(cfg)...)
	targets = append(targets, workstationOutputSchemaTargets(cfg)...)
	targets = append(targets, ManagedRuntimeDependencyTargets(cfg)...)
	result.Targets = append(result.Targets, targets...)
	return result
}

func workstationOutputSchemaTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	var targets []Target
	for index, workstation := range cfg.Workstations {
		schema := strings.TrimSpace(workstation.OutputSchema)
		if schema == "" || invocationParameterInterpolation(cfg.InvocationSignature, schema) || isLegacyOutputSchemaReference(schema) {
			continue
		}
		if err := validateWorkstationOutputSchema(schema); err == nil {
			continue
		} else {
			targets = append(targets, Target{
				Code:     CodeWorkstationInvalidOutputSchema,
				Severity: SeverityError,
				Message:  fmt.Sprintf("workstation outputSchema is invalid JSON Schema: %v", err),
				Subject:  Subject{Type: SubjectTypeWorkstation, ID: workstation.Name, Location: SubjectLocationDefinition},
				Path:     fmt.Sprintf("%s.workstations[%d](%s).outputSchema", validationRoot, index, workstation.Name),
			})
		}
	}
	return targets
}

// ExpectedArtifactTargets validates declaration-local contract syntax before a
// Factory can be persisted or activated. Rendering against real work inputs is
// intentionally deferred to runtime because those values are not definition
// data and must be checked again for workspace containment there.
func ExpectedArtifactTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	var targets []Target
	for index, workType := range cfg.WorkTypes {
		for artifactIndex, declaration := range workType.ExpectedArtifacts {
			if err := factorydefinitions.ValidateExpectedArtifactConfig(declaration, 1); err != nil {
				targets = append(targets, Target{
					Code:     CodeWorkTypeInvalidExpectedArtifact,
					Severity: SeverityError,
					Message:  fmt.Sprintf("work type %q expected artifact %q is invalid: %v", workType.Name, declaration.Name, err),
					Subject:  Subject{Type: SubjectTypeWorkType, ID: workType.Name, Location: SubjectLocationDefinition},
					Path:     fmt.Sprintf("%s.workTypes[%d](%s).expectedArtifacts[%d]", validationRoot, index, workType.Name, artifactIndex),
				})
			}
		}
	}
	for index, workstation := range cfg.Workstations {
		for artifactIndex, declaration := range workstation.ExpectedArtifacts {
			if err := factorydefinitions.ValidateExpectedArtifactConfig(declaration, len(workstation.Inputs)); err != nil {
				targets = append(targets, Target{
					Code:     CodeWorkstationInvalidExpectedArtifact,
					Severity: SeverityError,
					Message:  fmt.Sprintf("workstation %q expected artifact %q is invalid: %v", workstation.Name, declaration.Name, err),
					Subject:  Subject{Type: SubjectTypeWorkstation, ID: workstation.Name, Location: SubjectLocationDefinition},
					Path:     fmt.Sprintf("%s.workstations[%d](%s).expectedArtifacts[%d]", validationRoot, index, workstation.Name, artifactIndex),
				})
			}
		}
	}
	return targets
}

func isLegacyOutputSchemaReference(value string) bool {
	return strings.HasSuffix(strings.ToLower(value), ".json")
}

func validateWorkstationOutputSchema(value string) error {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(value)))
	if err != nil {
		return fmt.Errorf("malformed JSON: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const schemaID = "workstation-output-schema.json"
	if err := compiler.AddResource(schemaID, document); err != nil {
		return fmt.Errorf("cannot load schema: %w", err)
	}
	if _, err := compiler.Compile(schemaID); err != nil {
		return fmt.Errorf("cannot compile schema: %w", err)
	}
	return nil
}

// WebhookTargets validates Factory-owned outbound webhook declarations before
// any orchestrator-specific validation runs. The validator never resolves or
// stores secret values; it checks only the authored reference.
func WebhookTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Webhooks) == 0 {
		return nil
	}
	seenNames := make(map[string]int, len(cfg.Webhooks))
	var targets []Target
	for index, webhook := range cfg.Webhooks {
		basePath := fmt.Sprintf("%s.webhooks[%d](%s)", validationRoot, index, webhook.Name)
		name := strings.TrimSpace(webhook.Name)
		if name == "" {
			targets = append(targets, webhookTarget(CodeWebhookNameRequired, basePath+".name", "webhook name must be non-empty", webhook.Name))
		} else if previousIndex, exists := seenNames[name]; exists {
			targets = append(targets, webhookTarget(
				CodeWebhookNameDuplicate,
				basePath+".name",
				fmt.Sprintf("webhook name %q duplicates webhooks[%d]", name, previousIndex),
				name,
			))
		} else {
			seenNames[name] = index
		}

		if !validWebhookURL(webhook.URL) {
			targets = append(targets, webhookTarget(
				CodeWebhookURLInvalid,
				basePath+".url",
				"webhook url must be an absolute http or https URL with a host",
				name,
			))
		}
		if strings.TrimSpace(webhook.SigningSecretRef) == "" {
			targets = append(targets, webhookTarget(
				CodeWebhookSecretReferenceRequired,
				basePath+".signingSecretRef",
				"webhook signingSecretRef must be non-empty",
				name,
			))
		}
		targets = append(targets, webhookFilterTargets(webhook.Filter, basePath, name)...)
		targets = append(targets, webhookDeliveryPolicyTargets(webhook.DeliveryPolicy, basePath, name)...)
	}
	return targets
}

func validWebhookURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || !parsed.IsAbs() {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func webhookFilterTargets(filter factorydefinitions.FactoryWebhookFilterConfig, basePath, subjectID string) []Target {
	var targets []Target
	if len(filter.EventTypes) == 0 {
		targets = append(targets, webhookTarget(
			CodeWebhookEventTypesRequired,
			basePath+".filter.eventTypes",
			"webhook filter.eventTypes must contain at least one supported event type",
			subjectID,
		))
	}
	dispatchEventTypes := false
	seenEventTypes := make(map[string]int, len(filter.EventTypes))
	for index, eventType := range filter.EventTypes {
		path := fmt.Sprintf("%s.filter.eventTypes[%d]", basePath, index)
		if previousIndex, exists := seenEventTypes[eventType]; exists {
			targets = append(targets, webhookTarget(CodeWebhookFilterValueDuplicate, path, fmt.Sprintf("webhook filter value %q duplicates eventTypes[%d]", eventType, previousIndex), subjectID))
		} else {
			seenEventTypes[eventType] = index
		}
		if isWebhookDispatchEventType(eventType) {
			dispatchEventTypes = true
			continue
		}
		if !isSupportedWebhookEventType(eventType) {
			targets = append(targets, webhookTarget(CodeWebhookEventTypeUnsupported, path, fmt.Sprintf("unsupported webhook event type %q", eventType), subjectID))
		}
	}
	seenStatuses := make(map[string]int, len(filter.DispatchStatuses))
	if filter.DispatchStatuses != nil && len(filter.DispatchStatuses) == 0 {
		targets = append(targets, webhookTarget(
			CodeWebhookDispatchStatusesRequired,
			basePath+".filter.dispatchStatuses",
			"webhook filter.dispatchStatuses must contain at least one supported status when provided",
			subjectID,
		))
	}
	for index, status := range filter.DispatchStatuses {
		path := fmt.Sprintf("%s.filter.dispatchStatuses[%d]", basePath, index)
		if previousIndex, exists := seenStatuses[status]; exists {
			targets = append(targets, webhookTarget(CodeWebhookFilterValueDuplicate, path, fmt.Sprintf("webhook filter value %q duplicates dispatchStatuses[%d]", status, previousIndex), subjectID))
		} else {
			seenStatuses[status] = index
		}
		if status != factorydefinitions.FactoryWebhookDispatchStatusFailed && status != factorydefinitions.FactoryWebhookDispatchStatusInterrupted {
			targets = append(targets, webhookTarget(CodeWebhookDispatchStatusUnsupported, path, fmt.Sprintf("unsupported webhook dispatch status %q", status), subjectID))
		}
	}
	if len(filter.DispatchStatuses) > 0 && !dispatchEventTypes {
		targets = append(targets, webhookTarget(
			CodeWebhookDispatchStatusIncompatible,
			basePath+".filter.dispatchStatuses",
			"webhook filter.dispatchStatuses requires at least one dispatch event type",
			subjectID,
		))
	}
	return targets
}

func isSupportedWebhookEventType(value string) bool {
	switch value {
	case factorydefinitions.FactoryWebhookEventTypeWorkStateChange,
		factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
		factorydefinitions.FactoryWebhookEventTypeDispatchReconciled,
		factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted:
		return true
	default:
		return false
	}
}

func isWebhookDispatchEventType(value string) bool {
	switch value {
	case factorydefinitions.FactoryWebhookEventTypeDispatchResponse,
		factorydefinitions.FactoryWebhookEventTypeDispatchReconciled,
		factorydefinitions.FactoryWebhookEventTypeDispatchInterrupted:
		return true
	default:
		return false
	}
}

func webhookDeliveryPolicyTargets(policy *factorydefinitions.FactoryWebhookDeliveryPolicyConfig, basePath, subjectID string) []Target {
	if policy == nil {
		return nil
	}
	targets := webhookPolicyDurationTargets(policy, basePath, subjectID)
	if policy.MaxAttempts != nil && *policy.MaxAttempts <= 0 {
		targets = append(targets, webhookPolicyTarget(basePath, "maxAttempts", subjectID, "maxAttempts must be positive, including the initial attempt"))
	}
	if policy.BackoffMultiplier != nil && (math.IsNaN(*policy.BackoffMultiplier) || math.IsInf(*policy.BackoffMultiplier, 0) || *policy.BackoffMultiplier < 1) {
		targets = append(targets, webhookPolicyTarget(basePath, "backoffMultiplier", subjectID, "backoffMultiplier must be at least 1"))
	}
	targets = append(targets, webhookBackoffOrderingTarget(policy, basePath, subjectID)...)
	return targets
}

func webhookPolicyDurationTargets(policy *factorydefinitions.FactoryWebhookDeliveryPolicyConfig, basePath, subjectID string) []Target {
	var targets []Target
	if policy.RequestTimeout != nil && !positiveWebhookDuration(*policy.RequestTimeout) {
		targets = append(targets, webhookPolicyTarget(basePath, "requestTimeout", subjectID, "requestTimeout must be a positive Go duration"))
	}
	if policy.InitialBackoff != nil && !positiveWebhookDuration(*policy.InitialBackoff) {
		targets = append(targets, webhookPolicyTarget(basePath, "initialBackoff", subjectID, "initialBackoff must be a positive Go duration"))
	}
	if policy.MaxBackoff != nil && !positiveWebhookDuration(*policy.MaxBackoff) {
		targets = append(targets, webhookPolicyTarget(basePath, "maxBackoff", subjectID, "maxBackoff must be a positive Go duration"))
	}
	return targets
}

func webhookBackoffOrderingTarget(policy *factorydefinitions.FactoryWebhookDeliveryPolicyConfig, basePath, subjectID string) []Target {
	initial, initialOK := webhookDurationValue(policy.InitialBackoff, factorydefinitions.DefaultFactoryWebhookInitialBackoff)
	maximum, maximumOK := webhookDurationValue(policy.MaxBackoff, factorydefinitions.DefaultFactoryWebhookMaxBackoff)
	if initialOK && maximumOK && maximum < initial {
		return []Target{webhookPolicyTarget(basePath, "maxBackoff", subjectID, "maxBackoff must not be less than initialBackoff")}
	}
	return nil
}

func positiveWebhookDuration(value string) bool {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	return err == nil && duration > 0
}

func webhookDurationValue(value *string, fallback time.Duration) (time.Duration, bool) {
	if value == nil {
		return fallback, true
	}
	duration, err := time.ParseDuration(strings.TrimSpace(*value))
	return duration, err == nil && duration > 0
}

func webhookPolicyTarget(basePath, field, subjectID, message string) Target {
	return webhookTarget(CodeWebhookDeliveryPolicyInvalid, basePath+".deliveryPolicy."+field, message, subjectID)
}

func webhookTarget(code, path, message, subjectID string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject:  Subject{Type: SubjectTypeFactory, ID: subjectID, Location: SubjectLocationDefinition},
		Path:     path,
	}
}

// Validate runs structural factory validation for a complete factory definition and
// returns aggregated canonical targets.
func Validate(cfg *factorydefinitions.FactoryConfig) Result {
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
	result.Targets = append(result.Targets, workerReasoningEffortTargets(cfg)...)
	result.Targets = append(result.Targets, InvocationReturnTargets(cfg)...)
	result.Targets = append(result.Targets, InvocationSignatureTargets(cfg)...)
	result.Targets = append(result.Targets, WorkPropagationTargets(cfg)...)
	result.Targets = append(result.Targets, missingWorkTypeOutcomeStateTargets(cfg)...)
	result.Targets = append(result.Targets, missingTerminalCompletionPathTargets(cfg)...)

	topology := factorydefinitions.BuildPendingFactoryGraphTopology(cfg)
	result.Targets = append(result.Targets, ValidateLayout(cfg, topology).Targets...)
	return result
}

func workerReasoningEffortTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	var targets []Target
	for workerIndex, worker := range cfg.Workers {
		effort := strings.TrimSpace(worker.ReasoningEffort)
		if effort == "" || invocationParameterInterpolation(cfg.InvocationSignature, effort) {
			continue
		}
		if _, ok := factorydefinitions.CanonicalizeReasoningEffort(effort); ok {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkerUnsupportedReasoningEffort,
			Severity: SeverityError,
			Message:  fmt.Sprintf("worker reasoningEffort %q is unsupported; accepted values are minimal, low, medium, high, xhigh, and max", worker.ReasoningEffort),
			Subject:  Subject{Type: SubjectTypeWorker, ID: worker.Name, Location: SubjectLocationDefinition},
			Path:     fmt.Sprintf("%s.workers[%d](%s).reasoningEffort", validationRoot, workerIndex, worker.Name),
		})
	}
	return targets
}

func workerModelProviderTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil {
		return nil
	}
	var targets []Target
	for workerIndex, worker := range cfg.Workers {
		switch worker.Type {
		case factorydefinitions.WorkerTypeModel, factorydefinitions.WorkerTypeInference, factorydefinitions.WorkerTypeAgent:
		default:
			continue
		}
		provider := strings.TrimSpace(worker.ModelProvider)
		if strings.EqualFold(strings.TrimSpace(worker.ExecutorProvider), "ACP") && provider == "" {
			targets = append(targets, Target{
				Code:     CodeWorkerACPModelProviderRequired,
				Severity: SeverityError,
				Message:  "worker executorProvider ACP requires modelProvider to name an ACP integration",
				Subject:  Subject{Type: SubjectTypeWorker, ID: worker.Name, Location: SubjectLocationDefinition},
				Path:     fmt.Sprintf("%s.workers[%d](%s).modelProvider", validationRoot, workerIndex, worker.Name),
			})
			continue
		}
		if provider == "" || factorydefinitions.IsSymbolicWorkerModelProviderDefault(provider) || invocationParameterInterpolation(cfg.InvocationSignature, provider) {
			continue
		}
		if factorydefinitions.StrictPublicFactoryWorkerModelProvider(provider) != "" {
			continue
		}
		targets = append(targets, Target{
			Code:     CodeWorkerUnsupportedModelProvider,
			Severity: SeverityError,
			Message:  fmt.Sprintf("worker modelProvider %q is malformed; %s", worker.ModelProvider, factorydefinitions.AcceptedPublicWorkerModelProviderSummary()),
			Subject:  Subject{Type: SubjectTypeWorker, ID: worker.Name, Location: SubjectLocationDefinition},
			Path:     fmt.Sprintf("%s.workers[%d](%s).modelProvider", validationRoot, workerIndex, worker.Name),
		})
	}
	return targets
}

func invocationParameterInterpolation(signature *factorydefinitions.InvocationSignatureConfig, value string) bool {
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
func InvocationReturnTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || cfg.InvocationReturn == nil {
		return nil
	}

	policy := strings.TrimSpace(cfg.InvocationReturn.Policy)
	switch policy {
	case factorydefinitions.InvocationReturnPolicySubmittedWorkTerminal:
		return nil
	case factorydefinitions.InvocationReturnPolicyExplicit:
		return explicitInvocationReturnTargets(cfg)
	default:
		return []Target{invocationReturnTarget(
			CodeInvocationReturnUnsupportedPolicy,
			"policy",
			fmt.Sprintf("unsupported invocationReturn.policy %q", cfg.InvocationReturn.Policy),
		)}
	}
}

func explicitInvocationReturnTargets(cfg *factorydefinitions.FactoryConfig) []Target {
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
		if state.Type == factorydefinitions.StateTypeTerminal {
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

func findWorkType(cfg *factorydefinitions.FactoryConfig, name string) (factorydefinitions.WorkTypeConfig, bool) {
	for _, workType := range cfg.WorkTypes {
		if workType.Name == name {
			return workType, true
		}
	}
	return factorydefinitions.WorkTypeConfig{}, false
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
func WorkPropagationTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || len(cfg.Workstations) == 0 {
		return nil
	}

	var targets []Target
	for workstationIndex, workstation := range cfg.Workstations {
		if workstation.WorkPropagation == nil {
			continue
		}

		mode := strings.TrimSpace(string(workstation.WorkPropagation.Mode))
		switch factorydefinitions.WorkPropagationMode(mode) {
		case factorydefinitions.WorkPropagationModeOutputAsPayload,
			factorydefinitions.WorkPropagationModePreserveInput:
			continue
		default:
			basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
			targets = append(targets, Target{
				Code:     CodeWorkstationUnsupportedWorkPropagationMode,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"unsupported workPropagation.mode %q (supported: %q, %q)",
					workstation.WorkPropagation.Mode,
					factorydefinitions.WorkPropagationModeOutputAsPayload,
					factorydefinitions.WorkPropagationModePreserveInput,
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

func OrchestratorTargets(cfg *factorydefinitions.FactoryConfig) []Target {
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

	canonicalKind := factorydefinitions.StrictPublicFactoryOrchestratorKind(kind)
	if canonicalKind == "" {
		targets = append(targets, orchestratorTarget(
			CodeOrchestratorUnsupportedKind,
			"kind",
			fmt.Sprintf("unsupported orchestrator.kind %q (supported: %q, %q)", kind, factorydefinitions.OrchestratorKindPetri, factorydefinitions.OrchestratorKindJavaScript),
		))
		return targets
	}

	switch canonicalKind {
	case factorydefinitions.OrchestratorKindPetri:
		targets = append(targets, incompatibleJavaScriptOrchestratorTargets(cfg)...)
	case factorydefinitions.OrchestratorKindJavaScript:
		targets = append(targets, incompatiblePetriOrchestratorTargets(cfg)...)
		targets = append(targets, javascriptOrchestratorConfigTargets(cfg)...)
	}
	return targets
}

func incompatiblePetriOrchestratorTargets(cfg *factorydefinitions.FactoryConfig) []Target {
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

func incompatibleJavaScriptOrchestratorTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg.Orchestrator == nil || cfg.Orchestrator.JavaScript == nil {
		return nil
	}
	return []Target{orchestratorTarget(
		CodeOrchestratorIncompatibleJavaScriptConfig,
		"javascript",
		"orchestrator.javascript is only valid when orchestrator.kind = JAVASCRIPT",
	)}
}

func javascriptOrchestratorConfigTargets(cfg *factorydefinitions.FactoryConfig) []Target {
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
		if encoding != "" && encoding != factorydefinitions.OrchestratorInlineEncoding {
			targets = append(targets, orchestratorTarget(
				CodeOrchestratorJavaScriptInvalidInlineEncoding,
				"javascript.inlineSource.encoding",
				fmt.Sprintf("orchestrator.javascript.inlineSource.encoding must be %q when provided", factorydefinitions.OrchestratorInlineEncoding),
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
func IsPetriOrchestratorValidationScope(cfg *factorydefinitions.FactoryConfig) bool {
	return factorydefinitions.IsPetriOrchestratorFactory(cfg)
}
