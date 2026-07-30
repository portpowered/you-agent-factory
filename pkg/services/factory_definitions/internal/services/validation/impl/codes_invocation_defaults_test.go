package impl

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

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
