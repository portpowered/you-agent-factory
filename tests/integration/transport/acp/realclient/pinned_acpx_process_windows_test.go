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
	"unsafe"

	"golang.org/x/sys/windows"
)

type commandProcessTree struct {
	job windows.Handle
}

func configureCommandProcessTree(command *exec.Cmd) {}

func attachCommandProcessTree(command *exec.Cmd) (*commandProcessTree, error) {
	if command.Process == nil || command.Process.Pid <= 0 {
		return nil, nil
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	return &commandProcessTree{job: job}, nil
}

func closeCommandProcessTree(command *exec.Cmd, tree *commandProcessTree) {
	if tree == nil || tree.job == 0 {
		return
	}
	_ = terminateCommandProcessTree(command, tree)
	_ = windows.CloseHandle(tree.job)
	tree.job = 0
}

func terminateCommandProcessTree(command *exec.Cmd, tree *commandProcessTree) error {
	if tree != nil && tree.job != 0 {
		return windows.TerminateJobObject(tree.job, 1)
	}
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
