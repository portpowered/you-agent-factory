package authoredlayout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"gopkg.in/yaml.v3"
)

func decodeAuthoredFactory(
	path string,
	format factorydefinitions.AuthoredFactoryFormat,
	data []byte,
) ([]byte, error) {
	var (
		decoded []byte
		err     error
	)
	switch format {
	case factorydefinitions.AuthoredFactoryFormatJSON:
		var document json.RawMessage
		if decodeErr := json.Unmarshal(data, &document); decodeErr != nil {
			err = decodeErr
		} else {
			decoded = data
		}
	case factorydefinitions.AuthoredFactoryFormatYAML:
		decoded, err = strictYAMLToJSON(data)
	default:
		err = fmt.Errorf("unsupported authored Factory format %q", format)
	}
	if err != nil {
		return nil, fmt.Errorf(
			"decode Factory Definition %s as %s: %w",
			path,
			format,
			err,
		)
	}
	return decoded, nil
}

func strictYAMLToJSON(data []byte) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("YAML document is empty")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple YAML documents are not supported")
		}
		return nil, fmt.Errorf("decode trailing YAML content: %w", err)
	}
	value, err := yamlNodeToJSONValue(document.Content[0])
	if err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("Factory Definition root must be a mapping")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode JSON-compatible YAML value: %w", err)
	}
	return encoded, nil
}

func yamlNodeToJSONValue(node *yaml.Node) (any, error) {
	if node == nil {
		return nil, fmt.Errorf("YAML value is missing")
	}
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return nil, yamlNodeError(node, "YAML anchors and aliases are not supported")
	}
	switch node.Kind {
	case yaml.MappingNode:
		return yamlMappingToJSONValue(node)
	case yaml.SequenceNode:
		values := make([]any, len(node.Content))
		for index, item := range node.Content {
			value, err := yamlNodeToJSONValue(item)
			if err != nil {
				return nil, err
			}
			values[index] = value
		}
		return values, nil
	case yaml.ScalarNode:
		return yamlScalarToJSONValue(node)
	default:
		return nil, yamlNodeError(node, "unsupported YAML node")
	}
}

func yamlMappingToJSONValue(node *yaml.Node) (map[string]any, error) {
	if len(node.Content)%2 != 0 {
		return nil, yamlNodeError(node, "malformed YAML mapping")
	}
	values := make(map[string]any, len(node.Content)/2)
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Tag != "!!str" {
			return nil, yamlNodeError(keyNode, "YAML mapping keys must be strings")
		}
		key := keyNode.Value
		if _, exists := values[key]; exists {
			return nil, yamlNodeError(keyNode, fmt.Sprintf("duplicate YAML mapping key %q", key))
		}
		value, err := yamlNodeToJSONValue(node.Content[index+1])
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, nil
}

func yamlScalarToJSONValue(node *yaml.Node) (any, error) {
	switch node.Tag {
	case "!!str":
		return node.Value, nil
	case "!!null":
		return nil, nil
	case "!!bool":
		value, err := strconv.ParseBool(node.Value)
		if err != nil {
			return nil, yamlNodeError(node, "invalid YAML boolean")
		}
		return value, nil
	case "!!int":
		return yamlIntegerToJSONNumber(node)
	case "!!float":
		var value float64
		if err := node.Decode(&value); err != nil {
			return nil, yamlNodeError(node, "invalid YAML number")
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, yamlNodeError(node, "non-finite YAML numbers are not supported")
		}
		return json.Number(strconv.FormatFloat(value, 'g', -1, 64)), nil
	default:
		return nil, yamlNodeError(
			node,
			fmt.Sprintf("YAML tag %q is not JSON-compatible", node.Tag),
		)
	}
}

func yamlIntegerToJSONNumber(node *yaml.Node) (json.Number, error) {
	var value any
	if err := node.Decode(&value); err != nil {
		return "", yamlNodeError(node, "invalid YAML integer")
	}
	switch typed := value.(type) {
	case int:
		return json.Number(strconv.FormatInt(int64(typed), 10)), nil
	case int64:
		return json.Number(strconv.FormatInt(typed, 10)), nil
	case uint64:
		return json.Number(strconv.FormatUint(typed, 10)), nil
	default:
		return "", yamlNodeError(node, "YAML integer is not JSON-compatible")
	}
}

func yamlNodeError(node *yaml.Node, message string) error {
	if node.Line > 0 {
		return fmt.Errorf("line %d, column %d: %s", node.Line, node.Column, message)
	}
	return fmt.Errorf("%s", message)
}
