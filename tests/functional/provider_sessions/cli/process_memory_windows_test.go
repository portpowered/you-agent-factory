//go:build windows

package cli_test

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type boundedFleetProcessMemoryCounters struct {
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

func processWorkingSetHighWater() (uint64, error) {
	var counters boundedFleetProcessMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return 0, fmt.Errorf("get current process: %w", err)
	}
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
		return 0, fmt.Errorf("read process memory counters: %w", callErr)
	}
	return uint64(counters.peakWorkingSetSize), nil
}
