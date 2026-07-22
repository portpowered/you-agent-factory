package resolvedinput

import "fmt"

// AccessFailure classifies a failed typed lookup from a resolved input snapshot.
type AccessFailure string

const (
	AccessFailureMissingInput AccessFailure = "missing-input"
	AccessFailureKindMismatch AccessFailure = "kind-mismatch"
)

// AccessError is a structured diagnostic for a failed typed lookup.
type AccessError struct {
	Failure      AccessFailure
	InputID      string
	ExpectedKind ValueKind
	ActualKind   ValueKind
	Provenance   Source
	Changed      bool
	Default      bool
	Value        any
}

func (e *AccessError) Error() string {
	if e.Failure == AccessFailureKindMismatch {
		return fmt.Sprintf(
			"resolved CLI input %q requires accessor kind %q, got %q from %q (changed=%t, default=%t, value=%v; %s)",
			e.InputID,
			e.ExpectedKind,
			e.ActualKind,
			e.Provenance,
			e.Changed,
			e.Default,
			e.Value,
			e.Failure,
		)
	}
	return fmt.Sprintf(
		"resolved CLI input %q is missing for accessor kind %q (%s)",
		e.InputID,
		e.ExpectedKind,
		e.Failure,
	)
}

func newMissingInputError(inputID string, expected ValueKind) error {
	return &AccessError{
		Failure:      AccessFailureMissingInput,
		InputID:      inputID,
		ExpectedKind: expected,
	}
}

func newKindMismatchError(inputID string, expected ValueKind, resolved entry) error {
	return &AccessError{
		Failure:      AccessFailureKindMismatch,
		InputID:      inputID,
		ExpectedKind: expected,
		ActualKind:   resolved.value.Kind(),
		Provenance:   resolved.state.Provenance,
		Changed:      resolved.state.Changed,
		Default:      resolved.state.Default,
		Value:        observableValue(resolved.value, resolved.sensitive),
	}
}
