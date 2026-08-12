package server_binding_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/terminalportlock"
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

	waitForListenerRelease(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(actualPort)))
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
	waitForListenerRelease(t, parsed.Host)
}

// TestBuiltExecutableServerBindFailureExitsNonZeroWithoutReadinessOutput proves
// the OS process observes a deterministic listener-startup failure instead of
// treating the command as a successful server launch. Port 65535 is used so
// the normal auto-port scan has no higher candidate to select.
func TestBuiltExecutableServerBindFailureExitsNonZeroWithoutReadinessOutput(t *testing.T) {
	binaryPath := buildServerBindingBinary(t, t.Context(), testutil.MustRepoRoot(t))

	// The run_scoped_server package asserts the same terminal-port exhaustion
	// contract in a separate test process; this OS lock makes endpoint ownership
	// explicit across both package processes before either listener is opened.
	releasePortLock, err := terminalportlock.Acquire()
	if err != nil {
		t.Fatalf("acquire terminal loopback test lock: %v", err)
	}
	defer func() {
		if err := releasePortLock(); err != nil {
			t.Errorf("release terminal loopback test lock: %v", err)
		}
	}()

	busyListener, err := net.Listen("tcp4", "127.0.0.1:65535")
	if err != nil {
		t.Fatalf("reserve terminal loopback port while owning test lock: %v", err)
	}
	defer func() {
		if err := busyListener.Close(); err != nil {
			t.Errorf("close blocking listener: %v", err)
		}
	}()

	workingDirectory := t.TempDir()
	writeCurrentFactory(t, workingDirectory)
	homeDirectory := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), serverBindFailureProcessTimeout)
	defer cancel()
	command := exec.CommandContext(
		ctx,
		binaryPath,
		"--server", "http://127.0.0.1:65535", "server",
	)
	command.Dir = workingDirectory
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(homeDirectory)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()
	if ctx.Err() != nil {
		processState := "unavailable"
		if command.ProcessState != nil {
			processState = command.ProcessState.String()
		}
		t.Fatalf(
			"server bind-failure process timed out: %v; listener=%s listener_owned=%t process_state=%s stdout=%q stderr=%q",
			ctx.Err(), busyListener.Addr().String(), true, processState, stdout.String(), stderr.String(),
		)
	}
	var exitErr *exec.ExitError
	if err == nil || !errors.As(err, &exitErr) {
		t.Fatalf("server bind-failure process error = %v; stdout=%q stderr=%q, want non-zero process exit", err, stdout.String(), stderr.String())
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("server bind-failure process exit code = 0; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("server bind-failure stdout = %q, want no readiness or success output", stdout.String())
	}
	const legacyBindWarning = "warning: --server is deprecated for local listener binding; use --listen <host:port> instead"
	stderrLines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(stderrLines) != 2 || strings.TrimSpace(stderrLines[0]) != legacyBindWarning {
		t.Fatalf("server bind-failure stderr = %q, want the legacy migration warning followed by one diagnostic line", stderr.String())
	}
	diagnostic := strings.TrimSpace(stderrLines[1])
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(diagnostic), &response); err != nil {
		t.Fatalf("server bind-failure stderr is not one ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Code != factoryapi.ErrorResponseCode("SERVER_BIND_FAILED") {
		t.Fatalf("server bind-failure response = %#v, want SERVER_BIND_FAILED", response)
	}
}

func waitForListenerRelease(t *testing.T, address string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp4", address)
		if err == nil {
			_ = listener.Close()
			return
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server listener %s remained bound after process exit: %v", address, lastErr)
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
