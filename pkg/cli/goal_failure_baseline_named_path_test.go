package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
)

// Hermetic S02 failure-baseline fixtures for one-shot you run --named when the
// requested named-factory path cannot be resolved before invocation starts.

func TestFailureBaseline_NamedPath_RunNamedMissingLocalFactoryRejectsBeforeInvocation(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	restore := withNamedPackagedFactoryRunRoot(t)
	defer restore()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "missing-alpha",
		"--no-record",
		"--quiet",
		"named-path baseline prompt",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected missing named factory to fail before invocation")
	}
	if !strings.Contains(err.Error(), `resolve named factory "missing-alpha"`) {
		t.Fatalf("error = %q, want named-path resolution guidance", err.Error())
	}
	if !strings.Contains(err.Error(), `named factory "missing-alpha" not found`) {
		t.Fatalf("error = %q, want named factory not found guidance", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for unresolved named factory path")
	}
}

func TestFailureBaseline_NamedPath_RunNamedUnknownBuiltInGoalStyleNameRejectsBeforeInvocation(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	restore := withNamedPackagedFactoryRunRoot(t)
	defer restore()

	runCalled := false
	runCLI = func(context.Context, runcli.RunConfig) error {
		runCalled = true
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"run",
		"--named", "@you/missing",
		"--no-record",
		"--quiet",
		"named-path baseline prompt",
	})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected unknown built-in named factory to fail before invocation")
	}
	if !strings.Contains(err.Error(), `resolve named factory "@you/missing"`) {
		t.Fatalf("error = %q, want built-in named-path resolution guidance", err.Error())
	}
	if !strings.Contains(err.Error(), "project root") || !strings.Contains(err.Error(), "global root") {
		t.Fatalf("error = %q, want cross-root named-path context", err.Error())
	}
	if runCalled {
		t.Fatal("run should not start for unknown built-in named factory path")
	}
}
