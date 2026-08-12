package workstationexecution

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func interpolateExecutionWorker(
	worker FactoryWorkerConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) (FactoryWorkerConfig, error) {
	next := worker
	next.Args = append([]string(nil), worker.Args...)
	var err error
	fields := []struct {
		value *string
		path  string
	}{
		{&next.Provider, "worker.provider"},
		{&next.Model, "worker.model"},
		{&next.ModelProvider, "worker.modelProvider"},
		{&next.ReasoningEffort, "worker.reasoningEffort"},
		{&next.ModelLocality, "worker.modelLocality"},
		{&next.ExecutorProvider, "worker.executorProvider"},
		{&next.Command, "worker.command"},
		{&next.Timeout, "worker.timeout"},
		{&next.StopToken, "worker.stopToken"},
		{&next.Body, "worker.body"},
	}
	for _, field := range fields {
		*field.value, err = interpolateExecutionField(*field.value, args, field.path, readFile)
		if err != nil {
			return FactoryWorkerConfig{}, err
		}
	}
	for index := range next.Args {
		next.Args[index], err = interpolateExecutionField(
			next.Args[index], args, fmt.Sprintf("worker.args[%d]", index), readFile)
		if err != nil {
			return FactoryWorkerConfig{}, err
		}
	}
	return next, nil
}

func interpolateExecutionWorkstation(
	workstation FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) (FactoryWorkstationConfig, error) {
	next := CloneWorkstationConfig(workstation)
	steps := []func(*FactoryWorkstationConfig, *work.InvocationArguments, FileReader) error{
		interpolateExecutionWorkstationFields,
		interpolateExecutionWorkstationEnvironment,
		interpolateExecutionWorkstationWords,
		interpolateExecutionWorkstationRouting,
		interpolateExecutionWorkstationArtifacts,
		interpolateExecutionWorkstationGuards,
	}
	for _, step := range steps {
		if err := step(&next, args, readFile); err != nil {
			return FactoryWorkstationConfig{}, err
		}
	}
	return next, nil
}

func interpolateExecutionWorkstationFields(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) error {
	fields := []struct {
		value *string
		path  string
	}{
		{&workstation.WorkerTypeName, "workstation.worker"},
		{&workstation.Runner, "workstation.runner"},
		{&workstation.PromptFile, "workstation.promptFile"},
		{&workstation.OutputSchema, "workstation.outputSchema"},
		{&workstation.OutputContract, "workstation.outputContract"},
		{&workstation.Timeout, "workstation.timeout"},
		{&workstation.Body, "workstation.body"},
		{&workstation.PromptTemplate, "workstation.promptTemplate"},
		{&workstation.WorkingDirectory, "workstation.workingDirectory"},
		{&workstation.Worktree, "workstation.worktree"},
		{&workstation.OutcomeFormat, "workstation.outcomeFormat"},
		{&workstation.Limits.MaxExecutionTime, "workstation.limits.maxExecutionTime"},
		{&workstation.Limits.MaxGeneratedWorkItemsArgument, "workstation.limits.maxGeneratedWorkItemsArgument"},
	}
	for _, field := range fields {
		value, err := interpolateExecutionField(*field.value, args, field.path, readFile)
		if err != nil {
			return err
		}
		*field.value = value
	}
	return nil
}

func interpolateExecutionWorkstationEnvironment(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) error {
	for key, value := range workstation.Env {
		resolved, err := interpolateExecutionField(
			value, args, fmt.Sprintf("workstation.env[%q]", key), readFile,
		)
		if err != nil {
			return err
		}
		workstation.Env[key] = resolved
	}
	if workstation.Cron == nil {
		return nil
	}
	cron := *workstation.Cron
	fields := []struct {
		value *string
		path  string
	}{
		{&cron.Schedule, "workstation.cron.schedule"},
		{&cron.Every, "workstation.cron.every"},
		{&cron.Jitter, "workstation.cron.jitter"},
		{&cron.ExpiryWindow, "workstation.cron.expiryWindow"},
	}
	for _, field := range fields {
		resolved, err := interpolateExecutionField(*field.value, args, field.path, readFile)
		if err != nil {
			return err
		}
		*field.value = resolved
	}
	workstation.Cron = &cron
	return nil
}

func interpolateExecutionWorkstationWords(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) error {
	for index := range workstation.StopWords {
		value, err := interpolateExecutionField(
			workstation.StopWords[index], args,
			fmt.Sprintf("workstation.stopWords[%d]", index), readFile,
		)
		if err != nil {
			return err
		}
		workstation.StopWords[index] = value
	}
	for index := range workstation.RuntimeStopWords {
		value, err := interpolateExecutionField(
			workstation.RuntimeStopWords[index], args,
			fmt.Sprintf("workstation.runtimeStopWords[%d]", index), readFile,
		)
		if err != nil {
			return err
		}
		workstation.RuntimeStopWords[index] = value
	}
	return nil
}

func interpolateExecutionWorkstationRouting(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) error {
	var err error
	workstation.OperationBindings, err = interpolateExecutionOperationBindings(
		workstation.OperationBindings, args, readFile,
	)
	if err != nil {
		return err
	}
	routes := []struct {
		values *[]IOConfig
		path   string
	}{
		{&workstation.Inputs, "workstation.inputs"},
		{&workstation.Outputs, "workstation.outputs"},
		{&workstation.OnContinue, "workstation.onContinue"},
		{&workstation.OnRejection, "workstation.onRejection"},
		{&workstation.OnFailure, "workstation.onFailure"},
	}
	for _, route := range routes {
		*route.values, err = interpolateExecutionIO(*route.values, args, readFile, route.path)
		if err != nil {
			return err
		}
	}
	for index := range workstation.ClassificationRoutes {
		route := &workstation.ClassificationRoutes[index]
		route.Label, err = interpolateExecutionField(
			route.Label, args,
			fmt.Sprintf("workstation.classificationRoutes[%d].label", index), readFile,
		)
		if err != nil {
			return err
		}
		route.Outputs, err = interpolateExecutionIO(
			route.Outputs, args, readFile,
			fmt.Sprintf("workstation.classificationRoutes[%d].outputs", index),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func interpolateExecutionWorkstationArtifacts(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) error {
	for index := range workstation.ExpectedArtifacts {
		artifact := &workstation.ExpectedArtifacts[index]
		var err error
		artifact.Name, err = interpolateExecutionField(
			artifact.Name, args,
			fmt.Sprintf("workstation.expectedArtifacts[%d].name", index), readFile,
		)
		if err != nil {
			return err
		}
		artifact.Pattern, err = interpolateExecutionField(
			artifact.Pattern, args,
			fmt.Sprintf("workstation.expectedArtifacts[%d].pattern", index), readFile,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func interpolateExecutionWorkstationGuards(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
) error {
	for index := range workstation.Guards {
		guard := &workstation.Guards[index]
		var err error
		guard.Workstation, err = interpolateExecutionField(
			guard.Workstation, args,
			fmt.Sprintf("workstation.guards[%d].workstation", index), readFile,
		)
		if err != nil {
			return err
		}
		guard.MaxVisitsArgument, err = interpolateExecutionField(
			guard.MaxVisitsArgument, args,
			fmt.Sprintf("workstation.guards[%d].maxVisitsArgument", index), readFile,
		)
		if err != nil {
			return err
		}
		if guard.MatchConfig == nil {
			continue
		}
		guard.MatchConfig.InputKey, err = interpolateExecutionField(
			guard.MatchConfig.InputKey, args,
			fmt.Sprintf("workstation.guards[%d].matchConfig.inputKey", index), readFile,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func interpolateExecutionOperationBindings(
	bindings []ModelOperationBinding,
	args *work.InvocationArguments,
	readFile FileReader,
) ([]ModelOperationBinding, error) {
	if len(bindings) == 0 {
		return nil, nil
	}
	next := CloneModelOperationBindings(bindings)
	for index := range next {
		binding := &next[index]
		var err error
		binding.Slot, err = interpolateExecutionField(
			binding.Slot, args, fmt.Sprintf("workstation.operationBindings[%d].slot", index), readFile)
		if err != nil {
			return nil, err
		}
		if binding.Selector != nil {
			selector := *binding.Selector
			selector.Slot, err = interpolateExecutionField(
				selector.Slot, args, fmt.Sprintf("workstation.operationBindings[%d].selector.slot", index), readFile)
			if err != nil {
				return nil, err
			}
			selector.Label, err = interpolateExecutionField(
				selector.Label, args, fmt.Sprintf("workstation.operationBindings[%d].selector.label", index), readFile)
			if err != nil {
				return nil, err
			}
			selector.Type, err = interpolateExecutionField(
				selector.Type, args, fmt.Sprintf("workstation.operationBindings[%d].selector.type", index), readFile)
			if err != nil {
				return nil, err
			}
			selector.Role, err = interpolateExecutionField(
				selector.Role, args, fmt.Sprintf("workstation.operationBindings[%d].selector.role", index), readFile)
			if err != nil {
				return nil, err
			}
			binding.Selector = &selector
		}
		binding.Config, err = interpolateExecutionContentParts(
			binding.Config, args, fmt.Sprintf("workstation.operationBindings[%d].config", index), readFile)
		if err != nil {
			return nil, err
		}
		binding.DefaultContent, err = interpolateExecutionContentParts(
			binding.DefaultContent, args, fmt.Sprintf("workstation.operationBindings[%d].defaultContent", index), readFile)
		if err != nil {
			return nil, err
		}
	}
	return next, nil
}

func interpolateExecutionContentParts(
	parts []work.WorkContentPart,
	args *work.InvocationArguments,
	fieldPath string,
	readFile FileReader,
) ([]work.WorkContentPart, error) {
	next := work.CloneWorkContentParts(parts)
	for index := range next {
		part := &next[index]
		fields := []struct {
			value *string
			path  string
		}{
			{&part.Text, fmt.Sprintf("%s[%d].text", fieldPath, index)},
			{&part.URL, fmt.Sprintf("%s[%d].url", fieldPath, index)},
			{&part.File, fmt.Sprintf("%s[%d].file", fieldPath, index)},
			{&part.Slot, fmt.Sprintf("%s[%d].slot", fieldPath, index)},
			{&part.Label, fmt.Sprintf("%s[%d].label", fieldPath, index)},
			{&part.Role, fmt.Sprintf("%s[%d].role", fieldPath, index)},
			{&part.ContentType, fmt.Sprintf("%s[%d].contentType", fieldPath, index)},
		}
		for _, field := range fields {
			var err error
			*field.value, err = interpolateExecutionField(*field.value, args, field.path, readFile)
			if err != nil {
				return nil, err
			}
		}
		if len(part.JSON) > 0 {
			value, err := interpolateExecutionField(
				string(part.JSON), args, fmt.Sprintf("%s[%d].json", fieldPath, index), readFile)
			if err != nil {
				return nil, err
			}
			part.JSON = json.RawMessage(value)
		}
	}
	return next, nil
}

func interpolateExecutionIO(
	values []IOConfig,
	args *work.InvocationArguments,
	readFile FileReader,
	path string,
) ([]IOConfig, error) {
	if len(values) == 0 {
		return nil, nil
	}
	next := CloneIOConfigs(values)
	for index := range next {
		value := &next[index]
		var err error
		value.WorkTypeName, err = interpolateExecutionField(
			value.WorkTypeName, args, fmt.Sprintf("%s[%d].workType", path, index), readFile)
		if err != nil {
			return nil, err
		}
		value.StateName, err = interpolateExecutionField(
			value.StateName, args, fmt.Sprintf("%s[%d].state", path, index), readFile)
		if err != nil {
			return nil, err
		}
		if value.Guard != nil {
			guard := *value.Guard
			guard.MatchInput, err = interpolateExecutionField(
				guard.MatchInput, args, fmt.Sprintf("%s[%d].guard.matchInput", path, index), readFile)
			if err != nil {
				return nil, err
			}
			guard.ParentInput, err = interpolateExecutionField(
				guard.ParentInput, args, fmt.Sprintf("%s[%d].guard.parentInput", path, index), readFile)
			if err != nil {
				return nil, err
			}
			guard.SpawnedBy, err = interpolateExecutionField(
				guard.SpawnedBy, args, fmt.Sprintf("%s[%d].guard.spawnedBy", path, index), readFile)
			if err != nil {
				return nil, err
			}
			value.Guard = &guard
		}
	}
	return next, nil
}

func interpolateExecutionField(
	authored string,
	args *work.InvocationArguments,
	fieldPath string,
	readFile FileReader,
) (string, error) {
	if !strings.Contains(authored, "${") {
		return authored, nil
	}
	matches := executionInterpolationPattern.FindAllStringSubmatchIndex(authored, -1)
	if len(matches) == 0 {
		return authored, nil
	}
	if len(matches) == 1 && matches[0][0] == 0 && matches[0][1] == len(authored) {
		name := authored[matches[0][2]:matches[0][3]]
		argument, ok := executionInvocationArgument(args, name)
		if !ok {
			return "", nil
		}
		return executionInvocationScalar(argument, name, fieldPath, readFile)
	}
	var builder strings.Builder
	cursor := 0
	for _, match := range matches {
		builder.WriteString(authored[cursor:match[0]])
		name := authored[match[2]:match[3]]
		argument, ok := executionInvocationArgument(args, name)
		if !ok {
			return "", &work.ArgumentError{
				Code:      ArgumentErrorCodeInvalidInterpolation,
				Message:   fmt.Sprintf("%s references omitted invocation parameter %q", fieldPath, name),
				Parameter: name,
			}
		}
		replacement, err := executionInvocationScalar(argument, name, fieldPath, readFile)
		if err != nil {
			return "", err
		}
		builder.WriteString(replacement)
		cursor = match[1]
	}
	builder.WriteString(authored[cursor:])
	return builder.String(), nil
}

func executionInvocationArgument(args *work.InvocationArguments, name string) (work.InvocationArgument, bool) {
	if args == nil || len(args.Arguments) == 0 {
		return work.InvocationArgument{}, false
	}
	argument, ok := args.Arguments[strings.TrimSpace(name)]
	return argument, ok
}

func executionInvocationScalar(
	argument work.InvocationArgument,
	parameterName string,
	fieldPath string,
	readFile FileReader,
) (string, error) {
	if len(argument.Values) != 1 {
		return "", &work.ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("%s requires single-value invocation parameter %q", fieldPath, parameterName),
			Parameter: parameterName,
		}
	}
	value := argument.Values[0]
	if argument.ValueMode != work.InvocationParameterValueModeFileContents {
		return value, nil
	}
	if readFile == nil {
		return "", &work.ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation parameter %q requires a FILE_CONTENTS reader", parameterName),
			Parameter: parameterName,
		}
	}
	data, err := readFile(value)
	if err != nil {
		return "", &work.ArgumentError{
			Code:      ArgumentErrorCodeInvalidInterpolation,
			Message:   fmt.Sprintf("invocation parameter %q could not read FILE_CONTENTS input", parameterName),
			Parameter: parameterName,
		}
	}
	return string(data), nil
}

var executionInterpolationPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_.-]+)\}`)
