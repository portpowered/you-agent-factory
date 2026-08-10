//go:build windows

package packagedinstallation

import "golang.org/x/sys/windows"

const processStillActive = 259

func probeOwnerPID(pid int) ownerLiveness {
	if pid <= 0 {
		return ownerLivenessIndeterminate
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false,
		uint32(pid),
	)
	if err != nil {
		// A missing process is not enough to prove that an incomplete staged
		// layout is safe to remove, so report it conservatively.
		return ownerLivenessIndeterminate
	}
	defer windows.CloseHandle(handle)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
		return ownerLivenessPermissionDenied
	}
	if exitCode == processStillActive {
		// A live PID is only owner evidence. PID reuse is never treated as
		// identity proof and the active resource is never removed.
		return ownerLivenessActive
	}
	return ownerLivenessIndeterminate
}
