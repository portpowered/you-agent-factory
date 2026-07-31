//go:build windows

package agypty

import (
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
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

func sessionProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && event == uint32(windows.WAIT_TIMEOUT)
}

func terminateSessionTestProcess(pid int) {
	if pid <= 0 {
		return
	}
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	_ = windows.TerminateProcess(handle, 1)
}
