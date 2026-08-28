//go:build windows

package acp_test

import (
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

func acpHelperProcessExited(pid int) (bool, error) {
	command := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	command.Stderr = io.Discard
	output, err := command.Output()
	if err != nil {
		return false, err
	}
	return !strings.Contains(string(output), ",\""+strconv.Itoa(pid)+"\",\""), nil
}
