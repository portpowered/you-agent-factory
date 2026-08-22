//go:build windows

package restart_test

import (
	"errors"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureBoardPersistenceCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
}

func interruptBoardPersistenceProcess(command *exec.Cmd) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(command.Process.Pid))
}

func boardPersistenceCleanExit(err error) bool {
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	return exitErr.ExitCode() == 0 || exitErr.ExitCode() == 130
}
