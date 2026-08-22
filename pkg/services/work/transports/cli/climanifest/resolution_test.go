package climanifest

import (
	"reflect"
	"testing"
)

func TestResolveInputValueUsesHighestSourceAndReportsProvenance(t *testing.T) {
	stdin := "stdin"
	environment := "environment"
	manifestDefault := "default"
	result, found, err := ResolveInputValue(CanonicalPrecedence(),
		[]string{SourceStdin, SourceEnvironment, SourceManifestDefault}, false,
		[]ResolutionCandidate{
			{Source: SourceManifestDefault, BindingID: "input.default", Value: InputValue{String: &manifestDefault}},
			{Source: SourceEnvironment, BindingID: "input.env", Value: InputValue{String: &environment}},
			{Source: SourceStdin, BindingID: "input.stdin", Value: InputValue{String: &stdin}},
		})
	if err != nil || !found {
		t.Fatalf("ResolveInputValue() found=%v err=%v", found, err)
	}
	if result.Source != SourceStdin || result.BindingID != "input.stdin" || result.Value.String == nil || *result.Value.String != stdin {
		t.Fatalf("result = %#v, want stdin value and provenance", result)
	}
}

func TestResolveInputValueAppendsRepeatedValuesWithinWinningTier(t *testing.T) {
	first := []string{"one"}
	second := []string{"two", "three"}
	lower := []string{"ignored"}
	result, found, err := ResolveInputValue(CanonicalPrecedence(),
		[]string{SourceCLI, SourceEnvironment}, true,
		[]ResolutionCandidate{
			{Source: SourceEnvironment, BindingID: "input.env", Value: InputValue{StringArray: &lower}},
			{Source: SourceCLI, BindingID: "input.cli", Value: InputValue{StringArray: &first}},
			{Source: SourceCLI, BindingID: "input.cli", Value: InputValue{StringArray: &second}},
		})
	if err != nil || !found {
		t.Fatalf("ResolveInputValue() found=%v err=%v", found, err)
	}
	if result.Value.StringArray == nil || !reflect.DeepEqual(*result.Value.StringArray, []string{"one", "two", "three"}) {
		t.Fatalf("result = %#v, want appended CLI values", result)
	}
}

func TestResolveInputValueRejectsAmbiguousOrUndeclaredSources(t *testing.T) {
	value := "value"
	tests := []struct {
		name       string
		accepted   []string
		candidates []ResolutionCandidate
	}{
		{
			name:     "undeclared source",
			accepted: []string{SourceCLI},
			candidates: []ResolutionCandidate{
				{Source: SourceEnvironment, BindingID: "input.env", Value: InputValue{String: &value}},
			},
		},
		{
			name:     "multiple bindings in one tier",
			accepted: []string{SourceEnvironment},
			candidates: []ResolutionCandidate{
				{Source: SourceEnvironment, BindingID: "input.env.first", Value: InputValue{String: &value}},
				{Source: SourceEnvironment, BindingID: "input.env.second", Value: InputValue{String: &value}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := ResolveInputValue(CanonicalPrecedence(), test.accepted, false, test.candidates); err == nil {
				t.Fatal("ResolveInputValue() error = nil, want rejection")
			}
		})
	}
}
