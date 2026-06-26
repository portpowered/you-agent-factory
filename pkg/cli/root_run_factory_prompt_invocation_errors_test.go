package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
)

func TestRunCommand_FactoryPromptRejectsEmptyStdinWithStableCode(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected empty stdin rejection")
	}
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty stdin code", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for empty factory stdin")
	}
}

func assertRunStdoutFreeOfOperatorChatter(t *testing.T, stdout string) {
	t.Helper()
	for _, forbidden := range []string{
		"Factory initiated",
		"Dashboard URL",
		"Runtime log",
		"Opening dashboard",
		"Factory:",
		"Recording saved",
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("stdout = %q, want no %q chatter", stdout, forbidden)
		}
	}
}

func TestRunCommand_FactoryPromptRejectsAmbiguousPositionalAndStdin(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader("Fix from stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix from args", "-"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected ambiguous positional and stdin prompt rejection")
	}
	for _, want := range []string{"INVOCATION_INPUT_SOURCE_CONFLICT", "positional_text", "stdin_text"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
	if runCalled {
		t.Fatal("run should not start for ambiguous factory prompt input")
	}
}

func TestRunCommand_FactoryPromptRejectsWorkFlagConflict(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--factory", factoryPath, "--work", "work.json", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflict between positional prompt and --work")
	}
	if !strings.Contains(err.Error(), "cannot be used with --work") {
		t.Fatalf("error = %q, want --work conflict message", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start when prompt conflicts with --work")
	}
}

func TestRunCommand_PositionalPromptRequiresFactoryFlag(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--dir", "factory", "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected positional prompt without --factory to fail")
	}
	if !strings.Contains(err.Error(), "require --factory") {
		t.Fatalf("error = %q, want --factory requirement", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for positional prompt without --factory")
	}
}

func TestRunCommand_CleanInvocationFailureWritesPlaintextToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeFailed,
			Message: "clean invocation failed: mock worker rejected",
		}
	}

	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if got := stderr.String(); got != "RUN_INVOCATION_FAILED: clean invocation failed: mock worker rejected\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRunCommand_CleanInvocationJSONFailureWritesSingleErrorObjectToStderr(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	dir := t.TempDir()
	factoryPath := writePortableFactoryWithDefaultHandling(t, dir)

	var stdout, stderr bytes.Buffer
	runCLI = func(context.Context, runcli.RunConfig) error {
		return &runcli.InvocationError{
			Code:    runcli.InvocationErrorCodeTimeout,
			Message: "clean invocation timed out",
		}
	}

	root := NewRootCommand()
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"--json", "run", "--factory", factoryPath, "Fix the lint issues"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected clean invocation timeout")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var payload map[string]string
	if decodeErr := json.Unmarshal(stderr.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("stderr is not one JSON object: %v\n%s", decodeErr, stderr.String())
	}
	if payload["code"] != runcli.InvocationErrorCodeTimeout {
		t.Fatalf("code = %q, want %q", payload["code"], runcli.InvocationErrorCodeTimeout)
	}
	if payload["message"] != "clean invocation timed out" {
		t.Fatalf("message = %q", payload["message"])
	}
}
