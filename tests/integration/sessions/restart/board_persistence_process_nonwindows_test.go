//go:build !windows

package restart_test

import (
	"errors"
	"os"
	"os/exec"
)

func configureBoardPersistenceCommand(_ *exec.Cmd) {}

func interruptBoardPersistenceProcess(command *exec.Cmd) error {
	return command.Process.Signal(os.Interrupt)
}

func boardPersistenceCleanExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 130
}
