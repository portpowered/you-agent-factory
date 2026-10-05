//go:build linux

package process

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (IncarnationProbe) processStart(pid int) (string, error) {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	stat, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return "", err
	}
	// comm may contain spaces and parentheses. Fields after its closing paren
	// begin at field 3; starttime is field 22.
	end := strings.LastIndexByte(string(stat), ')')
	if end < 0 {
		return "", fmt.Errorf("invalid process identity")
	}
	fields := strings.Fields(string(stat[end+1:]))
	if len(fields) <= 19 {
		return "", fmt.Errorf("missing process creation token")
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", err
	}
	return strings.TrimSpace(string(boot)) + ":" + fields[19], nil
}
