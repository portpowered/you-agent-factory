package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
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

func TestProcessExitCodePreservesDeclaredLifecycleContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		args []string
		want int
	}{
		{name: "success", want: exitSuccess},
		{name: "failure", err: errors.New("failed"), want: exitFailure},
		{name: "run cancellation", err: context.Canceled, args: []string{"you", "run"}, want: 130},
		{name: "server cancellation", err: context.Canceled, args: []string{"you", "server"}, want: 130},
		{
			name: "wrapped cancellation",
			err:  fmt.Errorf("stop continuous run: %w", context.Canceled),
			args: []string{"you", "--server", "http://localhost:7437", "run", "--continuously"},
			want: 130,
		},
		{name: "other command cancellation", err: context.Canceled, args: []string{"you", "mcp", "serve"}, want: exitFailure},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := processExitCode(test.err, test.args); got != test.want {
				t.Fatalf("processExitCode(%v, %v) = %d, want %d", test.err, test.args, got, test.want)
			}
		})
	}
}
