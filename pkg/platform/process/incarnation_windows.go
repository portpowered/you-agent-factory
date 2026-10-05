//go:build windows

package process

import (
	"golang.org/x/sys/windows"
	"strconv"
)

func (IncarnationProbe) processStart(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), nil
}
