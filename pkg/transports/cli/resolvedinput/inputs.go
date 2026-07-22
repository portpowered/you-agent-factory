package resolvedinput

import "fmt"

// Definition is the schema-owned identity and value kind of one command input.
// Public spellings and parser details deliberately do not participate in
// resolution or lookup.
type Definition struct {
	ID         string
	Kind       ValueKind
	Precedence []Source
}

// Candidate is one already-collected value and its source provenance, addressed
// by stable schema input ID. Source collectors remain outside this pure model.
type Candidate struct {
	InputID string
	Source  Source
	Value   Value
}

// Inputs is a resolved CLI input snapshot keyed only by stable schema input ID.
type Inputs struct {
	entries map[string]entry
}

type entry struct {
	value Value
	state State
}

// Resolve validates schema definitions and resolves the first available source
// in each definition's precedence order. It performs no IO and does not mutate
// the supplied definitions or candidates.
func Resolve(definitions []Definition, candidates []Candidate) (Inputs, error) {
	definitionsByID := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if err := validateDefinition(definition, definitionsByID); err != nil {
			return Inputs{}, err
		}
		definitionsByID[definition.ID] = definition
	}

	byInput := make(map[string]map[Source]Value, len(candidates))
	for _, candidate := range candidates {
		definition, declared := definitionsByID[candidate.InputID]
		if !declared {
			return Inputs{}, newResolutionError(ResolutionFailureUndeclaredInput, candidate.InputID, candidate.Source, "candidate references an undeclared stable input ID")
		}
		if !containsSource(definition.Precedence, candidate.Source) {
			return Inputs{}, newResolutionError(ResolutionFailureUndeclaredSource, candidate.InputID, candidate.Source, "candidate source is absent from the input precedence")
		}
		if candidate.Value.Kind() != definition.Kind {
			return Inputs{}, newResolutionError(ResolutionFailureValueKind, candidate.InputID, candidate.Source,
				fmt.Sprintf("requires value kind %q, got %q", definition.Kind, candidate.Value.Kind()))
		}
		if byInput[candidate.InputID] == nil {
			byInput[candidate.InputID] = make(map[Source]Value)
		}
		if _, exists := byInput[candidate.InputID][candidate.Source]; exists {
			return Inputs{}, newResolutionError(ResolutionFailureDuplicateSource, candidate.InputID, candidate.Source, "multiple candidates use the same source")
		}
		byInput[candidate.InputID][candidate.Source] = candidate.Value
	}

	entries := make(map[string]entry, len(byInput))
	for _, definition := range definitions {
		for _, source := range definition.Precedence {
			value, found := byInput[definition.ID][source]
			if !found {
				continue
			}
			entries[definition.ID] = entry{value: value.clone(), state: stateFor(source)}
			break
		}
	}
	return Inputs{entries: entries}, nil
}

// Lookup returns a detached value by stable schema input ID.
func (i Inputs) Lookup(inputID string) (Value, bool) {
	resolved, ok := i.entries[inputID]
	return resolved.value.clone(), ok
}

// State reports the winning provenance and its explicit/default classification.
func (i Inputs) State(inputID string) (State, bool) {
	resolved, ok := i.entries[inputID]
	return resolved.state, ok
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
	resolved, ok := i.entries[inputID]
	if !ok {
		return Value{}, fmt.Errorf("resolved CLI input %q is missing", inputID)
	}
	value := resolved.value
	if value.Kind() != expected {
		return Value{}, fmt.Errorf("resolved CLI input %q requires accessor kind %q, got %q", inputID, expected, value.Kind())
	}
	return value, nil
}

func validateDefinition(definition Definition, existing map[string]Definition) error {
	if definition.ID == "" {
		return newResolutionError(ResolutionFailureInvalidDefinition, "", "", "definition has an empty stable ID")
	}
	if !definition.Kind.valid() {
		return newResolutionError(ResolutionFailureValueKind, definition.ID, "", fmt.Sprintf("has unsupported value kind %q", definition.Kind))
	}
	if _, exists := existing[definition.ID]; exists {
		return newResolutionError(ResolutionFailureInvalidDefinition, definition.ID, "", "definition repeats the stable input ID")
	}
	if len(definition.Precedence) == 0 {
		return newResolutionError(ResolutionFailureInvalidPrecedence, definition.ID, "", "precedence is empty")
	}
	seen := make(map[Source]bool, len(definition.Precedence))
	for _, source := range definition.Precedence {
		if !source.valid() {
			return newResolutionError(ResolutionFailureInvalidPrecedence, definition.ID, source, "precedence contains an unsupported source")
		}
		if seen[source] {
			return newResolutionError(ResolutionFailureInvalidPrecedence, definition.ID, source, "precedence repeats a source")
		}
		seen[source] = true
	}
	return nil
}

func containsSource(sources []Source, candidate Source) bool {
	for _, source := range sources {
		if source == candidate {
			return true
		}
	}
	return false
}

func (k ValueKind) valid() bool {
	switch k {
	case ValueKindBool, ValueKindString, ValueKindInt, ValueKindInt64, ValueKindStringArray:
		return true
	default:
		return false
	}
}
