//go:build windows

package agypty

import (
	"errors"

	"golang.org/x/sys/windows"
)

func (p *sessionProcess) Wait() error {
	if p == nil {
		return errors.New("agypty: process is not started")
	}
	if p.winHandle != 0 {
		event, err := windows.WaitForSingleObject(windows.Handle(p.winHandle), windows.INFINITE)
		if err != nil {
			return err
		}
		if event != windows.WAIT_OBJECT_0 {
			return errors.New("agypty: unexpected wait result")
		}
		var exitCode uint32
		if err := windows.GetExitCodeProcess(windows.Handle(p.winHandle), &exitCode); err != nil {
			return err
		}
		p.exitCode = int(exitCode)
		return nil
	}
	if p.cmd == nil || p.cmd.Process == nil {
		return errors.New("agypty: process is not started")
	}
	err := p.cmd.Wait()
	if err == nil {
		return nil
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		p.exitCode = exitErr.ExitCode()
		return nil
	}
	return err
}
