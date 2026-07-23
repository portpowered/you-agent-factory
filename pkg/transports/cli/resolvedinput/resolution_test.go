package resolvedinput_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
)

func TestResolveUsesEveryPrecedencePositionAndExposesWinningState(t *testing.T) {
	precedence := allSources()
	for position, wantSource := range precedence {
		t.Run(string(wantSource), func(t *testing.T) {
			candidates := make([]resolvedinput.Candidate, 0, len(precedence)-position)
			for _, source := range precedence[position:] {
				candidates = append(candidates, resolvedinput.Candidate{
					InputID: "command.input", Source: source, Value: resolvedinput.StringValue(string(source)),
				})
			}

			inputs, err := resolvedinput.Resolve(
				[]resolvedinput.Definition{{ID: "command.input", Kind: resolvedinput.ValueKindString, Precedence: precedence}},
				candidates,
			)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			got, err := inputs.String("command.input")
			if err != nil || got != string(wantSource) {
				t.Fatalf("String() = %q, %v; want %q, nil", got, err, wantSource)
			}
			state, ok := inputs.State("command.input")
			wantDefault := position >= len(precedence)-2
			wantState := resolvedinput.State{Provenance: wantSource, Changed: !wantDefault, Default: wantDefault}
			if !ok || state != wantState {
				t.Fatalf("State() = %#v, %t; want %#v, true", state, ok, wantState)
			}
		})
	}
}

func TestResolveIsDeterministicAcrossDefinitionAndCandidateOrder(t *testing.T) {
	definitions := []resolvedinput.Definition{
		{ID: "input.alpha", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceEnvironment}},
		{ID: "input.beta", Kind: resolvedinput.ValueKindInt, Precedence: []resolvedinput.Source{resolvedinput.SourceOperatorConfig, resolvedinput.SourceManifestDefault}},
	}
	candidates := []resolvedinput.Candidate{
		{InputID: "input.alpha", Source: resolvedinput.SourceEnvironment, Value: resolvedinput.StringValue("environment")},
		{InputID: "input.beta", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.IntValue(2)},
		{InputID: "input.alpha", Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("cli")},
		{InputID: "input.beta", Source: resolvedinput.SourceOperatorConfig, Value: resolvedinput.IntValue(4)},
	}

	first, err := resolvedinput.Resolve(definitions, candidates)
	if err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
	second, err := resolvedinput.Resolve(reverse(definitions), reverse(candidates))
	if err != nil {
		t.Fatalf("second Resolve() error = %v", err)
	}
	assertEquivalentInput(t, first, second, "input.alpha", resolvedinput.ValueKindString)
	assertEquivalentInput(t, first, second, "input.beta", resolvedinput.ValueKindInt)
}

func TestResolveRejectsInvalidResolutionInputWithTypedDiagnostic(t *testing.T) {
	tests := []struct {
		name       string
		definition resolvedinput.Definition
		candidate  resolvedinput.Candidate
		failure    resolvedinput.ResolutionFailure
		source     resolvedinput.Source
	}{
		{
			name: "empty precedence", definition: resolvedinput.Definition{ID: "input", Kind: resolvedinput.ValueKindString},
			failure: resolvedinput.ResolutionFailureInvalidPrecedence,
		},
		{
			name:       "unsupported precedence source",
			definition: resolvedinput.Definition{ID: "input", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{"unknown"}},
			failure:    resolvedinput.ResolutionFailureInvalidPrecedence, source: "unknown",
		},
		{
			name:       "duplicate precedence source",
			definition: resolvedinput.Definition{ID: "input", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag, resolvedinput.SourceCLIFlag}},
			failure:    resolvedinput.ResolutionFailureInvalidPrecedence, source: resolvedinput.SourceCLIFlag,
		},
		{
			name:       "undeclared candidate source",
			definition: resolvedinput.Definition{ID: "input", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			candidate:  resolvedinput.Candidate{InputID: "input", Source: resolvedinput.SourceEnvironment, Value: resolvedinput.StringValue("value")},
			failure:    resolvedinput.ResolutionFailureUndeclaredSource, source: resolvedinput.SourceEnvironment,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidates := []resolvedinput.Candidate(nil)
			if test.candidate.InputID != "" {
				candidates = append(candidates, test.candidate)
			}
			_, err := resolvedinput.Resolve([]resolvedinput.Definition{test.definition}, candidates)
			var diagnostic *resolvedinput.ResolutionError
			if !errors.As(err, &diagnostic) {
				t.Fatalf("Resolve() error = %v; want *ResolutionError", err)
			}
			if diagnostic.Failure != test.failure || diagnostic.InputID != "input" || diagnostic.Source != test.source {
				t.Fatalf("diagnostic = %#v; want failure=%q input=input source=%q", diagnostic, test.failure, test.source)
			}
		})
	}
}

func allSources() []resolvedinput.Source {
	return []resolvedinput.Source{
		resolvedinput.SourceCLIFlag,
		resolvedinput.SourcePositionalArgument,
		resolvedinput.SourceEnvironment,
		resolvedinput.SourceOperatorConfig,
		resolvedinput.SourceStdin,
		resolvedinput.SourceManifestDefault,
		resolvedinput.SourceFactorySignatureDefault,
	}
}

func reverse[T any](values []T) []T {
	reversed := append([]T(nil), values...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}

func assertEquivalentInput(t *testing.T, first, second resolvedinput.Inputs, inputID string, kind resolvedinput.ValueKind) {
	t.Helper()
	firstValue, firstOK := first.Lookup(inputID)
	secondValue, secondOK := second.Lookup(inputID)
	firstState, firstStateOK := first.State(inputID)
	secondState, secondStateOK := second.State(inputID)
	if !firstOK || !secondOK || !firstStateOK || !secondStateOK || kind != firstValue.Kind() ||
		!reflect.DeepEqual(firstValue, secondValue) || firstState != secondState {
		t.Fatalf("resolved input %q differs: values=(%#v, %#v), states=(%#v, %#v)", inputID, firstValue, secondValue, firstState, secondState)
	}
}
