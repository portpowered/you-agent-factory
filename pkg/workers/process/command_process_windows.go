//go:build windows

package process

import (
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

type commandProcessTree struct {
	job windows.Handle
}

func configureCommandProcessTree(_ *exec.Cmd) {}

func attachCommandProcessTree(cmd *exec.Cmd) (*commandProcessTree, error) {
	if cmd.Process == nil {
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

	process, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(cmd.Process.Pid),
	)
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

func terminateCommandProcessTree(cmd *exec.Cmd, tree *commandProcessTree) error {
	if tree != nil && tree.job != 0 {
		return windows.TerminateJobObject(tree.job, 1)
	}
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func closeCommandProcessTree(tree *commandProcessTree) {
	if tree == nil || tree.job == 0 {
		return
	}
	windows.CloseHandle(tree.job)
	tree.job = 0
}
