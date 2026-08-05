//go:build windows

package realclient_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func configureCommandProcessTree(command *exec.Cmd) {}

func terminateCommandProcessTree(command *exec.Cmd) error {
	if command.Process == nil || command.Process.Pid <= 0 {
		return nil
	}
	taskkill := exec.Command("taskkill", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
	taskkill.Stdout = io.Discard
	taskkill.Stderr = io.Discard
	if err := taskkill.Run(); err == nil {
		return nil
	}
	if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func processHasExited(pid int) (bool, error) {
	command := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	command.Stderr = io.Discard
	output, err := command.Output()
	if err != nil {
		return false, err
	}
	return !strings.Contains(string(output), fmt.Sprintf(",\"%d\",", pid)), nil
}
