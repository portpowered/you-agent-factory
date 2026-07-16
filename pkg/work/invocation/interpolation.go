package invocation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

var invocationInterpolationPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)

const ArgumentErrorCodeInvalidInterpolation ArgumentErrorCode = "INVOCATION_ARGUMENT_INVALID_INTERPOLATION"

// FileReader resolves FILE_CONTENTS arguments at an explicit IO boundary.
type FileReader func(string) ([]byte, error)

// RuntimeInvocationArguments converts normalized invocation arguments into the
// runtime-owned transport-independent metadata carried on submitted work.
func RuntimeInvocationArguments(
	signature *interfaces.InvocationSignatureConfig,
	normalized *NormalizedArguments,
) *work.InvocationArguments {
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
	args := &work.InvocationArguments{
		Arguments: make(map[string]work.InvocationArgument, len(normalized.Arguments)),
	}
	for name, argument := range normalized.Arguments {
		next := work.InvocationArgument{
			Values:    append([]string(nil), argument.Values...),
			ValueMode: valueModes[name],
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = make([]work.InvocationArgumentSource, len(argument.Sources))
			for i, source := range argument.Sources {
				next.Sources[i] = work.InvocationArgumentSource{
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
func ValidateInvocationInterpolation(cfg *interfaces.FactoryConfig, args *work.InvocationArguments, readFile FileReader) error {
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
func InterpolateWorkerConfig(worker workerconfig.Config, args *work.InvocationArguments, readFile FileReader) (workerconfig.Config, error) {
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
func InterpolateWorkstationConfig(workstation interfaces.FactoryWorkstationConfig, args *work.InvocationArguments, readFile FileReader) (interfaces.FactoryWorkstationConfig, error) {
	next := cloneWorkstationForInterpolation(workstation)
	var err error
	if next.WorkerTypeName, err = interpolateInvocationField(next.WorkerTypeName, args, "workstation.worker", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Runner, err = interpolateInvocationField(next.Runner, args, "workstation.runner", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.OpenCodeAgent, err = interpolateInvocationField(next.OpenCodeAgent, args, "workstation.openCodeAgent", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.PromptFile, err = interpolateInvocationField(next.PromptFile, args, "workstation.promptFile", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.OutputSchema, err = interpolateInvocationField(next.OutputSchema, args, "workstation.outputSchema", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Timeout, err = interpolateInvocationField(next.Timeout, args, "workstation.timeout", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Body, err = interpolateInvocationField(next.Body, args, "workstation prompt body", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.PromptTemplate, err = interpolateInvocationField(next.PromptTemplate, args, "workstation.promptTemplate", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.WorkingDirectory, err = interpolateInvocationField(next.WorkingDirectory, args, "workstation.workingDirectory", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	if next.Worktree, err = interpolateInvocationField(next.Worktree, args, "workstation.worktree", false, readFile); err != nil {
		return interfaces.FactoryWorkstationConfig{}, err
	}
	for key, value := range next.Env {
		resolved, err := interpolateInvocationField(value, args, fmt.Sprintf("workstation.env[%q]", key), false, readFile)
		if err != nil {
			return interfaces.FactoryWorkstationConfig{}, err
		}
		next.Env[key] = resolved
	}
	return next, nil
}

func validateInvocationOutputContract(output *interfaces.InvocationOutputContractConfig, args *work.InvocationArguments, readFile FileReader) error {
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
	readFile FileReader,
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
			return "", &ArgumentError{
				Code:      ArgumentErrorCodeInvalidInterpolation,
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

func invocationArgumentScalar(argument work.InvocationArgument, parameterName, fieldDescriptor string, readFile FileReader) (string, error) {
	if len(argument.Values) != 1 {
		return "", &ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("%s requires single-value invocation parameter %q", fieldDescriptor, parameterName),
			Parameter: parameterName,
		}
	}
	value := argument.Values[0]
	if argument.ValueMode != valueModeFileContents {
		return value, nil
	}
	if readFile == nil {
		return "", &ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation parameter %q requires a FILE_CONTENTS reader", parameterName),
			Parameter: parameterName,
		}
	}
	data, err := readFile(value)
	if err != nil {
		return "", &ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
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

func cloneWorkstationForInterpolation(workstation interfaces.FactoryWorkstationConfig) interfaces.FactoryWorkstationConfig {
	if workstation.Env != nil {
		env := make(map[string]string, len(workstation.Env))
		for key, value := range workstation.Env {
			env[key] = value
		}
		workstation.Env = env
	}
	return workstation
}

// InvocationDiagnostic returns a replay-safe invocation summary that preserves
// canonical parameter names, source kinds, and redaction state without raw
// values.
func InvocationDiagnostic(
	signature *interfaces.InvocationSignatureConfig,
	args *work.InvocationArguments,
) *workerexecution.InvocationDiagnostic {
	if signature == nil && (args == nil || len(args.Arguments) == 0) {
		return nil
	}
	diagnostic := &workerexecution.InvocationDiagnostic{
		SignatureHash: InvocationSignatureHash(signature),
	}
	if args == nil || len(args.Arguments) == 0 {
		if diagnostic.SignatureHash == "" {
			return nil
		}
		return diagnostic
	}
	names := make([]string, 0, len(args.Arguments))
	for name := range args.Arguments {
		names = append(names, name)
	}
	sort.Strings(names)
	diagnostic.Parameters = make([]workerexecution.InvocationParameterDiagnostic, 0, len(names))
	for _, name := range names {
		argument := args.Arguments[name]
		entry := workerexecution.InvocationParameterDiagnostic{
			Name:       name,
			ValueCount: len(argument.Values),
			Redacted:   argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			entry.SourceKinds = make([]string, 0, len(argument.Sources))
			for _, source := range argument.Sources {
				kind := strings.TrimSpace(source.Kind)
				if kind == "" {
					continue
				}
				entry.SourceKinds = append(entry.SourceKinds, kind)
				if source.Redact {
					entry.Redacted = true
				}
			}
		}
		if len(entry.SourceKinds) == 0 {
			entry.SourceKinds = nil
		}
		diagnostic.Parameters = append(diagnostic.Parameters, entry)
	}
	return diagnostic
}

// InvocationSignatureHash returns a stable digest for one authored invocation
// signature when present.
func InvocationSignatureHash(signature *interfaces.InvocationSignatureConfig) string {
	if signature == nil {
		return ""
	}
	encoded, err := json.Marshal(signature)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
