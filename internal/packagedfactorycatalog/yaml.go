package packagedfactorycatalog

import (
	"encoding/json"
	"fmt"
)

func yamlToJSON(document any) ([]byte, error) {
	normalized, err := normalizeYAMLValue(document)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode JSON: %w", err)
	}
	return payload, nil
}

func normalizeYAMLValue(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			normalized, err := normalizeYAMLValue(child)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case map[any]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			stringKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("mapping key %v is not a string", key)
			}
			normalized, err := normalizeYAMLValue(child)
			if err != nil {
				return nil, err
			}
			result[stringKey] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			normalized, err := normalizeYAMLValue(child)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	default:
		return value, nil
	}
}
