//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func configureUnitLaneCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		killErr := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		if killErr == nil {
			return os.ErrProcessDone
		}
		if processErr := cmd.Process.Kill(); processErr != nil {
			return killErr
		}
		return os.ErrProcessDone
	}
}
