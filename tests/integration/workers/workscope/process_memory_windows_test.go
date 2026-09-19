//go:build windows

package workscope_test

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type prebuiltWorkscopeProcessMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

func prebuiltProcessWorkingSetHighWater(pid int) (uint64, string, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, uint32(pid))
	if err != nil {
		return 0, "windows-GetProcessMemoryInfo", fmt.Errorf("open prebuilt process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	counters := prebuiltWorkscopeProcessMemoryCounters{cb: uint32(unsafe.Sizeof(prebuiltWorkscopeProcessMemoryCounters{}))}
	getProcessMemoryInfo := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	result, _, callErr := getProcessMemoryInfo.Call(
		uintptr(process),
		uintptr(unsafe.Pointer(&counters)),
		uintptr(unsafe.Sizeof(counters)),
	)
	if result == 0 {
		if callErr == nil {
			callErr = errors.New("GetProcessMemoryInfo returned failure")
		}
		return 0, "windows-GetProcessMemoryInfo", fmt.Errorf("read prebuilt process %d memory counters: %w", pid, callErr)
	}
	return uint64(counters.peakWorkingSetSize), "windows-GetProcessMemoryInfo-peak-working-set", nil
}
