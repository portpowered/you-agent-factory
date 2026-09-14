//go:build !windows

package review_failure_routing

import (
	"errors"
	"os"
	"os/exec"
)

func configureRoutingCommand(_ *exec.Cmd) {}

func interruptRoutingProcess(command *exec.Cmd) error {
	return command.Process.Signal(os.Interrupt)
}

func routingCleanExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 130
}
