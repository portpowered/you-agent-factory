//go:build !windows

package packagedinstallation

import (
	"errors"
	"syscall"
)

func probeOwnerPID(pid int) ownerLiveness {
	if pid <= 0 {
		return ownerLivenessIndeterminate
	}
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		// A live PID is only owner evidence. The installer never treats PID
		// reuse as proof of identity and never removes an active resource.
		return ownerLivenessActive
	case errors.Is(err, syscall.ESRCH):
		// A missing PID does not prove that an incomplete stage is safe to
		// remove: process identity can be reused and the stage contents may
		// still be incomplete. Keep the resource and require an explicit clear.
		return ownerLivenessIndeterminate
	case errors.Is(err, syscall.EPERM):
		return ownerLivenessPermissionDenied
	default:
		return ownerLivenessIndeterminate
	}
}
