package process

import (
	"errors"
	"fmt"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"os/exec"
	"strings"
	"testing"
)

func TestCommandLinePreflightRejectsBeforeCommandConstruction(t *testing.T) {
	t.Parallel()
	for _, streaming := range []bool{false, true} {
		for _, length := range []int{WindowsCommandLineLimit - 1, WindowsCommandLineLimit, WindowsCommandLineLimit + 1} {
			t.Run(fmt.Sprintf("streaming=%t/length=%d", streaming, length), func(t *testing.T) {
				t.Parallel()
				called := 0
				logger := &recordingCommandLogger{}
				factoryErr := errors.New("inert command factory reached")
				runner := ExecCommandRunner{CommandLineLimit: WindowsCommandLineLimit, Logger: logger,
					Clock: platformclock.Real{}, NewCommand: func(string, ...string) *exec.Cmd {
						called++
						return &exec.Cmd{Err: factoryErr}
					},
				}
				request := CommandRequest{Command: "codex", Args: []string{strings.Repeat("x", length-len("codex "))}, Stdin: []byte("private stdin 😀")}
				var err error
				if streaming {
					_, err = runner.RunStreaming(t.Context(), request, func(string, []byte) {
						t.Error("rejected command published output")
					})
				} else {
					_, err = runner.Run(t.Context(), request)
				}
				var startErr *CommandStartError
				if !errors.As(fmt.Errorf("wrapped: %w", err), &startErr) {
					t.Fatalf("error = %v, want typed command start error", err)
				}
				if startErr.CommandLineLength != length || startErr.CommandLineLimit != WindowsCommandLineLimit || startErr.StdinBytes != len(request.Stdin) {
					t.Fatalf("measured error = %#v", startErr)
				}
				overLimit := length >= WindowsCommandLineLimit
				if startErr.OverCommandLineLimit() != overLimit || (called == 0) != overLimit {
					t.Fatalf("over limit = %t, factory calls = %d", startErr.OverCommandLineLimit(), called)
				}
				if !overLimit && !errors.Is(err, factoryErr) {
					t.Fatalf("below-bound invocation did not retain factory failure: %v", err)
				}
				logs := commandStartFailureLogs(logger)
				if len(logs) != 1 || strings.Contains(fmt.Sprint(logs), "private stdin") || strings.Contains(err.Error(), request.Args[0]) {
					t.Fatalf("unsafe or absent preflight diagnostic: %v", err)
				}
			})
		}
	}
}

func TestCommandLinePreflightZeroLimitRetainsUnboundedFactory(t *testing.T) {
	t.Parallel()
	called := 0
	cause := errors.New("inert factory")
	runner := ExecCommandRunner{Clock: platformclock.Real{}, NewCommand: func(string, ...string) *exec.Cmd {
		called++
		return &exec.Cmd{Err: cause}
	}}
	_, err := runner.Run(t.Context(), CommandRequest{Command: "codex", Args: []string{strings.Repeat("x", WindowsCommandLineLimit)}})
	var startErr *CommandStartError
	if called != 1 || !errors.Is(err, cause) || !errors.As(err, &startErr) || startErr.OverCommandLineLimit() {
		t.Fatalf("unbounded invocation = %v, factory calls = %d", err, called)
	}
}
