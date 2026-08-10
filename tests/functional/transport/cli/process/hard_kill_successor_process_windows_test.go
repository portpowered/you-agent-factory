//go:build windows

package process_test

import (
	"fmt"

	"golang.org/x/sys/windows"
)

var ntSuspendProcess = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtSuspendProcess")

func suspendHardKillProcess(pid int) (func(), error) {
	handle, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return nil, fmt.Errorf("open process %d for suspension: %w", pid, err)
	}
	status, _, callErr := ntSuspendProcess.Call(uintptr(handle))
	if status != 0 {
		_ = windows.CloseHandle(handle)
		if callErr != windows.ERROR_SUCCESS {
			return nil, fmt.Errorf("suspend process %d: %w", pid, callErr)
		}
		return nil, fmt.Errorf("suspend process %d returned NTSTATUS 0x%x", pid, status)
	}
	return func() { _ = windows.CloseHandle(handle) }, nil
}
