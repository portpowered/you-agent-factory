//go:build windows

package tts_clean_install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processTree struct {
	job windows.Handle
}

func configureProcessCommand(*exec.Cmd) {}

func newProcessTree() (processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processTree{}, err
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
		return processTree{}, err
	}
	return processTree{job: job}, nil
}

func (tree *processTree) attach(process *os.Process) error {
	if tree == nil || tree.job == 0 {
		return errors.New("process tree job is not initialized")
	}
	if process == nil || process.Pid <= 0 {
		return errors.New("process has no attachable pid")
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false,
		uint32(process.Pid),
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(tree.job, handle); err != nil {
		return err
	}
	return nil
}

func (tree *processTree) terminate(process *os.Process) error {
	if tree == nil || tree.job == 0 {
		return killProcess(process)
	}
	if !tree.active() {
		return nil
	}
	if err := windows.TerminateJobObject(tree.job, 1); err != nil {
		if !tree.active() {
			return nil
		}
		return fmt.Errorf("terminate helper job: %w", err)
	}
	const waitMilliseconds = 5000
	event, err := windows.WaitForSingleObject(tree.job, waitMilliseconds)
	if err != nil {
		return fmt.Errorf("wait for helper job: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return errors.New("helper job did not signal termination")
	}
	return nil
}

func (tree *processTree) active() bool {
	if tree == nil || tree.job == 0 {
		return false
	}
	event, err := windows.WaitForSingleObject(tree.job, 0)
	return err != nil || event != windows.WAIT_OBJECT_0
}

func (tree *processTree) close() {
	if tree == nil || tree.job == 0 {
		return
	}
	_ = windows.CloseHandle(tree.job)
	tree.job = 0
}

func killProcess(process *os.Process) error {
	if process == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}
