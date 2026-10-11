//go:build windows

package subagent_test

import (
	"fmt"
	"os/exec"
	"syscall"
)

func prepareObserverSignal(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}
func interruptObserver(command *exec.Cmd) error {
	result, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("GenerateConsoleCtrlEvent").Call(1, uintptr(command.Process.Pid))
	if result == 0 {
		return fmt.Errorf("send console break to observer: %w", err)
	}
	return nil
}
