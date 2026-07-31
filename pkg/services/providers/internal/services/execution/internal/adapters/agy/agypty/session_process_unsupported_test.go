//go:build !windows && !linux && !darwin

package agypty

import (
	"os/exec"
	"testing"
)

func startBlockingTestProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	t.Skip("blocking process helper is unavailable on unsupported platforms")
	return &exec.Cmd{}
}
func sessionProcessRunning(int) bool  { return false }
func terminateSessionTestProcess(int) {}
