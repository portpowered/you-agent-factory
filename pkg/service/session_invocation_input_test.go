package service

import (
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
)

func TestResolveSessionInvocationInput_SignatureArgsNormalizeNamedInputs(t *testing.T) {
	request := factoryapi.InvocationRequest{
		Args: &map[string]any{
			"input": "hello",
			"mode":  []any{"fast", "review"},
		},
	}

	resolved, err := resolveSessionInvocationInput(signatureInvocationFactoryConfig(), request)
	if err != nil {
		t.Fatalf("resolveSessionInvocationInput: %v", err)
	}
	if resolved.Source != invocationInputSourceStructuredArgs {
		t.Fatalf("source = %q, want %q", resolved.Source, invocationInputSourceStructuredArgs)
	}
	if len(resolved.Content) != 0 {
		t.Fatalf("content = %#v, want no compatibility content", resolved.Content)
	}
	if resolved.NormalizedArguments == nil {
		t.Fatal("NormalizedArguments = nil, want normalized args")
	}
	if values := resolved.NormalizedArguments.Arguments["input"].Values; len(values) != 1 || values[0] != "hello" {
		t.Fatalf("input values = %#v, want [hello]", values)
	}
	if values := resolved.NormalizedArguments.Arguments["mode"].Values; len(values) != 2 || values[0] != "fast" || values[1] != "review" {
		t.Fatalf("mode values = %#v, want [fast review]", values)
	}
}

func TestResolveSessionInvocationInput_SignatureArgsRejectMissingRequiredInput(t *testing.T) {
	request := factoryapi.InvocationRequest{
		Args: &map[string]any{
			"mode": "fast",
		},
	}

	_, err := resolveSessionInvocationInput(signatureInvocationFactoryConfig(), request)
	assertSessionInvocationArgumentErrorCode(t, err, invocations.ArgumentErrorCodeMissingRequiredInput)
}

func TestResolveSessionInvocationInput_EmptyStructuredArgsStillUseSignature(t *testing.T) {
	request := factoryapi.InvocationRequest{
		Args: &map[string]any{},
	}

	resolved, err := resolveSessionInvocationInput(optionalSignatureInvocationFactoryConfig(), request)
	if err != nil {
		t.Fatalf("resolveSessionInvocationInput: %v", err)
	}
	if resolved.Source != invocationInputSourceStructuredArgs {
		t.Fatalf("source = %q, want %q", resolved.Source, invocationInputSourceStructuredArgs)
	}
	if resolved.NormalizedArguments == nil {
		t.Fatal("NormalizedArguments = nil, want normalized args")
	}
	if len(resolved.NormalizedArguments.Arguments) != 1 {
		t.Fatalf("normalized args = %#v, want defaulted optional args", resolved.NormalizedArguments.Arguments)
	}
	if values := resolved.NormalizedArguments.Arguments["mode"].Values; len(values) != 1 || values[0] != "fast" {
		t.Fatalf("mode values = %#v, want [fast]", values)
	}
}

func TestResolveSessionInvocationInput_StructuredArgsAcceptPositionalOnlyParameterKeys(t *testing.T) {
	request := factoryapi.InvocationRequest{
		Args: &map[string]any{
			"input": "hello",
		},
	}

	resolved, err := resolveSessionInvocationInput(positionalOnlySignatureInvocationFactoryConfig(), request)
	if err != nil {
		t.Fatalf("resolveSessionInvocationInput: %v", err)
	}
	if resolved.NormalizedArguments == nil {
		t.Fatal("NormalizedArguments = nil, want normalized args")
	}
	if values := resolved.NormalizedArguments.Arguments["input"].Values; len(values) != 1 || values[0] != "hello" {
		t.Fatalf("input values = %#v, want [hello]", values)
	}
	if source := resolved.NormalizedArguments.Arguments["input"].Sources[0]; source.Kind != invocations.ArgumentSourceKindStructured {
		t.Fatalf("input source kind = %q, want %q", source.Kind, invocations.ArgumentSourceKindStructured)
	}
}

func TestResolveSessionInvocationInput_SignatureArgsRejectCompatibilityContentMix(t *testing.T) {
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := invocationTextContent(t, "legacy compatibility text")
	request := factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
		Args: &map[string]any{
			"input": "hello",
		},
	}

	_, err := resolveSessionInvocationInput(signatureInvocationFactoryConfig(), request)
	assertSessionInvocationArgumentErrorCode(t, err, invocations.ArgumentErrorCodeSourceConflict)
}

func TestResolveSessionInvocationInput_RejectsStructuredArgsWithoutActiveSignature(t *testing.T) {
	request := factoryapi.InvocationRequest{
		Args: &map[string]any{
			"input": "hello",
		},
	}

	_, err := resolveSessionInvocationInput(&interfaces.FactoryConfig{}, request)
	assertSessionInvocationArgumentErrorCode(t, err, invocations.ArgumentErrorCodeInvalidActiveSignature)
}

func signatureInvocationFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		InvocationSignature: &interfaces.InvocationSignatureConfig{
			Parameters: []interfaces.InvocationParameterConfig{
				{
					Name:     "input",
					Required: true,
					Bindings: []interfaces.InvocationParameterBindingConfig{{
						Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed),
					}},
				},
				{
					Name:      "mode",
					ValueMode: string(factoryapi.FactoryInvocationParameterValueModeRepeated),
					Bindings: []interfaces.InvocationParameterBindingConfig{{
						Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed),
					}},
				},
			},
		},
	}
}

func optionalSignatureInvocationFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		InvocationSignature: &interfaces.InvocationSignatureConfig{
			Parameters: []interfaces.InvocationParameterConfig{{
				Name:         "mode",
				DefaultValue: "fast",
				Bindings: []interfaces.InvocationParameterBindingConfig{{
					Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed),
				}},
			}},
		},
	}
}

func positionalOnlySignatureInvocationFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		InvocationSignature: &interfaces.InvocationSignatureConfig{
			Parameters: []interfaces.InvocationParameterConfig{{
				Name:     "input",
				Required: true,
				Bindings: []interfaces.InvocationParameterBindingConfig{{
					Kind:     string(factoryapi.FactoryInvocationParameterBindingKindPositional),
					Position: 1,
				}},
			}},
		},
	}
}

func invocationTextContent(t *testing.T, text string) factoryapi.WorkContent {
	t.Helper()

	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		t.Fatalf("build invocation text content: %v", err)
	}
	return factoryapi.WorkContent{part}
}

func assertSessionInvocationArgumentErrorCode(t *testing.T, err error, want invocations.ArgumentErrorCode) {
	t.Helper()

	var argumentErr *invocations.ArgumentError
	if !errors.As(err, &argumentErr) {
		t.Fatalf("error = %v, want ArgumentError", err)
	}
	if argumentErr.Code != want {
		t.Fatalf("code = %q, want %q", argumentErr.Code, want)
	}
}
