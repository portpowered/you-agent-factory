//go:build windows

package restart_board_test

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func processResources() string {
	process := windows.CurrentProcess()
	var created, exited, kernel, user windows.Filetime
	err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user)
	if err != nil {
		return fmt.Sprintf("CPU/RSS unavailable: %v", err)
	}
	// PROCESS_MEMORY_COUNTERS ABI: size, faults, eight pointer-sized counters.
	var memory struct {
		size, faults                                                               uint32
		peakRSS, rss, peakPaged, paged, peakNonpaged, nonpaged, commit, peakCommit uintptr
	}
	memory.size = uint32(unsafe.Sizeof(memory))
	result, _, memoryErr := windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo").Call(
		uintptr(process), uintptr(unsafe.Pointer(&memory)), uintptr(memory.size))
	if result == 0 {
		return fmt.Sprintf("RSS unavailable: %v", memoryErr)
	}
	userTicks := uint64(user.HighDateTime)<<32 | uint64(user.LowDateTime)
	kernelTicks := uint64(kernel.HighDateTime)<<32 | uint64(kernel.LowDateTime)
	return fmt.Sprintf("process CPU cumulative user=%.3fs kernel=%.3fs peak_RSS_bytes=%d current_RSS_bytes=%d", float64(userTicks)/1e7, float64(kernelTicks)/1e7, memory.peakRSS, memory.rss)
}
