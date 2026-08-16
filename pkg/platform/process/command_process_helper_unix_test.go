//go:build !windows

package process

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func spawnCommandHelperEscapedChildMode() {
	pidFile := os.Getenv("COMMAND_HELPER_PID_FILE")
	child := exec.Command(os.Args[0],
		"-test.run=TestExecCommandRunner_HelperProcess",
		"--",
		"escaped-child",
	)
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	child.Env = append(os.Environ(),
		"GO_WANT_COMMAND_HELPER=1",
		"COMMAND_HELPER_PID_FILE="+pidFile,
		"COMMAND_HELPER_PID_WRITTEN_BY_PARENT=1",
	)
	if err := child.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start escaped child: %v\n", err)
		os.Exit(2)
	}
	if err := writeCommandHelperPIDFile(pidFile, child.Process.Pid); err != nil {
		fmt.Fprintf(os.Stderr, "write escaped child pid: %v\n", err)
		_ = child.Process.Kill()
		os.Exit(2)
	}
}
