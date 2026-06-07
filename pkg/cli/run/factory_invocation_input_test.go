package run

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/invocations"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
