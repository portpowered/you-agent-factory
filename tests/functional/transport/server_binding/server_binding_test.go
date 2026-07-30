package server_binding_test

import (
	"bufio"
	"bytes"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
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

	processHarness := newRootProcessHarness(t)
	workingDirectory := t.TempDir()
	writeCurrentFactory(t, workingDirectory)
	homeDirectory := t.TempDir()
	requestedURL := "http://127.0.0.1:" + strconv.Itoa(requestedPort)

	command := processHarness.Command(
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
		t.Fatalf("start root process: %v", err)
	}
	stopped := false
	defer func() {
		if !stopped {
			command.Cancel()
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

	cancelAndAssertShutdown(t, command)
	stopped = true

	rebound, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(actualPort)))
	if err != nil {
		t.Fatalf("listener remained bound after root process exit: %v", err)
	}
	_ = rebound.Close()
}

// TestBuiltExecutableServerInterruptExits130AndReleasesListener proves the
// shipped server command preserves its declared cancellation exit after joining
// the owned listener.
func TestBuiltExecutableServerInterruptExits130AndReleasesListener(t *testing.T) {
	processHarness := newRootProcessHarness(t)
	workingDirectory := t.TempDir()
	writeCurrentFactory(t, workingDirectory)
	homeDirectory := t.TempDir()
	requestedPort := reserveAvailablePort(t)
	requestedURL := "http://127.0.0.1:" + strconv.Itoa(requestedPort)

	command := processHarness.Command("--server", requestedURL, "server")
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
			command.Cancel()
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

	cancelAndAssertShutdown(t, command)
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

func cancelAndAssertShutdown(t *testing.T, command *builtcliacceptance.Command) {
	t.Helper()
	command.Cancel()
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
	}()
	select {
	case err := <-waitResult:
		if err != nil && !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("canceled root process exit = %v, want clean cancellation", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("canceled root process did not exit within 10s")
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
			t.Fatalf("root process exited before readiness: %v", err)
		case <-timer.C:
			t.Fatal("timed out waiting for root process readiness")
		}
	}
}

func newRootProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	return builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
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
