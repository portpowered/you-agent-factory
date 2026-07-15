package contractvalidator

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// RuntimeManifestSemanticsDiagnostics applies runtime-manifest semantic checks
// after schema validation succeeds.
func RuntimeManifestSemanticsDiagnostics(document string, value any) []Diagnostic {
	return runtimeManifestDiagnostics(document, value)
}

func runtimeManifestDiagnostics(document string, value any) []Diagnostic {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	symbols, ok := root["symbols"].(map[string]any)
	if !ok {
		return nil
	}

	keys := sortedStringKeys(symbols)

	symbolsByID := make(map[string]string, len(keys))
	symbolKindByKey := make(map[string]string, len(keys))
	pathByKey := make(map[string]string, len(keys))
	childrenByParent := make(map[string]map[string]struct{}, len(keys))

	for _, key := range keys {
		symbol, ok := symbols[key].(map[string]any)
		if !ok {
			continue
		}
		id, _ := symbol["id"].(string)
		path, _ := symbol["path"].(string)
		kind, _ := symbol["kind"].(string)
		if id != "" {
			symbolsByID[id] = key
		}
		pathByKey[key] = path
		symbolKindByKey[key] = kind

		parent, hasParent := symbol["parent"].(string)
		name, _ := symbol["name"].(string)
		if hasParent && parent != "" && name != "" {
			if childrenByParent[parent] == nil {
				childrenByParent[parent] = make(map[string]struct{})
			}
			childrenByParent[parent][name] = struct{}{}
		}
	}

	var diagnostics []Diagnostic

	pathToKeys := make(map[string][]string, len(keys))
	for _, key := range keys {
		path := pathByKey[key]
		if path == "" {
			continue
		}
		pathToKeys[path] = append(pathToKeys[path], key)
	}
	paths := make([]string, 0, len(pathToKeys))
	for path := range pathToKeys {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		keysWithPath := pathToKeys[path]
		if len(keysWithPath) <= 1 {
			continue
		}
		message := fmt.Sprintf("symbol path %s appears more than once", strconv.Quote(path))
		for _, key := range keysWithPath {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.path.duplicate",
				"/symbols/"+escapeJSONPointerToken(key)+"/path",
				message,
				document,
			))
		}
	}

	for _, key := range keys {
		symbol, ok := symbols[key].(map[string]any)
		if !ok {
			continue
		}
		symbolPath := "/symbols/" + escapeJSONPointerToken(key)

		if parent, ok := symbol["parent"].(string); ok && parent != "" {
			parentKey, exists := symbolsByID[parent]
			if !exists {
				diagnostics = append(diagnostics, newDiagnostic(
					"javascript.parent.unresolved",
					symbolPath+"/parent",
					fmt.Sprintf("parent symbol %s is not declared", strconv.Quote(parent)),
					document,
				))
			} else if symbolKindByKey[parentKey] != "namespace" {
				diagnostics = append(diagnostics, newDiagnostic(
					"javascript.parent.unresolved",
					symbolPath+"/parent",
					fmt.Sprintf("parent symbol %s is not a namespace", strconv.Quote(parent)),
					document,
				))
			}
		}

		members, ok := symbol["members"].([]any)
		if ok {
			symbolID, _ := symbol["id"].(string)
			children := childrenByParent[symbolID]
			for index, memberValue := range members {
				member, ok := memberValue.(string)
				if !ok {
					continue
				}
				if _, resolved := children[member]; !resolved {
					diagnostics = append(diagnostics, newDiagnostic(
						"javascript.member.unresolved",
						symbolPath+"/members/"+strconv.Itoa(index),
						fmt.Sprintf("member %s does not resolve to a declared child symbol", strconv.Quote(member)),
						document,
					))
				}
			}
		}

		if parameters, ok := symbol["parameters"].([]any); ok {
			diagnostics = append(diagnostics, runtimeManifestParameterListDiagnostics(document, symbolPath+"/parameters", parameters)...)
		}
		if callback, ok := symbol["callback"].(map[string]any); ok {
			if parameters, ok := callback["parameters"].([]any); ok {
				diagnostics = append(diagnostics, runtimeManifestParameterListDiagnostics(document, symbolPath+"/callback/parameters", parameters)...)
			}
		}
		if returnValue, ok := symbol["return"].(map[string]any); ok {
			if serializableValue, ok := returnValue["serializableValue"]; ok {
				diagnostics = append(diagnostics, openSerializableValueDiagnostics(
					document,
					symbolPath+"/return/serializableValue",
					serializableValue,
				)...)
			}
		}
	}

	diagnostics = append(diagnostics, runtimeManifestSupportedSurfaceDiagnostics(
		document,
		keys,
		pathByKey,
		symbolKindByKey,
	)...)

	sortDiagnostics(diagnostics)
	return diagnostics
}

func runtimeManifestParameterListDiagnostics(document, listPath string, parameters []any) []Diagnostic {
	var diagnostics []Diagnostic

	positions := make(map[int][]int)
	restIndexes := make([]int, 0)
	for index, parameterValue := range parameters {
		parameter, ok := parameterValue.(map[string]any)
		if !ok {
			continue
		}
		parameterPath := listPath + "/" + strconv.Itoa(index)
		if position, ok := parameterPosition(parameter["position"]); ok {
			positions[position] = append(positions[position], index)
		}
		if rest, ok := parameter["rest"].(bool); ok && rest {
			restIndexes = append(restIndexes, index)
		}
		if serializableValue, ok := parameter["serializableValue"]; ok {
			diagnostics = append(diagnostics, openSerializableValueDiagnostics(
				document,
				parameterPath+"/serializableValue",
				serializableValue,
			)...)
		}
	}

	positionKeys := make([]int, 0, len(positions))
	for position := range positions {
		positionKeys = append(positionKeys, position)
	}
	sort.Ints(positionKeys)

	for _, position := range positionKeys {
		indexes := positions[position]
		if len(indexes) <= 1 {
			continue
		}
		message := fmt.Sprintf("parameter position %d appears more than once", position)
		for _, index := range indexes {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.signature.duplicate_position",
				listPath+"/"+strconv.Itoa(index)+"/position",
				message,
				document,
			))
		}
	}

	if len(positionKeys) > 0 {
		for expected := 0; expected < len(parameters); expected++ {
			if _, ok := positions[expected]; !ok {
				firstIndex := 0
				if len(positionKeys) > 0 {
					firstIndex = positions[positionKeys[0]][0]
				}
				diagnostics = append(diagnostics, newDiagnostic(
					"javascript.signature.position_gap",
					listPath+"/"+strconv.Itoa(firstIndex)+"/position",
					fmt.Sprintf("parameter positions must be contiguous from 0 through %d", len(parameters)-1),
					document,
				))
				break
			}
		}
	}

	if len(restIndexes) > 1 {
		message := "only one rest parameter is allowed per callable signature"
		for _, index := range restIndexes {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.signature.multiple_rest",
				listPath+"/"+strconv.Itoa(index)+"/rest",
				message,
				document,
			))
		}
	}
	if len(restIndexes) == 1 && len(parameters) > 0 {
		restIndex := restIndexes[0]
		if restIndex != len(parameters)-1 {
			diagnostics = append(diagnostics, newDiagnostic(
				"javascript.signature.rest_not_last",
				listPath+"/"+strconv.Itoa(restIndex)+"/rest",
				"rest parameter must be the final parameter in the signature",
				document,
			))
		}
	}

	return diagnostics
}

func openSerializableValueDiagnostics(document, schemaPath string, value any) []Diagnostic {
	schema, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	var diagnostics []Diagnostic
	collectOpenSerializableValueDiagnostics(document, schemaPath, schema, &diagnostics)
	return diagnostics
}

func collectOpenSerializableValueDiagnostics(document, schemaPath string, schema map[string]any, diagnostics *[]Diagnostic) {
	if objectSchemaRequiresClosedProperties(schema) && !serializableObjectSchemaIsClosed(schema) {
		*diagnostics = append(*diagnostics, newDiagnostic(
			"javascript.serializable_value.open",
			schemaPath,
			"object serializable-value schemas must set additionalProperties to false",
			document,
		))
	}

	for key, child := range schema {
		childPath := schemaPath + "/" + escapeJSONPointerToken(key)
		switch typed := child.(type) {
		case map[string]any:
			collectOpenSerializableValueDiagnostics(document, childPath, typed, diagnostics)
		case []any:
			for index, item := range typed {
				if itemSchema, ok := item.(map[string]any); ok {
					collectOpenSerializableValueDiagnostics(document, childPath+"/"+strconv.Itoa(index), itemSchema, diagnostics)
				}
			}
		}
	}
}

func objectSchemaRequiresClosedProperties(schema map[string]any) bool {
	if typeIncludesObject(schema["type"]) {
		return true
	}
	if _, ok := schema["properties"]; ok {
		return true
	}
	return false
}

func serializableObjectSchemaIsClosed(schema map[string]any) bool {
	additionalProperties, exists := schema["additionalProperties"]
	if !exists {
		return false
	}
	if closed, ok := additionalProperties.(bool); ok {
		return !closed
	}
	return false
}

func typeIncludesObject(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == "object"
	case []any:
		for _, item := range typed {
			if label, ok := item.(string); ok && label == "object" {
				return true
			}
		}
	}
	return false
}

func parameterPosition(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		position, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(position), true
	default:
		return 0, false
	}
}
