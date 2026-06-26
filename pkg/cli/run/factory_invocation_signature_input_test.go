package run

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestResolveSignatureFactoryInvocationInput_NormalizesPositionalNamedBooleanAndStdin(t *testing.T) {
	stdinText := "from stdin"
	got, err := ResolveSignatureFactoryInvocationInput(SignatureFactoryInvocationInputConfig{
		PromptArgs: []string{"draft", "--mode", "fast", "--confirm", "--out=result.md", "-"},
		Signature:  signatureFactoryInvocationConfig(),
		Stdin:      strings.NewReader(stdinText),
		StdinIsTTY: func() bool { return true },
	})
	if err != nil {
		t.Fatalf("ResolveSignatureFactoryInvocationInput: %v", err)
	}

	if values := got.Arguments["input"].Values; len(values) != 1 || values[0] != "draft" {
		t.Fatalf("input values = %#v, want [draft]", values)
	}
	if values := got.Arguments["mode"].Values; len(values) != 1 || values[0] != "fast" {
		t.Fatalf("mode values = %#v, want [fast]", values)
	}
	if values := got.Arguments["confirm"].Values; len(values) != 1 || values[0] != "true" {
		t.Fatalf("confirm values = %#v, want [true]", values)
	}
	if values := got.Arguments["stdinText"].Values; len(values) != 1 || values[0] != stdinText {
		t.Fatalf("stdinText values = %#v, want [%q]", values, stdinText)
	}
	if values := got.Arguments["output"].Values; len(values) != 1 || values[0] != "result.md" {
		t.Fatalf("output values = %#v, want [result.md]", values)
	}
}

func TestResolveSignatureFactoryInvocationInput_RejectsMissingNamedValue(t *testing.T) {
	_, err := ResolveSignatureFactoryInvocationInput(SignatureFactoryInvocationInputConfig{
		PromptArgs: []string{"draft", "--mode"},
		Signature:  signatureFactoryInvocationConfig(),
		StdinIsTTY: func() bool { return true },
	})
	if err == nil {
		t.Fatal("expected missing named value error")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("error = %q, want missing value message", err.Error())
	}
}

func signatureFactoryInvocationConfig() *interfaces.InvocationSignatureConfig {
	return &interfaces.InvocationSignatureConfig{
		Parameters: []interfaces.InvocationParameterConfig{
			{
				Name: "input",
				Bindings: []interfaces.InvocationParameterBindingConfig{
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1},
				},
			},
			{
				Name:         "mode",
				ExternalName: "mode",
				Bindings: []interfaces.InvocationParameterBindingConfig{
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
				},
			},
			{
				Name:     "confirm",
				TypeHint: string(factoryapi.FactoryInvocationParameterTypeHintBooleanString),
				Bindings: []interfaces.InvocationParameterBindingConfig{
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
				},
			},
			{
				Name: "stdinText",
				Bindings: []interfaces.InvocationParameterBindingConfig{
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindStdin)},
				},
			},
			{
				Name:         "output",
				ExternalName: "output",
				Aliases:      []string{"out"},
				Bindings: []interfaces.InvocationParameterBindingConfig{
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
				},
			},
		},
	}
}
