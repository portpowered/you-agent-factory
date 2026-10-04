//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func commandTestProcessRunning(pid int) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	event, err := windows.WaitForSingleObject(process, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}

func commandTestTerminateProcess(pid int) {
	process, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(process)
	_ = windows.TerminateProcess(process, 1)
}

func spawnCommandHelperEscapedChildMode() {
	spawnCommandHelperChildMode("child-sleep")
}

func TestExecCommandRunner_SupersededCauseReachesCleanupTelemetry(t *testing.T) {
	requireProcessIntegration(t)
	logger := &recordingCommandLogger{}
	observer := &lifecycleObserverRecorder{started: make(chan ProcessInfo, 1), exited: make(chan ProcessInfo, 1)}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	req := commandCleanupTestRequest(t)
	req.Args = []string{"-test.run=TestExecCommandRunner_HelperProcess", "--", "pid-sleep"}
	req.ProcessLifecycleObserver = observer
	runDone := make(chan struct {
		result CommandResult
		err    error
	}, 1)
	go func() {
		result, err := testExecCommandRunner(t, logger).Run(ctx, req)
		runDone <- struct {
			result CommandResult
			err    error
		}{result: result, err: err}
	}()
	select {
	case <-observer.started:
	case <-time.After(commandHelperSpawnTimeoutBudget):
		t.Fatal("timed out waiting for superseded command to start")
	}
	cancel(NewCancellationCause(CancellationReasonSuperseded))
	run := <-runDone
	if !errors.Is(run.err, context.Canceled) || run.result.ExitCode != 0 || run.result.CancellationReason != CancellationReasonSuperseded {
		t.Fatalf("Run result = %#v, error=%v, want zero-exit SUPERSEDED cancellation", run.result, run.err)
	}
	completed := commandCleanupCompletedLogsForReason(logger, commandProcessCleanupReasonCancel)
	if len(completed) == 0 {
		t.Fatal("expected superseded cancel cleanup completion log")
	}
	last := completed[len(completed)-1]
	assertCommandCleanupLogFields(t, last.fields, req, commandProcessCleanupReasonCancel)
	if last.fields["cancellation_reason"] != string(CancellationReasonSuperseded) || last.fields["outcome"] != string(commandProcessCleanupOutcomeForceKillSuccess) {
		t.Fatalf("cleanup completion = %#v, want SUPERSEDED force-kill success", last.fields)
	}
}

func TestParentOwnedStdioOwnership(t *testing.T) {
	for _, detach := range []bool{false, true} {
		t.Run(fmt.Sprint(detach), func(t *testing.T) {
			ends := []*os.File{{}, {}, {}, {}}
			closed := map[*os.File]int{}
			opens := 0
			channel, err := openParentOwnedStdio(func() (*os.File, *os.File, error) {
				i := opens * 2
				opens++
				return ends[i], ends[i+1], nil
			}, func(file *os.File) error { closed[file]++; return nil })
			if err != nil {
				t.Fatal(err)
			}
			cmd := &exec.Cmd{}
			channel.Attach(cmd)
			if cmd.Stdin != ends[0] || cmd.Stdout != ends[3] || channel.Requests() != ends[1] || channel.Responses() != ends[2] {
				t.Fatal("pipe direction or inherited descriptor changed")
			}
			if detach {
				channel.Detach()
				channel.Detach()
				if closed[ends[0]] != 1 || closed[ends[3]] != 1 || closed[ends[1]] != 0 || closed[ends[2]] != 0 {
					t.Fatal("detach must retain parent endpoints and release child copies exactly once")
				}
			}
			channel.Close()
			channel.Close()
			for _, end := range ends {
				if closed[end] != 1 {
					t.Fatalf("endpoint closed %d times", closed[end])
				}
			}
		})
	}
}

func TestParentOwnedStdioOpenRollback(t *testing.T) {
	for _, failAt := range []int{1, 2} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			ends := []*os.File{{}, {}}
			closed := map[*os.File]int{}
			failure := errors.New("pipe allocation failed")
			opens := 0
			channel, err := openParentOwnedStdio(func() (*os.File, *os.File, error) {
				opens++
				if opens == failAt {
					return nil, nil, failure
				}
				return ends[0], ends[1], nil
			}, func(file *os.File) error { closed[file]++; return nil })
			if channel != nil || !errors.Is(err, failure) {
				t.Fatalf("allocation failure = %v, %v", channel, err)
			}
			for _, end := range ends {
				if closed[end] != failAt-1 {
					t.Fatalf("rollback closes = %d, want %d", closed[end], failAt-1)
				}
			}
		})
	}
}

// A released tree needs no OS effects. Each cleanup operation must still emit
// its terminal diagnostic only through the logger supplied by that caller.
func TestSubprocessTreeWithEffectsIsolatesCleanupLogs(t *testing.T) {
	t.Parallel()
	cancelLogger, closeLogger := &recordingCommandLogger{}, &recordingCommandLogger{}
	clock := &coverageProcessClock{times: []time.Time{time.Unix(42, 0)}}
	if err := TerminateSubprocessTreeWithEffects(nil, SubprocessTree{}, clock, cancelLogger); err != nil {
		t.Fatalf("terminate released tree: %v", err)
	}
	CloseSubprocessTreeWithEffects(nil, SubprocessTree{}, clock, closeLogger)
	for _, scenario := range []struct {
		logger *recordingCommandLogger
		reason commandProcessCleanupReason
	}{
		{cancelLogger, commandProcessCleanupReasonCancel}, {closeLogger, commandProcessCleanupReasonPostRun},
	} {
		logs := commandCleanupCompletedLogs(scenario.logger)
		if len(logs) != 1 {
			t.Fatalf("%s cleanup records = %d, want 1", scenario.reason, len(logs))
		}
		if logs[0].fields["cleanup_reason"] != string(scenario.reason) || logs[0].fields["outcome"] != string(commandProcessCleanupOutcomeNoOp) {
			t.Fatalf("%s cleanup fields = %#v", scenario.reason, logs[0].fields)
		}
	}
}
