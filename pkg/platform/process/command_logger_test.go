package process

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
)

// Logger compatibility is observed at the command preflight boundary without
// spawning a process. Each row owns its captured loggers and runs in parallel.
func TestExecCommandRunnerCallerLoggerSelection(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"constructor disabled", "constructor selected", "literal disabled", "literal selected"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			selected := &recordingCommandLogger{}
			requestLogger := &recordingCommandLogger{}
			var logger logging.Logger
			if strings.HasSuffix(name, "selected") {
				logger = selected
			}
			factory := func(string, ...string) *exec.Cmd { t.Fatal("preflight constructed command"); return nil }
			runner := ExecCommandRunner{NewCommand: factory, Clock: platformclock.Real{}, Logger: logger}
			if strings.HasPrefix(name, "constructor") {
				var err error
				runner, err = NewExecCommandRunner(factory, platformclock.Real{}, logger, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if runner.Logger == nil {
					t.Fatal("constructor did not store disabled logging")
				}
			}
			runner.CommandLineLimit = 1
			req := CommandRequest{Command: "provider", Args: []string{"private-prompt-value"}, ExecutionLogger: requestLogger}
			for _, streaming := range []bool{false, true} {
				var err error
				if streaming {
					_, err = runner.RunStreaming(t.Context(), req, nil)
				} else {
					_, err = runner.Run(t.Context(), req)
				}
				var start *CommandStartError
				if !errors.As(err, &start) || !start.OverCommandLineLimit() {
					t.Fatalf("error = %v, want named preflight denial", err)
				}
			}
			want := 0
			if logger != nil {
				want = 2
			}
			if len(selected.errors) != want {
				t.Fatalf("selected logs = %d, want %d", len(selected.errors), want)
			}
			if len(requestLogger.errors) != 0 {
				t.Fatal("cleanup logs replaced request execution logger")
			}
			for _, entry := range selected.errors {
				if strings.Contains(fmt.Sprint(entry.fields), "private-prompt-value") {
					t.Fatal("preflight log leaked prompt")
				}
			}
		})
	}
}

func TestCommandCleanupUsesSelectedLoggerWithoutChangingDiagnostics(t *testing.T) {
	t.Parallel()
	logger := &recordingCommandLogger{}
	cleanup := newCommandProcessCleanupContext(logger, CommandRequest{Command: "provider", Args: []string{"secret-prompt"}}, commandProcessCleanupReasonPostRun)
	cleanup.logStarted(41)
	cleanup.logCompleted(commandProcessCleanupOutcomePartialFailure, 41, errors.New("cleanup unavailable"), "")
	if len(logger.warns) != 1 {
		t.Fatalf("warnings = %d, want one partial failure", len(logger.warns))
	}
	if strings.Contains(fmt.Sprint(logger.warns), "secret-prompt") {
		t.Fatal("cleanup leaked prompt")
	}
	disabled := newCommandProcessCleanupContext(logging.NoopLogger{}, CommandRequest{}, commandProcessCleanupReasonPostRun)
	disabled.logStarted(41)
	disabled.logCompleted(commandProcessCleanupOutcomeNoOp, 41, nil, "")
}
