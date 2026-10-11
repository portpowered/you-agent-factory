//go:build !windows

package subagent_test

import (
	"os"
	"os/exec"
)

func prepareObserverSignal(_ *exec.Cmd)         {}
func interruptObserver(command *exec.Cmd) error { return command.Process.Signal(os.Interrupt) }
