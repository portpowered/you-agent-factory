//go:build windows

package process

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"strconv"
)

func (IncarnationProbe) processStart(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return "", ErrProcessGone
		}
		return "", err
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	// A terminated process can retain an open handle and creation time. The
	// handle's signaled state, rather than that token alone, establishes death.
	state, err := windows.WaitForSingleObject(handle, 0)
	if err != nil {
		return "", err
	}
	if state == windows.WAIT_OBJECT_0 {
		return "", ErrProcessGone
	}
	if state != uint32(windows.WAIT_TIMEOUT) {
		return "", fmt.Errorf("process incarnation: unknown wait state")
	}
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10), nil
}
