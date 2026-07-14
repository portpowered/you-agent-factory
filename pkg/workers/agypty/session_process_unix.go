//go:build !windows

package agypty

import "errors"

func (p *sessionProcess) Wait() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return errors.New("agypty: process is not started")
	}
	err := p.cmd.Wait()
	if err == nil {
		p.exitCode = 0
		return nil
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		p.exitCode = exitErr.ExitCode()
		return nil
	}
	return err
}
