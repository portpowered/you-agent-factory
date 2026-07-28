package validation

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	CodeDuplicateIdentifier                                      = "factory.duplicateIdentifier"
	CodeDanglingWorkerReference                                  = "factory.worker.danglingReference"
	CodeDanglingPlaceReference                                   = "factory.route.danglingPlaceReference"
	CodeDanglingResourceReference                                = "factory.resource.danglingReference"
	CodeWorkerWorkstationIncompatibleBehavior                    = "factory.workstation.incompatibleWorkerBehavior"
	CodeWorkerUnsupportedModelProvider                           = "factory.worker.unsupportedModelProvider"
	CodeWorkstationMissingOutputRoutes                           = "factory.workstation.missingOutputRoutes"
	CodeWorkstationMissingFailureRoute                           = "factory.workstation.missingFailureRoute"
	CodeWorkstationMissingRejectionRoute                         = "factory.workstation.missingRejectionRoute"
	CodeWorkstationConflictingOutputs                            = "factory.workstation.conflictingWorkStateOutputs"
	CodeWorkTypeMissingCompletionState                           = "factory.workType.missingCompletionState"
	CodeWorkTypeMissingFailureState                              = "factory.workType.missingFailureState"
	CodeWorkStateMissingTerminalPath                             = "factory.workState.missingTerminalCompletionPath"
	CodeWorkTypeHandlingBehaviorValue                            = "work-type-handling-behavior-value"
	CodeWorkTypeHandlingBehaviorDuplicate                        = "work-type-handling-behavior-duplicate"
	CodeWorkTypeHandlingBehaviorUniqueDefault                    = "work-type-handling-behavior-unique-default"
	CodeWorkTypeHandlingBehaviorRequiredDefault                  = "work-type-handling-behavior-required-default"
	CodeInvocationReturnUnsupportedPolicy                        = "factory.invocationReturn.unsupportedPolicy"
	CodeInvocationReturnMissingWorkTypeName                      = "factory.invocationReturn.missingWorkTypeName"
	CodeInvocationReturnMissingTerminalState                     = "factory.invocationReturn.missingTerminalState"
	CodeInvocationReturnUnknownWorkTypeName                      = "factory.invocationReturn.unknownWorkTypeName"
	CodeInvocationReturnInvalidTerminalState                     = "factory.invocationReturn.invalidTerminalState"
	CodeInvocationSignatureUnsupportedUnknownNamedArgumentPolicy = "factory.invocationSignature.unsupportedUnknownNamedArgumentPolicy"
	CodeInvocationSignatureDuplicateParameterName                = "factory.invocationSignature.duplicateParameterName"
	CodeInvocationSignatureDuplicateNamedKey                     = "factory.invocationSignature.duplicateNamedKey"
	CodeInvocationSignatureUnsupportedTypeHint                   = "factory.invocationSignature.unsupportedTypeHint"
	CodeInvocationSignatureUnsupportedValueMode                  = "factory.invocationSignature.unsupportedValueMode"
	CodeInvocationSignatureUnsupportedBindingKind                = "factory.invocationSignature.unsupportedBindingKind"
	CodeInvocationSignatureInvalidPositionalOrdering             = "factory.invocationSignature.invalidPositionalOrdering"
	CodeInvocationSignatureMultipleVariadicPositionals           = "factory.invocationSignature.multipleVariadicPositionals"
	CodeInvocationSignatureInvalidDefaultShape                   = "factory.invocationSignature.invalidDefaultShape"
	CodeInvocationSignatureInvalidDefaultChoice                  = "factory.invocationSignature.invalidDefaultChoice"
	CodeInvocationSignatureInvalidStdinRouting                   = "factory.invocationSignature.invalidStdinRouting"
	CodeInvocationSignatureSensitivePositional                   = "factory.invocationSignature.sensitivePositional"
	CodeInvocationSignatureInvalidNamedRestShape                 = "factory.invocationSignature.invalidNamedRestShape"
	CodeInvocationSignatureInvalidRepeatedBindingShape           = "factory.invocationSignature.invalidRepeatedBindingShape"
	CodeInvocationSignatureUnsupportedOutputContractMode         = "factory.invocationSignature.unsupportedOutputContractMode"
	CodeInvocationSignatureUnknownOutputPathParameter            = "factory.invocationSignature.unknownOutputPathParameter"
	CodeInvocationSignatureInvalidOutputPathParameter            = "factory.invocationSignature.invalidOutputPathParameter"
	CodeInvocationSignatureInvalidInterpolationReference         = "factory.invocationSignature.invalidInterpolationReference"
	CodeInvocationSignatureIncompatibleInterpolationReference    = "factory.invocationSignature.incompatibleInterpolationReference"
	CodeWorkstationUnsupportedWorkPropagationMode                = "factory.workstation.unsupportedWorkPropagationMode"
	CodeManagedRuntimeUnsupportedIdentity                        = "factory.managedRuntime.unsupportedIdentity"
	CodeManagedRuntimeInvalidBackend                             = "factory.managedRuntime.invalidBackend"
	CodeManagedRuntimeInvalidLoadPolicy                          = "factory.managedRuntime.invalidLoadPolicy"
	CodeManagedRuntimeWorkerMissingModel                         = "factory.managedRuntime.workerMissingModel"
	CodeManagedRuntimeWorkerMissingDep                           = "factory.managedRuntime.workerMissingDependency"
	CodeManagedRuntimeWorkerModelMismatch                        = "factory.managedRuntime.workerModelMismatch"
	CodeLayoutUnknownNodeReference                               = "factory.layout.unknownNodeReference"
	CodeLayoutUnknownEdgeReference                               = "factory.layout.unknownEdgeReference"
	CodeLayoutUnknownGroupMemberReference                        = "factory.layout.unknownGroupMemberReference"
	CodeLayoutUnsupportedSchemaVersion                           = "factory.layout.unsupportedSchemaVersion"
	CodeLayoutInvalidGeometry                                    = "factory.layout.invalidGeometry"
	CodeLayoutInvalidValue                                       = "factory.layout.invalidValue"
	CodeLayoutImageBudgetExceeded                                = "factory.layout.imageBudgetExceeded"
	CodeLayoutEmptyStateUnknownNodeReference                     = "factory.layout.emptyState.unknownNodeReference"
	CodeOrchestratorUnsupportedKind                              = "factory.orchestrator.unsupportedKind"
	CodeOrchestratorIncompatiblePetriConfig                      = "factory.orchestrator.incompatiblePetriConfig"
	CodeOrchestratorIncompatibleJavaScriptConfig                 = "factory.orchestrator.incompatibleJavaScriptConfig"
	CodeOrchestratorIncompatiblePetriField                       = "factory.orchestrator.incompatiblePetriField"
	CodeOrchestratorJavaScriptMissingConfig                      = "factory.orchestrator.javascriptMissingConfig"
	CodeOrchestratorJavaScriptMissingSource                      = "factory.orchestrator.javascriptMissingSource"
	CodeOrchestratorJavaScriptConflictingSource                  = "factory.orchestrator.javascriptConflictingSource"
	CodeOrchestratorJavaScriptInvalidInlineEncoding              = "factory.orchestrator.javascriptInvalidInlineEncoding"
	CodeOrchestratorJavaScriptInvalidAgent                       = "factory.orchestrator.javascriptInvalidAgent"
	CodeWorkerWorkstationBehaviorCompatibility                   = "workstation-worker-behavior-compatibility"
	CodePollerRunWorkstationKindMismatch                         = "workstation-poller-run-kind-mismatch"
	CodeRequiredToolName                                         = "factory.requiredTool.name"
	CodeRequiredToolCommand                                      = "factory.requiredTool.command"
	CodeRequiredToolVersionArgs                                  = "factory.requiredTool.versionArgs"
	CodeRequiredToolMissing                                      = "factory.requiredTool.missing"
	CodeRequiredToolVersionProbe                                 = "factory.requiredTool.versionProbe"
)

var invocationSignatureReferencePattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)

var supportedInvocationTypeHints = []string{
	factorydefinitions.InvocationParameterTypeHintString,
	factorydefinitions.InvocationParameterTypeHintPath,
	factorydefinitions.InvocationParameterTypeHintFilePath,
	factorydefinitions.InvocationParameterTypeHintDirectoryPath,
	factorydefinitions.InvocationParameterTypeHintNumberString,
	factorydefinitions.InvocationParameterTypeHintBooleanString,
	factorydefinitions.InvocationParameterTypeHintJSON,
}

var supportedInvocationValueModes = []string{
	factorydefinitions.InvocationParameterValueModeExact,
	factorydefinitions.InvocationParameterValueModeRepeated,
	factorydefinitions.InvocationParameterValueModeVariadic,
	factorydefinitions.InvocationParameterValueModeFileContents,
}

var supportedInvocationBindingKinds = []string{
	factorydefinitions.InvocationParameterBindingKindPositional,
	factorydefinitions.InvocationParameterBindingKindNamed,
	factorydefinitions.InvocationParameterBindingKindStdin,
	factorydefinitions.InvocationParameterBindingKindNamedRest,
}

var supportedUnknownNamedArgumentPolicies = []string{
	factorydefinitions.InvocationUnknownNamedArgumentPolicyReject,
	factorydefinitions.InvocationUnknownNamedArgumentPolicyAllow,
	factorydefinitions.InvocationUnknownNamedArgumentPolicyCollect,
}

var supportedOutputContractModes = []string{
	factorydefinitions.InvocationOutputContractModeInline,
	factorydefinitions.InvocationOutputContractModeFile,
	factorydefinitions.InvocationOutputContractModeJSON,
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

type invocationSignatureAggregateState struct {
	parametersByName        map[string]invocationParameterSummary
	namedKeys               map[string]string
	positionalNames         map[int]string
	positionalSlots         []int
	stdinParameters         []string
	namedRestParameters     []string
	variadicPositionalNames []string
}

type invocationSignatureNamedKey struct {
	value string
	field string
}

type invocationSignatureBindingState struct {
	hasPositionalBinding bool
	hasNamedBinding      bool
	hasNamedRestBinding  bool
	hasStdinBinding      bool
}

// InvocationSignatureTargets validates the public invocation signature contract
// and supported ${parameter} references before runtime normalization runs.
func InvocationSignatureTargets(cfg *factorydefinitions.FactoryConfig) []Target {
	if cfg == nil || cfg.InvocationSignature == nil {
		return nil
	}
	signature := cfg.InvocationSignature
	targets := invocationSignatureUnknownNamedArgumentPolicyTargets(signature)
	state := invocationSignatureAggregateState{
		parametersByName: make(map[string]invocationParameterSummary, len(signature.Parameters)),
		namedKeys:        map[string]string{},
		positionalNames:  map[int]string{},
	}
	for index, parameter := range signature.Parameters {
		targets = append(targets, invocationSignatureParameterTargets(parameter, index)...)
		targets = append(targets, invocationSignatureAggregateParameterTargets(parameter, index, &state)...)
	}
	targets = append(targets, invocationSignatureAggregateStateTargets(signature.UnknownNamedArgumentPolicy, state)...)
	if signature.OutputContract != nil {
		targets = append(targets, invocationSignatureOutputContractTargets(signature.OutputContract, state.parametersByName)...)
	}
	return append(targets, invocationSignatureInterpolationTargets(cfg, state.parametersByName)...)
}

func invocationSignatureUnknownNamedArgumentPolicyTargets(signature *factorydefinitions.InvocationSignatureConfig) []Target {
	policy := strings.TrimSpace(signature.UnknownNamedArgumentPolicy)
	if policy == "" || slices.Contains(supportedUnknownNamedArgumentPolicies, policy) {
		return nil
	}
	return []Target{invocationSignatureTarget(
		CodeInvocationSignatureUnsupportedUnknownNamedArgumentPolicy,
		"unknownNamedArgumentPolicy",
		fmt.Sprintf("invocationSignature.unknownNamedArgumentPolicy %q is not supported", signature.UnknownNamedArgumentPolicy),
	)}
}

func invocationSignatureAggregateParameterTargets(parameter factorydefinitions.InvocationParameterConfig, index int, state *invocationSignatureAggregateState) []Target {
	var targets []Target
	name := strings.TrimSpace(parameter.Name)
	targets = append(targets, registerInvocationParameterName(parameter, index, state)...)
	targets = append(targets, registerInvocationNamedKeys(parameter, index, name, state)...)
	targets = append(targets, registerInvocationAggregateBindings(parameter, index, name, state)...)
	return targets
}

func registerInvocationParameterName(parameter factorydefinitions.InvocationParameterConfig, index int, state *invocationSignatureAggregateState) []Target {
	name := strings.TrimSpace(parameter.Name)
	if name == "" {
		return nil
	}
	if _, exists := state.parametersByName[name]; exists {
		return []Target{invocationSignatureParameterTarget(
			CodeInvocationSignatureDuplicateParameterName,
			index,
			"name",
			name,
			fmt.Sprintf("invocationSignature parameter name %q is declared more than once", name),
		)}
	}
	state.parametersByName[name] = invocationParameterSummary{name: name, valueMode: strings.TrimSpace(parameter.ValueMode)}
	return nil
}

func registerInvocationNamedKeys(parameter factorydefinitions.InvocationParameterConfig, index int, name string, state *invocationSignatureAggregateState) []Target {
	var targets []Target
	for _, key := range invocationSignatureNamedKeys(parameter) {
		if previousName, exists := state.namedKeys[key.value]; exists {
			targets = append(targets, invocationSignatureParameterTarget(
				CodeInvocationSignatureDuplicateNamedKey,
				index,
				key.field,
				name,
				fmt.Sprintf("invocationSignature named argument key %q is already used by parameter %q", key.value, previousName),
			))
			continue
		}
		state.namedKeys[key.value] = name
	}
	return targets
}

func invocationSignatureNamedKeys(parameter factorydefinitions.InvocationParameterConfig) []invocationSignatureNamedKey {
	var keys []invocationSignatureNamedKey
	if externalName := strings.TrimSpace(parameter.ExternalName); externalName != "" {
		keys = append(keys, invocationSignatureNamedKey{value: externalName, field: "externalName"})
	}
	for aliasIndex, alias := range parameter.Aliases {
		if trimmedAlias := strings.TrimSpace(alias); trimmedAlias != "" {
			keys = append(keys, invocationSignatureNamedKey{
				value: trimmedAlias,
				field: fmt.Sprintf("aliases[%d]", aliasIndex),
			})
		}
	}
	return keys
}

func registerInvocationAggregateBindings(parameter factorydefinitions.InvocationParameterConfig, index int, name string, state *invocationSignatureAggregateState) []Target {
	var targets []Target
	for bindingIndex, binding := range parameter.Bindings {
		switch strings.TrimSpace(binding.Kind) {
		case factorydefinitions.InvocationParameterBindingKindPositional:
			targets = append(targets, registerInvocationPositionalBinding(parameter, index, bindingIndex, binding.Position, name, state)...)
		case factorydefinitions.InvocationParameterBindingKindStdin:
			state.stdinParameters = append(state.stdinParameters, name)
		case factorydefinitions.InvocationParameterBindingKindNamedRest:
			state.namedRestParameters = append(state.namedRestParameters, name)
		}
	}
	return targets
}

func registerInvocationPositionalBinding(parameter factorydefinitions.InvocationParameterConfig, index int, bindingIndex int, position int, name string, state *invocationSignatureAggregateState) []Target {
	state.positionalSlots = append(state.positionalSlots, position)
	if strings.TrimSpace(parameter.ValueMode) == factorydefinitions.InvocationParameterValueModeVariadic {
		state.variadicPositionalNames = append(state.variadicPositionalNames, name)
	}
	if previousName, exists := state.positionalNames[position]; exists {
		return []Target{invocationSignatureParameterBindingTarget(
			CodeInvocationSignatureInvalidPositionalOrdering,
			index,
			bindingIndex,
			name,
			fmt.Sprintf("positional slot %d is already bound to parameter %q", position, previousName),
		)}
	}
	state.positionalNames[position] = name
	return nil
}

func invocationSignatureAggregateStateTargets(policy string, state invocationSignatureAggregateState) []Target {
	var targets []Target
	targets = append(targets, invocationSignatureVariadicPositionalTargets(state.variadicPositionalNames)...)
	targets = append(targets, invocationSignaturePositionalOrderingTargets(state.positionalSlots)...)
	targets = append(targets, invocationSignatureStdinRoutingTargets(state.stdinParameters)...)
	return append(targets, invocationSignatureNamedRestTargets(policy, state.namedRestParameters)...)
}

func invocationSignatureVariadicPositionalTargets(names []string) []Target {
	if len(names) <= 1 {
		return nil
	}
	return []Target{invocationSignatureTarget(
		CodeInvocationSignatureMultipleVariadicPositionals,
		"parameters",
		fmt.Sprintf("invocationSignature declares multiple variadic positional parameters (%s)", strings.Join(names, ", ")),
	)}
}

func invocationSignaturePositionalOrderingTargets(positionalSlots []int) []Target {
	if len(positionalSlots) == 0 {
		return nil
	}
	slices.Sort(positionalSlots)
	for index, slot := range positionalSlots {
		want := index + 1
		if slot == want {
			continue
		}
		return []Target{invocationSignatureTarget(
			CodeInvocationSignatureInvalidPositionalOrdering,
			"parameters",
			fmt.Sprintf("invocationSignature positional bindings must start at 1 and stay contiguous; found slot %d where slot %d was expected", slot, want),
		)}
	}
	return nil
}

func invocationSignatureStdinRoutingTargets(stdinParameters []string) []Target {
	if len(stdinParameters) <= 1 {
		return nil
	}
	return []Target{invocationSignatureTarget(
		CodeInvocationSignatureInvalidStdinRouting,
		"parameters",
		fmt.Sprintf("invocationSignature routes stdin to multiple parameters (%s)", strings.Join(stdinParameters, ", ")),
	)}
}

func invocationSignatureNamedRestTargets(policy string, namedRestParameters []string) []Target {
	var targets []Target
	if len(namedRestParameters) > 1 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidNamedRestShape,
			"parameters",
			fmt.Sprintf("invocationSignature declares multiple NAMED_REST parameters (%s)", strings.Join(namedRestParameters, ", ")),
		))
	}
	trimmedPolicy := strings.TrimSpace(policy)
	if trimmedPolicy == factorydefinitions.InvocationUnknownNamedArgumentPolicyCollect && len(namedRestParameters) != 1 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidNamedRestShape,
			"unknownNamedArgumentPolicy",
			"invocationSignature.unknownNamedArgumentPolicy COLLECT requires exactly one parameter with a NAMED_REST binding",
		))
	}
	if trimmedPolicy != factorydefinitions.InvocationUnknownNamedArgumentPolicyCollect && len(namedRestParameters) > 0 {
		targets = append(targets, invocationSignatureTarget(
			CodeInvocationSignatureInvalidNamedRestShape,
			"unknownNamedArgumentPolicy",
			"invocationSignature parameters can only use NAMED_REST when unknownNamedArgumentPolicy is COLLECT",
		))
	}
	return targets
}

func invocationSignatureParameterTargets(parameter factorydefinitions.InvocationParameterConfig, index int) []Target {
	var targets []Target
	targets = append(targets, invocationSignatureParameterTypeTargets(parameter, index)...)
	targets = append(targets, invocationSignatureParameterDefaultTargets(parameter, index)...)
	targets = append(targets, invocationSignatureParameterChoiceTargets(parameter, index)...)
	bindingState, bindingTargets := invocationSignatureParameterBindingTargets(parameter, index)
	targets = append(targets, bindingTargets...)
	return append(targets, invocationSignatureParameterBindingShapeTargets(parameter, index, bindingState)...)
}

func invocationSignatureParameterTypeTargets(parameter factorydefinitions.InvocationParameterConfig, index int) []Target {
	var targets []Target
	name := strings.TrimSpace(parameter.Name)
	if typeHint := strings.TrimSpace(parameter.TypeHint); typeHint != "" && !slices.Contains(supportedInvocationTypeHints, typeHint) {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureUnsupportedTypeHint,
			index,
			"typeHint",
			name,
			fmt.Sprintf("invocationSignature parameter %q uses unsupported typeHint %q", name, parameter.TypeHint),
		))
	}
	if valueMode := strings.TrimSpace(parameter.ValueMode); valueMode != "" && !slices.Contains(supportedInvocationValueModes, valueMode) {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureUnsupportedValueMode,
			index,
			"valueMode",
			name,
			fmt.Sprintf("invocationSignature parameter %q uses unsupported valueMode %q", name, parameter.ValueMode),
		))
	}
	return targets
}

func invocationSignatureParameterDefaultTargets(parameter factorydefinitions.InvocationParameterConfig, index int) []Target {
	var targets []Target
	name := strings.TrimSpace(parameter.Name)
	valueMode := strings.TrimSpace(parameter.ValueMode)
	if parameter.DefaultValue != "" && len(parameter.DefaultValues) > 0 {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureInvalidDefaultShape,
			index,
			"defaultValue",
			name,
			fmt.Sprintf("invocationSignature parameter %q cannot declare both defaultValue and defaultValues", name),
		))
	}
	if invocationParameterSupportsListDefaults(valueMode) && parameter.DefaultValue != "" {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureInvalidDefaultShape,
			index,
			"defaultValue",
			name,
			fmt.Sprintf("invocationSignature parameter %q only supports defaultValues for its multi-value mode", name),
		))
	}
	if !invocationParameterSupportsListDefaults(valueMode) && len(parameter.DefaultValues) > 0 {
		targets = append(targets, invocationSignatureParameterTarget(
			CodeInvocationSignatureInvalidDefaultShape,
			index,
			"defaultValues",
			name,
			fmt.Sprintf("invocationSignature parameter %q only supports defaultValue for its single-value mode", name),
		))
	}
	return targets
}

func invocationParameterSupportsListDefaults(valueMode string) bool {
	switch valueMode {
	case factorydefinitions.InvocationParameterValueModeRepeated, factorydefinitions.InvocationParameterValueModeVariadic:
		return true
	default:
		return false
	}
}

func invocationSignatureParameterChoiceTargets(parameter factorydefinitions.InvocationParameterConfig, index int) []Target {
	if len(parameter.Choices) == 0 {
		return nil
	}
	var targets []Target
	name := strings.TrimSpace(parameter.Name)
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
	return targets
}

func invocationSignatureParameterBindingTargets(parameter factorydefinitions.InvocationParameterConfig, index int) (invocationSignatureBindingState, []Target) {
	var targets []Target
	var state invocationSignatureBindingState
	name := strings.TrimSpace(parameter.Name)
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
		case factorydefinitions.InvocationParameterBindingKindPositional:
			state.hasPositionalBinding = true
			if binding.Position < 1 {
				targets = append(targets, invocationSignatureParameterBindingTarget(
					CodeInvocationSignatureInvalidPositionalOrdering,
					index,
					bindingIndex,
					name,
					fmt.Sprintf("invocationSignature parameter %q positional bindings must use positions starting at 1", name),
				))
			}
		case factorydefinitions.InvocationParameterBindingKindNamed:
			state.hasNamedBinding = true
		case factorydefinitions.InvocationParameterBindingKindStdin:
			state.hasStdinBinding = true
		case factorydefinitions.InvocationParameterBindingKindNamedRest:
			state.hasNamedRestBinding = true
		}
	}
	return state, targets
}

func invocationSignatureParameterBindingShapeTargets(parameter factorydefinitions.InvocationParameterConfig, index int, state invocationSignatureBindingState) []Target {
	var targets []Target
	name := strings.TrimSpace(parameter.Name)
	valueMode := strings.TrimSpace(parameter.ValueMode)
	if parameter.Sensitive && state.hasPositionalBinding {
		targets = append(targets, invocationSignatureParameterTarget(CodeInvocationSignatureSensitivePositional, index, "bindings", name, fmt.Sprintf("invocationSignature parameter %q is sensitive and cannot be exposed as a positional argument", name)))
	}
	if state.hasStdinBinding && state.hasNamedBinding {
		targets = append(targets, invocationSignatureParameterTarget(CodeInvocationSignatureInvalidStdinRouting, index, "bindings", name, fmt.Sprintf("invocationSignature parameter %q cannot combine STDIN and NAMED bindings", name)))
	}
	if state.hasStdinBinding && state.hasNamedRestBinding {
		targets = append(targets, invocationSignatureParameterTarget(CodeInvocationSignatureInvalidStdinRouting, index, "bindings", name, fmt.Sprintf("invocationSignature parameter %q cannot combine STDIN and NAMED_REST bindings", name)))
	}
	targets = append(targets, invocationSignatureNamedRestBindingTargets(parameter, index, name, valueMode, state)...)
	targets = append(targets, invocationSignatureRepeatedBindingTargets(index, name, valueMode, state)...)
	return append(targets, invocationSignatureVariadicBindingTargets(index, name, valueMode, state)...)
}

func invocationSignatureNamedRestBindingTargets(parameter factorydefinitions.InvocationParameterConfig, index int, name string, valueMode string, state invocationSignatureBindingState) []Target {
	if !state.hasNamedRestBinding {
		return nil
	}
	var targets []Target
	if valueMode != factorydefinitions.InvocationParameterValueModeRepeated {
		targets = append(targets, invocationSignatureParameterTarget(CodeInvocationSignatureInvalidNamedRestShape, index, "bindings", name, fmt.Sprintf("invocationSignature parameter %q must use valueMode REPEATED when it declares a NAMED_REST binding", name)))
	}
	if state.hasPositionalBinding || state.hasNamedBinding || state.hasStdinBinding || len(parameter.Bindings) != 1 {
		targets = append(targets, invocationSignatureParameterTarget(CodeInvocationSignatureInvalidNamedRestShape, index, "bindings", name, fmt.Sprintf("invocationSignature parameter %q must dedicate its bindings to NAMED_REST only", name)))
	}
	return targets
}

func invocationSignatureRepeatedBindingTargets(index int, name string, valueMode string, state invocationSignatureBindingState) []Target {
	if valueMode != factorydefinitions.InvocationParameterValueModeRepeated || (!state.hasPositionalBinding && !state.hasStdinBinding) {
		return nil
	}
	return []Target{invocationSignatureParameterTarget(
		CodeInvocationSignatureInvalidRepeatedBindingShape,
		index,
		"bindings",
		name,
		fmt.Sprintf("invocationSignature parameter %q with valueMode REPEATED may only use NAMED or NAMED_REST bindings", name),
	)}
}

func invocationSignatureVariadicBindingTargets(index int, name string, valueMode string, state invocationSignatureBindingState) []Target {
	if valueMode != factorydefinitions.InvocationParameterValueModeVariadic {
		return nil
	}
	if state.hasPositionalBinding && !state.hasNamedBinding && !state.hasStdinBinding && !state.hasNamedRestBinding {
		return nil
	}
	return []Target{invocationSignatureParameterTarget(
		CodeInvocationSignatureInvalidRepeatedBindingShape,
		index,
		"bindings",
		name,
		fmt.Sprintf("invocationSignature parameter %q with valueMode VARIADIC must use POSITIONAL bindings only", name),
	)}
}

func invocationSignatureOutputContractTargets(output *factorydefinitions.InvocationOutputContractConfig, parameters map[string]invocationParameterSummary) []Target {
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
		return append(targets, invocationSignatureTarget(
			CodeInvocationSignatureUnknownOutputPathParameter,
			"outputContract.pathParameter",
			fmt.Sprintf("invocationSignature.outputContract.pathParameter %q does not match a declared invocation parameter", pathParameter),
		))
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

func invocationSignatureInterpolationTargets(cfg *factorydefinitions.FactoryConfig, parameters map[string]invocationParameterSummary) []Target {
	fields := invocationInterpolationFieldTargets(cfg)
	var targets []Target
	for _, field := range fields {
		references := invocationSignatureReferencePattern.FindAllStringSubmatch(field.value, -1)
		for _, reference := range references {
			targets = append(targets, invocationSignatureInterpolationReferenceTargets(field, reference[1], parameters)...)
		}
	}
	return targets
}

func invocationSignatureInterpolationReferenceTargets(field interpolationFieldTarget, parameterName string, parameters map[string]invocationParameterSummary) []Target {
	parameter, ok := parameters[parameterName]
	if !ok {
		return []Target{invocationSignatureInterpolationFieldTarget(
			field,
			CodeInvocationSignatureInvalidInterpolationReference,
			fmt.Sprintf("%s references invocation parameter %q, but that parameter is not declared", field.fieldDescriptor, parameterName),
		)}
	}
	if field.allowsRepeated || invocationParameterSupportsScalarInterpolation(parameter.valueMode) {
		return nil
	}
	return []Target{invocationSignatureInterpolationFieldTarget(
		field,
		CodeInvocationSignatureIncompatibleInterpolationReference,
		fmt.Sprintf("%s cannot reference multi-value invocation parameter %q", field.fieldDescriptor, parameterName),
	)}
}

func invocationSignatureInterpolationFieldTarget(field interpolationFieldTarget, code string, message string) Target {
	return Target{
		Code:     code,
		Severity: SeverityError,
		Message:  message,
		Subject: Subject{
			Type:     field.subjectType,
			ID:       field.subjectID,
			Location: field.location,
		},
		Path: field.path,
	}
}

func invocationInterpolationFieldTargets(cfg *factorydefinitions.FactoryConfig) []interpolationFieldTarget {
	fields := invocationWorkerInterpolationFieldTargets(cfg.Workers)
	return append(fields, invocationWorkstationInterpolationFieldTargets(cfg.Workstations)...)
}

func invocationWorkerInterpolationFieldTargets(workers []workerconfig.Config) []interpolationFieldTarget {
	var fields []interpolationFieldTarget
	for workerIndex, worker := range workers {
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
	return fields
}

func invocationWorkstationInterpolationFieldTargets(workstations []factorydefinitions.FactoryWorkstationConfig) []interpolationFieldTarget {
	var fields []interpolationFieldTarget
	for workstationIndex, workstation := range workstations {
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

func appendInterpolationField(fields []interpolationFieldTarget, subjectType SubjectType, subjectID string, location SubjectLocation, path string, value string, allowsRepeated bool, fieldDescriptor string) []interpolationFieldTarget {
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
	case factorydefinitions.InvocationParameterValueModeRepeated, factorydefinitions.InvocationParameterValueModeVariadic:
		return false
	default:
		return true
	}
}

func invocationSignatureTarget(code string, field string, message string) Target {
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
