//go:build windows

package processmemory

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processMemoryCounters mirrors the Windows PROCESS_MEMORY_COUNTERS layout.
// PagefileUsage is the current committed process-memory value; WorkingSetSize
// is intentionally retained only as a neighbouring field to preserve the ABI
// layout and is never used as the commit signal.
type processMemoryCounters struct {
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

// CurrentCommit returns the Windows PagefileUsage-equivalent committed bytes
// for this process. It deliberately does not use RSS or working-set values.
func CurrentCommit() (uint64, error) {
	var counters processMemoryCounters
	if err := readProcessMemoryCounters(&counters); err != nil {
		return 0, err
	}
	return pagefileUsageBytes(counters), nil
}

func readProcessMemoryCounters(counters *processMemoryCounters) error {
	if counters == nil {
		return errors.New("process memory counters are required")
	}
	counters.cb = uint32(unsafe.Sizeof(*counters))
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return fmt.Errorf("get current process: %w", err)
	}
	getProcessMemoryInfo := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	result, _, callErr := getProcessMemoryInfo.Call(
		uintptr(process),
		uintptr(unsafe.Pointer(counters)),
		uintptr(unsafe.Sizeof(*counters)),
	)
	if result == 0 {
		if callErr == nil {
			callErr = errors.New("GetProcessMemoryInfo returned failure")
		}
		return fmt.Errorf("read process memory counters: %w", callErr)
	}
	return nil
}

func pagefileUsageBytes(counters processMemoryCounters) uint64 {
	return uint64(counters.pagefileUsage)
}
