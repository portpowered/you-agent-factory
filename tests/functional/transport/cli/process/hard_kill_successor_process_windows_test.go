//go:build windows

package process_test

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

func isPreRuntimeStagingMetadataUnavailable(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

var ntSuspendProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSuspendProcess")
var ntResumeProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")

func suspendHardKillProcess(pid int) (hardKillProcessControl, error) {
	suspendHandle, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return hardKillProcessControl{}, fmt.Errorf("open process %d for suspension: %w", pid, err)
	}
	terminateHandle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		_ = windows.CloseHandle(suspendHandle)
		return hardKillProcessControl{}, fmt.Errorf("open process %d for termination: %w", pid, err)
	}
	status, _, callErr := ntSuspendProcess.Call(uintptr(suspendHandle))
	if status != 0 {
		_ = windows.CloseHandle(suspendHandle)
		_ = windows.CloseHandle(terminateHandle)
		if callErr != windows.ERROR_SUCCESS {
			return hardKillProcessControl{}, fmt.Errorf("suspend process %d: %w", pid, callErr)
		}
		return hardKillProcessControl{}, fmt.Errorf("suspend process %d returned NTSTATUS 0x%x", pid, status)
	}

	var mu sync.Mutex
	suspended := true
	terminated := false
	resume := func() error {
		mu.Lock()
		defer mu.Unlock()
		if !suspended {
			return nil
		}
		var resumeErr error
		if !terminated {
			status, _, callErr := ntResumeProcess.Call(uintptr(suspendHandle))
			if status != 0 {
				if callErr != windows.ERROR_SUCCESS {
					resumeErr = fmt.Errorf("resume process %d: %w", pid, callErr)
				} else {
					resumeErr = fmt.Errorf("resume process %d returned NTSTATUS 0x%x", pid, status)
				}
			}
		}
		closeErr := windows.CloseHandle(suspendHandle)
		_ = windows.CloseHandle(terminateHandle)
		suspended = false
		if resumeErr != nil {
			return resumeErr
		}
		return closeErr
	}
	terminate := func() error {
		mu.Lock()
		defer mu.Unlock()
		if terminated {
			return nil
		}
		if err := windows.TerminateProcess(terminateHandle, 1); err != nil {
			return err
		}
		terminated = true
		_ = windows.CloseHandle(terminateHandle)
		_ = windows.CloseHandle(suspendHandle)
		suspended = false
		return nil
	}
	return hardKillProcessControl{resume: resume, terminate: terminate}, nil
}
