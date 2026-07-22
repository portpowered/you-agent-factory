package resolvedinput

import "fmt"

// Definition is the schema-owned identity and value kind of one command input.
// Public spellings and parser details deliberately do not participate in
// resolution or lookup.
type Definition struct {
	ID   string
	Kind ValueKind
}

// Candidate is one already-collected value addressed by stable schema input ID.
// Source collectors remain outside this pure transport model.
type Candidate struct {
	InputID string
	Value   Value
}

// Inputs is a resolved CLI input snapshot keyed only by stable schema input ID.
type Inputs struct {
	values map[string]Value
}

// Resolve validates schema definitions and retains at most one candidate for
// each declared input. Source precedence is added separately when candidates
// can contain more than one source observation.
func Resolve(definitions []Definition, candidates []Candidate) (Inputs, error) {
	kinds := make(map[string]ValueKind, len(definitions))
	for _, definition := range definitions {
		if definition.ID == "" {
			return Inputs{}, fmt.Errorf("resolved CLI input definition has an empty stable ID")
		}
		if !definition.Kind.valid() {
			return Inputs{}, fmt.Errorf("resolved CLI input %q has unsupported value kind %q", definition.ID, definition.Kind)
		}
		if _, exists := kinds[definition.ID]; exists {
			return Inputs{}, fmt.Errorf("resolved CLI input definition repeats stable ID %q", definition.ID)
		}
		kinds[definition.ID] = definition.Kind
	}

	values := make(map[string]Value, len(candidates))
	for _, candidate := range candidates {
		kind, declared := kinds[candidate.InputID]
		if !declared {
			return Inputs{}, fmt.Errorf("resolved CLI input candidate references undeclared stable ID %q", candidate.InputID)
		}
		if _, exists := values[candidate.InputID]; exists {
			return Inputs{}, fmt.Errorf("resolved CLI input %q has multiple candidates without a source policy", candidate.InputID)
		}
		if candidate.Value.Kind() != kind {
			return Inputs{}, fmt.Errorf(
				"resolved CLI input %q requires value kind %q, got %q",
				candidate.InputID,
				kind,
				candidate.Value.Kind(),
			)
		}
		values[candidate.InputID] = candidate.Value.clone()
	}
	return Inputs{values: values}, nil
}

// Lookup returns a detached value by stable schema input ID.
func (i Inputs) Lookup(inputID string) (Value, bool) {
	value, ok := i.values[inputID]
	return value.clone(), ok
}

func (i Inputs) Bool(inputID string) (bool, error) {
	value, err := i.valueOfKind(inputID, ValueKindBool)
	return value.boolValue, err
}

func (i Inputs) String(inputID string) (string, error) {
	value, err := i.valueOfKind(inputID, ValueKindString)
	return value.stringValue, err
}

func (i Inputs) Int(inputID string) (int, error) {
	value, err := i.valueOfKind(inputID, ValueKindInt)
	return value.intValue, err
}

func (i Inputs) Int64(inputID string) (int64, error) {
	value, err := i.valueOfKind(inputID, ValueKindInt64)
	return value.int64Value, err
}

// StringArray returns a detached string collection. Positional collections and
// repeated string flags share this canonical representation.
func (i Inputs) StringArray(inputID string) ([]string, error) {
	value, err := i.valueOfKind(inputID, ValueKindStringArray)
	return cloneStrings(value.strings), err
}

func (i Inputs) valueOfKind(inputID string, expected ValueKind) (Value, error) {
	value, ok := i.values[inputID]
	if !ok {
		return Value{}, fmt.Errorf("resolved CLI input %q is missing", inputID)
	}
	if value.Kind() != expected {
		return Value{}, fmt.Errorf("resolved CLI input %q requires accessor kind %q, got %q", inputID, expected, value.Kind())
	}
	return value, nil
}

func (k ValueKind) valid() bool {
	switch k {
	case ValueKindBool, ValueKindString, ValueKindInt, ValueKindInt64, ValueKindStringArray:
		return true
	default:
		return false
	}
}
