package contractvalidator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type cliInputRecord struct {
	id     string
	path   []string
	record map[string]any
}

func cliCommandValueAndBindingDiagnostics(document, commandKey string, command map[string]any) []Diagnostic {
	inputs := collectCLIInputs(commandKey, command)
	var diagnostics []Diagnostic
	for _, input := range inputs {
		if !cliInputIsCanonical(input.record) {
			continue
		}
		diagnostics = append(diagnostics, cliInputValueDiagnostics(document, input)...)
		diagnostics = append(diagnostics, cliInputCardinalityDiagnostics(document, input)...)
	}
	diagnostics = append(diagnostics, cliHandlerBindingDiagnostics(document, commandKey, command, inputs)...)
	diagnostics = append(diagnostics, cliSourceBindingDiagnostics(document, commandKey, command, inputs)...)
	return diagnostics
}

func cliInputIsCanonical(input map[string]any) bool {
	bindingID, _ := input["handlerBindingId"].(string)
	return bindingID != ""
}

func collectCLIInputs(commandKey string, command map[string]any) []cliInputRecord {
	var inputs []cliInputRecord
	for _, field := range []string{"arguments", "flags"} {
		records, _ := command[field].(map[string]any)
		for _, key := range sortedStringKeys(records) {
			record, _ := records[key].(map[string]any)
			id, _ := record["id"].(string)
			inputs = append(inputs, cliInputRecord{id: id, path: []string{"commands", commandKey, field, key}, record: record})
		}
	}
	return inputs
}

func cliInputValueDiagnostics(document string, input cliInputRecord) []Diagnostic {
	var diagnostics []Diagnostic
	valueType, _ := input.record["valueType"].(string)
	normalization, _ := input.record["normalization"].(string)
	if normalization != "" && valueType != "string" && valueType != "stringArray" {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "normalization", "cli.input.normalization-value-type",
			"normalization is supported only for string and stringArray inputs"))
	}
	for _, field := range []string{"defaultValue", "noOptionDefaultValue"} {
		value, exists := input.record[field].(map[string]any)
		if !exists {
			continue
		}
		valueKind, typedValue := singleCLIInputValue(value)
		if valueKind != canonicalCLIValueKind(valueType) {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, field, "cli.input.value-type",
				fmt.Sprintf("%s kind %q does not match declared valueType %q", field, valueKind, valueType)))
			continue
		}
		if !cliInputValueAllowed(input.record, typedValue) {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, field, "cli.input.value-choice",
				fmt.Sprintf("%s contains a value outside the declared enum", field)))
		}
		if !cliInputValueNormalized(input.record, typedValue) {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, field, "cli.input.value-normalization",
				fmt.Sprintf("%s is not in declared normalized form", field)))
		}
		if field == "noOptionDefaultValue" && valueType != "bool" && valueType != "string" {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, field, "cli.input.no-option-invalid",
				"no-option defaults are supported only for boolean and string named inputs"))
		}
	}
	return diagnostics
}

func singleCLIInputValue(value map[string]any) (string, any) {
	for _, key := range sortedStringKeys(value) {
		return key, value[key]
	}
	return "", nil
}

func canonicalCLIValueKind(valueType string) string {
	if valueType == "bool" {
		return "boolean"
	}
	return valueType
}

func cliInputValueAllowed(input map[string]any, value any) bool {
	choices, _ := input["enum"].([]any)
	if len(choices) == 0 {
		return true
	}
	allowed := make(map[string]bool, len(choices))
	for _, choice := range choices {
		allowed[fmt.Sprint(choice)] = true
	}
	values, ok := value.([]any)
	if !ok {
		return allowed[fmt.Sprint(value)]
	}
	for _, item := range values {
		if !allowed[fmt.Sprint(item)] {
			return false
		}
	}
	return true
}

func cliInputValueNormalized(input map[string]any, value any) bool {
	normalization, _ := input["normalization"].(string)
	if normalization == "" {
		return true
	}
	values, ok := value.([]any)
	if !ok {
		text, ok := value.(string)
		return !ok || normalizeCLIInputValue(text, normalization) == text
	}
	for _, item := range values {
		text, ok := item.(string)
		if ok && normalizeCLIInputValue(text, normalization) != text {
			return false
		}
	}
	return true
}

func normalizeCLIInputValue(value, normalization string) string {
	switch normalization {
	case "lowercase":
		return strings.ToLower(value)
	case "trim":
		return strings.TrimSpace(value)
	case "lowercase-trim":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return value
	}
}

func cliInputCardinalityDiagnostics(document string, input cliInputRecord) []Diagnostic {
	minimum := cliInt(input.record["minCardinality"])
	maximum := cliInt(input.record["maxCardinality"])
	required, _ := input.record["required"].(bool)
	valueType, _ := input.record["valueType"].(string)
	repeatable, _ := input.record["repeatable"].(bool)
	variadic, _ := input.record["variadic"].(bool)
	var diagnostics []Diagnostic
	if required && minimum < 1 {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "minCardinality", "cli.input.cardinality-required", "required input must have a minimum cardinality of at least one"))
	}
	if maximum != -1 && minimum > maximum {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "maxCardinality", "cli.input.cardinality-range", "maximum cardinality must be unbounded or at least the minimum"))
	}
	if (repeatable || variadic || maximum == -1 || maximum > 1) && valueType != "stringArray" {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "valueType", "cli.input.cardinality-value-type", "repeated or unbounded inputs must use stringArray valueType"))
	}
	if repeatable && maximum == 1 {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "maxCardinality", "cli.input.cardinality-repeatable", "repeatable input must accept more than one value"))
	}
	if variadic && maximum != -1 {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "maxCardinality", "cli.input.cardinality-variadic", "variadic input must have unbounded maximum cardinality"))
	}
	if value, exists := input.record["defaultValue"].(map[string]any); exists {
		_, typedValue := singleCLIInputValue(value)
		count := 1
		if values, ok := typedValue.([]any); ok {
			count = len(values)
		}
		if count < minimum || (maximum != -1 && count > maximum) {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "defaultValue", "cli.input.default-cardinality", "default value count is outside the declared cardinality"))
		}
	}
	if _, exists := input.record["noOptionDefaultValue"]; exists && maximum == 0 {
		diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "noOptionDefaultValue", "cli.input.no-option-cardinality", "no-option default cannot target an input that accepts no values"))
	}
	return diagnostics
}

func cliInt(value any) int {
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case float64:
		return int(number)
	case json.Number:
		parsed, _ := strconv.Atoi(number.String())
		return parsed
	default:
		return 0
	}
}

func cliHandlerBindingDiagnostics(document, commandKey string, command map[string]any, inputs []cliInputRecord) []Diagnostic {
	bindings, _ := command["handlerBindings"].(map[string]any)
	known := cliInputsByID(inputs)
	var diagnostics []Diagnostic
	for _, key := range sortedStringKeys(bindings) {
		binding, _ := bindings[key].(map[string]any)
		inputID, _ := binding["inputId"].(string)
		if _, exists := known[inputID]; !exists {
			diagnostics = append(diagnostics, newDiagnostic("cli.binding.unknown-input", instancePath([]string{"commands", commandKey, "handlerBindings", key, "inputId"}), fmt.Sprintf("handler binding references unknown input %q", inputID), document))
		}
	}
	owners := make(map[string][]cliInputRecord)
	for _, input := range inputs {
		bindingID, _ := input.record["handlerBindingId"].(string)
		if bindingID == "" {
			continue
		}
		owners[bindingID] = append(owners[bindingID], input)
		binding, exists := bindings[bindingID].(map[string]any)
		if !exists {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "handlerBindingId", "cli.binding.unknown-handler", fmt.Sprintf("input references unknown handler binding %q", bindingID)))
			continue
		}
		if binding["inputId"] != input.id {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "handlerBindingId", "cli.binding.target-mismatch", fmt.Sprintf("handler binding %q targets a different input", bindingID)))
		}
	}
	for bindingID, bindingOwners := range owners {
		if len(bindingOwners) < 2 {
			continue
		}
		for _, input := range bindingOwners {
			diagnostics = append(diagnostics, cliInputDiagnostic(document, input, "handlerBindingId", "cli.binding.multiple-targets", fmt.Sprintf("handler binding %q is claimed by multiple inputs", bindingID)))
		}
	}
	return diagnostics
}

func cliSourceBindingDiagnostics(document, commandKey string, command map[string]any, inputs []cliInputRecord) []Diagnostic {
	bindings, _ := command["sourceBindings"].(map[string]any)
	known := cliInputsByID(inputs)
	bound := make(map[string]map[string]bool)
	owners := make(map[string][]string)
	var diagnostics []Diagnostic
	for _, key := range sortedStringKeys(bindings) {
		binding, _ := bindings[key].(map[string]any)
		source, _ := binding["source"].(string)
		inputID, _ := binding["inputId"].(string)
		path := []string{"commands", commandKey, "sourceBindings", key, "inputId"}
		input, exists := known[inputID]
		if !exists {
			diagnostics = append(diagnostics, newDiagnostic("cli.source.unknown-input", instancePath(path), fmt.Sprintf("source binding references unknown input %q", inputID), document))
			continue
		}
		if !cliInputAcceptsSource(input.record, source) {
			diagnostics = append(diagnostics, newDiagnostic("cli.source.not-accepted", instancePath(path), fmt.Sprintf("input %q does not accept source %q", inputID, source), document))
		}
		if bound[inputID] == nil {
			bound[inputID] = make(map[string]bool)
		}
		bound[inputID][source] = true
		ownerKey := source + ":" + fmt.Sprint(binding["externalKey"])
		if source == "stdin" {
			ownerKey = source
			if !cliInputConsumesStdin(input.record) {
				diagnostics = append(diagnostics, newDiagnostic("cli.source.stdin-shape", instancePath(path), fmt.Sprintf("input %q cannot consume stdin", inputID), document))
			}
		}
		owners[ownerKey] = append(owners[ownerKey], instancePath(path))
	}
	for ownerKey, paths := range owners {
		if len(paths) < 2 {
			continue
		}
		sort.Strings(paths)
		for _, path := range paths {
			diagnostics = append(diagnostics, newDiagnostic("cli.source.multiple-targets", path, fmt.Sprintf("source route %q targets multiple inputs", ownerKey), document))
		}
	}
	for _, input := range inputs {
		for _, source := range cliAcceptedExternalSources(input.record) {
			if !bound[input.id][source.value] {
				path := instancePath(append(input.path, "acceptedSources", fmt.Sprint(source.index)))
				diagnostics = append(diagnostics, newDiagnostic("cli.source.missing-binding", path, fmt.Sprintf("accepted source %q has no explicit binding", source.value), document))
			}
		}
	}
	return diagnostics
}

type cliAcceptedSource struct {
	index int
	value string
}

func cliAcceptedExternalSources(input map[string]any) []cliAcceptedSource {
	values, _ := input["acceptedSources"].([]any)
	var sources []cliAcceptedSource
	for index, raw := range values {
		value, _ := raw.(string)
		if value == "stdin" || value == "environment" || value == "operator-config" {
			sources = append(sources, cliAcceptedSource{index: index, value: value})
		}
	}
	return sources
}

func cliInputAcceptsSource(input map[string]any, source string) bool {
	for _, accepted := range cliAcceptedExternalSources(input) {
		if accepted.value == source {
			return true
		}
	}
	return false
}

func cliInputConsumesStdin(input map[string]any) bool {
	valueType, _ := input["valueType"].(string)
	return (valueType == "string" || valueType == "stringArray") && cliInt(input["maxCardinality"]) != 0
}

func cliInputsByID(inputs []cliInputRecord) map[string]cliInputRecord {
	known := make(map[string]cliInputRecord, len(inputs))
	for _, input := range inputs {
		known[input.id] = input
	}
	return known
}

func cliInputDiagnostic(document string, input cliInputRecord, field, code, message string) Diagnostic {
	return newDiagnostic(code, instancePath(append(input.path, field)), message, document)
}
