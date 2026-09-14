//go:build windows

package review_failure_routing

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureRoutingCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

func interruptRoutingProcess(command *exec.Cmd) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(command.Process.Pid))
}

func routingCleanExit(err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && (exitErr.ExitCode() == 0 || exitErr.ExitCode() == 130)
}
