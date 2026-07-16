package invocation

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
)

const (
	bindingKindPositional = "POSITIONAL"
	bindingKindNamed      = "NAMED"
	bindingKindStdin      = "STDIN"
	bindingKindNamedRest  = "NAMED_REST"

	valueModeExact        = "EXACT"
	valueModeRepeated     = "REPEATED"
	valueModeVariadic     = "VARIADIC"
	valueModeFileContents = "FILE_CONTENTS"

	unknownNamedReject  = "REJECT"
	unknownNamedAllow   = "ALLOW"
	unknownNamedCollect = "COLLECT"

	typeHintString        = "STRING"
	typeHintPath          = "PATH"
	typeHintFilePath      = "FILE_PATH"
	typeHintDirectoryPath = "DIRECTORY_PATH"
	typeHintNumberString  = "NUMBER_STRING"
	typeHintBooleanString = "BOOLEAN_STRING"
)

type ArgumentSourceKind string

const (
	ArgumentSourceKindPositional           ArgumentSourceKind = "POSITIONAL"
	ArgumentSourceKindNamed                ArgumentSourceKind = "NAMED"
	ArgumentSourceKindStructured           ArgumentSourceKind = "STRUCTURED"
	ArgumentSourceKindStdin                ArgumentSourceKind = "STDIN"
	ArgumentSourceKindDefault              ArgumentSourceKind = "DEFAULT"
	ArgumentSourceKindCompatibilityText    ArgumentSourceKind = "COMPATIBILITY_TEXT"
	ArgumentSourceKindCompatibilityContent ArgumentSourceKind = "COMPATIBILITY_CONTENT"
)

type ArgumentErrorCode string

const (
	ArgumentErrorCodeInvalidActiveSignature   ArgumentErrorCode = "INVOCATION_ARGUMENT_INVALID_ACTIVE_SIGNATURE"
	ArgumentErrorCodeMissingRequiredInput     ArgumentErrorCode = "INVOCATION_ARGUMENT_MISSING_REQUIRED_INPUT"
	ArgumentErrorCodeUnknownArgument          ArgumentErrorCode = "INVOCATION_ARGUMENT_UNKNOWN_ARGUMENT"
	ArgumentErrorCodeSourceConflict           ArgumentErrorCode = "INVOCATION_ARGUMENT_SOURCE_CONFLICT"
	ArgumentErrorCodeStringValidationMismatch ArgumentErrorCode = "INVOCATION_ARGUMENT_STRING_VALIDATION_MISMATCH"
	ArgumentErrorCodePositionalOverflow       ArgumentErrorCode = "INVOCATION_ARGUMENT_POSITIONAL_OVERFLOW"
	ArgumentErrorCodeUnroutableStdin          ArgumentErrorCode = "INVOCATION_ARGUMENT_UNROUTABLE_STDIN"
)

type NamedArgumentInput struct {
	Key    string
	Values []string
}

type ArgumentSource struct {
	Kind   ArgumentSourceKind
	Name   string
	Redact bool
}

type NormalizedArgument struct {
	Values    []string
	Sensitive bool
	Sources   []ArgumentSource
}

type NormalizedArguments struct {
	Arguments          map[string]NormalizedArgument
	UnknownNamedArgs   map[string][]string
	CompatibilityInput *ResolvedInput
}

type NormalizeArgumentsInput struct {
	Signature            *interfaces.InvocationSignatureConfig
	PositionalArgs       []string
	NamedArgs            []NamedArgumentInput
	DirectArgs           []NamedArgumentInput
	StdinText            *string
	CompatibilityText    *string
	CompatibilityContent []work.WorkContentPart
}

type ArgumentError struct {
	Code       ArgumentErrorCode
	Message    string
	Parameter  string
	Argument   string
	SourceKind ArgumentSourceKind
}

func (e *ArgumentError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func NamedArgumentInputsFromAnyMap(values map[string]any) ([]NamedArgumentInput, error) {
	if len(values) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	inputs := make([]NamedArgumentInput, 0, len(keys))
	for _, key := range keys {
		parsed, err := namedArgumentValuesFromAny(values[key])
		if err != nil {
			return nil, fmt.Errorf("args.%s %w", key, err)
		}
		inputs = append(inputs, NamedArgumentInput{
			Key:    key,
			Values: parsed,
		})
	}
	return inputs, nil
}

func NormalizeArguments(input NormalizeArgumentsInput) (NormalizedArguments, error) {
	if input.Signature == nil {
		return normalizeCompatibilityArguments(input)
	}
	return normalizeSignatureArguments(input)
}

func namedArgumentValuesFromAny(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		return []string{typed}, nil
	case []string:
		return slices.Clone(typed), nil
	case []any:
		if len(typed) == 0 {
			return []string{}, nil
		}
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("must be a string or array of strings")
			}
			values = append(values, text)
		}
		return values, nil
	case json.RawMessage:
		var scalar string
		if err := json.Unmarshal(typed, &scalar); err == nil {
			return []string{scalar}, nil
		}
		var list []string
		if err := json.Unmarshal(typed, &list); err == nil {
			return list, nil
		}
		return nil, fmt.Errorf("must be a string or array of strings")
	default:
		return nil, fmt.Errorf("must be a string or array of strings")
	}
}

func normalizeCompatibilityArguments(input NormalizeArgumentsInput) (NormalizedArguments, error) {
	if len(input.NamedArgs) > 0 {
		return NormalizedArguments{}, &ArgumentError{
			Code:     ArgumentErrorCodeInvalidActiveSignature,
			Message:  "named arguments require a factory invocationSignature",
			Argument: strings.TrimSpace(input.NamedArgs[0].Key),
		}
	}
	if len(input.CompatibilityContent) > 0 {
		if input.CompatibilityText != nil || len(input.PositionalArgs) > 0 || input.StdinText != nil {
			return NormalizedArguments{}, &ArgumentError{
				Code:    ArgumentErrorCodeSourceConflict,
				Message: "compatibility content cannot be combined with positional or stdin input",
			}
		}
		resolved, err := ResolveAPITextInputContent(input.CompatibilityContent)
		if err != nil {
			return NormalizedArguments{}, err
		}
		resolved.Source = InputSourceLabel(ArgumentSourceKindCompatibilityContent)
		return NormalizedArguments{CompatibilityInput: &resolved}, nil
	}

	if input.CompatibilityText != nil && (len(input.PositionalArgs) > 0 || input.StdinText != nil) {
		return NormalizedArguments{}, &ArgumentError{
			Code:    ArgumentErrorCodeSourceConflict,
			Message: "compatibility text cannot be combined with positional or stdin input",
		}
	}

	sources := TextInputSources{StdinText: input.StdinText}
	if input.CompatibilityText != nil {
		sources.PositionalText = input.CompatibilityText
	} else if len(input.PositionalArgs) > 0 {
		text := strings.Join(input.PositionalArgs, " ")
		sources.PositionalText = &text
	}
	resolved, err := ResolveTextInput(sources)
	if err != nil {
		return NormalizedArguments{}, err
	}
	if input.CompatibilityText != nil {
		resolved.Source = InputSourceLabel(ArgumentSourceKindCompatibilityText)
	}
	return NormalizedArguments{CompatibilityInput: &resolved}, nil
}

func normalizeSignatureArguments(input NormalizeArgumentsInput) (NormalizedArguments, error) {
	if input.CompatibilityText != nil || len(input.CompatibilityContent) > 0 {
		return NormalizedArguments{}, &ArgumentError{
			Code:    ArgumentErrorCodeSourceConflict,
			Message: "compatibility text or content cannot be combined with invocationSignature arguments",
		}
	}

	index, err := buildSignatureIndex(input.Signature)
	if err != nil {
		return NormalizedArguments{}, err
	}
	state := normalizationState{
		arguments: make(map[string]NormalizedArgument, len(index.parameters)),
		unknown:   map[string][]string{},
	}

	if err := applyPositionalArguments(index, input.PositionalArgs, &state); err != nil {
		return NormalizedArguments{}, err
	}
	if err := applyNamedArguments(index, input.NamedArgs, &state); err != nil {
		return NormalizedArguments{}, err
	}
	if err := applyDirectArguments(index, input.DirectArgs, &state); err != nil {
		return NormalizedArguments{}, err
	}
	if err := applyStdinArgument(index, input.StdinText, &state); err != nil {
		return NormalizedArguments{}, err
	}
	if err := applyDefaultArguments(index, &state); err != nil {
		return NormalizedArguments{}, err
	}
	if err := validateRequiredArguments(index, state.arguments); err != nil {
		return NormalizedArguments{}, err
	}

	return NormalizedArguments{
		Arguments:        state.arguments,
		UnknownNamedArgs: state.unknown,
	}, nil
}

type signatureIndex struct {
	parameters       map[string]parameterDefinition
	orderedSlots     []int
	positionalBySlot map[int]string
	namedByKey       map[string]string
	stdinParameter   string
	namedRest        string
	unknownPolicy    string
}

type parameterDefinition struct {
	name      string
	valueMode string
	typeHint  string
	required  bool
	sensitive bool
	choices   []string
	named     bool
	stdin     bool
	namedRest bool
	variadic  bool
	defaults  []string
}

func buildSignatureIndex(signature *interfaces.InvocationSignatureConfig) (signatureIndex, error) {
	index := signatureIndex{
		parameters:       make(map[string]parameterDefinition, len(signature.Parameters)),
		positionalBySlot: map[int]string{},
		namedByKey:       map[string]string{},
		unknownPolicy:    strings.TrimSpace(signature.UnknownNamedArgumentPolicy),
	}
	for _, parameter := range signature.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			return signatureIndex{}, newArgumentError(ArgumentErrorCodeInvalidActiveSignature, "invocationSignature parameter name is required", "", "")
		}
		def := parameterDefinition{
			name:      name,
			valueMode: normalizedValueMode(parameter.ValueMode),
			typeHint:  strings.TrimSpace(parameter.TypeHint),
			required:  parameter.Required,
			sensitive: parameter.Sensitive,
			choices:   slices.Clone(parameter.Choices),
			defaults:  parameterDefaults(parameter),
		}
		for _, binding := range parameter.Bindings {
			switch strings.TrimSpace(binding.Kind) {
			case bindingKindPositional:
				index.positionalBySlot[binding.Position] = name
				index.orderedSlots = append(index.orderedSlots, binding.Position)
				if def.valueMode == valueModeVariadic {
					def.variadic = true
				}
			case bindingKindNamed:
				def.named = true
			case bindingKindStdin:
				def.stdin = true
				index.stdinParameter = name
			case bindingKindNamedRest:
				def.namedRest = true
				index.namedRest = name
			}
		}
		index.parameters[name] = def
		index.namedByKey[name] = name
		if external := strings.TrimSpace(parameter.ExternalName); external != "" {
			index.namedByKey[external] = name
		}
		for _, alias := range parameter.Aliases {
			if trimmed := strings.TrimSpace(alias); trimmed != "" {
				index.namedByKey[trimmed] = name
			}
		}
	}
	sort.Ints(index.orderedSlots)
	return index, nil
}

func parameterDefaults(parameter interfaces.InvocationParameterConfig) []string {
	if len(parameter.DefaultValues) > 0 {
		return slices.Clone(parameter.DefaultValues)
	}
	if parameter.DefaultValue == "" {
		return nil
	}
	return []string{parameter.DefaultValue}
}

func normalizedValueMode(valueMode string) string {
	trimmed := strings.TrimSpace(valueMode)
	if trimmed == "" {
		return valueModeExact
	}
	return trimmed
}

type normalizationState struct {
	arguments map[string]NormalizedArgument
	unknown   map[string][]string
}

func applyPositionalArguments(index signatureIndex, positionalArgs []string, state *normalizationState) error {
	if len(positionalArgs) == 0 {
		return nil
	}
	consumed := 0
	for _, slot := range index.orderedSlots {
		if consumed >= len(positionalArgs) {
			break
		}
		def := index.parameters[index.positionalBySlot[slot]]
		if def.variadic {
			values := slices.Clone(positionalArgs[slot-1:])
			return addArgumentValues(state.arguments, def, values, ArgumentSource{Kind: ArgumentSourceKindPositional, Name: fmt.Sprintf("%d+", slot)})
		}
		value := positionalArgs[slot-1]
		if err := addArgumentValues(state.arguments, def, []string{value}, ArgumentSource{Kind: ArgumentSourceKindPositional, Name: strconv.Itoa(slot)}); err != nil {
			return err
		}
		consumed = slot
	}
	if len(index.orderedSlots) == 0 || consumed < len(positionalArgs) {
		return &ArgumentError{
			Code:       ArgumentErrorCodePositionalOverflow,
			Message:    fmt.Sprintf("received %d positional arguments but the active invocationSignature only accepts %d", len(positionalArgs), consumed),
			SourceKind: ArgumentSourceKindPositional,
		}
	}
	return nil
}

func applyNamedArguments(index signatureIndex, namedArgs []NamedArgumentInput, state *normalizationState) error {
	for _, named := range namedArgs {
		key := strings.TrimSpace(named.Key)
		if key == "" {
			continue
		}
		paramName, ok := index.namedByKey[key]
		if ok {
			def := index.parameters[paramName]
			if !def.named {
				return &ArgumentError{
					Code:       ArgumentErrorCodeSourceConflict,
					Message:    fmt.Sprintf("parameter %q does not accept named input %q", def.name, key),
					Parameter:  def.name,
					Argument:   key,
					SourceKind: ArgumentSourceKindNamed,
				}
			}
			if err := addArgumentValues(state.arguments, def, named.Values, ArgumentSource{Kind: ArgumentSourceKindNamed, Name: key}); err != nil {
				return err
			}
			continue
		}
		switch index.unknownPolicy {
		case "", unknownNamedReject:
			return &ArgumentError{
				Code:       ArgumentErrorCodeUnknownArgument,
				Message:    fmt.Sprintf("unknown named argument %q", key),
				Argument:   key,
				SourceKind: ArgumentSourceKindNamed,
			}
		case unknownNamedAllow:
			state.unknown[key] = append(state.unknown[key], slices.Clone(named.Values)...)
		case unknownNamedCollect:
			def, ok := index.parameters[index.namedRest]
			if !ok {
				return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, "invocationSignature COLLECT policy requires a NAMED_REST parameter", "", key)
			}
			values := make([]string, 0, len(named.Values))
			for _, value := range named.Values {
				values = append(values, fmt.Sprintf("%s=%s", key, value))
			}
			if err := addArgumentValues(state.arguments, def, values, ArgumentSource{Kind: ArgumentSourceKindNamed, Name: key}); err != nil {
				return err
			}
		default:
			return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, "invocationSignature unknownNamedArgumentPolicy is invalid", "", key)
		}
	}
	return nil
}

func applyDirectArguments(index signatureIndex, directArgs []NamedArgumentInput, state *normalizationState) error {
	for _, direct := range directArgs {
		key := strings.TrimSpace(direct.Key)
		if key == "" {
			continue
		}
		paramName, ok := index.namedByKey[key]
		if ok {
			def := index.parameters[paramName]
			if err := addArgumentValues(state.arguments, def, direct.Values, ArgumentSource{Kind: ArgumentSourceKindStructured, Name: key}); err != nil {
				return err
			}
			continue
		}
		switch index.unknownPolicy {
		case "", unknownNamedReject:
			return &ArgumentError{
				Code:       ArgumentErrorCodeUnknownArgument,
				Message:    fmt.Sprintf("unknown named argument %q", key),
				Argument:   key,
				SourceKind: ArgumentSourceKindStructured,
			}
		case unknownNamedAllow:
			state.unknown[key] = append(state.unknown[key], slices.Clone(direct.Values)...)
		case unknownNamedCollect:
			def, ok := index.parameters[index.namedRest]
			if !ok {
				return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, "invocationSignature COLLECT policy requires a NAMED_REST parameter", "", key)
			}
			values := make([]string, 0, len(direct.Values))
			for _, value := range direct.Values {
				values = append(values, fmt.Sprintf("%s=%s", key, value))
			}
			if err := addArgumentValues(state.arguments, def, values, ArgumentSource{Kind: ArgumentSourceKindStructured, Name: key}); err != nil {
				return err
			}
		default:
			return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, "invocationSignature unknownNamedArgumentPolicy is invalid", "", key)
		}
	}
	return nil
}

func applyStdinArgument(index signatureIndex, stdinText *string, state *normalizationState) error {
	if stdinText == nil {
		return nil
	}
	if strings.TrimSpace(index.stdinParameter) == "" {
		return &ArgumentError{
			Code:       ArgumentErrorCodeUnroutableStdin,
			Message:    "invocationSignature does not route stdin to any parameter",
			SourceKind: ArgumentSourceKindStdin,
		}
	}
	def := index.parameters[index.stdinParameter]
	return addArgumentValues(state.arguments, def, []string{*stdinText}, ArgumentSource{Kind: ArgumentSourceKindStdin, Name: "stdin"})
}

func applyDefaultArguments(index signatureIndex, state *normalizationState) error {
	names := make([]string, 0, len(index.parameters))
	for name := range index.parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, exists := state.arguments[name]; exists {
			continue
		}
		def := index.parameters[name]
		if len(def.defaults) == 0 {
			continue
		}
		if err := addArgumentValues(state.arguments, def, def.defaults, ArgumentSource{Kind: ArgumentSourceKindDefault, Name: "default"}); err != nil {
			return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, err.Error(), name, "")
		}
	}
	return nil
}

func validateRequiredArguments(index signatureIndex, arguments map[string]NormalizedArgument) error {
	names := make([]string, 0, len(index.parameters))
	for name := range index.parameters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := index.parameters[name]
		if def.required {
			if argument, ok := arguments[name]; !ok || len(argument.Values) == 0 {
				return &ArgumentError{
					Code:      ArgumentErrorCodeMissingRequiredInput,
					Message:   fmt.Sprintf("required invocation parameter %q is missing", name),
					Parameter: name,
				}
			}
		}
	}
	return nil
}

func addArgumentValues(arguments map[string]NormalizedArgument, def parameterDefinition, values []string, source ArgumentSource) error {
	current := arguments[def.name]
	if current.Sources == nil {
		current.Sensitive = def.sensitive
	}
	if len(values) == 0 {
		values = []string{""}
	}
	for _, value := range values {
		if err := validateArgumentValue(def, value); err != nil {
			return err
		}
	}
	switch def.valueMode {
	case valueModeExact, valueModeFileContents:
		if len(values) != 1 {
			return &ArgumentError{
				Code:       ArgumentErrorCodeSourceConflict,
				Message:    fmt.Sprintf("parameter %q accepts exactly one value", def.name),
				Parameter:  def.name,
				Argument:   source.Name,
				SourceKind: source.Kind,
			}
		}
		if len(current.Values) > 0 {
			return &ArgumentError{
				Code:       ArgumentErrorCodeSourceConflict,
				Message:    fmt.Sprintf("parameter %q was supplied more than once", def.name),
				Parameter:  def.name,
				Argument:   source.Name,
				SourceKind: source.Kind,
			}
		}
		current.Values = append(current.Values, values[0])
	case valueModeRepeated, valueModeVariadic:
		current.Values = append(current.Values, values...)
	default:
		return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, fmt.Sprintf("parameter %q uses unsupported valueMode %q", def.name, def.valueMode), def.name, source.Name)
	}
	source.Redact = def.sensitive
	current.Sources = append(current.Sources, source)
	arguments[def.name] = current
	return nil
}

func validateArgumentValue(def parameterDefinition, value string) error {
	if len(def.choices) > 0 && !slices.Contains(def.choices, value) {
		return &ArgumentError{
			Code:      ArgumentErrorCodeStringValidationMismatch,
			Message:   fmt.Sprintf("parameter %q value %q is not one of the declared choices", def.name, value),
			Parameter: def.name,
		}
	}
	switch def.typeHint {
	case "", typeHintString:
		return nil
	case typeHintPath, typeHintFilePath, typeHintDirectoryPath:
		if strings.TrimSpace(value) == "" {
			return &ArgumentError{
				Code:      ArgumentErrorCodeStringValidationMismatch,
				Message:   fmt.Sprintf("parameter %q path value must not be empty", def.name),
				Parameter: def.name,
			}
		}
		return nil
	case typeHintBooleanString:
		if _, err := strconv.ParseBool(strings.TrimSpace(value)); err != nil {
			return &ArgumentError{
				Code:      ArgumentErrorCodeStringValidationMismatch,
				Message:   fmt.Sprintf("parameter %q value %q is not a valid BOOLEAN_STRING", def.name, value),
				Parameter: def.name,
			}
		}
	case typeHintNumberString:
		if _, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err != nil {
			return &ArgumentError{
				Code:      ArgumentErrorCodeStringValidationMismatch,
				Message:   fmt.Sprintf("parameter %q value %q is not a valid NUMBER_STRING", def.name, value),
				Parameter: def.name,
			}
		}
	default:
		return newArgumentError(ArgumentErrorCodeInvalidActiveSignature, fmt.Sprintf("parameter %q uses unsupported typeHint %q", def.name, def.typeHint), def.name, "")
	}
	return nil
}

func newArgumentError(code ArgumentErrorCode, message, parameter, argument string) *ArgumentError {
	return &ArgumentError{
		Code:      code,
		Message:   message,
		Parameter: parameter,
		Argument:  argument,
	}
}
