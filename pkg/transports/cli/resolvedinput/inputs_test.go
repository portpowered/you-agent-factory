package resolvedinput_test

import (
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
)

func TestResolveRetainsEveryCanonicalValueKindByStableInputID(t *testing.T) {
	definitions := []resolvedinput.Definition{
		definition("command.flag.enabled", resolvedinput.ValueKindBool),
		definition("command.flag.name", resolvedinput.ValueKindString),
		definition("command.flag.count", resolvedinput.ValueKindInt),
		definition("command.flag.limit", resolvedinput.ValueKindInt64),
		definition("command.flag.labels", resolvedinput.ValueKindStringArray),
		definition("command.flag.empty-labels", resolvedinput.ValueKindStringArray),
		definition("command.arg.0", resolvedinput.ValueKindStringArray),
	}
	candidates := []resolvedinput.Candidate{
		candidate("command.flag.enabled", resolvedinput.BoolValue(true)),
		candidate("command.flag.name", resolvedinput.StringValue("factory")),
		candidate("command.flag.count", resolvedinput.IntValue(3)),
		candidate("command.flag.limit", resolvedinput.Int64Value(1<<40)),
		candidate("command.flag.labels", resolvedinput.StringArrayValue([]string{"one", "two"})),
		candidate("command.flag.empty-labels", resolvedinput.StringArrayValue([]string{})),
		candidate("command.arg.0", resolvedinput.StringArrayValue([]string{"first", "second"})),
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
		[]resolvedinput.Definition{definition(syntheticID, resolvedinput.ValueKindString)},
		[]resolvedinput.Candidate{candidate(syntheticID, resolvedinput.StringValue("resolved"))},
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
		[]resolvedinput.Definition{definition("command.flag.count", resolvedinput.ValueKindInt)},
		[]resolvedinput.Candidate{candidate("command.flag.count", resolvedinput.StringValue("3"))},
	)
	if err == nil {
		t.Fatal("Resolve() error = nil, want value-kind rejection")
	}
}

func definition(id string, kind resolvedinput.ValueKind) resolvedinput.Definition {
	return resolvedinput.Definition{ID: id, Kind: kind, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}}
}

func candidate(id string, value resolvedinput.Value) resolvedinput.Candidate {
	return resolvedinput.Candidate{InputID: id, Source: resolvedinput.SourceCLIFlag, Value: value}
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
