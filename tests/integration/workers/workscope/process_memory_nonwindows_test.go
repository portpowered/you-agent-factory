//go:build !windows

package workscope_test

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func prebuiltProcessWorkingSetHighWater(pid int) (uint64, string, error) {
	if runtime.GOOS == "linux" {
		contents, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		if err != nil {
			return 0, "linux-proc-vmhwm", fmt.Errorf("read prebuilt process %d status: %w", pid, err)
		}
		for _, line := range strings.Split(string(contents), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 || fields[0] != "VmHWM:" {
				continue
			}
			kilobytes, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, "linux-proc-vmhwm", fmt.Errorf("parse prebuilt process %d VmHWM %q: %w", pid, fields[1], err)
			}
			return kilobytes * 1024, "linux-proc-vmhwm-peak-working-set", nil
		}
		return 0, "linux-proc-vmhwm", fmt.Errorf("prebuilt process %d status has no VmHWM value", pid)
	}
	output, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, "ps-rss-sampled", fmt.Errorf("sample prebuilt process %d RSS: %w", pid, err)
	}
	kilobytes, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, "ps-rss-sampled", fmt.Errorf("parse prebuilt process %d RSS %q: %w", pid, strings.TrimSpace(string(output)), err)
	}
	return kilobytes * 1024, "ps-rss-sampled-high-water", nil
}
