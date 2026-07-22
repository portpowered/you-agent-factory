package resolvedinput_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
)

func TestTypedAccessorsClassifyMissingInputs(t *testing.T) {
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{definition("input.unresolved", resolvedinput.ValueKindString)},
		nil,
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	tests := []struct {
		name     string
		inputID  string
		expected resolvedinput.ValueKind
		access   func() error
	}{
		{name: "bool", inputID: "input.unknown", expected: resolvedinput.ValueKindBool, access: func() error { _, err := inputs.Bool("input.unknown"); return err }},
		{name: "string", inputID: "input.unresolved", expected: resolvedinput.ValueKindString, access: func() error { _, err := inputs.String("input.unresolved"); return err }},
		{name: "int", inputID: "input.unknown", expected: resolvedinput.ValueKindInt, access: func() error { _, err := inputs.Int("input.unknown"); return err }},
		{name: "int64", inputID: "input.unknown", expected: resolvedinput.ValueKindInt64, access: func() error { _, err := inputs.Int64("input.unknown"); return err }},
		{name: "string array", inputID: "input.unknown", expected: resolvedinput.ValueKindStringArray, access: func() error { _, err := inputs.StringArray("input.unknown"); return err }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.access()
			assertAccessError(t, err, resolvedinput.AccessError{
				Failure:      resolvedinput.AccessFailureMissingInput,
				InputID:      test.inputID,
				ExpectedKind: test.expected,
			})
		})
	}
}

func TestTypedAccessorsClassifyScalarAndCollectionKindMismatches(t *testing.T) {
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{
			definition("input.scalar", resolvedinput.ValueKindBool),
			definition("input.collection", resolvedinput.ValueKindStringArray),
		},
		[]resolvedinput.Candidate{
			candidate("input.scalar", resolvedinput.BoolValue(true)),
			candidate("input.collection", resolvedinput.StringArrayValue([]string{"value"})),
		},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	_, scalarErr := inputs.String("input.scalar")
	assertAccessError(t, scalarErr, resolvedinput.AccessError{
		Failure:      resolvedinput.AccessFailureKindMismatch,
		InputID:      "input.scalar",
		ExpectedKind: resolvedinput.ValueKindString,
		ActualKind:   resolvedinput.ValueKindBool,
	})

	_, collectionErr := inputs.Int64("input.collection")
	assertAccessError(t, collectionErr, resolvedinput.AccessError{
		Failure:      resolvedinput.AccessFailureKindMismatch,
		InputID:      "input.collection",
		ExpectedKind: resolvedinput.ValueKindInt64,
		ActualKind:   resolvedinput.ValueKindStringArray,
	})
}

func assertAccessError(t *testing.T, err error, want resolvedinput.AccessError) {
	t.Helper()
	wrapped := fmt.Errorf("translate resolved input: %w", err)
	var diagnostic *resolvedinput.AccessError
	if !errors.As(wrapped, &diagnostic) {
		t.Fatalf("wrapped error = %v; want *AccessError", wrapped)
	}
	if *diagnostic != want {
		t.Fatalf("diagnostic = %#v; want %#v", diagnostic, want)
	}
	if !strings.Contains(diagnostic.Error(), want.InputID) {
		t.Fatalf("Error() = %q; want stable input ID %q", diagnostic.Error(), want.InputID)
	}
}
