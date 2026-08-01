//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

func configureUnitLaneCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if processErr := cmd.Process.Kill(); processErr != nil {
				return err
			}
		}
		return os.ErrProcessDone
	}
}
