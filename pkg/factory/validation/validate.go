package validation

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/validation"
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
	result.Targets = append(result.Targets, InvocationReturnTargets(cfg)...)
	result.Targets = append(result.Targets, InvocationSignatureTargets(cfg)...)
	result.Targets = append(result.Targets, WorkPropagationTargets(cfg)...)
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

var invocationSignatureReferencePattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)

var supportedInvocationTypeHints = []string{
	string(factoryapi.FactoryInvocationParameterTypeHintString),
	string(factoryapi.FactoryInvocationParameterTypeHintPath),
	string(factoryapi.FactoryInvocationParameterTypeHintFilePath),
	string(factoryapi.FactoryInvocationParameterTypeHintDirectoryPath),
	string(factoryapi.FactoryInvocationParameterTypeHintNumberString),
	string(factoryapi.FactoryInvocationParameterTypeHintBooleanString),
}

var supportedInvocationValueModes = []string{
	string(factoryapi.FactoryInvocationParameterValueModeExact),
	string(factoryapi.FactoryInvocationParameterValueModeRepeated),
	string(factoryapi.FactoryInvocationParameterValueModeVariadic),
	string(factoryapi.FactoryInvocationParameterValueModeFileContents),
}

var supportedInvocationBindingKinds = []string{
	string(factoryapi.FactoryInvocationParameterBindingKindPositional),
	string(factoryapi.FactoryInvocationParameterBindingKindNamed),
	string(factoryapi.FactoryInvocationParameterBindingKindStdin),
	string(factoryapi.FactoryInvocationParameterBindingKindNamedRest),
}

var supportedUnknownNamedArgumentPolicies = []string{
	string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyReject),
	string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyAllow),
	string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect),
}

var supportedOutputContractModes = []string{
	string(factoryapi.FactoryInvocationOutputContractModeInline),
	string(factoryapi.FactoryInvocationOutputContractModeFile),
	string(factoryapi.FactoryInvocationOutputContractModeJson),
}

type invocationParameterSummary struct {
	name      string
	valueMode string
}

type interpolationFieldTarget struct {
	subjectType     SubjectType
	subjectID       string
	path            string
	location        SubjectLocation
	value           string
	allowsRepeated  bool
	fieldDescriptor string
}

// InvocationSignatureTargets validates the public invocation signature contract
// and supported ${parameter} references before runtime normalization runs.
func InvocationSignatureTargets(cfg *interfaces.FactoryConfig) []Target {
	if cfg == nil || cfg.InvocationSignature == nil {
		return nil
	}

	signature := cfg.InvocationSignature
	var targets []Target

	policy := strings.TrimSpace(signature.UnknownNamedArgumentPolicy)
	if policy != "" && !slices.Contains(supportedUnknownNamedArgumentPolicies, policy) {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureUnsupportedUnknownNamedArgumentPolicy,
			"unknownNamedArgumentPolicy",
			fmt.Sprintf("invocationSignature.unknownNamedArgumentPolicy %q is not supported", signature.UnknownNamedArgumentPolicy),
		))
	}

	parametersByName := make(map[string]invocationParameterSummary, len(signature.Parameters))
	namedKeys := map[string]string{}
	positionalNames := map[int]string{}
	var positionalSlots []int
	stdinParameters := []string{}
	namedRestParameters := []string{}
	var variadicPositionalNames []string

	for index, parameter := range signature.Parameters {
		targets = append(targets, invocationSignatureParameterTargets(parameter, index)...)

		name := strings.TrimSpace(parameter.Name)
		if _, exists := parametersByName[name]; exists {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureDuplicateParameterName,
				index,
				"name",
				name,
				fmt.Sprintf("invocationSignature parameter name %q is declared more than once", name),
			))
		} else if name != "" {
			parametersByName[name] = invocationParameterSummary{name: name, valueMode: strings.TrimSpace(parameter.ValueMode)}
		}

		keys := []struct {
			value string
			field string
		}{}
		if externalName := strings.TrimSpace(parameter.ExternalName); externalName != "" {
			keys = append(keys, struct {
				value string
				field string
			}{value: externalName, field: "externalName"})
		}
		for aliasIndex, alias := range parameter.Aliases {
			trimmedAlias := strings.TrimSpace(alias)
			if trimmedAlias == "" {
				continue
			}
			keys = append(keys, struct {
				value string
				field string
			}{value: trimmedAlias, field: fmt.Sprintf("aliases[%d]", aliasIndex)})
		}
		for _, key := range keys {
			if previousName, exists := namedKeys[key.value]; exists {
				targets = append(targets, invocationSignatureParameterTarget(
					CodeInvocationSignatureDuplicateNamedKey,
					index,
					key.field,
					name,
					fmt.Sprintf("invocationSignature named argument key %q is already used by parameter %q", key.value, previousName),
				))
				continue
			}
			namedKeys[key.value] = name
		}

		for bindingIndex, binding := range parameter.Bindings {
			kind := strings.TrimSpace(binding.Kind)
			switch kind {
			case string(factoryapi.FactoryInvocationParameterBindingKindPositional):
				positionalSlots = append(positionalSlots, binding.Position)
				if previousName, exists := positionalNames[binding.Position]; exists {
					targets = append(targets, invocationSignatureParameterBindingTarget(
						CodeInvocationSignatureInvalidPositionalOrdering,
						index,
						bindingIndex,
						name,
						fmt.Sprintf("positional slot %d is already bound to parameter %q", binding.Position, previousName),
					))
				} else {
					positionalNames[binding.Position] = name
				}
				if strings.TrimSpace(parameter.ValueMode) == string(factoryapi.FactoryInvocationParameterValueModeVariadic) {
					variadicPositionalNames = append(variadicPositionalNames, name)
				}
			case string(factoryapi.FactoryInvocationParameterBindingKindStdin):
				stdinParameters = append(stdinParameters, name)
			case string(factoryapi.FactoryInvocationParameterBindingKindNamedRest):
				namedRestParameters = append(namedRestParameters, name)
			}
		}
	}

	if len(variadicPositionalNames) > 1 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureMultipleVariadicPositionals,
			"parameters",
			fmt.Sprintf("invocationSignature declares multiple variadic positional parameters (%s)", strings.Join(variadicPositionalNames, ", ")),
		))
	}

	if len(positionalSlots) > 0 {
		slices.Sort(positionalSlots)
		for index, slot := range positionalSlots {
			want := index + 1
			if slot != want {
				targets = append(targets, invocationSignatureTarget(
					CodeInvocationSignatureInvalidPositionalOrdering,
					"parameters",
					fmt.Sprintf("invocationSignature positional bindings must start at 1 and stay contiguous; found slot %d where slot %d was expected", slot, want),
				))
				break
			}
		}
	}

	if len(stdinParameters) > 1 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidStdinRouting,
			"parameters",
			fmt.Sprintf("invocationSignature routes stdin to multiple parameters (%s)", strings.Join(stdinParameters, ", ")),
		))
	}

	if len(namedRestParameters) > 1 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidNamedRestShape,
			"parameters",
			fmt.Sprintf("invocationSignature declares multiple NAMED_REST parameters (%s)", strings.Join(namedRestParameters, ", ")),
		))
	}
	if policy == string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect) && len(namedRestParameters) != 1 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidNamedRestShape,
			"unknownNamedArgumentPolicy",
			"invocationSignature.unknownNamedArgumentPolicy COLLECT requires exactly one parameter with a NAMED_REST binding",
		))
	}
	if policy != string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect) && len(namedRestParameters) > 0 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidNamedRestShape,
			"unknownNamedArgumentPolicy",
			"invocationSignature parameters can only use NAMED_REST when unknownNamedArgumentPolicy is COLLECT",
		))
	}

	if signature.OutputContract != nil {
		targets = append(targets, invocationSignatureOutputContractTargets(signature.OutputContract, parametersByName)...)
	}
	targets = append(targets, invocationSignatureInterpolationTargets(cfg, parametersByName)...)

	return targets
}

func invocationSignatureParameterTargets(parameter interfaces.InvocationParameterConfig, index int) []Target {
	var targets []Target
	name := strings.TrimSpace(parameter.Name)
	valueMode := strings.TrimSpace(parameter.ValueMode)
	typeHint := strings.TrimSpace(parameter.TypeHint)

	if typeHint != "" && !slices.Contains(supportedInvocationTypeHints, typeHint) {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureUnsupportedTypeHint,
			index,
			"typeHint",
			name,
			fmt.Sprintf("invocationSignature parameter %q uses unsupported typeHint %q", name, parameter.TypeHint),
		))
	}
	if valueMode != "" && !slices.Contains(supportedInvocationValueModes, valueMode) {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureUnsupportedValueMode,
			index,
			"valueMode",
			name,
			fmt.Sprintf("invocationSignature parameter %q uses unsupported valueMode %q", name, parameter.ValueMode),
		))
	}
	if parameter.DefaultValue != "" && len(parameter.DefaultValues) > 0 {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureInvalidDefaultShape,
			index,
			"defaultValue",
			name,
			fmt.Sprintf("invocationSignature parameter %q cannot declare both defaultValue and defaultValues", name),
		))
	}
	switch valueMode {
	case "", string(factoryapi.FactoryInvocationParameterValueModeExact), string(factoryapi.FactoryInvocationParameterValueModeFileContents):
		if len(parameter.DefaultValues) > 0 {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidDefaultShape,
				index,
				"defaultValues",
				name,
				fmt.Sprintf("invocationSignature parameter %q only supports defaultValue for its single-value mode", name),
			))
		}
	case string(factoryapi.FactoryInvocationParameterValueModeRepeated), string(factoryapi.FactoryInvocationParameterValueModeVariadic):
		if parameter.DefaultValue != "" {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidDefaultShape,
				index,
				"defaultValue",
				name,
				fmt.Sprintf("invocationSignature parameter %q only supports defaultValues for its multi-value mode", name),
			))
		}
	}
	if len(parameter.Choices) > 0 {
		if parameter.DefaultValue != "" && !slices.Contains(parameter.Choices, parameter.DefaultValue) {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidDefaultChoice,
				index,
				"defaultValue",
				name,
				fmt.Sprintf("invocationSignature parameter %q defaultValue %q is not one of the declared choices", name, parameter.DefaultValue),
			))
		}
		for defaultIndex, defaultValue := range parameter.DefaultValues {
			if slices.Contains(parameter.Choices, defaultValue) {
				continue
			}
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidDefaultChoice,
				index,
				fmt.Sprintf("defaultValues[%d]", defaultIndex),
				name,
				fmt.Sprintf("invocationSignature parameter %q defaultValues[%d] %q is not one of the declared choices", name, defaultIndex, defaultValue),
			))
		}
	}

	hasPositionalBinding := false
	hasNamedRestBinding := false
	hasNamedBinding := false
	hasStdinBinding := false
	for bindingIndex, binding := range parameter.Bindings {
		kind := strings.TrimSpace(binding.Kind)
		if kind == "" || !slices.Contains(supportedInvocationBindingKinds, kind) {
			targets = append(targets, invocationSignatureParameterBindingTarget(
				CodeInvocationSignatureUnsupportedBindingKind,
				index,
				bindingIndex,
				name,
				fmt.Sprintf("invocationSignature parameter %q uses unsupported binding kind %q", name, binding.Kind),
			))
			continue
		}
		switch kind {
		case string(factoryapi.FactoryInvocationParameterBindingKindPositional):
			hasPositionalBinding = true
			if binding.Position < 1 {
				targets = append(targets, invocationSignatureParameterBindingTarget(
					CodeInvocationSignatureInvalidPositionalOrdering,
					index,
					bindingIndex,
					name,
					fmt.Sprintf("invocationSignature parameter %q positional bindings must use positions starting at 1", name),
				))
			}
		case string(factoryapi.FactoryInvocationParameterBindingKindNamed):
			hasNamedBinding = true
		case string(factoryapi.FactoryInvocationParameterBindingKindStdin):
			hasStdinBinding = true
		case string(factoryapi.FactoryInvocationParameterBindingKindNamedRest):
			hasNamedRestBinding = true
		}
	}

	if parameter.Sensitive && hasPositionalBinding {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureSensitivePositional,
			index,
			"bindings",
			name,
			fmt.Sprintf("invocationSignature parameter %q is sensitive and cannot be exposed as a positional argument", name),
		))
	}
	if hasStdinBinding && hasNamedBinding {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureInvalidStdinRouting,
			index,
			"bindings",
			name,
			fmt.Sprintf("invocationSignature parameter %q cannot combine STDIN and NAMED bindings", name),
		))
	}
	if hasStdinBinding && hasNamedRestBinding {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureInvalidStdinRouting,
			index,
			"bindings",
			name,
			fmt.Sprintf("invocationSignature parameter %q cannot combine STDIN and NAMED_REST bindings", name),
		))
	}
	if hasNamedRestBinding {
		if valueMode != string(factoryapi.FactoryInvocationParameterValueModeRepeated) {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidNamedRestShape,
				index,
				"bindings",
				name,
				fmt.Sprintf("invocationSignature parameter %q must use valueMode REPEATED when it declares a NAMED_REST binding", name),
			))
		}
		if hasPositionalBinding || hasNamedBinding || hasStdinBinding || len(parameter.Bindings) != 1 {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidNamedRestShape,
				index,
				"bindings",
				name,
				fmt.Sprintf("invocationSignature parameter %q must dedicate its bindings to NAMED_REST only", name),
			))
		}
	}
	if valueMode == string(factoryapi.FactoryInvocationParameterValueModeRepeated) {
		if hasPositionalBinding || hasStdinBinding {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidRepeatedBindingShape,
				index,
				"bindings",
				name,
				fmt.Sprintf("invocationSignature parameter %q with valueMode REPEATED may only use NAMED or NAMED_REST bindings", name),
			))
		}
	}
	if valueMode == string(factoryapi.FactoryInvocationParameterValueModeVariadic) {
		if !hasPositionalBinding || hasNamedBinding || hasStdinBinding || hasNamedRestBinding {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureInvalidRepeatedBindingShape,
				index,
				"bindings",
				name,
				fmt.Sprintf("invocationSignature parameter %q with valueMode VARIADIC must use POSITIONAL bindings only", name),
			))
		}
	}

	return targets
}

func invocationSignatureOutputContractTargets(
	output *interfaces.InvocationOutputContractConfig,
	parameters map[string]invocationParameterSummary,
) []Target {
	var targets []Target
	mode := strings.TrimSpace(output.Mode)
	if mode != "" && !slices.Contains(supportedOutputContractModes, mode) {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureUnsupportedOutputContractMode,
			"outputContract.mode",
			fmt.Sprintf("invocationSignature.outputContract.mode %q is not supported", output.Mode),
		))
	}
	pathParameter := strings.TrimSpace(output.PathParameter)
	if pathParameter == "" {
		return targets
	}
	parameter, ok := parameters[pathParameter]
	if !ok {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureUnknownOutputPathParameter,
			"outputContract.pathParameter",
			fmt.Sprintf("invocationSignature.outputContract.pathParameter %q does not match a declared invocation parameter", pathParameter),
		))
		return targets
	}
	if !invocationParameterSupportsScalarInterpolation(parameter.valueMode) {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidOutputPathParameter,
			"outputContract.pathParameter",
			fmt.Sprintf("invocationSignature.outputContract.pathParameter %q must reference a single-value parameter", parameter.name),
		))
	}
	return targets
}

func invocationSignatureInterpolationTargets(
	cfg *interfaces.FactoryConfig,
	parameters map[string]invocationParameterSummary,
) []Target {
	fields := invocationInterpolationFieldTargets(cfg)
	var targets []Target
	for _, field := range fields {
		references := invocationSignatureReferencePattern.FindAllStringSubmatch(field.value, -1)
		for _, reference := range references {
			parameterName := reference[1]
			parameter, ok := parameters[parameterName]
			if !ok {
				targets = append(targets, Target{
					Code:     CodeInvocationSignatureInvalidInterpolationReference,
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s references invocation parameter %q, but that parameter is not declared", field.fieldDescriptor, parameterName),
					Subject: Subject{
						Type:     field.subjectType,
						ID:       field.subjectID,
						Location: field.location,
					},
					Path: field.path,
				})
				continue
			}
			if field.allowsRepeated || invocationParameterSupportsScalarInterpolation(parameter.valueMode) {
				continue
			}
			targets = append(targets, Target{
				Code:     CodeInvocationSignatureIncompatibleInterpolationReference,
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s cannot reference multi-value invocation parameter %q", field.fieldDescriptor, parameterName),
				Subject: Subject{
					Type:     field.subjectType,
					ID:       field.subjectID,
					Location: field.location,
				},
				Path: field.path,
			})
		}
	}
	return targets
}

func invocationInterpolationFieldTargets(cfg *interfaces.FactoryConfig) []interpolationFieldTarget {
	var fields []interpolationFieldTarget
	for workerIndex, worker := range cfg.Workers {
		subjectID := worker.Name
		basePath := fmt.Sprintf("%s.workers[%d](%s)", validationRoot, workerIndex, worker.Name)
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".provider", worker.Provider, false, "worker.provider")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".model", worker.Model, false, "worker.model")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".modelProvider", worker.ModelProvider, false, "worker.modelProvider")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".executorProvider", worker.ExecutorProvider, false, "worker.executorProvider")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".command", worker.Command, false, "worker.command")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".timeout", worker.Timeout, false, "worker.timeout")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".stopToken", worker.StopToken, false, "worker.stopToken")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".openCodeAgent", worker.OpenCodeAgent, false, "worker.openCodeAgent")
		fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, basePath+".body", worker.Body, false, "worker body")
		for argIndex, arg := range worker.Args {
			fields = appendInterpolationField(fields, SubjectTypeWorker, subjectID, SubjectLocationDefinition, fmt.Sprintf("%s.args[%d]", basePath, argIndex), arg, false, "worker.args entry")
		}
	}
	for workstationIndex, workstation := range cfg.Workstations {
		subjectID := workstation.Name
		basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".worker", workstation.WorkerTypeName, false, "workstation.worker")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".runner", workstation.Runner, false, "workstation.runner")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".openCodeAgent", workstation.OpenCodeAgent, false, "workstation.openCodeAgent")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".promptFile", workstation.PromptFile, false, "workstation.promptFile")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".outputSchema", workstation.OutputSchema, false, "workstation.outputSchema")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".timeout", workstation.Timeout, false, "workstation.timeout")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".body", workstation.Body, false, "workstation prompt body")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".promptTemplate", workstation.PromptTemplate, false, "workstation.promptTemplate")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".workingDirectory", workstation.WorkingDirectory, false, "workstation.workingDirectory")
		fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, basePath+".worktree", workstation.Worktree, false, "workstation.worktree")
		for key, value := range workstation.Env {
			fields = appendInterpolationField(fields, SubjectTypeWorkstation, subjectID, SubjectLocationDefinition, fmt.Sprintf("%s.env[%q]", basePath, key), value, false, fmt.Sprintf("workstation.env[%q]", key))
		}
	}
	return fields
}

func appendInterpolationField(
	fields []interpolationFieldTarget,
	subjectType SubjectType,
	subjectID string,
	location SubjectLocation,
	path string,
	value string,
	allowsRepeated bool,
	fieldDescriptor string,
) []interpolationFieldTarget {
	if !strings.Contains(value, "${") {
		return fields
	}
	return append(fields, interpolationFieldTarget{
		subjectType:     subjectType,
		subjectID:       subjectID,
		path:            path,
		location:        location,
		value:           value,
		allowsRepeated:  allowsRepeated,
		fieldDescriptor: fieldDescriptor,
	})
}

func invocationParameterSupportsScalarInterpolation(valueMode string) bool {
	switch valueMode {
	case string(factoryapi.FactoryInvocationParameterValueModeRepeated), string(factoryapi.FactoryInvocationParameterValueModeVariadic):
		return false
	default:
		return true
	}
}

func invocationSignatureTarget(code, field, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "invocationSignature",
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.invocationSignature.%s", validationRoot, field),
	}
}

func invocationSignatureParameterTarget(code string, index int, field string, name string, message string) Target {
	subjectID := name
	if subjectID == "" {
		subjectID = fmt.Sprintf("parameter[%d]", index)
	}
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       subjectID,
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.invocationSignature.parameters[%d].%s", validationRoot, index, field),
	}
}

func invocationSignatureParameterBindingTarget(code string, parameterIndex int, bindingIndex int, name string, message string) Target {
	subjectID := name
	if subjectID == "" {
		subjectID = fmt.Sprintf("parameter[%d]", parameterIndex)
	}
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       subjectID,
			Location: SubjectLocationDefinition,
		},
		Path: fmt.Sprintf("%s.invocationSignature.parameters[%d].bindings[%d]", validationRoot, parameterIndex, bindingIndex),
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
		switch factoryapi.WorkPropagationMode(mode) {
		case factoryapi.WorkPropagationModeOutputAsPayload,
			factoryapi.WorkPropagationModePreserveInput:
			continue
		default:
			basePath := fmt.Sprintf("%s.workstations[%d](%s)", validationRoot, workstationIndex, workstation.Name)
			targets = append(targets, Target{
				Code:     CodeWorkstationUnsupportedWorkPropagationMode,
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"unsupported workPropagation.mode %q (supported: %q, %q)",
					workstation.WorkPropagation.Mode,
					factoryapi.WorkPropagationModeOutputAsPayload,
					factoryapi.WorkPropagationModePreserveInput,
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
	targets = append(targets, javascriptWorkflowConfigAndInlineTargets(jsCfg)...)
	targets = append(targets, javascriptWorkflowPolicyTargets(jsCfg)...)
	return targets
}

func javascriptWorkflowPolicyTargets(jsCfg *interfaces.FactoryOrchestratorJavaScriptConfig) []Target {
	if jsCfg == nil {
		return nil
	}
	resolution := workflowpolicy.ResolveFromFactoryDefault(jsCfg.DefaultPolicy)
	return workflowPolicyIssuesToTargets(resolution.Issues)
}

func workflowPolicyIssuesToTargets(issues []workflowpolicy.Issue) []Target {
	if len(issues) == 0 {
		return nil
	}
	targets := make([]Target, 0, len(issues))
	for _, issue := range issues {
		targetPath := "javascript.defaultPolicy"
		switch {
		case issue.Path == "orchestrator.javascript.defaultPolicy":
			targetPath = "javascript.defaultPolicy"
		case strings.HasPrefix(issue.Path, "policy."):
			targetPath = "javascript.defaultPolicy." + strings.TrimPrefix(issue.Path, "policy.")
		}
		targets = append(targets, Target{
			Code:     issue.Code,
			Severity: SeverityError,
			Message:  issue.Message,
			Path:     fmt.Sprintf("%s.orchestrator.%s", validationRoot, targetPath),
			Subject: Subject{
				Type:     SubjectTypeFactory,
				ID:       "factory",
				Location: SubjectLocationDefinition,
			},
		})
	}
	return targets
}

// WorkflowSourceTargets validates file-backed workflow source when a reader is provided.
func WorkflowSourceTargets(jsCfg *interfaces.FactoryOrchestratorJavaScriptConfig, reader WorkflowSourceReader) []Target {
	if jsCfg == nil || reader == nil {
		return nil
	}
	sourceRef := strings.TrimSpace(jsCfg.SourceRef)
	if sourceRef == "" {
		return nil
	}
	if jsCfg.InlineSource != nil && strings.TrimSpace(jsCfg.InlineSource.Inline) != "" {
		return nil
	}
	content, err := reader.ReadWorkflowSource(sourceRef)
	if err != nil {
		return []Target{workflowSourceTarget(workflowvalidation.Issue{
			Code:    workflowvalidation.CodeSourceUnreadable,
			Message: fmt.Sprintf("unable to read workflow source %q: %v", sourceRef, err),
			Path:    "orchestrator.javascript.sourceRef",
		})}
	}
	loaded, loadIssues := workflowvalidation.Load(workflowvalidation.LoadRequest{
		SourceRef: sourceRef,
		Content:   content,
	})
	if len(loadIssues) > 0 {
		return workflowSourceIssuesToTargets(loadIssues)
	}
	if expectedHash := strings.TrimSpace(jsCfg.SourceHash); expectedHash != "" && expectedHash != loaded.SourceHash {
		return []Target{workflowSourceTarget(workflowvalidation.Issue{
			Code:    workflowvalidation.CodeSourceHashMismatch,
			Message: fmt.Sprintf("orchestrator.javascript.sourceHash %q does not match loaded workflow source hash %q", expectedHash, loaded.SourceHash),
			Path:    "orchestrator.javascript.sourceHash",
		})}
	}
	fileResult := workflowvalidation.ValidateLoaded(loaded, workflowvalidation.Request{
		ConfigPath: "orchestrator.javascript.sourceRef",
		Metadata:   jsCfg.Metadata,
		ArgsSchema: jsCfg.ArgsSchema,
	})
	return workflowSourceIssuesToTargets(fileResult.Issues)
}

func javascriptWorkflowConfigAndInlineTargets(jsCfg *interfaces.FactoryOrchestratorJavaScriptConfig) []Target {
	if jsCfg == nil {
		return nil
	}
	var targets []Target
	configResult := workflowvalidation.Validate(workflowvalidation.Request{
		ConfigPath: "orchestrator.javascript",
		Metadata:   jsCfg.Metadata,
		ArgsSchema: jsCfg.ArgsSchema,
	})
	targets = append(targets, workflowSourceIssuesToTargets(configResult.Issues)...)

	if jsCfg.InlineSource == nil {
		return targets
	}
	inline := strings.TrimSpace(jsCfg.InlineSource.Inline)
	if inline == "" {
		return targets
	}
	inlineResult := workflowvalidation.Validate(workflowvalidation.Request{
		Source:     inline,
		SourceRef:  "inline",
		ConfigPath: "orchestrator.javascript.inlineSource",
	})
	targets = append(targets, workflowSourceIssuesToTargets(inlineResult.Issues)...)
	return targets
}

func workflowSourceIssuesToTargets(issues []workflowvalidation.Issue) []Target {
	if len(issues) == 0 {
		return nil
	}
	targets := make([]Target, 0, len(issues))
	for _, issue := range issues {
		targets = append(targets, workflowSourceTarget(issue))
	}
	return targets
}

func workflowSourceTarget(issue workflowvalidation.Issue) Target {
	targetPath := "javascript"
	switch {
	case strings.HasPrefix(issue.Path, "orchestrator.javascript."):
		targetPath = strings.TrimPrefix(issue.Path, "orchestrator.")
	case issue.Path == "inline":
		targetPath = "javascript.inlineSource"
	case issue.Path != "":
		targetPath = "javascript.sourceRef"
	}
	return Target{
		Code:     issue.Code,
		Severity: SeverityError,
		Message:  issue.Message + issue.LocationSuffix(),
		Path:     fmt.Sprintf("%s.%s", validationRoot, targetPath),
		Subject: Subject{
			Type:     SubjectTypeFactory,
			ID:       "factory",
			Location: SubjectLocationDefinition,
		},
	}
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
