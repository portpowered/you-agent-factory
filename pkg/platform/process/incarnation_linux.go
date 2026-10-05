//go:build linux

package process

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func (probe IncarnationProbe) processStart(pid int) (string, error) {
	if probe.ReadFile == nil {
		return "", fmt.Errorf("process incarnation: file reader is required")
	}
	boot, err := probe.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", err
	}
	stat, err := probe.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrProcessGone
		}
		return "", err
	}
	return processCreationToken(string(boot), string(stat))
}

func processCreationToken(boot, stat string) (string, error) {
	// comm may contain spaces and parentheses. Fields after its closing paren
	// begin at field 3; starttime is field 22.
	end := strings.LastIndexByte(stat, ')')
	if end < 0 {
		return "", fmt.Errorf("invalid process identity")
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) <= 19 {
		return "", fmt.Errorf("missing process creation token")
	}
	if fields[0] == "Z" || fields[0] == "X" || fields[0] == "x" {
		return "", ErrProcessGone
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", err
	}
	if strings.TrimSpace(boot) == "" {
		return "", fmt.Errorf("missing boot identity")
	}
	return strings.TrimSpace(boot) + ":" + fields[19], nil
}
