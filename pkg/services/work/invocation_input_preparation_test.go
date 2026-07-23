package work

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveFactoryInvocationInput_NoInputReturnsEmptyWithoutError(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared != (PreparedInvocationInput{}) {
		t.Fatalf("prepared = %#v, want empty", prepared)
	}
}

func TestResolveFactoryInvocationInput_PositionalOnly(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"Fix", "the", "lint", "issues"}})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != InputSourcePositionalText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix the lint issues" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromDash(t *testing.T) {
	stdin := "Fix the tests\n"
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &stdin})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != InputSourceStdinText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix the tests\n" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromPipe(t *testing.T) {
	stdin := "Fix from pipe\n"
	prepared, err := NewInvocationInputPreparation().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != InputSourceStdinText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix from pipe\n" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromOverriddenReaderWithoutTTYHook(t *testing.T) {
	stdin := "Fix from overridden reader\n"
	prepared, err := NewInvocationInputPreparation().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{StdinText: &stdin})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix from overridden reader\n" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_PreservesSurroundingWhitespace(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"  keep", "surrounding", "whitespace  "}})
	if err != nil || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "  keep surrounding whitespace  " {
		t.Fatalf("prepared = %#v, err = %v", prepared, err)
	}
}

func TestResolveFactoryInvocationInput_StdinPreservesSurroundingWhitespace(t *testing.T) {
	want := "  keep surrounding whitespace  "
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &want})
	if err != nil || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != want {
		t.Fatalf("prepared = %#v, err = %v", prepared, err)
	}
}

func TestResolveFactoryInvocationInput_ExplicitEmptyPositionalUsesStableEmptyCode(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{""}})
	assertInputErrorCode(t, err, InputErrorCodeEmpty)
}

func TestResolveFactoryInvocationInput_RejectsWhitespaceOnlyPositional(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"   "}})
	assertInputErrorCode(t, err, InputErrorCodeEmpty)
}

func TestResolveFactoryInvocationInput_ExplicitEmptyStdinUsesStableEmptyCode(t *testing.T) {
	empty := ""
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &empty})
	assertInputErrorCode(t, err, InputErrorCodeEmpty)
}

func TestResolveFactoryInvocationInput_RejectsPositionalAndStdinConflict(t *testing.T) {
	stdin := "Fix from stdin\n"
	_, err := NewInvocationInputPreparation().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"Fix", "the", "lint", "issues"}, StdinText: &stdin,
	})
	assertInputErrorCode(t, err, InputErrorCodeSourceConflict)
}

func TestResolveSignatureFactoryInvocationInput_NormalizesPositionalNamedBooleanAndStdin(t *testing.T) {
	stdin := "from stdin"
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Arguments: []string{"draft", "--mode", "fast", "--confirm", "--out=result.md", "-"},
		Signature: signatureFactoryInvocationConfig(), StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	got := prepared.NormalizedArguments
	if got == nil {
		t.Fatal("normalized arguments are nil")
	}
	for name, want := range map[string]string{"input": "draft", "mode": "fast", "confirm": "true", "stdinText": "from stdin", "output": "result.md"} {
		if values := got.Arguments[name].Values; len(values) != 1 || values[0] != want {
			t.Fatalf("%s values = %#v, want [%q]", name, values, want)
		}
	}
}

func TestResolveSignatureFactoryInvocationInput_RejectsMissingNamedValue(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Arguments: []string{"draft", "--mode"}, Signature: signatureFactoryInvocationConfig(),
	})
	if err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("error = %v, want missing value", err)
	}
}

func TestNormalizeLegacyInvocationExampleRejectsUnstructuredInputs(t *testing.T) {
	t.Parallel()

	normalizer := InvocationExampleNormalizer{}
	if _, err := normalizer.NormalizeLegacyInvocationExample(
		[]string{"draft", "--mode"},
		signatureFactoryInvocationConfig(),
		nil,
	); err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("missing named value error = %v", err)
	}
	if _, err := normalizer.NormalizeLegacyInvocationExample([]string{"free-form input"}, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "does not resolve to structured invocation arguments") {
		t.Fatalf("unstructured compatibility input error = %v", err)
	}
}

func TestPrepareInvocationInputReturnsDetachedCanonicalValues(t *testing.T) {
	signature := signatureFactoryInvocationConfig()
	arguments := []string{"draft", "--mode=fast"}
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: arguments, Signature: signature})
	if err != nil {
		t.Fatal(err)
	}
	arguments[0] = "changed"
	signature.Parameters[0].Name = "changed"
	if got := prepared.NormalizedArguments.Arguments["input"].Values[0]; got != "draft" {
		t.Fatalf("detached input = %q", got)
	}
}

func TestPrepareInvocationInputRequiresLiveContext(t *testing.T) {
	preparation := NewInvocationInputPreparation()
	if _, err := preparation.PrepareInvocationInput(nil, InvocationInputPreparationRequest{}); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("nil context error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := preparation.PrepareInvocationInput(ctx, InvocationInputPreparationRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}
}

func prepareInvocationInput(t *testing.T, request InvocationInputPreparationRequest) (PreparedInvocationInput, error) {
	t.Helper()
	return NewInvocationInputPreparation().PrepareInvocationInput(context.Background(), request)
}

func assertInputErrorCode(t *testing.T, err error, code InputErrorCode) {
	t.Helper()
	var inputErr *InputError
	if !errors.As(err, &inputErr) || inputErr.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func signatureFactoryInvocationConfig() *InvocationSignatureConfig {
	return &InvocationSignatureConfig{Parameters: []InvocationParameterConfig{
		{Name: "input", Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindPositional, Position: 1}}},
		{Name: "mode", ExternalName: "mode", Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}}},
		{Name: "confirm", TypeHint: typeHintBooleanString, Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}}},
		{Name: "stdinText", Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindStdin}}},
		{Name: "output", ExternalName: "output", Aliases: []string{"out"}, Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}}},
	}}
}
