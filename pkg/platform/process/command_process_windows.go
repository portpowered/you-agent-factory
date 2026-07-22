//go:build windows

package process

import (
	"errors"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
func terminateCommandJobGroup(job windows.Handle, grace time.Duration, clock Clock, logCtx commandProcessCleanupContext) error {
	if job == 0 {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "job handle not attached")
		return nil
	}
	supervisorID := int(job)
	// Windows can briefly report zero active processes for a newly assigned job
	// even while its process is running. Cancellation must still terminate the
	// job; otherwise the runner waits for the command to exit on its own.
	if logCtx.reason != commandProcessCleanupReasonCancel && commandJobActiveProcesses(job) == 0 {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, supervisorID, nil, "job has no active processes")
		return nil
	}

	logCtx.logStarted(supervisorID)

	if grace > 0 {
		if clock == nil {
			return errors.New("platform process clock is required")
		}
		logCtx.logGraceful(supervisorID)
		deadline := clock.Now().Add(grace)
		for clock.Now().Before(deadline) {
			if commandJobActiveProcesses(job) == 0 {
				logCtx.logCompleted(commandProcessCleanupOutcomeGracefulSuccess, supervisorID, nil, "")
				return nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	logCtx.logForceKill(supervisorID)
	if logCtx.reason != commandProcessCleanupReasonCancel && commandJobActiveProcesses(job) == 0 {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, supervisorID, nil, "job members exited before force kill")
		return nil
	}
	if err := windows.TerminateJobObject(job, 1); err != nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeFailure, supervisorID, err, "TerminateJobObject failed")
		return err
	}
	if active := commandJobActiveProcesses(job); active > 0 {
		logCtx.logCompleted(
			commandProcessCleanupOutcomePartialFailure,
			supervisorID,
			nil,
			"job still has active processes after TerminateJobObject",
		)
		return errCommandProcessCleanupPartialFailure
	}
	logCtx.logCompleted(commandProcessCleanupOutcomeForceKillSuccess, supervisorID, nil, "")
	return nil
}

func terminateCommandProcessTree(cmd *exec.Cmd, tree *commandProcessTree, clock Clock, logCtx commandProcessCleanupContext) error {
	if tree != nil && tree.job != 0 {
		return terminateCommandJobGroup(tree.job, 0, clock, logCtx)
	}
	if cmd == nil || cmd.Process == nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "process already exited")
		return nil
	}
	supervisorID := cmd.Process.Pid
	logCtx.logStarted(supervisorID)
	logCtx.logForceKill(supervisorID)
	if err := cmd.Process.Kill(); err != nil {
		logCtx.logCompleted(commandProcessCleanupOutcomeFailure, supervisorID, err, "parent process kill failed")
		return err
	}
	logCtx.logCompleted(commandProcessCleanupOutcomeForceKillSuccess, supervisorID, nil, "")
	return nil
}

func closeCommandProcessTree(_ *exec.Cmd, tree *commandProcessTree, clock Clock, logCtx commandProcessCleanupContext) {
	if tree == nil || tree.job == 0 {
		logCtx.logCompleted(commandProcessCleanupOutcomeNoOp, 0, nil, "job handle not attached")
		return
	}
	_ = terminateCommandJobGroup(tree.job, postRunCleanupGracePeriod(), clock, logCtx)
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
