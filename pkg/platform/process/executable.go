package process

import (
	"os"
	"os/exec"
)

// ExecutableLocator is the exact host search-path effect used after a caller
// has selected an executable name.
type ExecutableLocator interface {
	LookPath(string) (string, error)
}

// CurrentExecutableLocator is the platform effect for launching this process
// through its exact binary path, without searching PATH.
type CurrentExecutableLocator interface {
	CurrentExecutable() (string, error)
}

// HostExecutableLocator supplies the policy-free host implementation selected
// by Wire. Services receive the interface and never call os/exec directly.
type HostExecutableLocator struct{}

func (HostExecutableLocator) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (HostExecutableLocator) CurrentExecutable() (string, error) {
	return os.Executable()
}

var _ ExecutableLocator = HostExecutableLocator{}
