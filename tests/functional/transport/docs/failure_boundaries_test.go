package docs_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// TestDocsFailureBoundariesAndReuse characterizes invalid input, duplicate
// input, repeated canonical/alias reads, and output recovery through the same
// package process used by the happy-path docs tests.
func TestDocsFailureBoundariesAndReuse(t *testing.T) {
	fixture := documentationProcess(t)
	environment := isolatedDocumentationEnvironment(t)

	t.Run("unsupported topic leaves a later topic clean", func(t *testing.T) {
		providerCallsBefore := fixture.providerRunner.CallCount()
		result := executeDocumentationCommandResult(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			"docs", "unknown",
		)
		if result.err == nil || !strings.Contains(result.err.Error(), `unsupported docs topic "unknown"`) {
			t.Fatalf("unsupported topic error = %v, want unsupported-topic diagnostic", result.err)
		}
		if result.stdout != "" {
			t.Fatalf("unsupported topic stdout = %q, want empty", result.stdout)
		}
		if got := fixture.providerRunner.CallCount(); got != providerCallsBefore {
			t.Fatalf("unsupported topic provider calls = %d, want unchanged %d", got, providerCallsBefore)
		}
		valid := executeDocumentationCommand(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			"docs", "models",
		)
		if !strings.Contains(valid, "local Models composition") {
			t.Fatalf("valid topic after unsupported input omitted Models content")
		}
	})

	t.Run("duplicate topic is rejected and next invocation is clean", func(t *testing.T) {
		providerCallsBefore := fixture.providerRunner.CallCount()
		result := executeDocumentationCommandResult(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			"docs", "models", "models",
		)
		if result.err == nil || !strings.Contains(result.err.Error(), "accepts at most 1 arg(s)") {
			t.Fatalf("duplicate topic error = %v, want exact-argument rejection", result.err)
		}
		if result.stdout != "" {
			t.Fatalf("duplicate topic stdout = %q, want empty", result.stdout)
		}
		if got := fixture.providerRunner.CallCount(); got != providerCallsBefore {
			t.Fatalf("duplicate topic provider calls = %d, want unchanged %d", got, providerCallsBefore)
		}
		valid := executeDocumentationCommand(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			"docs", "models",
		)
		if !strings.Contains(valid, "local Models composition") {
			t.Fatalf("valid topic after duplicate input omitted Models content")
		}
	})

	t.Run("canonical and alias reads are detached and repeatable", func(t *testing.T) {
		commands := []string{"models", "workstation", "batch-work"}
		first := make(map[string]string, len(commands))
		for _, topic := range commands {
			first[topic] = executeDocumentationCommand(
				t,
				fixture.process,
				environment,
				fixture.tempDir(t),
				"docs", topic,
			)
		}
		for index := len(commands) - 1; index >= 0; index-- {
			topic := commands[index]
			got := executeDocumentationCommand(
				t,
				fixture.process,
				environment,
				fixture.tempDir(t),
				"docs", topic,
			)
			if got != first[topic] {
				t.Fatalf("repeated docs %s output differs or contains prior output", topic)
			}
		}
	})

	t.Run("writer failure recovers with a fresh buffer", func(t *testing.T) {
		want := executeDocumentationCommand(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			"docs", "models",
		)
		writer := &boundedDocumentationWriter{limit: 32}
		var stderr bytes.Buffer
		err := executeDocumentationCommandInto(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			writer,
			&stderr,
			"docs", "models",
		)
		if !errors.Is(err, errDocumentationOutput) {
			t.Fatalf("writer failure error = %v, want injected writer error", err)
		}
		if writer.String() == "" || len(writer.String()) >= len(want) {
			t.Fatalf("bounded writer output length = %d, want partial output below %d", len(writer.String()), len(want))
		}
		retry := executeDocumentationCommand(
			t,
			fixture.process,
			environment,
			fixture.tempDir(t),
			"docs", "models",
		)
		if retry != want {
			t.Fatalf("fresh-buffer retry differs from clean output")
		}
	})
}
