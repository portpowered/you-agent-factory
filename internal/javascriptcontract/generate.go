// Package javascriptcontract projects the runtime-owned agent.run descriptor
// into the published JavaScript runtime catalog.
package javascriptcontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const RuntimeCatalogPath = "contracts/javascript/runtime-api.json"

// GenerateRuntimeCatalog replaces the agent.run schema projection with the
// current runtime descriptor while preserving the catalog's hand-authored
// symbols and explanatory metadata.
func GenerateRuntimeCatalog(catalog []byte) ([]byte, error) {
	return ProjectRuntimeCatalog(catalog, factoryruntime.JavaScriptChildFieldDescriptors())
}

// ProjectRuntimeCatalog projects fields into a catalog. The explicit fields
// parameter keeps forward-evolution tests independent from filesystem state
// while production generation always supplies the runtime-owned descriptor.
func ProjectRuntimeCatalog(catalog []byte, fields []factoryruntime.JavaScriptChildFieldDescriptor) ([]byte, error) {
	var document map[string]any
	decoder := json.NewDecoder(bytes.NewReader(catalog))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("decode %s: %w", RuntimeCatalogPath, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("decode %s: trailing JSON value", RuntimeCatalogPath)
		}
		return nil, fmt.Errorf("decode %s: trailing content: %w", RuntimeCatalogPath, err)
	}

	if err := validateFields(fields); err != nil {
		return nil, err
	}
	if _, err := agentRunSchema(document); err != nil {
		return nil, err
	}
	properties, err := marshalProperties(fields)
	if err != nil {
		return nil, err
	}
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		if field.Required {
			required = append(required, field.Name)
		}
	}
	requiredPayload, err := json.MarshalIndent(required, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated required fields: %w", err)
	}

	propertyRange, err := jsonPathValueRange(catalog, "properties")
	if err != nil {
		return nil, err
	}
	requiredRange, err := jsonPathValueRange(catalog, "required")
	if err != nil {
		return nil, err
	}
	replacements := []jsonReplacement{
		{start: propertyRange.valueStart, end: propertyRange.valueEnd, payload: indentJSON(properties, propertyRange.keyIndent)},
		{start: requiredRange.valueStart, end: requiredRange.valueEnd, payload: indentJSON(requiredPayload, requiredRange.keyIndent)},
	}
	sort.Slice(replacements, func(left, right int) bool {
		return replacements[left].start > replacements[right].start
	})
	projected := bytes.Clone(catalog)
	for _, replacement := range replacements {
		projected = append(projected[:replacement.start], append(replacement.payload, projected[replacement.end:]...)...)
	}
	return projected, nil
}

func agentRunSchema(document map[string]any) (map[string]any, error) {
	sharedSchemas, ok := document["sharedSchemas"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is missing object /sharedSchemas", RuntimeCatalogPath)
	}
	agentRun, ok := sharedSchemas["javascript.schema.agent_run_spec"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is missing object /sharedSchemas/javascript.schema.agent_run_spec", RuntimeCatalogPath)
	}
	schema, ok := agentRun["schema"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s is missing object /sharedSchemas/javascript.schema.agent_run_spec/schema", RuntimeCatalogPath)
	}
	return schema, nil
}

func validateFields(fields []factoryruntime.JavaScriptChildFieldDescriptor) error {
	if len(fields) == 0 {
		return fmt.Errorf("agent.run runtime descriptor is empty")
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.Name == "" {
			return fmt.Errorf("agent.run runtime descriptor contains an empty field name")
		}
		if _, exists := seen[field.Name]; exists {
			return fmt.Errorf("agent.run runtime descriptor contains duplicate field %q", field.Name)
		}
		seen[field.Name] = struct{}{}
		switch field.JSONType {
		case "string", "boolean", "object":
		default:
			return fmt.Errorf("agent.run runtime descriptor field %q has unsupported JSON type %q", field.Name, field.JSONType)
		}
		seenEnums := make(map[string]struct{}, len(field.Enum))
		for _, allowed := range field.Enum {
			if allowed == "" {
				return fmt.Errorf("agent.run runtime descriptor field %q contains an empty enum value", field.Name)
			}
			if _, exists := seenEnums[allowed]; exists {
				return fmt.Errorf("agent.run runtime descriptor field %q contains duplicate enum value %q", field.Name, allowed)
			}
			seenEnums[allowed] = struct{}{}
		}
		if len(field.Enum) > 0 && field.JSONType != "string" {
			return fmt.Errorf("agent.run runtime descriptor field %q cannot declare enum values for JSON type %q", field.Name, field.JSONType)
		}
		if field.AdditionalProperties != nil && field.JSONType != "object" {
			return fmt.Errorf("agent.run runtime descriptor field %q cannot declare additionalProperties for JSON type %q", field.Name, field.JSONType)
		}
	}
	return nil
}

type jsonValueRange struct {
	valueStart int
	valueEnd   int
	keyIndent  int
}

type jsonReplacement struct {
	start   int
	end     int
	payload []byte
}

func jsonPathValueRange(document []byte, leaf string) (jsonValueRange, error) {
	path := []string{"sharedSchemas", "javascript.schema.agent_run_spec", "schema", leaf}
	objectStart := skipJSONWhitespace(document, 0)
	for _, segment := range path {
		rangeValue, err := findObjectMember(document, objectStart, segment)
		if err != nil {
			return jsonValueRange{}, fmt.Errorf("%s cannot locate /%s: %w", RuntimeCatalogPath, joinJSONPath(path), err)
		}
		if segment == leaf {
			return rangeValue, nil
		}
		objectStart = skipJSONWhitespace(document, rangeValue.valueStart)
		if objectStart >= len(document) || document[objectStart] != '{' {
			return jsonValueRange{}, fmt.Errorf("%s path segment %q is not an object", RuntimeCatalogPath, segment)
		}
	}
	return jsonValueRange{}, fmt.Errorf("%s cannot locate generated field %q", RuntimeCatalogPath, leaf)
}

func findObjectMember(document []byte, objectStart int, wanted string) (jsonValueRange, error) {
	if objectStart >= len(document) || document[objectStart] != '{' {
		return jsonValueRange{}, fmt.Errorf("expected object")
	}
	position := skipJSONWhitespace(document, objectStart+1)
	if position < len(document) && document[position] == '}' {
		return jsonValueRange{}, fmt.Errorf("member %q is missing", wanted)
	}
	for position < len(document) {
		keyStart := position
		keyEnd, err := scanJSONString(document, keyStart)
		if err != nil {
			return jsonValueRange{}, err
		}
		var key string
		if err := json.Unmarshal(document[keyStart:keyEnd], &key); err != nil {
			return jsonValueRange{}, fmt.Errorf("decode member name: %w", err)
		}
		position = skipJSONWhitespace(document, keyEnd)
		if position >= len(document) || document[position] != ':' {
			return jsonValueRange{}, fmt.Errorf("member %q is missing a colon", key)
		}
		valueStart := skipJSONWhitespace(document, position+1)
		valueEnd, err := scanJSONValue(document, valueStart)
		if err != nil {
			return jsonValueRange{}, err
		}
		if key == wanted {
			return jsonValueRange{
				valueStart: valueStart,
				valueEnd:   valueEnd,
				keyIndent:  lineIndent(document, keyStart),
			}, nil
		}
		position = skipJSONWhitespace(document, valueEnd)
		if position >= len(document) || document[position] == '}' {
			break
		}
		if document[position] != ',' {
			return jsonValueRange{}, fmt.Errorf("member %q is missing a comma", key)
		}
		position = skipJSONWhitespace(document, position+1)
	}
	return jsonValueRange{}, fmt.Errorf("member %q is missing", wanted)
}

func scanJSONString(document []byte, start int) (int, error) {
	if start >= len(document) || document[start] != '"' {
		return 0, fmt.Errorf("expected JSON string")
	}
	escaped := false
	for position := start + 1; position < len(document); position++ {
		switch {
		case escaped:
			escaped = false
		case document[position] == '\\':
			escaped = true
		case document[position] == '"':
			return position + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

func scanJSONValue(document []byte, start int) (int, error) {
	if start >= len(document) {
		return 0, fmt.Errorf("missing JSON value")
	}
	if document[start] == '"' {
		return scanJSONString(document, start)
	}
	if document[start] == '{' || document[start] == '[' {
		opening := document[start]
		closing := byte('}')
		if opening == '[' {
			closing = ']'
		}
		depth := 0
		inString := false
		escaped := false
		for position := start; position < len(document); position++ {
			character := document[position]
			if inString {
				if escaped {
					escaped = false
				} else if character == '\\' {
					escaped = true
				} else if character == '"' {
					inString = false
				}
				continue
			}
			switch character {
			case '"':
				inString = true
			case opening:
				depth++
			case closing:
				depth--
				if depth == 0 {
					return position + 1, nil
				}
			}
		}
		return 0, fmt.Errorf("unterminated JSON composite value")
	}
	position := start
	for position < len(document) {
		character := document[position]
		if character == ',' || character == '}' || character == ']' || character == ' ' || character == '\n' || character == '\r' || character == '\t' {
			break
		}
		position++
	}
	return position, nil
}

func skipJSONWhitespace(document []byte, start int) int {
	for start < len(document) {
		switch document[start] {
		case ' ', '\n', '\r', '\t':
			start++
		default:
			return start
		}
	}
	return start
}

func lineIndent(document []byte, position int) int {
	lineStart := bytes.LastIndexByte(document[:position], '\n') + 1
	indent := 0
	for lineStart+indent < len(document) && (document[lineStart+indent] == ' ' || document[lineStart+indent] == '\t') {
		indent++
	}
	return indent
}

func indentJSON(payload []byte, indent int) []byte {
	if indent == 0 || !bytes.Contains(payload, []byte{'\n'}) {
		return payload
	}
	prefix := bytes.Repeat([]byte(" "), indent)
	return bytes.ReplaceAll(payload, []byte{'\n'}, append([]byte{'\n'}, prefix...))
}

func marshalProperties(fields []factoryruntime.JavaScriptChildFieldDescriptor) ([]byte, error) {
	var result bytes.Buffer
	result.WriteString("{")
	for index, field := range fields {
		name, err := json.Marshal(field.Name)
		if err != nil {
			return nil, fmt.Errorf("encode field name %q: %w", field.Name, err)
		}
		if index == 0 {
			result.WriteString("\n  ")
		} else {
			result.WriteString(",\n  ")
		}
		result.Write(name)
		result.WriteString(": {\n    \"type\": ")
		typeName, err := json.Marshal(field.JSONType)
		if err != nil {
			return nil, fmt.Errorf("encode JSON type for %q: %w", field.Name, err)
		}
		result.Write(typeName)
		if field.AdditionalProperties != nil {
			result.WriteString(",\n    \"additionalProperties\": ")
			additionalProperties, err := json.Marshal(*field.AdditionalProperties)
			if err != nil {
				return nil, fmt.Errorf("encode additionalProperties for %q: %w", field.Name, err)
			}
			result.Write(additionalProperties)
		}
		if len(field.Enum) > 0 {
			result.WriteString(",\n    \"enum\": [")
			for enumIndex, allowed := range field.Enum {
				encoded, err := json.Marshal(allowed)
				if err != nil {
					return nil, fmt.Errorf("encode enum value for %q: %w", field.Name, err)
				}
				if enumIndex == 0 {
					result.WriteString("\n      ")
				} else {
					result.WriteString(",\n      ")
				}
				result.Write(encoded)
			}
			result.WriteString("\n    ]")
		}
		result.WriteString("\n  }")
	}
	if len(fields) > 0 {
		result.WriteString("\n")
	}
	result.WriteString("}")
	return result.Bytes(), nil
}

func joinJSONPath(path []string) string {
	result := ""
	for _, segment := range path {
		result += "/" + segment
	}
	return result
}
