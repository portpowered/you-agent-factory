package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
)

func TestRunCommand_NamedFactoryPromptCarriesInvocationText(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/tts", "--no-record", "hi", "there"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/tts with prompt: %v", err)
	}
	if got.InvocationPositionalText == nil {
		t.Fatal("expected invocation positional text for named factory prompt run")
	}
	if gotText := *got.InvocationPositionalText; gotText != "hi there" {
		t.Fatalf("invocation positional text = %q, want joined prompt text", gotText)
	}
	if got.NamedFactoryName != "@you/tts" {
		t.Fatalf("named factory = %q, want @you/tts", got.NamedFactoryName)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected named text invocation to suppress dashboard rendering")
	}
}

func TestRunCommand_NamedFactoryStdinPromptCarriesInvocationText(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader("hi from stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/tts", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/tts with piped stdin: %v", err)
	}
	if got.InvocationPositionalText != nil {
		t.Fatal("expected no invocation positional text for named factory stdin run")
	}
	if got.InvocationStdinText == nil {
		t.Fatal("expected invocation stdin text for named factory stdin run")
	}
	if gotText := *got.InvocationStdinText; gotText != "hi from stdin\n" {
		t.Fatalf("invocation stdin text = %q, want raw stdin prompt text", gotText)
	}
	if got.NamedFactoryName != "@you/tts" {
		t.Fatalf("named factory = %q, want @you/tts", got.NamedFactoryName)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected named stdin invocation to suppress dashboard rendering")
	}
}

func TestRunCommand_NamedFactoryExplicitStdinPromptCarriesInvocationText(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDir)

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader("hi from explicit stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/tts", "--no-record", "-"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/tts with explicit stdin: %v", err)
	}
	if got.InvocationStdinText == nil {
		t.Fatal("expected invocation stdin text for named factory explicit stdin run")
	}
	if gotText := *got.InvocationStdinText; gotText != "hi from explicit stdin\n" {
		t.Fatalf("invocation stdin text = %q, want raw stdin prompt text", gotText)
	}
}

func TestRunCommand_NamedFactoryPromptRejectsAmbiguousPositionalAndStdin(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	workingDirectory := t.TempDir()
	homeDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()
	t.Setenv("HOME", homeDir)

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetIn(strings.NewReader("Fix from stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/tts", "--no-record", "Fix from args", "-"})

	err = root.Execute()
	if err == nil {
		t.Fatal("expected ambiguous positional and stdin prompt rejection for named factory")
	}
	for _, want := range []string{
		"INVOCATION_INPUT_SOURCE_CONFLICT",
		"positional_text",
		"stdin_text",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want %q", err.Error(), want)
		}
	}
	if runCalled {
		t.Fatal("run should not start for ambiguous named factory prompt input")
	}
}
