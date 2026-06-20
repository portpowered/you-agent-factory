package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
)

func TestRunCommand_NamedGoalPromptCarriesInvocationText(t *testing.T) {
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
	root.SetArgs([]string{"run", "--named", "@you/goal", "--no-record", "Plan", "the", "sprint"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/goal with prompt: %v", err)
	}
	if got.InvocationPositionalText == nil {
		t.Fatal("expected invocation positional text for named goal prompt run")
	}
	if gotText := *got.InvocationPositionalText; gotText != "Plan the sprint" {
		t.Fatalf("invocation positional text = %q, want joined prompt text", gotText)
	}
	if got.NamedFactoryName != "@you/goal" {
		t.Fatalf("named factory = %q, want @you/goal", got.NamedFactoryName)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected named text invocation to suppress dashboard rendering")
	}
}

func TestRunCommand_NamedGoalStdinPromptCarriesInvocationText(t *testing.T) {
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
	root.SetIn(strings.NewReader("Ship from stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/goal", "--no-record"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/goal with piped stdin: %v", err)
	}
	if got.InvocationPositionalText != nil {
		t.Fatal("expected no invocation positional text for named goal stdin run")
	}
	if got.InvocationStdinText == nil {
		t.Fatal("expected invocation stdin text for named goal stdin run")
	}
	if gotText := *got.InvocationStdinText; gotText != "Ship from stdin\n" {
		t.Fatalf("invocation stdin text = %q, want raw stdin prompt text", gotText)
	}
	if got.NamedFactoryName != "@you/goal" {
		t.Fatalf("named factory = %q, want @you/goal", got.NamedFactoryName)
	}
	if !got.SuppressDashboardRendering {
		t.Fatal("expected named stdin invocation to suppress dashboard rendering")
	}
}

func TestRunCommand_NamedGoalExplicitStdinPromptCarriesInvocationText(t *testing.T) {
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
	root.SetIn(strings.NewReader("Ship from explicit stdin\n"))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--named", "@you/goal", "--no-record", "-"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --named @you/goal with explicit stdin: %v", err)
	}
	if got.InvocationStdinText == nil {
		t.Fatal("expected invocation stdin text for named goal explicit stdin run")
	}
	if gotText := *got.InvocationStdinText; gotText != "Ship from explicit stdin\n" {
		t.Fatalf("invocation stdin text = %q, want raw stdin prompt text", gotText)
	}
}
