package invocations

import (
	"errors"
	"reflect"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestNormalizeArguments_SignatureCanonicalizesNamedKeysAndDefaults(t *testing.T) {
	stdin := "true"
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: signatureConfig(
			positionalParameter("input", 1, true),
			namedParameter("output", "out", nil, false, string(factoryapi.FactoryInvocationParameterTypeHintPath), "report.txt"),
			namedParameter("mode", "", []string{"m"}, false, "", "fast"),
			stdinParameter("confirm", string(factoryapi.FactoryInvocationParameterTypeHintBooleanString)),
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

func TestNormalizeArguments_SignatureSupportsRepeatedUnknownCollectionAndSensitiveRedaction(t *testing.T) {
	got, err := NormalizeArguments(NormalizeArgumentsInput{
		Signature: &interfaces.InvocationSignatureConfig{
			UnknownNamedArgumentPolicy: string(factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect),
			Parameters: []interfaces.InvocationParameterConfig{
				{
					Name:          "tag",
					ValueMode:     string(factoryapi.FactoryInvocationParameterValueModeRepeated),
					Bindings:      []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)}},
					Aliases:       []string{"t"},
					Sensitive:     true,
					DefaultValues: []string{},
				},
				{
					Name:      "extras",
					ValueMode: string(factoryapi.FactoryInvocationParameterValueModeRepeated),
					Bindings:  []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamedRest)}},
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
		Signature: &interfaces.InvocationSignatureConfig{
			Parameters: []interfaces.InvocationParameterConfig{{
				Name:         "output",
				ExternalName: "output",
				Bindings: []interfaces.InvocationParameterBindingConfig{
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindPositional), Position: 1},
					{Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed)},
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
		Signature: signatureConfig(namedParameter("count", "", nil, false, string(factoryapi.FactoryInvocationParameterTypeHintNumberString), "")),
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
		CompatibilityContent: []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeText, Text: "hello"}},
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

func signatureConfig(parameters ...interfaces.InvocationParameterConfig) *interfaces.InvocationSignatureConfig {
	return &interfaces.InvocationSignatureConfig{Parameters: parameters}
}

func positionalParameter(name string, position int, required bool) interfaces.InvocationParameterConfig {
	return interfaces.InvocationParameterConfig{
		Name:     name,
		Required: required,
		Bindings: []interfaces.InvocationParameterBindingConfig{{
			Kind:     string(factoryapi.FactoryInvocationParameterBindingKindPositional),
			Position: position,
		}},
	}
}

func namedParameter(name, externalName string, aliases []string, required bool, typeHint string, defaultValue string) interfaces.InvocationParameterConfig {
	parameter := interfaces.InvocationParameterConfig{
		Name:         name,
		ExternalName: externalName,
		Aliases:      aliases,
		Required:     required,
		TypeHint:     typeHint,
		DefaultValue: defaultValue,
		Bindings: []interfaces.InvocationParameterBindingConfig{{
			Kind: string(factoryapi.FactoryInvocationParameterBindingKindNamed),
		}},
	}
	return parameter
}

func stdinParameter(name, typeHint string) interfaces.InvocationParameterConfig {
	return interfaces.InvocationParameterConfig{
		Name:     name,
		TypeHint: typeHint,
		Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: string(factoryapi.FactoryInvocationParameterBindingKindStdin)}},
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
