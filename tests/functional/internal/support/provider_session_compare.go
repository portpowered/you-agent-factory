package support

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProviderSessionNormalizedFieldPlaceholder replaces values of manifest-declared
// normalized fields before structural comparison.
const ProviderSessionNormalizedFieldPlaceholder = "<normalized>"

// ProviderSessionObservedGoldens holds observed public metadata for comparison.
// Callers supply these values from the system under test; comparison never
// invokes mappers or adapters to synthesize expected output.
type ProviderSessionObservedGoldens struct {
	ProviderSession  json.RawMessage
	ResponseEvents   []json.RawMessage
	InvocationResult json.RawMessage
}

// ProviderSessionCompareDifference describes one structural mismatch after
// normalizing only manifest.normalizedFields.
type ProviderSessionCompareDifference struct {
	Path     string
	Expected string
	Actual   string
}

// ProviderSessionCompareError reports golden drift for a case and artifact role.
type ProviderSessionCompareError struct {
	CaseID      string
	Role        string
	Differences []ProviderSessionCompareDifference
}

func (e *ProviderSessionCompareError) Error() string {
	caseID := e.CaseID
	if caseID == "" {
		caseID = "(unknown)"
	}
	role := e.Role
	if role == "" {
		role = "(unknown)"
	}
	if len(e.Differences) == 0 {
		return fmt.Sprintf("provider-session golden compare case %q role %q: mismatch", caseID, role)
	}
	first := e.Differences[0]
	return fmt.Sprintf(
		"provider-session golden compare case %q role %q path %q: expected %s, actual %s",
		caseID,
		role,
		first.Path,
		first.Expected,
		first.Actual,
	)
}

// CompareProviderSessionGoldens normalizes only manifest.normalizedFields on
// both sides, then structurally compares Provider Session, response-event, and
// invocation-result goldens. Whitespace-only differences do not fail.
func CompareProviderSessionGoldens(
	manifest ProviderSessionGoldenManifest,
	expected ProviderSessionExpectedGoldens,
	observed ProviderSessionObservedGoldens,
) error {
	caseID := strings.TrimSpace(manifest.ID)
	normalizedFields := append([]string(nil), manifest.NormalizedFields...)

	if err := CompareProviderSessionJSON(
		caseID,
		"expected-provider-session",
		normalizedFields,
		expected.ProviderSession,
		observed.ProviderSession,
	); err != nil {
		return err
	}
	if err := CompareProviderSessionNDJSON(
		caseID,
		"expected-response-events",
		normalizedFields,
		expected.ResponseEvents,
		observed.ResponseEvents,
	); err != nil {
		return err
	}
	if err := CompareProviderSessionJSON(
		caseID,
		"expected-invocation-result",
		normalizedFields,
		expected.InvocationResult,
		observed.InvocationResult,
	); err != nil {
		return err
	}
	return nil
}

// CompareProviderSessionJSON structurally compares two JSON documents after
// normalizing only the declared field names.
func CompareProviderSessionJSON(
	caseID, role string,
	normalizedFields []string,
	expected, observed json.RawMessage,
) error {
	expectedValue, err := decodeProviderSessionJSONValue(caseID, role, "expected", expected)
	if err != nil {
		return err
	}
	observedValue, err := decodeProviderSessionJSONValue(caseID, role, "observed", observed)
	if err != nil {
		return err
	}

	expectedNormalized := normalizeProviderSessionValue(expectedValue, normalizedFieldSet(normalizedFields))
	observedNormalized := normalizeProviderSessionValue(observedValue, normalizedFieldSet(normalizedFields))

	var differences []ProviderSessionCompareDifference
	compareProviderSessionJSONValue("", expectedNormalized, observedNormalized, &differences)
	if len(differences) == 0 {
		return nil
	}
	return &ProviderSessionCompareError{
		CaseID:      caseID,
		Role:        role,
		Differences: differences,
	}
}

// CompareProviderSessionNDJSON structurally compares NDJSON record sequences
// after normalizing only the declared field names.
func CompareProviderSessionNDJSON(
	caseID, role string,
	normalizedFields []string,
	expected, observed []json.RawMessage,
) error {
	expectedValues := make([]any, 0, len(expected))
	for i, record := range expected {
		value, err := decodeProviderSessionJSONValue(caseID, role, fmt.Sprintf("expected[%d]", i), record)
		if err != nil {
			return err
		}
		expectedValues = append(expectedValues, value)
	}
	observedValues := make([]any, 0, len(observed))
	for i, record := range observed {
		value, err := decodeProviderSessionJSONValue(caseID, role, fmt.Sprintf("observed[%d]", i), record)
		if err != nil {
			return err
		}
		observedValues = append(observedValues, value)
	}

	fields := normalizedFieldSet(normalizedFields)
	expectedNormalized := normalizeProviderSessionValue(expectedValues, fields)
	observedNormalized := normalizeProviderSessionValue(observedValues, fields)

	var differences []ProviderSessionCompareDifference
	compareProviderSessionJSONValue("", expectedNormalized, observedNormalized, &differences)
	if len(differences) == 0 {
		return nil
	}
	return &ProviderSessionCompareError{
		CaseID:      caseID,
		Role:        role,
		Differences: differences,
	}
}

func decodeProviderSessionJSONValue(caseID, role, side string, raw json.RawMessage) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, &ProviderSessionCompareError{
			CaseID: caseID,
			Role:   role,
			Differences: []ProviderSessionCompareDifference{{
				Path:     side,
				Expected: `""`,
				Actual:   `"<empty>"`,
			}},
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, &ProviderSessionCompareError{
			CaseID: caseID,
			Role:   role,
			Differences: []ProviderSessionCompareDifference{{
				Path:     side,
				Expected: `"valid JSON"`,
				Actual:   fmt.Sprintf("%q", err.Error()),
			}},
		}
	}
	return value, nil
}

func normalizedFieldSet(fields []string) map[string]struct{} {
	set := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		name := strings.TrimSpace(field)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	return set
}

func normalizeProviderSessionValue(value any, fields map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if _, ok := fields[key]; ok {
				out[key] = ProviderSessionNormalizedFieldPlaceholder
				continue
			}
			out[key] = normalizeProviderSessionValue(child, fields)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = normalizeProviderSessionValue(child, fields)
		}
		return out
	default:
		return value
	}
}

func compareProviderSessionJSONValue(path string, expected, actual any, differences *[]ProviderSessionCompareDifference) {
	switch expectedValue := expected.(type) {
	case map[string]any:
		actualValue, ok := actual.(map[string]any)
		if !ok {
			appendProviderSessionCompareDifference(path, expected, actual, differences)
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
			compareProviderSessionJSONValue(providerSessionFieldPath(path, key), expectedValue[key], actualValue[key], differences)
		}
	case []any:
		actualValue, ok := actual.([]any)
		if !ok {
			appendProviderSessionCompareDifference(path, expected, actual, differences)
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
			compareProviderSessionJSONValue(fmt.Sprintf("%s[%d]", path, index), expectedItem, actualItem, differences)
		}
	default:
		if !providerSessionJSONScalarsEqual(expected, actual) {
			appendProviderSessionCompareDifference(path, expected, actual, differences)
		}
	}
}

func appendProviderSessionCompareDifference(path string, expected, actual any, differences *[]ProviderSessionCompareDifference) {
	*differences = append(*differences, ProviderSessionCompareDifference{
		Path:     path,
		Expected: marshalProviderSessionCompareValue(expected),
		Actual:   marshalProviderSessionCompareValue(actual),
	})
}

func marshalProviderSessionCompareValue(value any) string {
	if value == nil {
		return "null"
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(encoded)
}

func providerSessionJSONScalarsEqual(expected, actual any) bool {
	return marshalProviderSessionCompareValue(expected) == marshalProviderSessionCompareValue(actual)
}

func providerSessionFieldPath(path, field string) string {
	if path == "" {
		return field
	}
	return path + "." + field
}
