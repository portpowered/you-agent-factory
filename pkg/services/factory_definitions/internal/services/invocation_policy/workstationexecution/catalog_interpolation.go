package workstationexecution

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type invocationInterpolationContext struct {
	basePointer string
	provenance  *invocationInterpolationProvenance
}

func firstInvocationInterpolationContext(
	contexts []invocationInterpolationContext,
) invocationInterpolationContext {
	if len(contexts) == 0 {
		return invocationInterpolationContext{}
	}
	return contexts[0]
}

func (context invocationInterpolationContext) pointer(tokens ...string) string {
	if context.provenance == nil || strings.TrimSpace(context.basePointer) == "" {
		return ""
	}
	pointer := context.basePointer
	for _, token := range tokens {
		pointer += "/" + escapeInvocationJSONPointerToken(token)
	}
	return pointer
}

func (context invocationInterpolationContext) field(
	authored string,
	args *work.InvocationArguments,
	fieldPath string,
	readFile FileReader,
	tokens ...string,
) (string, error) {
	pointer := ""
	if len(tokens) > 0 {
		pointer = context.pointer(tokens...)
	}
	return interpolateExecutionFieldWithProvenance(
		authored,
		args,
		fieldPath,
		readFile,
		pointer,
		context.provenance,
	)
}

type invocationInterpolationProvenance struct {
	spans []factorydefinitions.InvocationSensitiveJSONSpan
}

func (provenance *invocationInterpolationProvenance) record(
	pointer string,
	start int,
	end int,
) {
	if provenance == nil || pointer == "" || start == end {
		return
	}
	provenance.spans = append(provenance.spans, factorydefinitions.InvocationSensitiveJSONSpan{
		JSONPointer: pointer,
		Start:       start,
		End:         end,
	})
}

func (provenance *invocationInterpolationProvenance) values() []factorydefinitions.InvocationSensitiveJSONSpan {
	if provenance == nil || len(provenance.spans) == 0 {
		return nil
	}
	spans := append([]factorydefinitions.InvocationSensitiveJSONSpan(nil), provenance.spans...)
	sort.Slice(spans, func(left, right int) bool {
		if spans[left].JSONPointer != spans[right].JSONPointer {
			return spans[left].JSONPointer < spans[right].JSONPointer
		}
		if spans[left].Start != spans[right].Start {
			return spans[left].Start < spans[right].Start
		}
		return spans[left].End < spans[right].End
	})
	unique := spans[:0]
	for _, span := range spans {
		if len(unique) > 0 && unique[len(unique)-1] == span {
			continue
		}
		unique = append(unique, span)
	}
	return unique
}

func escapeInvocationJSONPointerToken(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	return strings.ReplaceAll(value, "/", "~1")
}

func interpolateExecutionWorker(
	worker FactoryWorkerConfig,
	args *work.InvocationArguments,
	readFile FileReader,
	contexts ...invocationInterpolationContext,
) (FactoryWorkerConfig, error) {
	context := firstInvocationInterpolationContext(contexts)
	next := worker
	next.Args = append([]string(nil), worker.Args...)
	var err error
	fields := []struct {
		value *string
		path  string
		field string
	}{
		{&next.Provider, "worker.provider", "provider"},
		{&next.Model, "worker.model", "model"},
		{&next.ModelProvider, "worker.modelProvider", "modelProvider"},
		{&next.ReasoningEffort, "worker.reasoningEffort", "reasoningEffort"},
		{&next.ModelLocality, "worker.modelLocality", "modelLocality"},
		{&next.ExecutorProvider, "worker.executorProvider", "executorProvider"},
		{&next.Command, "worker.command", "command"},
		{&next.Timeout, "worker.timeout", "timeout"},
		{&next.StopToken, "worker.stopToken", "stopToken"},
		{&next.Body, "worker.body", "body"},
	}
	for _, field := range fields {
		*field.value, err = context.field(*field.value, args, field.path, readFile, field.field)
		if err != nil {
			return FactoryWorkerConfig{}, err
		}
	}
	for index := range next.Args {
		next.Args[index], err = context.field(
			next.Args[index], args, fmt.Sprintf("worker.args[%d]", index), readFile,
			"args", strconv.Itoa(index),
		)
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
	contexts ...invocationInterpolationContext,
) (FactoryWorkstationConfig, error) {
	context := firstInvocationInterpolationContext(contexts)
	next := CloneWorkstationConfig(workstation)
	if err := interpolateExecutionWorkstationFields(&next, args, readFile, context); err != nil {
		return FactoryWorkstationConfig{}, err
	}
	if err := interpolateExecutionWorkstationEnvironment(&next, args, readFile, context); err != nil {
		return FactoryWorkstationConfig{}, err
	}
	if err := interpolateExecutionWorkstationWords(&next, args, readFile, context); err != nil {
		return FactoryWorkstationConfig{}, err
	}
	if err := interpolateExecutionWorkstationRouting(&next, args, readFile, context); err != nil {
		return FactoryWorkstationConfig{}, err
	}
	if err := interpolateExecutionWorkstationArtifacts(&next, args, readFile, context); err != nil {
		return FactoryWorkstationConfig{}, err
	}
	if err := interpolateExecutionWorkstationGuards(&next, args, readFile, context); err != nil {
		return FactoryWorkstationConfig{}, err
	}
	return next, nil
}

func interpolateExecutionWorkstationFields(
	workstation *FactoryWorkstationConfig,
	args *work.InvocationArguments,
	readFile FileReader,
	contexts ...invocationInterpolationContext,
) error {
	context := firstInvocationInterpolationContext(contexts)
	promptTemplateSelected := workstation.PromptTemplate != ""
	timeoutUsesLimit := strings.TrimSpace(workstation.Limits.MaxExecutionTime) == ""
	fields := []struct {
		value *string
		path  string
		field []string
	}{
		{&workstation.WorkerTypeName, "workstation.worker", []string{"worker"}},
		{&workstation.Runner, "workstation.runner", []string{"runner"}},
		{&workstation.PromptFile, "workstation.promptFile", []string{"promptFile"}},
		{&workstation.OutputSchema, "workstation.outputSchema", []string{"outputSchema"}},
		{&workstation.OutputContract, "workstation.outputContract", []string{"outputContract"}},
		{&workstation.Timeout, "workstation.timeout", nil},
		{&workstation.Body, "workstation.body", nil},
		{&workstation.PromptTemplate, "workstation.promptTemplate", nil},
		{&workstation.WorkingDirectory, "workstation.workingDirectory", []string{"workingDirectory"}},
		{&workstation.Worktree, "workstation.worktree", []string{"worktree"}},
		{&workstation.OutcomeFormat, "workstation.outcomeFormat", []string{"outcomeFormat"}},
		{&workstation.Limits.MaxExecutionTime, "workstation.limits.maxExecutionTime", []string{"limits", "maxExecutionTime"}},
		{&workstation.Limits.MaxGeneratedWorkItemsArgument, "workstation.limits.maxGeneratedWorkItemsArgument", []string{"limits", "maxGeneratedWorkItemsArgument"}},
	}
	for _, field := range fields {
		pointer := field.field
		switch field.path {
		case "workstation.timeout":
			if timeoutUsesLimit {
				pointer = []string{"limits", "maxExecutionTime"}
			}
		case "workstation.body":
			if !promptTemplateSelected {
				pointer = []string{"body"}
			}
		case "workstation.promptTemplate":
			if promptTemplateSelected {
				pointer = []string{"body"}
			}
		}
		value, err := context.field(*field.value, args, field.path, readFile, pointer...)
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
	contexts ...invocationInterpolationContext,
) error {
	context := firstInvocationInterpolationContext(contexts)
	keys := make([]string, 0, len(workstation.Env))
	for key := range workstation.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := workstation.Env[key]
		resolved, err := context.field(
			value, args, fmt.Sprintf("workstation.env[%q]", key), readFile,
			"env", key,
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
		field string
	}{
		{&cron.Schedule, "workstation.cron.schedule", "schedule"},
		{&cron.Every, "workstation.cron.every", "every"},
		{&cron.Jitter, "workstation.cron.jitter", "jitter"},
		{&cron.ExpiryWindow, "workstation.cron.expiryWindow", "expiryWindow"},
	}
	for _, field := range fields {
		resolved, err := context.field(*field.value, args, field.path, readFile, "cron", field.field)
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
	contexts ...invocationInterpolationContext,
) error {
	context := firstInvocationInterpolationContext(contexts)
	for index := range workstation.StopWords {
		value, err := context.field(
			workstation.StopWords[index], args,
			fmt.Sprintf("workstation.stopWords[%d]", index), readFile,
			"stopWords", strconv.Itoa(index),
		)
		if err != nil {
			return err
		}
		workstation.StopWords[index] = value
	}
	for index := range workstation.RuntimeStopWords {
		value, err := context.field(
			workstation.RuntimeStopWords[index], args,
			fmt.Sprintf("workstation.runtimeStopWords[%d]", index), readFile,
			"stopWords", strconv.Itoa(len(workstation.StopWords)+index),
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
	contexts ...invocationInterpolationContext,
) error {
	context := firstInvocationInterpolationContext(contexts)
	var err error
	workstation.OperationBindings, err = interpolateExecutionOperationBindings(
		workstation.OperationBindings, args, readFile, context,
	)
	if err != nil {
		return err
	}
	routes := []struct {
		values *[]IOConfig
		path   string
		field  string
	}{
		{&workstation.Inputs, "workstation.inputs", "inputs"},
		{&workstation.Outputs, "workstation.outputs", "outputs"},
		{&workstation.OnContinue, "workstation.onContinue", "onContinue"},
		{&workstation.OnRejection, "workstation.onRejection", "onRejection"},
		{&workstation.OnFailure, "workstation.onFailure", "onFailure"},
	}
	for _, route := range routes {
		*route.values, err = interpolateExecutionIO(
			*route.values, args, readFile, route.path,
			[]string{route.field}, context,
		)
		if err != nil {
			return err
		}
	}
	for index := range workstation.ClassificationRoutes {
		route := &workstation.ClassificationRoutes[index]
		route.Label, err = context.field(
			route.Label, args,
			fmt.Sprintf("workstation.classificationRoutes[%d].label", index), readFile,
			"classificationRoutes", strconv.Itoa(index), "label",
		)
		if err != nil {
			return err
		}
		route.Outputs, err = interpolateExecutionIO(
			route.Outputs, args, readFile,
			fmt.Sprintf("workstation.classificationRoutes[%d].outputs", index),
			[]string{"classificationRoutes", strconv.Itoa(index), "outputs"}, context,
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
	contexts ...invocationInterpolationContext,
) error {
	context := firstInvocationInterpolationContext(contexts)
	for index := range workstation.ExpectedArtifacts {
		artifact := &workstation.ExpectedArtifacts[index]
		var err error
		artifact.Name, err = context.field(
			artifact.Name, args,
			fmt.Sprintf("workstation.expectedArtifacts[%d].name", index), readFile,
			"expectedArtifacts", strconv.Itoa(index), "name",
		)
		if err != nil {
			return err
		}
		artifact.Pattern, err = context.field(
			artifact.Pattern, args,
			fmt.Sprintf("workstation.expectedArtifacts[%d].pattern", index), readFile,
			"expectedArtifacts", strconv.Itoa(index), "pattern",
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
	contexts ...invocationInterpolationContext,
) error {
	context := firstInvocationInterpolationContext(contexts)
	for index := range workstation.Guards {
		guard := &workstation.Guards[index]
		var err error
		guard.Workstation, err = context.field(
			guard.Workstation, args,
			fmt.Sprintf("workstation.guards[%d].workstation", index), readFile,
			"guards", strconv.Itoa(index), "workstation",
		)
		if err != nil {
			return err
		}
		guard.MaxVisitsArgument, err = context.field(
			guard.MaxVisitsArgument, args,
			fmt.Sprintf("workstation.guards[%d].maxVisitsArgument", index), readFile,
			"guards", strconv.Itoa(index), "maxVisitsArgument",
		)
		if err != nil {
			return err
		}
		if guard.MatchConfig == nil {
			continue
		}
		guard.MatchConfig.InputKey, err = context.field(
			guard.MatchConfig.InputKey, args,
			fmt.Sprintf("workstation.guards[%d].matchConfig.inputKey", index), readFile,
			"guards", strconv.Itoa(index), "matchConfig", "inputKey",
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
	contexts ...invocationInterpolationContext,
) ([]ModelOperationBinding, error) {
	context := firstInvocationInterpolationContext(contexts)
	if len(bindings) == 0 {
		return nil, nil
	}
	next := CloneModelOperationBindings(bindings)
	for index := range next {
		binding := &next[index]
		var err error
		binding.Slot, err = context.field(
			binding.Slot, args, fmt.Sprintf("workstation.operationBindings[%d].slot", index), readFile,
			"operationBindings", strconv.Itoa(index), "slot",
		)
		if err != nil {
			return nil, err
		}
		if binding.Selector != nil {
			selector := *binding.Selector
			selector.Slot, err = context.field(
				selector.Slot, args, fmt.Sprintf("workstation.operationBindings[%d].selector.slot", index), readFile,
				"operationBindings", strconv.Itoa(index), "selector", "slot",
			)
			if err != nil {
				return nil, err
			}
			selector.Label, err = context.field(
				selector.Label, args, fmt.Sprintf("workstation.operationBindings[%d].selector.label", index), readFile,
				"operationBindings", strconv.Itoa(index), "selector", "label",
			)
			if err != nil {
				return nil, err
			}
			selector.Type, err = context.field(
				selector.Type, args, fmt.Sprintf("workstation.operationBindings[%d].selector.type", index), readFile,
				"operationBindings", strconv.Itoa(index), "selector", "type",
			)
			if err != nil {
				return nil, err
			}
			selector.Role, err = context.field(
				selector.Role, args, fmt.Sprintf("workstation.operationBindings[%d].selector.role", index), readFile,
				"operationBindings", strconv.Itoa(index), "selector", "role",
			)
			if err != nil {
				return nil, err
			}
			binding.Selector = &selector
		}
		binding.Config, err = interpolateExecutionContentParts(
			binding.Config, args, fmt.Sprintf("workstation.operationBindings[%d].config", index), readFile,
			[]string{"operationBindings", strconv.Itoa(index), "config"}, context,
		)
		if err != nil {
			return nil, err
		}
		binding.DefaultContent, err = interpolateExecutionContentParts(
			binding.DefaultContent, args, fmt.Sprintf("workstation.operationBindings[%d].defaultContent", index), readFile,
			[]string{"operationBindings", strconv.Itoa(index), "defaultContent"}, context,
		)
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
	publicPath []string,
	contexts ...invocationInterpolationContext,
) ([]work.WorkContentPart, error) {
	context := firstInvocationInterpolationContext(contexts)
	next := work.CloneWorkContentParts(parts)
	for index := range next {
		part := &next[index]
		fields := []struct {
			value *string
			path  string
			field string
		}{
			{&part.Text, fmt.Sprintf("%s[%d].text", fieldPath, index), "text"},
			{&part.URL, fmt.Sprintf("%s[%d].url", fieldPath, index), "url"},
			{&part.File, fmt.Sprintf("%s[%d].file", fieldPath, index), "file"},
			{&part.Slot, fmt.Sprintf("%s[%d].slot", fieldPath, index), "slot"},
			{&part.Label, fmt.Sprintf("%s[%d].label", fieldPath, index), "label"},
			{&part.Role, fmt.Sprintf("%s[%d].role", fieldPath, index), "role"},
			{&part.ContentType, fmt.Sprintf("%s[%d].contentType", fieldPath, index), "contentType"},
		}
		for _, field := range fields {
			var err error
			*field.value, err = context.field(
				*field.value, args, field.path, readFile,
				append(append([]string(nil), publicPath...), strconv.Itoa(index), field.field)...,
			)
			if err != nil {
				return nil, err
			}
		}
		if len(part.JSON) > 0 {
			value, err := context.field(
				string(part.JSON), args, fmt.Sprintf("%s[%d].json", fieldPath, index), readFile,
				append(append([]string(nil), publicPath...), strconv.Itoa(index), "json")...,
			)
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
	publicPath []string,
	contexts ...invocationInterpolationContext,
) ([]IOConfig, error) {
	context := firstInvocationInterpolationContext(contexts)
	if len(values) == 0 {
		return nil, nil
	}
	next := CloneIOConfigs(values)
	for index := range next {
		value := &next[index]
		var err error
		value.WorkTypeName, err = context.field(
			value.WorkTypeName, args, fmt.Sprintf("%s[%d].workType", path, index), readFile,
			append(append([]string(nil), publicPath...), strconv.Itoa(index), "workType")...,
		)
		if err != nil {
			return nil, err
		}
		value.StateName, err = context.field(
			value.StateName, args, fmt.Sprintf("%s[%d].state", path, index), readFile,
			append(append([]string(nil), publicPath...), strconv.Itoa(index), "state")...,
		)
		if err != nil {
			return nil, err
		}
		if value.Guard != nil {
			guard := *value.Guard
			guard.MatchInput, err = context.field(
				guard.MatchInput, args, fmt.Sprintf("%s[%d].guard.matchInput", path, index), readFile,
				append(append([]string(nil), publicPath...), strconv.Itoa(index), "guard", "matchInput")...,
			)
			if err != nil {
				return nil, err
			}
			guard.ParentInput, err = context.field(
				guard.ParentInput, args, fmt.Sprintf("%s[%d].guard.parentInput", path, index), readFile,
				append(append([]string(nil), publicPath...), strconv.Itoa(index), "guard", "parentInput")...,
			)
			if err != nil {
				return nil, err
			}
			guard.SpawnedBy, err = context.field(
				guard.SpawnedBy, args, fmt.Sprintf("%s[%d].guard.spawnedBy", path, index), readFile,
				append(append([]string(nil), publicPath...), strconv.Itoa(index), "guard", "spawnedBy")...,
			)
			if err != nil {
				return nil, err
			}
			value.Guard = &guard
		}
	}
	return next, nil
}

func interpolateExecutionFieldWithProvenance(
	authored string,
	args *work.InvocationArguments,
	fieldPath string,
	readFile FileReader,
	jsonPointer string,
	provenance *invocationInterpolationProvenance,
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
		replacement, err := executionInvocationScalar(argument, name, fieldPath, readFile)
		if err != nil {
			return "", err
		}
		if argument.Sensitive {
			provenance.record(jsonPointer, 0, len(replacement))
		}
		return replacement, nil
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
		replacementStart := builder.Len()
		builder.WriteString(replacement)
		if argument.Sensitive {
			provenance.record(jsonPointer, replacementStart, builder.Len())
		}
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
