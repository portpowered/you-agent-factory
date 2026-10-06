//go:build !linux

package process

import "os/exec"

func waitForOwnedCommandExit(_ *exec.Cmd, _ *ownedCommandControl) {}
