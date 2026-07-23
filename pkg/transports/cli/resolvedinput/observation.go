package resolvedinput

import "sort"

// RedactedValue is the stable replacement used wherever sensitive resolved
// values cross a diagnostic or observation boundary.
const RedactedValue = "<redacted>"

// Observation is a detached, serialization-safe view of one resolved input.
// Value retains its canonical Go type for non-sensitive inputs and is always
// RedactedValue for sensitive inputs, including collections.
type Observation struct {
	InputID    string    `json:"inputId"`
	Kind       ValueKind `json:"kind"`
	Provenance Source    `json:"provenance"`
	Changed    bool      `json:"changed"`
	Default    bool      `json:"default"`
	Value      any       `json:"value"`
}

// Observe returns a detached observation by stable schema input ID.
func (i Inputs) Observe(inputID string) (Observation, bool) {
	resolved, ok := i.entries[inputID]
	if !ok {
		return Observation{}, false
	}
	return observation(inputID, resolved), true
}

// Observations returns detached observations ordered by stable input ID.
func (i Inputs) Observations() []Observation {
	inputIDs := make([]string, 0, len(i.entries))
	for inputID := range i.entries {
		inputIDs = append(inputIDs, inputID)
	}
	sort.Strings(inputIDs)

	observations := make([]Observation, 0, len(inputIDs))
	for _, inputID := range inputIDs {
		observations = append(observations, observation(inputID, i.entries[inputID]))
	}
	return observations
}

func observation(inputID string, resolved entry) Observation {
	return Observation{
		InputID:    inputID,
		Kind:       resolved.value.Kind(),
		Provenance: resolved.state.Provenance,
		Changed:    resolved.state.Changed,
		Default:    resolved.state.Default,
		Value:      observableValue(resolved.value, resolved.sensitive),
	}
}

func observableValue(value Value, sensitive bool) any {
	if sensitive {
		return RedactedValue
	}
	switch value.Kind() {
	case ValueKindBool:
		return value.boolValue
	case ValueKindString:
		return value.stringValue
	case ValueKindInt:
		return value.intValue
	case ValueKindInt64:
		return value.int64Value
	case ValueKindStringArray:
		return cloneStrings(value.strings)
	default:
		return nil
	}
}
