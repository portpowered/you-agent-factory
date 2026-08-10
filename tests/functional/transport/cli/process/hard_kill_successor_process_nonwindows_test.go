//go:build !windows

package process_test

import (
	"os"
	"syscall"
)

func suspendHardKillProcess(pid int) (func() error, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil, err
	}
	if err := process.Signal(syscall.SIGSTOP); err != nil {
		return nil, err
	}
	return func() error {
		return process.Signal(syscall.SIGCONT)
	}, nil
}
