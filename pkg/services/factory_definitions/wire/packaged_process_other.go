//go:build !windows && !linux

package wire

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func packagedInstallationProcessProbe(factorydefinitions.PackagedInstallationFileSystem) factorydefinitions.PackagedInstallationProcessProbe {
	return (packagedPIDProbe{}).LookupProcess
}

// Systems without creation-token support retain PID-only observation. The
// installer still owns the decision whether this observation permits recovery.
type packagedPIDProbe struct{}

func (packagedPIDProbe) LookupProcess(pid int) (platformprocess.Incarnation, error) {
	if pid <= 0 {
		return platformprocess.Incarnation{}, fmt.Errorf("invalid process PID")
	}
	err := syscall.Kill(pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return platformprocess.Incarnation{}, platformprocess.ErrProcessGone
	}
	if errors.Is(err, syscall.EPERM) {
		return platformprocess.Incarnation{}, os.ErrPermission
	}
	if err != nil {
		return platformprocess.Incarnation{}, err
	}
	host, err := os.Hostname()
	return platformprocess.Incarnation{PID: pid, Host: host}, err
}
