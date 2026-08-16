//go:build !windows

package process

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func commandProcessLeaderRunning(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return false
	}
	if state, ok := commandProcessState(cmd.Process.Pid); ok && state == 'Z' {
		return false
	}
	err := syscall.Kill(cmd.Process.Pid, 0)
	return err == nil || err == syscall.EPERM
}

// commandProcessState observes the Linux process state without reaping the
// child. exec.Cmd.Wait remains the sole owner of reaping, so this probe can
// safely run concurrently with the command runner's wait goroutine. Platforms
// without procfs simply fall back to the signal-0 liveness probe above.
func commandProcessState(pid int) (byte, bool) {
	if pid <= 0 {
		return 0, false
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, false
	}
	closeParen := bytes.LastIndexByte(data, ')')
	if closeParen < 0 || closeParen+2 >= len(data) {
		return 0, false
	}
	return data[closeParen+2], true
}
