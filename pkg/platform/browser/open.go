// Package browser supplies the policy-free host browser-launch effect selected
// by Wire for customer-facing presentation flows.
package browser

import (
	"context"
	"os/exec"
)

// Opener launches one URL using the host operating system.
type Opener func(context.Context, string) error

// Host is the policy-free browser-launch adapter for one Wire-selected host
// operating system.
type Host struct {
	operatingSystem string
}

// NewHost binds browser launching to the host operating system selected by
// Wire. Keeping this value explicit makes native command selection testable
// without consulting process-global state.
func NewHost(operatingSystem string) Host {
	return Host{operatingSystem: operatingSystem}
}

// Open launches url with the native browser command and releases the detached
// process after a successful start.
func (host Host) Open(ctx context.Context, url string) error {
	name, args := Command(host.operatingSystem, url)
	cmd := exec.CommandContext(ctx, name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Command maps one GOOS value to its native browser-launch command.
func Command(goos, url string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		return "open", []string{url}
	default:
		return "xdg-open", []string{url}
	}
}
