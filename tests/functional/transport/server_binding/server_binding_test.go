package server_binding_test

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
)

// TestBuiltExecutableFallsBackFromOccupiedLoopbackPortAndReportsActualURL proves the shipped socket fallback contract.
func TestBuiltExecutableFallsBackFromOccupiedLoopbackPortAndReportsActualURL(t *testing.T) {
	busyListener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve requested loopback port: %v", err)
	}
	defer busyListener.Close()
	requestedPort := busyListener.Addr().(*net.TCPAddr).Port
	if requestedPort >= 65535 {
		t.Skip("OS selected the terminal TCP port; no higher fallback candidate exists")
	}

	binaryPath := buildYouBinary(t)
	workingDirectory := t.TempDir()
	writeCurrentFactory(t, workingDirectory)
	homeDirectory := t.TempDir()
	requestedURL := "http://127.0.0.1:" + strconv.Itoa(requestedPort)

	command := exec.Command(
		binaryPath,
		"--server", requestedURL,
		"run", "--continuously", "--with-server", "--no-record",
	)
	command.Dir = workingDirectory
	command.Env = append(os.Environ(), "HOME="+homeDirectory, "USERPROFILE="+homeDirectory)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built CLI: %v", err)
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()

	lines := make(chan string, 128)
	scanErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanErr <- scanner.Err()
	}()

	actualURL := waitForDashboardURL(t, lines, scanErr)
	parsed, err := url.Parse(actualURL)
	if err != nil {
		t.Fatalf("parse reported dashboard URL %q: %v", actualURL, err)
	}
	actualPort, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("parse reported dashboard port %q: %v", parsed.Port(), err)
	}
	if parsed.Hostname() != "127.0.0.1" || actualPort <= requestedPort {
		t.Fatalf(
			"reported dashboard URL = %q, want IPv4 loopback above occupied port %d",
			actualURL,
			requestedPort,
		)
	}
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(actualURL)
	if err != nil {
		t.Fatalf("GET reported dashboard URL %q: %v", actualURL, err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET reported dashboard status = %d, want 200", response.StatusCode)
	}

	if runtime.GOOS == "windows" {
		_ = command.Process.Kill()
		if err := command.Wait(); err == nil {
			t.Fatal("killed continuous CLI unexpectedly exited successfully")
		}
	} else {
		interruptAndAssertCancellationExit(t, command)
	}
	stopped = true

	rebound, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(actualPort)))
	if err != nil {
		t.Fatalf("listener remained bound after built CLI exit: %v", err)
	}
	_ = rebound.Close()
}

// TestBuiltExecutableServerInterruptExits130AndReleasesListener proves the
// shipped server command preserves its declared cancellation exit after joining
// the owned listener.
func TestBuiltExecutableServerInterruptExits130AndReleasesListener(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt is not supported for child processes on Windows")
	}

	binaryPath := buildYouBinary(t)
	workingDirectory := t.TempDir()
	writeCurrentFactory(t, workingDirectory)
	homeDirectory := t.TempDir()
	requestedPort := reserveAvailablePort(t)
	requestedURL := "http://127.0.0.1:" + strconv.Itoa(requestedPort)

	command := exec.Command(binaryPath, "--server", requestedURL, "server")
	command.Dir = workingDirectory
	command.Env = append(
		os.Environ(),
		"HOME="+homeDirectory,
		"USERPROFILE="+homeDirectory,
		"PATH=",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built server CLI: %v", err)
	}
	stopped := false
	defer func() {
		if !stopped {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()

	lines := make(chan string, 128)
	scanErr := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanErr <- scanner.Err()
	}()

	actualURL := waitForDashboardURL(t, lines, scanErr)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Get(actualURL)
	if err != nil {
		t.Fatalf("GET reported dashboard URL %q: %v", actualURL, err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET reported dashboard status = %d, want 200", response.StatusCode)
	}

	interruptAndAssertCancellationExit(t, command)
	stopped = true

	parsed, err := url.Parse(actualURL)
	if err != nil {
		t.Fatalf("parse reported dashboard URL %q: %v", actualURL, err)
	}
	rebound, err := net.Listen("tcp4", parsed.Host)
	if err != nil {
		t.Fatalf("server listener remained bound after interrupt: %v", err)
	}
	_ = rebound.Close()
}

func reserveAvailablePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve available loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release available loopback port: %v", err)
	}
	return port
}

func interruptAndAssertCancellationExit(t *testing.T, command *exec.Cmd) {
	t.Helper()
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt built CLI: %v", err)
	}
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()
	select {
	case err := <-waitResult:
		exitErr, ok := err.(*exec.ExitError)
		if !ok || exitErr.ExitCode() != 130 {
			t.Fatalf("interrupted built CLI exit = %v, want exit code 130", err)
		}
	case <-time.After(10 * time.Second):
		_ = command.Process.Kill()
		<-waitResult
		t.Fatal("interrupted built CLI did not exit within 10s")
	}
}

func waitForDashboardURL(
	t *testing.T,
	lines <-chan string,
	scanErr <-chan error,
) string {
	t.Helper()
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line := <-lines:
			if target, ok := strings.CutPrefix(line, "Dashboard URL: "); ok {
				return target
			}
		case err := <-scanErr:
			t.Fatalf("built CLI exited before readiness: %v", err)
		case <-timer.C:
			t.Fatal("timed out waiting for built CLI readiness")
		}
	}
}

func buildYouBinary(t *testing.T) string {
	t.Helper()
	name := "you"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-o", path, "./cmd/factory")
	command.Dir = testutil.MustRepoRoot(t)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build you CLI: %v\n%s", err, output)
	}
	return path
}

func writeCurrentFactory(t *testing.T, workingDirectory string) {
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
