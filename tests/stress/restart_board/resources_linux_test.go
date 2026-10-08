//go:build linux

package restart_board_test

import (
	"fmt"
	"syscall"
)

func processResources() string {
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		return fmt.Sprintf("CPU/RSS unavailable: %v", err)
	}
	return fmt.Sprintf("process CPU cumulative user=%.3fs kernel=%.3fs peak_RSS_bytes=%d", float64(usage.Utime.Sec)+float64(usage.Utime.Usec)/1e6, float64(usage.Stime.Sec)+float64(usage.Stime.Usec)/1e6, usage.Maxrss*1024)
}
