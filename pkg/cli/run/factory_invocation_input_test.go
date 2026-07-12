package run

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/invocations"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestResolveFactoryInvocationInput_NoInputReturnsEmptyWithoutError(t *testing.T) {
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		StdinIsTTY: func() bool {
			return true
		},
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	if got.Payload != "" || got.Source != "" {
		t.Fatalf("input = %#v, want empty invocation input", got)
	}
}

func TestResolveSignatureFactoryInvocationInput_MaterializesNamedFileParameters(t *testing.T) {
	signature := &interfaces.InvocationSignatureConfig{Parameters: []interfaces.InvocationParameterConfig{
		{Name: "text", Required: true, Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "POSITIONAL", Position: 1}}},
		{Name: "reference_audio", ExternalName: "ref-wav", TypeHint: "FILE_PATH", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
		{Name: "reference_text", ExternalName: "ref-text", ValueMode: "FILE_CONTENTS", Bindings: []interfaces.InvocationParameterBindingConfig{{Kind: "NAMED"}}},
	}}
	normalized, err := ResolveSignatureFactoryInvocationInput(SignatureFactoryInvocationInputConfig{
		PromptArgs: []string{"hello there", "--ref-wav", "reference.wav", "--ref-text", "reference.txt"},
		Signature:  signature,
		StdinIsTTY: func() bool { return true },
	})
	if err != nil {
		t.Fatalf("ResolveSignatureFactoryInvocationInput: %v", err)
	}
	if got := normalized.Arguments["text"].Values; len(got) != 1 || got[0] != "hello there" {
		t.Fatalf("text = %#v", got)
	}
	if got := normalized.Arguments["reference_audio"].Values; len(got) != 1 || got[0] != "reference.wav" {
		t.Fatalf("reference_audio = %#v", got)
	}
	if got := normalized.Arguments["reference_text"].Values; len(got) != 1 || got[0] != "reference.txt" {
		t.Fatalf("reference_text = %#v", got)
	}
}

func TestResolveFactoryInvocationInput_PositionalOnly(t *testing.T) {
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"Fix", "the", "lint", "issues"},
		StdinIsTTY: func() bool {
			return true
		},
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	if got.Source != InvocationInputSourcePositional {
		t.Fatalf("source = %q, want positional prompt", got.Source)
	}
	if got.Payload != "Fix the lint issues" {
		t.Fatalf("payload = %q, want joined prompt text", got.Payload)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromDash(t *testing.T) {
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"-"},
		Stdin:      strings.NewReader("Fix the tests\n"),
		StdinIsTTY: func() bool {
			return true
		},
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	if got.Source != InvocationInputSourceStdin {
		t.Fatalf("source = %q, want stdin", got.Source)
	}
	if got.Payload != "Fix the tests\n" {
		t.Fatalf("payload = %q, want raw stdin payload", got.Payload)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromPipe(t *testing.T) {
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		Stdin: strings.NewReader("Fix from pipe\n"),
		StdinIsTTY: func() bool {
			return false
		},
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	if got.Source != InvocationInputSourceStdin {
		t.Fatalf("source = %q, want stdin", got.Source)
	}
	if got.Payload != "Fix from pipe\n" {
		t.Fatalf("payload = %q, want raw stdin payload", got.Payload)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromOverriddenReaderWithoutTTYHook(t *testing.T) {
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		Stdin: strings.NewReader("Fix from overridden reader\n"),
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	if got.Source != InvocationInputSourceStdin {
		t.Fatalf("source = %q, want stdin", got.Source)
	}
	if got.Payload != "Fix from overridden reader\n" {
		t.Fatalf("payload = %q, want raw stdin payload", got.Payload)
	}
}

func TestResolveFactoryInvocationInput_PreservesSurroundingWhitespace(t *testing.T) {
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"  keep", "surrounding", "whitespace  "},
		StdinIsTTY: func() bool { return true },
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	want := "  keep surrounding whitespace  "
	if got.Payload != want {
		t.Fatalf("payload = %q, want %q", got.Payload, want)
	}
}

func TestResolveFactoryInvocationInput_StdinPreservesSurroundingWhitespace(t *testing.T) {
	want := "  keep surrounding whitespace  "
	got, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"-"},
		Stdin:      strings.NewReader(want),
		StdinIsTTY: func() bool { return true },
	})
	if err != nil {
		t.Fatalf("ResolveFactoryInvocationInput: %v", err)
	}
	if got.Payload != want {
		t.Fatalf("payload = %q, want %q", got.Payload, want)
	}
}

func TestResolveFactoryInvocationInput_ExplicitEmptyPositionalUsesStableEmptyCode(t *testing.T) {
	_, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{""},
		StdinIsTTY: func() bool { return true },
	})
	if err == nil {
		t.Fatal("expected explicit empty positional rejection")
	}
	if !strings.Contains(err.Error(), string(invocations.InputErrorCodeEmpty)) {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
}

func TestResolveFactoryInvocationInput_RejectsWhitespaceOnlyPositional(t *testing.T) {
	_, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"   "},
		StdinIsTTY: func() bool { return true },
	})
	if err == nil {
		t.Fatal("expected whitespace-only positional rejection")
	}
	if !strings.Contains(err.Error(), string(invocations.InputErrorCodeEmpty)) {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
}

func TestResolveFactoryInvocationInput_ExplicitEmptyStdinUsesStableEmptyCode(t *testing.T) {
	_, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"-"},
		Stdin:      strings.NewReader(""),
		StdinIsTTY: func() bool { return true },
	})
	if err == nil {
		t.Fatal("expected empty stdin error")
	}
	if !strings.Contains(err.Error(), string(invocations.InputErrorCodeEmpty)) {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
}

func TestResolveFactoryInvocationInput_RejectsPositionalAndStdinConflict(t *testing.T) {
	_, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"Fix", "the", "lint", "issues"},
		Stdin:      strings.NewReader("Fix from stdin\n"),
		StdinIsTTY: func() bool {
			return false
		},
	})
	if err == nil {
		t.Fatal("expected ambiguous input error")
	}
	for _, want := range []string{
		string(invocations.InputErrorCodeSourceConflict),
		"positional_text",
		"stdin_text",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}

func TestObserveInvocationRejection_AmbiguousInputRecordsStructuredLogAndMetrics(t *testing.T) {
	resetCleanInvocationMetricsForTest()

	_, err := ResolveFactoryInvocationInput(FactoryInvocationInputConfig{
		PromptArgs: []string{"Fix", "tests"},
		Stdin:      strings.NewReader("Fix from stdin\n"),
		StdinIsTTY: func() bool { return false },
	})
	if err == nil {
		t.Fatal("expected ambiguous input error")
	}

	core, observed := observer.New(zap.InfoLevel)
	ObserveInvocationRejection(zap.New(core), err)

	entry := observed.FilterMessage(cleanInvocationLogMessageRejected).AllUntimed()
	if len(entry) != 1 {
		t.Fatalf("rejected logs = %d, want 1", len(entry))
	}
	fields := entry[0].ContextMap()
	if fields["mode"] != cleanInvocationModeLabel {
		t.Fatalf("mode = %#v, want clean", fields["mode"])
	}
	if fields["reason"] != cleanInvocationRejectReason {
		t.Fatalf("reason = %#v, want ambiguous_input", fields["reason"])
	}
	conflictingAny, ok := fields["conflictingSources"].([]interface{})
	if !ok {
		t.Fatalf("conflictingSources = %#v, want []interface{}", fields["conflictingSources"])
	}
	conflicting := make([]string, 0, len(conflictingAny))
	for _, value := range conflictingAny {
		label, ok := value.(string)
		if !ok {
			t.Fatalf("conflictingSources entry = %#v, want string", value)
		}
		conflicting = append(conflicting, label)
	}
	if len(conflicting) != 2 || conflicting[0] != "positional_prompt" || conflicting[1] != "stdin" {
		t.Fatalf("conflictingSources = %#v", conflicting)
	}

	if got := snapshotCleanInvocationMetrics(); got != (CleanInvocationMetricsSnapshot{
		Attempts:          1,
		AmbiguityRejected: 1,
	}) {
		t.Fatalf("metrics = %#v", got)
	}
}

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
