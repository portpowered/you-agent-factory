//go:build windows

package platform_conformance

import (
	"errors"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

type controlledProcessTree struct {
	job windows.Handle
}

func configureControlledProcessTree(*exec.Cmd) {}

func attachControlledProcessTree(command *exec.Cmd) (controlledProcessTree, error) {
	if command == nil || command.Process == nil || command.Process.Pid <= 0 {
		return controlledProcessTree{}, errors.New("controlled process has no attachable pid")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return controlledProcessTree{}, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return controlledProcessTree{}, err
	}
	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		_ = windows.CloseHandle(job)
		return controlledProcessTree{}, err
	}
	defer windows.CloseHandle(processHandle)
	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		_ = windows.CloseHandle(job)
		return controlledProcessTree{}, err
	}
	return controlledProcessTree{job: job}, nil
}

func terminateControlledProcessTree(command *exec.Cmd, tree controlledProcessTree) error {
	if tree.job != 0 {
		if err := windows.TerminateJobObject(tree.job, 1); err != nil {
			return err
		}
		event, err := windows.WaitForSingleObject(tree.job, 5000)
		if err != nil {
			return err
		}
		if event != windows.WAIT_OBJECT_0 {
			return errors.New("controlled process job did not signal termination")
		}
		return nil
	}
	if command == nil || command.Process == nil {
		return nil
	}
	if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func requestControlledProcessTreeStop(command *exec.Cmd, tree controlledProcessTree) error {
	return terminateControlledProcessTree(command, tree)
}

func closeControlledProcessTree(tree controlledProcessTree, attached bool) {
	if !attached || tree.job == 0 {
		return
	}
	_ = windows.CloseHandle(tree.job)
}

func controlledProcessTreeAlive(tree controlledProcessTree) bool {
	if tree.job == 0 {
		return false
	}
	if event, err := windows.WaitForSingleObject(tree.job, 0); err == nil && event == windows.WAIT_OBJECT_0 {
		return false
	}
	var info controlledJobAccountingInformation
	if err := windows.QueryInformationJobObject(
		tree.job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	); err != nil {
		return true
	}
	return info.ActiveProcesses > 0
}

type controlledJobAccountingInformation struct {
	TotalUserTime            int64
	TotalKernelTime          int64
	TotalPageFaultCount      uint32
	TotalProcesses           uint32
	ActiveProcesses          uint32
	TotalTerminatedProcesses uint32
}
