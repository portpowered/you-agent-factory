package impl

import (
	"reflect"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestInvocationSignatureReportsExactDeclarationConflicts(t *testing.T) {
	t.Parallel()
	named := []factorydefinitions.InvocationParameterBindingConfig{{Kind: factorydefinitions.InvocationParameterBindingKindNamed}}
	positional := []factorydefinitions.InvocationParameterBindingConfig{{Kind: factorydefinitions.InvocationParameterBindingKindPositional, Position: 1}}
	cases := []struct {
		name       string
		parameters []factorydefinitions.InvocationParameterConfig
		want       map[string]int
	}{
		{"valid named and positional", []factorydefinitions.InvocationParameterConfig{{Name: "input", Bindings: positional}, {Name: "model", ExternalName: "model", Aliases: []string{"m"}, Bindings: named}}, map[string]int{}},
		{"duplicate name and key", []factorydefinitions.InvocationParameterConfig{{Name: "model", ExternalName: "model", Bindings: named}, {Name: "model", ExternalName: "model", Bindings: named}}, map[string]int{CodeInvocationSignatureDuplicateParameterName: 1, CodeInvocationSignatureDuplicateNamedKey: 1}},
		{"sensitive positional", []factorydefinitions.InvocationParameterConfig{{Name: "secret", Sensitive: true, Bindings: positional}}, map[string]int{CodeInvocationSignatureSensitivePositional: 1}},
		{"unsupported modes", []factorydefinitions.InvocationParameterConfig{{Name: "model", TypeHint: "unsupported", ValueMode: "unsupported", Bindings: named}}, map[string]int{CodeInvocationSignatureUnsupportedTypeHint: 1, CodeInvocationSignatureUnsupportedValueMode: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			targets := InvocationSignatureTargets(&factorydefinitions.FactoryConfig{InvocationSignature: &factorydefinitions.InvocationSignatureConfig{Parameters: tc.parameters}})
			got := map[string]int{}
			for _, target := range targets {
				got[target.Code]++
				if target.Severity != SeverityError || target.Path == "" || target.Message == "" {
					t.Fatalf("diagnostic = %+v", target)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("codes = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInvocationSignatureParameterDefaultTargetsSingleValueDefaultValuesCardinality(t *testing.T) {
	t.Run("one empty fallback is valid", func(t *testing.T) {
		targets := invocationSignatureParameterDefaultTargets(factorydefinitions.InvocationParameterConfig{
			Name: "model", DefaultValues: []string{""},
		}, 0)
		if len(targets) != 0 {
			t.Fatalf("targets = %#v, want valid single empty defaultValues entry", targets)
		}
	})

	t.Run("multiple scalar fallbacks are invalid", func(t *testing.T) {
		targets := invocationSignatureParameterDefaultTargets(factorydefinitions.InvocationParameterConfig{
			Name: "model", DefaultValues: []string{"one", "two"},
		}, 0)
		if len(targets) != 1 || targets[0].Code != CodeInvocationSignatureInvalidDefaultShape {
			t.Fatalf("targets = %#v, want invalid default shape", targets)
		}
	})
}
