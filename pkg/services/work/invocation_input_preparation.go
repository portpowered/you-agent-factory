package work

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// InvocationInputPreparationRequest contains the raw invocation values
// observed at a transport edge. Work owns interpreting those values; the
// transport owns only collecting argv, stdin, and terminal metadata.
type InvocationInputPreparationRequest struct {
	Arguments []string
	Signature *InvocationSignatureConfig
	StdinText *string
}

// PreparedInvocationInput is a detached canonical invocation input. Exactly
// one of ResolvedInput and NormalizedArguments is populated when input exists.
type PreparedInvocationInput struct {
	Source              InputSourceLabel
	ResolvedInput       *ResolvedInput
	NormalizedArguments *NormalizedArguments
}

// InvocationInputPreparation is the exact Work-owned role used by transports
// to turn raw invocation-edge values into canonical Work input.
type InvocationInputPreparation interface {
	PrepareInvocationInput(context.Context, InvocationInputPreparationRequest) (PreparedInvocationInput, error)
}

type invocationInputPreparation struct{}

// NewInvocationInputPreparation constructs Work's invocation-input policy.
// Wire is the sole application caller of this constructor.
func NewInvocationInputPreparation() InvocationInputPreparation {
	return invocationInputPreparation{}
}

// InvocationExampleNormalizer owns pure compatibility normalization for
// retired Factory invocation examples.
type InvocationExampleNormalizer struct{}

// NormalizeLegacyInvocationExample converts the retired argv/stdin example
// carrier through Work's canonical argument policy without starting a runtime
// service or performing IO. It exists only for Factory definition read
// compatibility; canonical examples already contain structured arguments.
func (InvocationExampleNormalizer) NormalizeLegacyInvocationExample(
	arguments []string,
	signature *InvocationSignatureConfig,
	stdinText *string,
) (*NormalizedArguments, error) {
	positional, named, _, err := parseInvocationArguments(arguments, signature)
	if err != nil {
		return nil, err
	}
	result, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signature, PositionalArgs: positional, NamedArgs: named, StdinText: stdinText,
	})
	if err != nil {
		return nil, err
	}
	if result.CompatibilityInput != nil {
		return nil, errors.New("legacy example does not resolve to structured invocation arguments")
	}
	return cloneNormalizedArguments(&result), nil
}

func (invocationInputPreparation) PrepareInvocationInput(
	ctx context.Context,
	request InvocationInputPreparationRequest,
) (PreparedInvocationInput, error) {
	if ctx == nil {
		return PreparedInvocationInput{}, errors.New("Work invocation-input preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return PreparedInvocationInput{}, err
	}

	positional, named, _, err := parseInvocationArguments(request.Arguments, request.Signature)
	if err != nil {
		return PreparedInvocationInput{}, err
	}

	if request.Signature == nil && len(positional) == 0 && request.StdinText == nil {
		return PreparedInvocationInput{}, nil
	}
	input := NormalizeArgumentsInput{
		Signature:      request.Signature,
		PositionalArgs: positional,
		NamedArgs:      named,
	}
	input.StdinText = request.StdinText
	result, err := NormalizeArguments(input)
	if err != nil {
		return PreparedInvocationInput{}, err
	}
	prepared := PreparedInvocationInput{NormalizedArguments: cloneNormalizedArguments(&result)}
	if result.CompatibilityInput != nil {
		resolved := cloneResolvedInput(*result.CompatibilityInput)
		prepared.Source = resolved.Source
		prepared.ResolvedInput = &resolved
		prepared.NormalizedArguments = nil
	}
	return prepared, nil
}

func parseInvocationArguments(
	arguments []string,
	signature *InvocationSignatureConfig,
) ([]string, []NamedArgumentInput, bool, error) {
	positional := make([]string, 0, len(arguments))
	named := make([]NamedArgumentInput, 0)
	explicitStdin := false
	booleanNamedKeys := invocationBooleanNamedKeys(signature)
	for index := 0; index < len(arguments); index++ {
		token := arguments[index]
		if strings.TrimSpace(token) == "-" {
			explicitStdin = true
			continue
		}
		if signature == nil || !strings.HasPrefix(token, "--") || token == "--" {
			positional = append(positional, token)
			continue
		}

		raw := strings.TrimPrefix(token, "--")
		if name, value, ok := strings.Cut(raw, "="); ok {
			named = append(named, NamedArgumentInput{Key: strings.TrimSpace(name), Values: []string{value}})
			continue
		}
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, nil, false, errors.New("factory argument name is required after --")
		}
		if booleanNamedKeys[name] {
			if index+1 < len(arguments) && isExplicitBooleanStringValue(arguments[index+1]) {
				index++
				named = append(named, NamedArgumentInput{Key: name, Values: []string{arguments[index]}})
			} else {
				named = append(named, NamedArgumentInput{Key: name, Values: []string{"true"}})
			}
			continue
		}
		if index+1 >= len(arguments) {
			return nil, nil, false, fmt.Errorf("factory argument --%s requires a value", name)
		}
		index++
		named = append(named, NamedArgumentInput{Key: name, Values: []string{arguments[index]}})
	}
	return positional, named, explicitStdin, nil
}

func invocationBooleanNamedKeys(signature *InvocationSignatureConfig) map[string]bool {
	keys := map[string]bool{}
	if signature == nil {
		return keys
	}
	for _, parameter := range signature.Parameters {
		if strings.TrimSpace(parameter.TypeHint) != typeHintBooleanString {
			continue
		}
		hasNamedBinding := false
		for _, binding := range parameter.Bindings {
			if strings.TrimSpace(binding.Kind) == bindingKindNamed {
				hasNamedBinding = true
				break
			}
		}
		if !hasNamedBinding {
			continue
		}
		for _, key := range append([]string{parameter.Name, parameter.ExternalName}, parameter.Aliases...) {
			if key = strings.TrimSpace(key); key != "" {
				keys[key] = true
			}
		}
	}
	return keys
}

func isExplicitBooleanStringValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "false", "1", "0", "yes", "no", "on", "off":
		return true
	default:
		return false
	}
}

func cloneResolvedInput(input ResolvedInput) ResolvedInput {
	input.Content = CloneWorkContentParts(input.Content)
	return input
}

func cloneNormalizedArguments(input *NormalizedArguments) *NormalizedArguments {
	if input == nil {
		return nil
	}
	clone := &NormalizedArguments{
		Arguments:        make(map[string]NormalizedArgument, len(input.Arguments)),
		UnknownNamedArgs: make(map[string][]string, len(input.UnknownNamedArgs)),
	}
	for name, argument := range input.Arguments {
		argument.Values = slices.Clone(argument.Values)
		argument.Sources = slices.Clone(argument.Sources)
		clone.Arguments[name] = argument
	}
	for name, values := range input.UnknownNamedArgs {
		clone.UnknownNamedArgs[name] = slices.Clone(values)
	}
	if input.CompatibilityInput != nil {
		resolved := cloneResolvedInput(*input.CompatibilityInput)
		clone.CompatibilityInput = &resolved
	}
	return clone
}
