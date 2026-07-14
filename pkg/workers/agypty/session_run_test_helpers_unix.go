//go:build linux || darwin

package agypty

import (
	"os/exec"
	"testing"
)

func startBlockingTestProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	cmd := exec.Command("/bin/sleep", "120")
	processConfigureForTest(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return cmd
}
