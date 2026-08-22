package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestMainDelegatesExitCodeToRootProcess(t *testing.T) {
	originalRun := runProcess
	originalExit := exitProcess
	t.Cleanup(func() {
		runProcess = originalRun
		exitProcess = originalExit
	})

	runCalls := 0
	runProcess = func() int {
		runCalls++
		return 23
	}
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}

	main()

	if runCalls != 1 {
		t.Fatalf("root process calls = %d, want 1", runCalls)
	}
	if exitCode != 23 {
		t.Fatalf("exit code = %d, want 23", exitCode)
	}
}

var processExitCodeTests = []struct {
	name       string
	err        error
	contextErr error
	args       []string
	want       int
}{
	{name: "success", want: exitSuccess},
	{name: "failure", err: errors.New("failed"), want: exitFailure},
	{
		name: "incomplete finite drain",
		err:  &factoryruntime.IncompleteDrainError{NonTerminalWorkCount: 1},
		want: exitFailure,
	},
	{
		name:       "run cancellation normalized by lifecycle",
		contextErr: context.Canceled,
		args:       []string{"you", "run"},
		want:       130,
	},
	{
		name:       "run prompt is not part of command path",
		contextErr: context.Canceled,
		args:       []string{"you", "run", "Fix lint"},
		want:       130,
	},
	{
		name:       "run flag values are not part of command path",
		contextErr: context.Canceled,
		args: []string{
			"you", "run", "--dir", `C:\workspace`, "--work", "Fix lint",
			"--named", "factory-name", "--factory", "factory-path",
		},
		want: 130,
	},
	{
		name:       "server cancellation normalized by lifecycle",
		contextErr: context.Canceled,
		args:       []string{"you", "server"},
		want:       130,
	},
	{
		name:       "worker session stream cancellation normalized by lifecycle",
		contextErr: context.Canceled,
		args:       []string{"you", "--server", "http://localhost:7437", "worker-sessions", "stream", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-1"},
		want:       130,
	},
	{
		name: "wrapped cancellation",
		err:  fmt.Errorf("stop continuous run: %w", context.Canceled),
		args: []string{"you", "--server", "http://localhost:7437", "run", "--continuously"},
		want: 130,
	},
	{
		name: "wrapped server cancellation normalized by lifecycle",
		err:  fmt.Errorf("stop server: %w", context.Canceled),
		args: []string{"you", "--server", "http://localhost:7437", "server"},
		want: 130,
	},
	{
		name: "wrapped worker session stream cancellation normalized by lifecycle",
		err:  fmt.Errorf("stop worker session stream: %w", context.Canceled),
		args: []string{"you", "--server", "http://localhost:7437", "worker-sessions", "stream"},
		want: 130,
	},
	{
		name:       "MCP child cancellation uses its declared lifecycle code",
		contextErr: context.Canceled,
		args:       []string{"you", "server", "mcp"},
		want:       exitFailure,
	},
	{
		name:       "ACP child cancellation uses its declared lifecycle code",
		contextErr: context.Canceled,
		args:       []string{"you", "server", "acp"},
		want:       exitFailure,
	},
	{
		name: "wrapped MCP child cancellation uses its declared lifecycle code",
		err:  fmt.Errorf("stop MCP server: %w", context.Canceled),
		args: []string{"you", "server", "mcp"},
		want: exitFailure,
	},
	{
		name:       "ordinary failure wins over canceled process context",
		err:        errors.New("server startup failed"),
		contextErr: context.Canceled,
		args:       []string{"you", "server"},
		want:       exitFailure,
	},
}

func TestProcessExitCodePreservesDeclaredLifecycleContract(t *testing.T) {
	t.Parallel()

	for _, test := range processExitCodeTests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := processExitCode(test.err, test.contextErr, test.args); got != test.want {
				t.Fatalf(
					"processExitCode(%v, %v, %v) = %d, want %d",
					test.err,
					test.contextErr,
					test.args,
					got,
					test.want,
				)
			}
		})
	}
}
