package process

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

type lifecycleObserverRecorder struct {
	started chan ProcessInfo
	exited  chan ProcessInfo
}

func (observer *lifecycleObserverRecorder) ProcessStarted(info ProcessInfo) {
	observer.started <- info
}

func (observer *lifecycleObserverRecorder) ProcessExited(info ProcessInfo) {
	observer.exited <- info
}

func TestProcessLifecycleMonitorObservesGoneParentBeforeWaitCompletes(t *testing.T) {
	requireProcessIntegration(t)

	pidFile := t.TempDir() + string(os.PathSeparator) + "child.pid"
	cmd := exec.Command(
		os.Args[0],
		"-test.run=TestExecCommandRunner_HelperProcess",
		"--",
		"spawn-child",
	)
	cmd.Env = append(os.Environ(),
		"GO_WANT_COMMAND_HELPER=1",
		"COMMAND_HELPER_PID_FILE="+pidFile,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	waitDone := make(chan struct{})
	observer := &lifecycleObserverRecorder{
		started: make(chan ProcessInfo, 1),
		exited:  make(chan ProcessInfo, 1),
	}
	monitor := startProcessLifecycleMonitor(cmd, waitDone, observer)
	t.Cleanup(func() {
		close(waitDone)
		monitor.stopAndWait()
		if cmd.Process != nil {
			commandTestTerminateProcess(cmd.Process.Pid)
		}
		if childPID, err := readCommandHelperPIDFile(pidFile); err == nil {
			commandTestTerminateProcess(childPID)
		}
		_ = cmd.Wait()
	})

	childPID := waitForCommandHelperPID(t, pidFile, 3*time.Second)
	select {
	case started := <-observer.started:
		if cmd.Process == nil || started.PID != cmd.Process.Pid {
			t.Fatalf("started process info = %#v, want command PID", started)
		}
	case <-time.After(time.Second):
		t.Fatal("process lifecycle monitor did not report process start")
	}

	commandTestTerminateProcess(cmd.Process.Pid)
	select {
	case exited := <-observer.exited:
		if exited.PID != cmd.Process.Pid {
			t.Fatalf("exited process info = %#v, want command PID %d", exited, cmd.Process.Pid)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("process lifecycle monitor did not report gone parent; child %d was retained", childPID)
	}
}
