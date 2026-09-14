//go:build windows

package process

import (
	"errors"
	"fmt"
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
	job     windows.Handle
	rootPID uint32
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
	return &commandProcessTree{job: job, rootPID: uint32(cmd.Process.Pid)}, nil
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
	// A process can spawn a descendant between CreateProcess and the
	// AssignProcessToJobObject call above. That descendant is not a job member,
	// so JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE cannot reach it after the root exits.
	// Reconcile the parent-PID tree before publishing post-run completion so the
	// managed-child boundary still guarantees that the full owned tree is gone.
	if err := terminateEscapedProcessDescendants(tree.rootPID, postRunCleanupGracePeriod()); err != nil {
		logCtx.logger.Warn(
			"command runner: escaped descendant cleanup failed",
			"event_name", "command_runner.escaped_descendant_cleanup_failed",
			"process_group_id", tree.rootPID,
			"error", err.Error(),
		)
	}
	windows.CloseHandle(tree.job)
	tree.job = 0
}

func terminateEscapedProcessDescendants(rootPID uint32, wait time.Duration) error {
	if rootPID == 0 {
		return nil
	}
	descendants, err := windowsDescendantProcessIDs(rootPID)
	if err != nil {
		return err
	}
	var cleanupErr error
	for _, pid := range descendants {
		process, openErr := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
		if openErr != nil {
			if errors.Is(openErr, windows.ERROR_INVALID_PARAMETER) {
				continue
			}
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("open escaped descendant %d: %w", pid, openErr))
			continue
		}

		terminateErr := windows.TerminateProcess(process, 1)
		waitResult, waitErr := windows.WaitForSingleObject(process, windowsWaitMilliseconds(wait))
		closeErr := windows.CloseHandle(process)
		if closeErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("close escaped descendant %d: %w", pid, closeErr))
		}
		if waitErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("wait for escaped descendant %d: %w", pid, waitErr))
			continue
		}
		if waitResult == windows.WAIT_OBJECT_0 {
			continue
		}
		if terminateErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("terminate escaped descendant %d: %w", pid, terminateErr))
		}
		if waitResult == uint32(windows.WAIT_TIMEOUT) {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("escaped descendant %d remained alive after %s", pid, wait))
		}
	}
	return cleanupErr
}

func windowsDescendantProcessIDs(rootPID uint32) ([]uint32, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snapshot)

	children := make(map[uint32][]uint32)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	if err := windows.Process32First(snapshot, &entry); err != nil {
		return nil, err
	}
	for {
		if entry.ProcessID != 0 && entry.ParentProcessID != 0 {
			children[entry.ParentProcessID] = append(children[entry.ParentProcessID], entry.ProcessID)
		}
		if err := windows.Process32Next(snapshot, &entry); err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
				break
			}
			return nil, err
		}
	}

	visited := make(map[uint32]struct{})
	var descendants []uint32
	var visit func(uint32)
	visit = func(parentPID uint32) {
		for _, childPID := range children[parentPID] {
			if childPID == rootPID {
				continue
			}
			if _, ok := visited[childPID]; ok {
				continue
			}
			visited[childPID] = struct{}{}
			visit(childPID)
			descendants = append(descendants, childPID)
		}
	}
	visit(rootPID)
	return descendants, nil
}

func windowsWaitMilliseconds(wait time.Duration) uint32 {
	if wait <= 0 {
		return 0
	}
	milliseconds := uint64(wait / time.Millisecond)
	if milliseconds >= uint64(windows.INFINITE) {
		return windows.INFINITE - 1
	}
	return uint32(milliseconds)
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

const processStillActive = 259

func commandProcessLeaderRunning(cmd *exec.Cmd, _ ProcessStateReader) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false,
		uint32(cmd.Process.Pid),
	)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(process)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(process, &exitCode); err != nil {
		return false
	}
	return exitCode == processStillActive
}
