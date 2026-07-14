//go:build windows

package agypty

import (
	"os"
	"os/exec"
	"testing"
)

func startBlockingTestProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	executable := `C:\Windows\System32\ping.exe`
	if _, err := os.Stat(executable); err != nil {
		t.Skip("ping.exe is unavailable")
	}
	cmd := exec.Command(executable, "-n", "120", "127.0.0.1")
	processConfigureForTest(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return cmd
}
