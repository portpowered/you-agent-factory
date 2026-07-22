package resolvedinput_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
)

func TestResolveRetainsEveryCanonicalValueKindByStableInputID(t *testing.T) {
	definitions := []resolvedinput.Definition{
		{ID: "command.flag.enabled", Kind: resolvedinput.ValueKindBool},
		{ID: "command.flag.name", Kind: resolvedinput.ValueKindString},
		{ID: "command.flag.count", Kind: resolvedinput.ValueKindInt},
		{ID: "command.flag.limit", Kind: resolvedinput.ValueKindInt64},
		{ID: "command.flag.labels", Kind: resolvedinput.ValueKindStringArray},
		{ID: "command.flag.empty-labels", Kind: resolvedinput.ValueKindStringArray},
		{ID: "command.arg.0", Kind: resolvedinput.ValueKindStringArray},
	}
	candidates := []resolvedinput.Candidate{
		{InputID: "command.flag.enabled", Value: resolvedinput.BoolValue(true)},
		{InputID: "command.flag.name", Value: resolvedinput.StringValue("factory")},
		{InputID: "command.flag.count", Value: resolvedinput.IntValue(3)},
		{InputID: "command.flag.limit", Value: resolvedinput.Int64Value(1 << 40)},
		{InputID: "command.flag.labels", Value: resolvedinput.StringArrayValue([]string{"one", "two"})},
		{InputID: "command.flag.empty-labels", Value: resolvedinput.StringArrayValue([]string{})},
		{InputID: "command.arg.0", Value: resolvedinput.StringArrayValue([]string{"first", "second"})},
	}

	inputs, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	gotBool, err := inputs.Bool("command.flag.enabled")
	assertScalar(t, gotBool, err, true)
	gotString, err := inputs.String("command.flag.name")
	assertScalar(t, gotString, err, "factory")
	gotInt, err := inputs.Int("command.flag.count")
	assertScalar(t, gotInt, err, 3)
	gotInt64, err := inputs.Int64("command.flag.limit")
	assertScalar(t, gotInt64, err, int64(1<<40))
	assertStrings(t, inputs, "command.flag.labels", []string{"one", "two"})
	assertStrings(t, inputs, "command.flag.empty-labels", []string{})
	assertStrings(t, inputs, "command.arg.0", []string{"first", "second"})
}

func TestResolveUsesOnlyStableIDsForSyntheticInputs(t *testing.T) {
	const syntheticID = "synthetic.input.customer-defined"
	inputs, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{ID: syntheticID, Kind: resolvedinput.ValueKindString}},
		[]resolvedinput.Candidate{{InputID: syntheticID, Value: resolvedinput.StringValue("resolved")}},
	)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	got, err := inputs.String(syntheticID)
	if err != nil || got != "resolved" {
		t.Fatalf("String(%q) = %q, %v; want resolved, nil", syntheticID, got, err)
	}
	value, ok := inputs.Lookup(syntheticID)
	if !ok || value.Kind() != resolvedinput.ValueKindString {
		t.Fatalf("Lookup(%q) = %#v, %t; want string value, true", syntheticID, value, ok)
	}
}

func TestResolveRejectsSchemaAndCandidateKindMismatch(t *testing.T) {
	_, err := resolvedinput.Resolve(
		[]resolvedinput.Definition{{ID: "command.flag.count", Kind: resolvedinput.ValueKindInt}},
		[]resolvedinput.Candidate{{InputID: "command.flag.count", Value: resolvedinput.StringValue("3")}},
	)
	if err == nil {
		t.Fatal("Resolve() error = nil, want value-kind rejection")
	}
}

func assertScalar[T comparable](t *testing.T, got T, err error, want T) {
	t.Helper()
	if err != nil || got != want {
		t.Fatalf("accessor = %#v, %v; want %#v, nil", got, err, want)
	}
}

func assertStrings(t *testing.T, inputs resolvedinput.Inputs, inputID string, want []string) {
	t.Helper()
	got, err := inputs.StringArray(inputID)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("StringArray(%q) = %#v, %v; want %#v, nil", inputID, got, err, want)
	}
}
