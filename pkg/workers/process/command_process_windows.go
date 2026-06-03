//go:build windows

package process

import (
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// commandProcessJobGracePeriod is the default bounded wait for job members to exit
// after the parent command completes before force-terminating the job (post-run cleanup).
const commandProcessJobGracePeriod = 2 * time.Second

// jobobjectBasicAccountingInformation mirrors JOBOBJECT_BASIC_ACCOUNTING_INFORMATION
// (see golang.org/x/sys/windows JobObjectBasicAccountingInformation).
type jobobjectBasicAccountingInformation struct {
	TotalUserTime            int64
	TotalKernelTime          int64
	TotalPageFaultCount      uint32
	TotalProcesses           uint32
	ActiveProcesses          uint32
	TotalTerminatedProcesses uint32
}

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

// terminateCommandJobGroup waits up to grace for job members to exit, then
// force-terminates remaining members. When grace is zero, TerminateJobObject runs
// immediately so cancel/timeout behavior matches the prior path.
//
// Post-run cleanup closes the job handle after this call; JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
// terminates any survivors when the handle is released. Children started with
// CREATE_BREAKAWAY_FROM_JOB or that detach to a new job may escape cleanup.
func terminateCommandJobGroup(job windows.Handle, grace time.Duration) error {
	if job == 0 {
		return nil
	}
	if grace > 0 {
		deadline := time.Now().Add(grace)
		for time.Now().Before(deadline) {
			if commandJobActiveProcesses(job) == 0 {
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	if commandJobActiveProcesses(job) == 0 {
		return nil
	}
	return windows.TerminateJobObject(job, 1)
}

func terminateCommandProcessTree(cmd *exec.Cmd, tree *commandProcessTree) error {
	if tree != nil && tree.job != 0 {
		return terminateCommandJobGroup(tree.job, 0)
	}
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func closeCommandProcessTree(_ *exec.Cmd, tree *commandProcessTree) {
	if tree == nil || tree.job == 0 {
		return
	}
	_ = terminateCommandJobGroup(tree.job, commandProcessJobGracePeriod)
	windows.CloseHandle(tree.job)
	tree.job = 0
}

func commandJobActiveProcesses(job windows.Handle) uint32 {
	var info jobobjectBasicAccountingInformation
	err := windows.QueryInformationJobObject(
		job,
		windows.JobObjectBasicAccountingInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	)
	if err != nil {
		return 0
	}
	return info.ActiveProcesses
}
