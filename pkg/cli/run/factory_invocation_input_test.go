package run

import (
	"strings"
	"testing"
)

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
	if got.Payload != "Fix the tests" {
		t.Fatalf("payload = %q, want stdin payload", got.Payload)
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
	if got.Payload != "Fix from pipe" {
		t.Fatalf("payload = %q, want stdin payload", got.Payload)
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
		InvocationErrorCodeAmbiguousInput,
		string(InvocationInputSourcePositional),
		string(InvocationInputSourceStdin),
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
}
