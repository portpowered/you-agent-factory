package work

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestInvocationErrorsAreNilSafeAndPreserveConstructedMessage(t *testing.T) {
	t.Parallel()

	var argumentErr *ArgumentError
	var contentErr *TextContentValidationError
	if argumentErr.Error() != "" || contentErr.Error() != "" {
		t.Fatal("nil invocation errors returned non-empty messages")
	}
	constructed := newArgumentError(ArgumentErrorCodeInvalidActiveSignature, "invalid signature", "input", "value")
	if constructed.Error() != "invalid signature" || constructed.Parameter != "input" || constructed.Argument != "value" {
		t.Fatalf("newArgumentError() = %+v, want preserved diagnostic fields", constructed)
	}
}

func TestNormalizeArguments_SignatureCanonicalizesNamedKeysAndDefaults(t *testing.T) {
	stdin := "true"
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(
			positionalParameter("input", 1, true),
			namedParameter("output", "out", nil, false, typeHintPath, "report.txt"),
			namedParameter("mode", "", []string{"m"}, false, "", "fast"),
			stdinParameter("confirm", typeHintBooleanString),
		),
		PositionalArgs: []string{"hello"},
		NamedArgs: []NamedArgumentInput{
			{Key: "out", Values: []string{"artifact.json"}},
			{Key: "m", Values: []string{"slow"}},
		},
		StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}

	assertArgumentValues(t, got.Arguments, "input", []string{"hello"})
	assertArgumentValues(t, got.Arguments, "output", []string{"artifact.json"})
	assertArgumentValues(t, got.Arguments, "mode", []string{"slow"})
	assertArgumentValues(t, got.Arguments, "confirm", []string{"true"})

	if source := got.Arguments["output"].Sources[0]; source.Kind != ArgumentSourceKindNamed || source.Name != "out" {
		t.Fatalf("output source = %#v, want named alias metadata", source)
	}
	if source := got.Arguments["mode"].Sources[0]; source.Kind != ArgumentSourceKindNamed || source.Name != "m" {
		t.Fatalf("mode source = %#v, want named alias metadata", source)
	}
}

func TestNormalizeArguments_SignatureAcceptsStructuredArgsForPositionalAndStdinBindings(t *testing.T) {
	stdinParameter := InvocationParameterConfig{
		Name: "prompt",
		Bindings: []InvocationParameterBindingConfig{{
			Kind: bindingKindStdin,
		}},
	}
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(
			positionalParameter("input", 1, true),
			stdinParameter,
		),
		DirectArgs: []NamedArgumentInput{
			{Key: "input", Values: []string{"hello"}},
			{Key: "prompt", Values: []string{"from api args"}},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}

	assertArgumentValues(t, got.Arguments, "input", []string{"hello"})
	assertArgumentValues(t, got.Arguments, "prompt", []string{"from api args"})
	if source := got.Arguments["input"].Sources[0]; source.Kind != ArgumentSourceKindStructured || source.Name != "input" {
		t.Fatalf("input source = %#v, want structured canonical metadata", source)
	}
	if source := got.Arguments["prompt"].Sources[0]; source.Kind != ArgumentSourceKindStructured || source.Name != "prompt" {
		t.Fatalf("prompt source = %#v, want structured canonical metadata", source)
	}
}

func TestNormalizeArguments_SignatureSupportsRepeatedUnknownCollectionAndSensitiveRedaction(t *testing.T) {
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: &InvocationSignatureConfig{
			UnknownNamedArgumentPolicy: unknownNamedCollect,
			Parameters: []InvocationParameterConfig{
				{
					Name:          "tag",
					ValueMode:     valueModeRepeated,
					Bindings:      []InvocationParameterBindingConfig{{Kind: bindingKindNamed}},
					Aliases:       []string{"t"},
					Sensitive:     true,
					DefaultValues: []string{},
				},
				{
					Name:      "extras",
					ValueMode: valueModeRepeated,
					Bindings:  []InvocationParameterBindingConfig{{Kind: bindingKindNamedRest}},
				},
			},
		},
		NamedArgs: []NamedArgumentInput{
			{Key: "tag", Values: []string{"alpha", "beta"}},
			{Key: "provider", Values: []string{"anthropic"}},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}

	assertArgumentValues(t, got.Arguments, "tag", []string{"alpha", "beta"})
	assertArgumentValues(t, got.Arguments, "extras", []string{"provider=anthropic"})
	if source := got.Arguments["tag"].Sources[0]; !source.Redact {
		t.Fatalf("tag source = %#v, want redaction marker", source)
	}
}

func TestNormalizeArguments_SignatureRejectsUnknownArgumentsByDefault(t *testing.T) {
	_, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(positionalParameter("input", 1, false)),
		NamedArgs: []NamedArgumentInput{{Key: "mystery", Values: []string{"value"}}},
	})

	assertArgumentErrorCode(t, err, ArgumentErrorCodeUnknownArgument)
}

func TestNormalizeArguments_SignatureRejectsPositionalOverflow(t *testing.T) {
	_, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature:      signatureConfig(positionalParameter("input", 1, false)),
		PositionalArgs: []string{"one", "two"},
	})

	assertArgumentErrorCode(t, err, ArgumentErrorCodePositionalOverflow)
}

func TestNormalizeArguments_SignatureRejectsSourceConflict(t *testing.T) {
	_, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: &InvocationSignatureConfig{
			Parameters: []InvocationParameterConfig{{
				Name:         "output",
				ExternalName: "output",
				Bindings: []InvocationParameterBindingConfig{
					{Kind: bindingKindPositional, Position: 1},
					{Kind: bindingKindNamed},
				},
			}},
		},
		PositionalArgs: []string{"first"},
		NamedArgs:      []NamedArgumentInput{{Key: "output", Values: []string{"second"}}},
	})

	assertArgumentErrorCode(t, err, ArgumentErrorCodeSourceConflict)
}

func TestNormalizeArguments_SignatureRejectsMissingRequiredInput(t *testing.T) {
	_, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(positionalParameter("input", 1, true)),
	})

	assertArgumentErrorCode(t, err, ArgumentErrorCodeMissingRequiredInput)
}

func TestNormalizeArguments_SignatureRejectsStringValidationMismatch(t *testing.T) {
	_, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(namedParameter("count", "", nil, false, typeHintNumberString, "")),
		NamedArgs: []NamedArgumentInput{{Key: "count", Values: []string{"nope"}}},
	})

	assertArgumentErrorCode(t, err, ArgumentErrorCodeStringValidationMismatch)
}

func TestNormalizeArguments_SignatureRejectsUnroutableStdin(t *testing.T) {
	stdin := "content"
	_, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(namedParameter("output", "", nil, false, "", "")),
		StdinText: &stdin,
	})

	assertArgumentErrorCode(t, err, ArgumentErrorCodeUnroutableStdin)
}

func TestNormalizeArguments_CompatibilityFallsBackToSharedTextResolver(t *testing.T) {
	stdin := "from stdin"
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		PositionalArgs: []string{"from", "args"},
		StdinText:      &stdin,
	})

	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if inputErr.Code != InputErrorCodeSourceConflict {
		t.Fatalf("code = %q, want %q", inputErr.Code, InputErrorCodeSourceConflict)
	}
	if got.CompatibilityInput != nil {
		t.Fatalf("compatibility input = %#v, want nil on conflict", got.CompatibilityInput)
	}
}

func TestNormalizeArguments_CompatibilityPreservesAPIContentFallback(t *testing.T) {
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		CompatibilityContent: []WorkContentPart{{Type: WorkContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("NormalizeArguments: %v", err)
	}
	if got.CompatibilityInput == nil {
		t.Fatal("CompatibilityInput = nil, want resolved input")
	}
	if got.CompatibilityInput.Source != InputSourceLabel(ArgumentSourceKindCompatibilityContent) {
		t.Fatalf("source = %q, want %q", got.CompatibilityInput.Source, ArgumentSourceKindCompatibilityContent)
	}
	if got.CompatibilityInput.Text != "hello" {
		t.Fatalf("text = %q, want hello", got.CompatibilityInput.Text)
	}
}

func TestNamedArgumentInputsFromAnyMap_SortsAndAcceptsStringsAndStringArrays(t *testing.T) {
	got, err := NamedArgumentInputsFromAnyMap(map[string]any{
		"mode": "fast",
		"tag":  []any{"alpha", "beta"},
	})
	if err != nil {
		t.Fatalf("NamedArgumentInputsFromAnyMap: %v", err)
	}

	want := []NamedArgumentInput{
		{Key: "mode", Values: []string{"fast"}},
		{Key: "tag", Values: []string{"alpha", "beta"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("named args = %#v, want %#v", got, want)
	}
}

func TestNamedArgumentInputsFromAnyMap_RejectsNonStringValues(t *testing.T) {
	_, err := NamedArgumentInputsFromAnyMap(map[string]any{
		"count": 7,
	})
	if err == nil || !strings.Contains(err.Error(), "args.count") {
		t.Fatalf("error = %v, want args.count validation", err)
	}
}

func signatureConfig(parameters ...InvocationParameterConfig) *InvocationSignatureConfig {
	return &InvocationSignatureConfig{Parameters: parameters}
}

func positionalParameter(name string, position int, required bool) InvocationParameterConfig {
	return InvocationParameterConfig{
		Name:     name,
		Required: required,
		Bindings: []InvocationParameterBindingConfig{{
			Kind:     bindingKindPositional,
			Position: position,
		}},
	}
}

func namedParameter(name, externalName string, aliases []string, required bool, typeHint string, defaultValue string) InvocationParameterConfig {
	parameter := InvocationParameterConfig{
		Name:         name,
		ExternalName: externalName,
		Aliases:      aliases,
		Required:     required,
		TypeHint:     typeHint,
		DefaultValue: defaultValue,
		Bindings: []InvocationParameterBindingConfig{{
			Kind: bindingKindNamed,
		}},
	}
	return parameter
}

func stdinParameter(name, typeHint string) InvocationParameterConfig {
	return InvocationParameterConfig{
		Name:     name,
		TypeHint: typeHint,
		Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindStdin}},
	}
}

func assertArgumentValues(t *testing.T, arguments map[string]NormalizedArgument, name string, want []string) {
	t.Helper()
	got, ok := arguments[name]
	if !ok {
		t.Fatalf("argument %q missing from %#v", name, arguments)
	}
	if !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("argument %q values = %#v, want %#v", name, got.Values, want)
	}
}

func assertArgumentErrorCode(t *testing.T, err error, want ArgumentErrorCode) {
	t.Helper()
	var argumentErr *ArgumentError
	if !errors.As(err, &argumentErr) {
		t.Fatalf("error = %v, want ArgumentError", err)
	}
	if argumentErr.Code != want {
		t.Fatalf("code = %q, want %q", argumentErr.Code, want)
	}
}
