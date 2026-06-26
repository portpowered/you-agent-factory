package invocations

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var invocationInterpolationPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)

const ArgumentErrorCodeInvalidInterpolation ArgumentErrorCode = "INVOCATION_ARGUMENT_INVALID_INTERPOLATION"

// RuntimeInvocationArguments converts normalized invocation arguments into the
// runtime-owned transport-independent metadata carried on submitted work.
func RuntimeInvocationArguments(
	signature *interfaces.InvocationSignatureConfig,
	normalized *NormalizedArguments,
) *interfaces.InvocationArguments {
	if signature == nil || normalized == nil || len(normalized.Arguments) == 0 {
		return nil
	}
	valueModes := make(map[string]string, len(signature.Parameters))
	for _, parameter := range signature.Parameters {
		name := strings.TrimSpace(parameter.Name)
		if name == "" {
			continue
		}
		valueModes[name] = normalizedValueMode(parameter.ValueMode)
	}
	args := &interfaces.InvocationArguments{
		Arguments: make(map[string]interfaces.InvocationArgument, len(normalized.Arguments)),
	}
	for name, argument := range normalized.Arguments {
		next := interfaces.InvocationArgument{
			Values:    append([]string(nil), argument.Values...),
			ValueMode: valueModes[name],
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = make([]interfaces.InvocationArgumentSource, len(argument.Sources))
			for i, source := range argument.Sources {
				next.Sources[i] = interfaces.InvocationArgumentSource{
					Kind:   string(source.Kind),
					Name:   source.Name,
					Redact: source.Redact,
				}
			}
		}
		args.Arguments[name] = next
	}
	if len(args.Arguments) == 0 {
		return nil
	}
	return args
}

// ValidateInvocationInterpolation verifies that runtime-supported invocation
// interpolation can resolve the authored worker and workstation fields for the
// supplied normalized argument set without mutating the canonical runtime config.
func ValidateInvocationInterpolation(cfg *interfaces.FactoryConfig, args *interfaces.InvocationArguments) error {
	if cfg == nil || args == nil {
		return nil
	}
	for _, worker := range cfg.Workers {
		if _, err := InterpolateWorkerConfig(worker, args); err != nil {
			return err
		}
	}
	for _, workstation := range cfg.Workstations {
		if _, err := InterpolateWorkstationConfig(workstation, args); err != nil {
			return err
		}
	}
	if signature := cfg.InvocationSignature; signature != nil && signature.OutputContract != nil {
		if err := validateInvocationOutputContract(signature.OutputContract, args); err != nil {
			return err
		}
	}
	return nil
}

// InterpolateWorkerConfig resolves supported `${parameter}` placeholders on one
// effective worker definition using runtime invocation arguments.
func InterpolateWorkerConfig(worker interfaces.WorkerConfig, args *interfaces.InvocationArguments) (interfaces.WorkerConfig, error) {
	next := config.CloneWorkerConfig(worker)
	var err error
	if next.Provider, err = interpolateInvocationField(next.Provider, args, "worker.provider", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.Model, err = interpolateInvocationField(next.Model, args, "worker.model", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.ModelProvider, err = interpolateInvocationField(next.ModelProvider, args, "worker.modelProvider", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.ExecutorProvider, err = interpolateInvocationField(next.ExecutorProvider, args, "worker.executorProvider", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.Command, err = interpolateInvocationField(next.Command, args, "worker.command", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.Timeout, err = interpolateInvocationField(next.Timeout, args, "worker.timeout", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.StopToken, err = interpolateInvocationField(next.StopToken, args, "worker.stopToken", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.OpenCodeAgent, err = interpolateInvocationField(next.OpenCodeAgent, args, "worker.openCodeAgent", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	if next.Body, err = interpolateInvocationField(next.Body, args, "worker body", false); err != nil {
		return interfaces.WorkerConfig{}, err
	}
	for i := range next.Args {
		if next.Args[i], err = interpolateInvocationField(next.Args[i], args, "worker.args entry", false); err != nil {
			return interfaces.WorkerConfig{}, err
		}
	}
	return next, nil
}

// InterpolateWorkstationConfig resolves supported `${parameter}` placeholders on
// one effective workstation definition using runtime invocation arguments.
func InterpolateWorkstationConfig(workstation interfaces.FactoryWorkstationConfig, args *interfaces.InvocationArguments) (interfaces.FactoryWorkstationConfig, error) {
	next := config.CloneWorkstationConfig(workstation)
	var err error
	if next.WorkerTypeName, err = interpolateInvocationField(next.WorkerTypeName, args, "workstation.worker", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Runner, err = interpolateInvocationField(next.Runner, args, "workstation.runner", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.OpenCodeAgent, err = interpolateInvocationField(next.OpenCodeAgent, args, "workstation.openCodeAgent", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.PromptFile, err = interpolateInvocationField(next.PromptFile, args, "workstation.promptFile", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.OutputSchema, err = interpolateInvocationField(next.OutputSchema, args, "workstation.outputSchema", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Timeout, err = interpolateInvocationField(next.Timeout, args, "workstation.timeout", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Body, err = interpolateInvocationField(next.Body, args, "workstation prompt body", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.PromptTemplate, err = interpolateInvocationField(next.PromptTemplate, args, "workstation.promptTemplate", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.WorkingDirectory, err = interpolateInvocationField(next.WorkingDirectory, args, "workstation.workingDirectory", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Worktree, err = interpolateInvocationField(next.Worktree, args, "workstation.worktree", false); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	for key, value := range next.Env {
		resolved, err := interpolateInvocationField(value, args, fmt.Sprintf("workstation.env[%q]", key), false)
		if err != nil {
			return interfaces.FactoryWorkstationConfig{}, err
		}
		next.Env[key] = resolved
	}
	return next, nil
}

func validateInvocationOutputContract(output *interfaces.InvocationOutputContractConfig, args *interfaces.InvocationArguments) error {
	if output == nil {
		return nil
	}
	pathParameter := strings.TrimSpace(output.PathParameter)
	if pathParameter == "" {
		return nil
	}
	argument, ok := invocationArgumentByName(args, pathParameter)
	if !ok {
		return nil
	}
	if len(argument.Values) != 1 {
		return &ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation output pathParameter %q requires exactly one value", pathParameter),
			Parameter: pathParameter,
		}
	}
	if _, err := invocationArgumentScalar(argument, pathParameter, "invocationSignature.outputContract.pathParameter"); err != nil {
		return err
	}
	return nil
}

func interpolateInvocationField(
	authored string,
	args *interfaces.InvocationArguments,
	fieldDescriptor string,
	allowsRepeated bool,
) (string, error) {
	if !strings.Contains(authored, "${") {
		return authored, nil
	}
	matches := invocationInterpolationPattern.FindAllStringSubmatchIndex(authored, -1)
	if len(matches) == 0 {
		return authored, nil
	}
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(authored) {
		name := authored[matches[0][2]:matches[0][3]]
		argument, ok := invocationArgumentByName(args, name)
		if !ok {
			return "", nil
		}
		if allowsRepeated {
			return strings.Join(argument.Values, ","), nil
		}
		return invocationArgumentScalar(argument, name, fieldDescriptor)
	}
	var builder strings.Builder
	cursor := 0
	for _, match := range matches {
		builder.WriteString(authored[cursor:match[0]])
		name := authored[match[2]:match[3]]
		argument, ok := invocationArgumentByName(args, name)
		if !ok {
			return "", &ArgumentError{
				Code:      ArgumentErrorCodeInvalidInterpolation,
				Message:   fmt.Sprintf("%s references omitted invocation parameter %q", fieldDescriptor, name),
				Parameter: name,
			}
		}
		replacement, err := invocationArgumentScalar(argument, name, fieldDescriptor)
		if err != nil {
			return "", err
		}
		builder.WriteString(replacement)
		cursor = match[1]
	}
	builder.WriteString(authored[cursor:])
	return builder.String(), nil
}

func invocationArgumentByName(args *interfaces.InvocationArguments, name string) (interfaces.InvocationArgument, bool) {
	if args == nil || len(args.Arguments) == 0 {
		return interfaces.InvocationArgument{}, false
	}
	argument, ok := args.Arguments[strings.TrimSpace(name)]
	return argument, ok
}

func invocationArgumentScalar(argument interfaces.InvocationArgument, parameterName, fieldDescriptor string) (string, error) {
	if len(argument.Values) != 1 {
		return "", &ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("%s requires single-value invocation parameter %q", fieldDescriptor, parameterName),
			Parameter: parameterName,
		}
	}
	value := argument.Values[0]
	if argument.ValueMode != string(factoryapi.FactoryInvocationParameterValueModeFileContents) {
		return value, nil
	}
	data, err := os.ReadFile(value)
	if err != nil {
		return "", &ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation parameter %q could not read FILE_CONTENTS path %q: %v", parameterName, value, err),
			Parameter: parameterName,
		}
	}
	return string(data), nil
}
