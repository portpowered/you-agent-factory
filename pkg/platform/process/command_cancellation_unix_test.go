//go:build !windows

package process

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecCommandRunner_ContextCancelReturnsWhenDescendantKeepsPipeOpen(t *testing.T) {
	requireProcessIntegration(t)
	const testGrace = 100 * time.Millisecond
	postRunCleanupGracePeriodForTest = testGrace
	t.Cleanup(func() {
		postRunCleanupGracePeriodForTest = 0
	})

	pidFile := filepath.Join(t.TempDir(), "escaped-child.pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type runOutcome struct {
		result CommandResult
		err    error
	}
	runDone := make(chan runOutcome, 1)
	go func() {
		result, err := testExecCommandRunner(t, nil).Run(ctx, CommandRequest{
			Command: os.Args[0],
			Args: []string{
				"-test.run=TestExecCommandRunner_HelperProcess",
				"--",
				"spawn-child-orphan-pipe",
			},
			Env: append(os.Environ(),
				"GO_WANT_COMMAND_HELPER=1",
				"COMMAND_HELPER_PID_FILE="+pidFile,
			),
		})
		runDone <- runOutcome{result: result, err: err}
	}()

	childPID := waitForCommandHelperPID(t, pidFile, commandHelperSpawnTimeoutBudget)
	t.Cleanup(func() {
		commandTestTerminateProcess(childPID)
	})

	started := time.Now()
	cancel()
	select {
	case outcome := <-runDone:
		if outcome.err == nil || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", outcome.err)
		}
		if elapsed := time.Since(started); elapsed > testGrace+time.Second {
			t.Fatalf("Run() cancellation took %v, want bounded return near %v", elapsed, testGrace)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after cancellation while descendant held the output pipe open")
	}

	commandTestTerminateProcess(childPID)
}
