package invocationinterpolation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

var invocationInterpolationPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)

// Service implements Factory invocation interpolation policy.
type Service struct{}

var _ factorydefinitions.InvocationInterpolationService = Service{}

// NewService returns the canonical Factory invocation interpolator.
func NewService() factorydefinitions.InvocationInterpolationService {
	return Service{}
}

func (Service) ValidateInvocationInterpolation(
	cfg *factorydefinitions.FactoryConfig,
	args *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) error {
	return ValidateInvocationInterpolation(cfg, args, readFile)
}

func (Service) InterpolateWorkerConfig(
	worker factorydefinitions.FactoryWorkerConfig,
	args *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) (factorydefinitions.FactoryWorkerConfig, error) {
	return InterpolateWorkerConfig(worker, args, readFile)
}

func (Service) InterpolateWorkstationConfig(
	workstation factorydefinitions.FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) (factorydefinitions.FactoryWorkstationConfig, error) {
	return InterpolateWorkstationConfig(workstation, args, readFile)
}

// ValidateInvocationInterpolation verifies that runtime-supported invocation
// interpolation can resolve the authored worker and workstation fields for the
// supplied normalized argument set without mutating the canonical runtime config.
func ValidateInvocationInterpolation(cfg *factorydefinitions.FactoryConfig, args *work.InvocationArguments, readFile factorydefinitions.FileReader) error {
	if cfg == nil || args == nil {
		return nil
	}
	for _, worker := range cfg.Workers {
		if _, err := InterpolateWorkerConfig(worker, args, readFile); err != nil {
			return err
		}
	}
	for _, workstation := range cfg.Workstations {
		if _, err := InterpolateWorkstationConfig(workstation, args, readFile); err != nil {
			return err
		}
	}
	if signature := cfg.InvocationSignature; signature != nil && signature.OutputContract != nil {
		if err := validateInvocationOutputContract(signature.OutputContract, args, readFile); err != nil {
			return err
		}
	}
	return nil
}

// InterpolateWorkerConfig resolves supported `${parameter}` placeholders on one
// effective worker definition using runtime invocation arguments.
func InterpolateWorkerConfig(worker workerconfig.Config, args *work.InvocationArguments, readFile factorydefinitions.FileReader) (workerconfig.Config, error) {
	next := cloneWorkerForInterpolation(worker)
	var err error
	if next.Provider, err = interpolateInvocationField(next.Provider, args, "worker.provider", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.Model, err = interpolateInvocationField(next.Model, args, "worker.model", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.ModelProvider, err = interpolateInvocationField(next.ModelProvider, args, "worker.modelProvider", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.ExecutorProvider, err = interpolateInvocationField(next.ExecutorProvider, args, "worker.executorProvider", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.Command, err = interpolateInvocationField(next.Command, args, "worker.command", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.Timeout, err = interpolateInvocationField(next.Timeout, args, "worker.timeout", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.StopToken, err = interpolateInvocationField(next.StopToken, args, "worker.stopToken", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.OpenCodeAgent, err = interpolateInvocationField(next.OpenCodeAgent, args, "worker.openCodeAgent", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	if next.Body, err = interpolateInvocationField(next.Body, args, "worker body", false, readFile); err != nil {
		return workerconfig.Config{}, err
	}
	for i := range next.Args {
		if next.Args[i], err = interpolateInvocationField(next.Args[i], args, "worker.args entry", false, readFile); err != nil {
			return workerconfig.Config{}, err
		}
	}
	return next, nil
}

// InterpolateWorkstationConfig resolves supported `${parameter}` placeholders on
// one effective workstation definition using runtime invocation arguments.
func InterpolateWorkstationConfig(workstation factorydefinitions.FactoryWorkstationConfig, args *work.InvocationArguments, readFile factorydefinitions.FileReader) (factorydefinitions.FactoryWorkstationConfig, error) {
	next := cloneWorkstationForInterpolation(workstation)
	var err error
	if next.WorkerTypeName, err = interpolateInvocationField(next.WorkerTypeName, args, "workstation.worker", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.Runner, err = interpolateInvocationField(next.Runner, args, "workstation.runner", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.OpenCodeAgent, err = interpolateInvocationField(next.OpenCodeAgent, args, "workstation.openCodeAgent", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.PromptFile, err = interpolateInvocationField(next.PromptFile, args, "workstation.promptFile", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.OutputSchema, err = interpolateInvocationField(next.OutputSchema, args, "workstation.outputSchema", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.Timeout, err = interpolateInvocationField(next.Timeout, args, "workstation.timeout", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.Body, err = interpolateInvocationField(next.Body, args, "workstation prompt body", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.PromptTemplate, err = interpolateInvocationField(next.PromptTemplate, args, "workstation.promptTemplate", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.WorkingDirectory, err = interpolateInvocationField(next.WorkingDirectory, args, "workstation.workingDirectory", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	if next.Worktree, err = interpolateInvocationField(next.Worktree, args, "workstation.worktree", false, readFile); err != nil {
		return factorydefinitions.FactoryWorkstationConfig{}, err
	}
	for key, value := range next.Env {
		resolved, err := interpolateInvocationField(value, args, fmt.Sprintf("workstation.env[%q]", key), false, readFile)
		if err != nil {
			return factorydefinitions.FactoryWorkstationConfig{}, err
		}
		next.Env[key] = resolved
	}
	if len(next.OperationBindings) > 0 {
		bindings, err := interpolateModelOperationBindings(next.OperationBindings, args, readFile)
		if err != nil {
			return factorydefinitions.FactoryWorkstationConfig{}, err
		}
		next.OperationBindings = bindings
	}
	return next, nil
}

func interpolateModelOperationBindings(
	bindings []factorydefinitions.ModelOperationBinding,
	args *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) ([]factorydefinitions.ModelOperationBinding, error) {
	if len(bindings) == 0 {
		return bindings, nil
	}
	next := make([]factorydefinitions.ModelOperationBinding, len(bindings))
	for index, binding := range bindings {
		next[index] = binding
		pathPrefix := fmt.Sprintf("workstations.operationBindings[%d](%s)", index, binding.Slot)
		var err error
		next[index].Config, err = interpolateWorkContentParts(binding.Config, args, pathPrefix+".config", readFile)
		if err != nil {
			return nil, err
		}
		next[index].DefaultContent, err = interpolateWorkContentParts(binding.DefaultContent, args, pathPrefix+".defaultContent", readFile)
		if err != nil {
			return nil, err
		}
	}
	return next, nil
}

func interpolateWorkContentParts(
	parts []work.WorkContentPart,
	args *work.InvocationArguments,
	fieldDescriptor string,
	readFile factorydefinitions.FileReader,
) ([]work.WorkContentPart, error) {
	if len(parts) == 0 {
		return parts, nil
	}
	next := make([]work.WorkContentPart, len(parts))
	for index, part := range parts {
		interpolated, err := interpolateWorkContentPart(part, args, fmt.Sprintf("%s[%d]", fieldDescriptor, index), readFile)
		if err != nil {
			return nil, err
		}
		next[index] = interpolated
	}
	return next, nil
}

func interpolateWorkContentPart(
	part work.WorkContentPart,
	args *work.InvocationArguments,
	fieldDescriptor string,
	readFile factorydefinitions.FileReader,
) (work.WorkContentPart, error) {
	next := part
	var err error
	if next.Text, err = interpolateInvocationField(next.Text, args, fieldDescriptor+".text", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if next.URL, err = interpolateInvocationField(next.URL, args, fieldDescriptor+".url", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if next.File, err = interpolateInvocationField(next.File, args, fieldDescriptor+".file", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if next.Slot, err = interpolateInvocationField(next.Slot, args, fieldDescriptor+".slot", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if next.Label, err = interpolateInvocationField(next.Label, args, fieldDescriptor+".label", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if next.Role, err = interpolateInvocationField(next.Role, args, fieldDescriptor+".role", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if next.ContentType, err = interpolateInvocationField(next.ContentType, args, fieldDescriptor+".contentType", false, readFile); err != nil {
		return work.WorkContentPart{}, err
	}
	if len(next.JSON) > 0 {
		interpolated, err := interpolateInvocationField(string(next.JSON), args, fieldDescriptor+".json", false, readFile)
		if err != nil {
			return work.WorkContentPart{}, err
		}
		next.JSON = json.RawMessage(interpolated)
	}
	return next, nil
}

func validateInvocationOutputContract(output *work.InvocationOutputContractConfig, args *work.InvocationArguments, readFile factorydefinitions.FileReader) error {
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
		return &work.ArgumentError{
			Code:      factorydefinitions.ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation output pathParameter %q requires exactly one value", pathParameter),
			Parameter: pathParameter,
		}
	}
	if _, err := invocationArgumentScalar(argument, pathParameter, "invocationSignature.outputContract.pathParameter", readFile); err != nil {
		return err
	}
	return nil
}

func interpolateInvocationField(
	authored string,
	args *work.InvocationArguments,
	fieldDescriptor string,
	allowsRepeated bool,
	readFile factorydefinitions.FileReader,
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
		return invocationArgumentScalar(argument, name, fieldDescriptor, readFile)
	}
	var builder strings.Builder
	cursor := 0
	for _, match := range matches {
		builder.WriteString(authored[cursor:match[0]])
		name := authored[match[2]:match[3]]
		argument, ok := invocationArgumentByName(args, name)
		if !ok {
			return "", &work.ArgumentError{
				Code:      factorydefinitions.ArgumentErrorCodeInvalidInterpolation,
				Message:   fmt.Sprintf("%s references omitted invocation parameter %q", fieldDescriptor, name),
				Parameter: name,
			}
		}
		replacement, err := invocationArgumentScalar(argument, name, fieldDescriptor, readFile)
		if err != nil {
			return "", err
		}
		builder.WriteString(replacement)
		cursor = match[1]
	}
	builder.WriteString(authored[cursor:])
	return builder.String(), nil
}

func invocationArgumentByName(args *work.InvocationArguments, name string) (work.InvocationArgument, bool) {
	if args == nil || len(args.Arguments) == 0 {
		return work.InvocationArgument{}, false
	}
	argument, ok := args.Arguments[strings.TrimSpace(name)]
	return argument, ok
}

func invocationArgumentScalar(argument work.InvocationArgument, parameterName, fieldDescriptor string, readFile factorydefinitions.FileReader) (string, error) {
	if len(argument.Values) != 1 {
		return "", &work.ArgumentError{
			Code:      factorydefinitions.ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("%s requires single-value invocation parameter %q", fieldDescriptor, parameterName),
			Parameter: parameterName,
		}
	}
	value := argument.Values[0]
	if argument.ValueMode != work.InvocationParameterValueModeFileContents {
		return value, nil
	}
	if readFile == nil {
		return "", &work.ArgumentError{
			Code:      factorydefinitions.ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation parameter %q requires a FILE_CONTENTS reader", parameterName),
			Parameter: parameterName,
		}
	}
	data, err := readFile(value)
	if err != nil {
		return "", &work.ArgumentError{
			Code:      factorydefinitions.ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation parameter %q could not read FILE_CONTENTS path %q: %v", parameterName, value, err),
			Parameter: parameterName,
		}
	}
	return string(data), nil
}

func cloneWorkerForInterpolation(worker workerconfig.Config) workerconfig.Config {
	worker.Args = append([]string(nil), worker.Args...)
	return worker
}

func cloneWorkstationForInterpolation(workstation factorydefinitions.FactoryWorkstationConfig) factorydefinitions.FactoryWorkstationConfig {
	if workstation.Env != nil {
		env := make(map[string]string, len(workstation.Env))
		for key, value := range workstation.Env {
			env[key] = value
		}
		workstation.Env = env
	}
	return workstation
}
