//go:build windows

package recordingsprocess_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	windowsGracefulStopReadinessTimeout = 30 * time.Second
	windowsGracefulStopTimeout          = 20 * time.Second
	windowsGracefulStopScanTimeout      = 5 * time.Second
	windowsGracefulStopCleanupTimeout   = 5 * time.Second
)

const idleCurrentFactoryJSON = `{
  "name": "current",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "processor"}],
  "workstations": [{
    "name": "process",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "worker": "processor"
  }]
}`

type windowsGracefulStopTarget struct {
	command  *exec.Cmd
	server   string
	scanErr  <-chan error
	stderr   *bytes.Buffer
	waitDone chan error

	mu     sync.Mutex
	waited bool
}

func writeIdleCurrentFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	factoryDirectory := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(factoryDirectory, 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	path := filepath.Join(factoryDirectory, "factory.json")
	if err := os.WriteFile(path, []byte(idleCurrentFactoryJSON), 0o600); err != nil {
		t.Fatalf("write Current Factory: %v", err)
	}
}

func waitForDashboardURL(
	t testing.TB,
	lines <-chan string,
	scanErr <-chan error,
	stderr *bytes.Buffer,
	timeout time.Duration,
) string {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case line := <-lines:
			if target, ok := strings.CutPrefix(line, "Dashboard URL: "); ok {
				return target
			}
		case err := <-scanErr:
			t.Fatalf("root process exited before readiness: %v; stderr=%q", err, stderr.String())
		case <-timer.C:
			t.Fatalf("timed out waiting for root process readiness; stderr=%q", stderr.String())
		}
	}
}

func waitForScannerCompletion(t testing.TB, scanErr <-chan error, role string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-scanErr:
		if err != nil && !errors.Is(err, os.ErrClosed) {
			t.Fatalf("%s stdout scanner failed: %v", role, err)
		}
	case <-timer.C:
		t.Fatalf("%s stdout scanner did not finish within %s", role, timeout)
	}
}

func reserveWindowsGracefulStopPort(t testing.TB) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Windows graceful-stop port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func windowsTasklist(t testing.TB, pid int) string {
	t.Helper()
	command := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("tasklist for pid %d: %v; output=%q", pid, err, string(output))
	}
	return string(output)
}

func windowsTasklistContainsPID(output string, pid int) bool {
	reader := csv.NewReader(strings.NewReader(output))
	wantPID := strconv.Itoa(pid)
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			return strings.Contains(output, fmt.Sprintf(",\"%s\",", wantPID))
		}
		if len(record) > 1 && record[1] == wantPID {
			return true
		}
	}
}

func (target *windowsGracefulStopTarget) wait(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	select {
	case err := <-target.waitDone:
		target.mu.Lock()
		target.waited = true
		target.mu.Unlock()
		return err
	case <-ctx.Done():
		return fmt.Errorf("target process %d did not exit within %s: %w", target.command.Process.Pid, timeout, ctx.Err())
	}
}

func (target *windowsGracefulStopTarget) cleanup() {
	target.mu.Lock()
	waited := target.waited
	target.mu.Unlock()
	if waited || target.command.Process == nil {
		return
	}
	args := []string{"/PID", strconv.Itoa(target.command.Process.Pid), "/T", "/F"}
	_ = exec.Command("taskkill", args...).Run()
	select {
	case <-target.waitDone:
		target.mu.Lock()
		target.waited = true
		target.mu.Unlock()
	case <-time.After(windowsGracefulStopCleanupTimeout):
	}
}
