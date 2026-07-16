package sessionparity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Difference describes one customer-visible Factory Session fact that differs
// between normalized projections. Expected and Actual are compact JSON values
// so a failure report is independent of interface-specific formatting.
type Difference struct {
	Path     string `json:"path"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Compare returns every semantic difference between normalized Factory Session
// projections in deterministic field-path order. Collection indexes are part of
// the path because dispatch, artifact, result, failure, and event cursor order
// is customer-visible.
func Compare(expected, actual Projection) []Difference {
	expectedValue := projectionJSONValue(expected)
	actualValue := projectionJSONValue(actual)
	var differences []Difference
	compareJSONValue("", expectedValue, actualValue, &differences)
	return differences
}

func projectionJSONValue(projection Projection) any {
	encoded, err := json.Marshal(projection)
	if err != nil {
		panic(fmt.Sprintf("marshal parity projection: %v", err))
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		panic(fmt.Sprintf("unmarshal parity projection: %v", err))
	}
	return value
}

func compareJSONValue(path string, expected, actual any, differences *[]Difference) {
	switch expectedValue := expected.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok {
			appendDifference(path, expected, actual, differences)
			return
		}
		keys := make(map[string]struct{}, len(expectedValue)+len(actualValue))
		for key := range expectedValue {
			keys[key] = struct{}{}
		}
		for key := range actualValue {
			keys[key] = struct{}{}
		}
		sortedKeys := make([]string, 0, len(keys))
		for key := range keys {
			sortedKeys = append(sortedKeys, key)
		}
		sort.Strings(sortedKeys)
		for _, key := range sortedKeys {
			compareJSONValue(fieldPath(path, key), expectedValue[key], actualValue[key], differences)
		}
	case []any:
		actualValue, ok := actual.([]any)
		if !ok {
			appendDifference(path, expected, actual, differences)
			return
		}
		length := len(expectedValue)
		if len(actualValue) > length {
			length = len(actualValue)
		}
		for index := 0; index < length; index++ {
			var expectedItem, actualItem any
			if index < len(expectedValue) {
				expectedItem = expectedValue[index]
			}
			if index < len(actualValue) {
				actualItem = actualValue[index]
			}
			compareJSONValue(fmt.Sprintf("%s[%d]", path, index), expectedItem, actualItem, differences)
		}
	default:
		if !equalJSONScalar(expected, actual) {
			appendDifference(path, expected, actual, differences)
		}
	}
}

func appendDifference(path string, expected, actual any, differences *[]Difference) {
	*differences = append(*differences, Difference{
		Path:     path,
		Expected: normalizedJSON(expected),
		Actual:   normalizedJSON(actual),
	})
}

func normalizedJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal parity difference value: %v", err))
	}
	return string(encoded)
}

func equalJSONScalar(expected, actual any) bool {
	return normalizedJSON(expected) == normalizedJSON(actual)
}

func fieldPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}
