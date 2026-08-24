//go:build windows

package process

import (
	"context"
	"errors"
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
