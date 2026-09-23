package restart_test

import (
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

const (
	boardPersistenceDefaultPort          = 7437
	boardPersistenceAdjacentReservedPort = 7438
)

type boardPersistenceDaemon struct {
	cmd               *exec.Cmd
	baseURL           string
	sessionID         string
	factoryDir        string
	homeDir           string
	logDir            string
	recordPath        string
	startedAt         time.Time
	readyAt           time.Time
	shutdownStartedAt time.Time
	stoppedAt         time.Time
	stdout            *bytes.Buffer
	stderr            *bytes.Buffer
	done              chan struct{}
	mu                sync.Mutex
	waitErr           error
	stopped           bool
}

type boardPersistenceWorkerBarrier struct {
	server      *httptest.Server
	ready       chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func newBoardPersistenceWorkerBarrier(t *testing.T) *boardPersistenceWorkerBarrier {
	t.Helper()
	barrier := &boardPersistenceWorkerBarrier{
		ready:   make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	barrier.server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			response.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		select {
		case barrier.ready <- struct{}{}:
		case <-request.Context().Done():
			return
		}
		select {
		case <-barrier.release:
			response.WriteHeader(http.StatusNoContent)
		case <-request.Context().Done():
		}
	}))
	t.Cleanup(func() {
		barrier.releaseWorkers()
		barrier.server.Close()
	})
	return barrier
}

func (barrier *boardPersistenceWorkerBarrier) endpoint() string {
	return barrier.server.URL
}

func (barrier *boardPersistenceWorkerBarrier) waitForReadyWorkers(t *testing.T, expected int, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for ready := 0; ready < expected; ready++ {
		select {
		case <-barrier.ready:
		case <-timer.C:
			t.Fatalf("script workers at hold barrier = %d, want %d before release", ready, expected)
		case <-t.Context().Done():
			t.Fatalf("wait for script worker hold barrier: %v", t.Context().Err())
		}
	}
}

func (barrier *boardPersistenceWorkerBarrier) releaseWorkers() {
	barrier.releaseOnce.Do(func() { close(barrier.release) })
}

func startBoardPersistenceDaemon(
	t *testing.T,
	binaryPath, factoryDir, homeDir, recordPath, releasePath string,
) *boardPersistenceDaemon {
	t.Helper()
	daemon := startBoardPersistenceDaemonProcessWithResume(t, binaryPath, factoryDir, homeDir, "", recordPath, releasePath)
	waitForBoardDaemonReady(t, daemon, 45*time.Second)
	daemon.sessionID = waitForBoardSessionID(t, daemon.baseURL, 30*time.Second)
	t.Logf("isolated daemon live session ID: %q", daemon.sessionID)
	return daemon
}

func startBoardPersistenceResumeDaemon(
	t *testing.T,
	binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath string,
	workerReadyEndpoint ...string,
) *boardPersistenceDaemon {
	t.Helper()
	daemon := startBoardPersistenceDaemonProcessWithResumeOutput(t, binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath, false, false, workerReadyEndpoint...)
	waitForBoardDaemonReady(t, daemon, 45*time.Second)
	daemon.sessionID = waitForBoardSessionID(t, daemon.baseURL, 30*time.Second)
	t.Logf("isolated daemon live session ID: %q", daemon.sessionID)
	return daemon
}

func startBoardPersistenceDaemonProcess(
	t *testing.T,
	binaryPath, factoryDir, homeDir, recordPath, releasePath string,
) *boardPersistenceDaemon {
	t.Helper()
	return startBoardPersistenceDaemonProcessWithResume(t, binaryPath, factoryDir, homeDir, "", recordPath, releasePath)
}

func startBoardPersistenceDaemonProcessWithResume(
	t *testing.T,
	binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath string,
) *boardPersistenceDaemon {
	return startBoardPersistenceDaemonProcessWithResumeOutput(t, binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath, false, false)
}

func startBoardPersistenceJSONResumeProcess(
	t *testing.T,
	binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath string,
) *boardPersistenceDaemon {
	return startBoardPersistenceDaemonProcessWithResumeOutput(t, binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath, true, true)
}

func startBoardPersistenceObservedResumeDaemon(
	t *testing.T,
	binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath string,
) *boardPersistenceDaemon {
	t.Helper()
	daemon := startBoardPersistenceDaemonProcessWithResumeOutput(t, binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath, false, true)
	waitForBoardDaemonReady(t, daemon, 45*time.Second)
	daemon.sessionID = waitForBoardSessionID(t, daemon.baseURL, 30*time.Second)
	t.Logf("isolated observed resume session ID: %q", daemon.sessionID)
	return daemon
}

func startBoardPersistenceDaemonProcessWithResumeOutput(
	t *testing.T,
	binaryPath, factoryDir, homeDir, resumePath, recordPath, releasePath string,
	jsonOutput, debugOutput bool,
	workerReadyEndpoint ...string,
) *boardPersistenceDaemon {
	t.Helper()
	address := reserveBoardPersistenceAddress(t)
	args := make([]string, 0, 12)
	if jsonOutput {
		args = append(args, "--json")
	}
	if debugOutput {
		args = append(args, "--debug")
	}
	args = append(args,
		"run", "--dir", factoryDir,
		"--continuously", "--with-server",
		"--listen", address,
	)
	if resumePath != "" {
		args = append(args, "--resume", resumePath)
	}
	args = append(args, "--record", recordPath)
	command := exec.CommandContext(t.Context(), binaryPath, args...)
	startedAt := time.Now()
	command.Dir = factoryDir
	commandEnv := append(
		builtcliacceptance.ProcessEnvForIsolatedHome(homeDir),
		boardPersistenceHelperEnv+"="+boardPersistenceHelperEnvValue,
		boardPersistenceReleaseEnv+"="+releasePath,
	)
	if len(workerReadyEndpoint) > 0 && workerReadyEndpoint[0] != "" {
		commandEnv = append(commandEnv, boardPersistenceWorkerReadyEnv+"="+workerReadyEndpoint[0])
	}
	command.Env = commandEnv
	configureBoardPersistenceCommand(command)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	daemon := &boardPersistenceDaemon{
		cmd:        command,
		baseURL:    "http://" + address,
		factoryDir: factoryDir,
		homeDir:    homeDir,
		recordPath: recordPath,
		startedAt:  startedAt,
		stdout:     &stdout,
		stderr:     &stderr,
		done:       make(chan struct{}),
		logDir:     filepath.Join(homeDir, ".you-agent-factory", "logs"),
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start isolated you daemon: %v", err)
	}
	go func() {
		err := command.Wait()
		daemon.mu.Lock()
		daemon.waitErr = err
		daemon.mu.Unlock()
		close(daemon.done)
	}()
	t.Cleanup(daemon.cleanup)
	return daemon
}

func waitForBoardPersistenceDaemonExit(t *testing.T, daemon *boardPersistenceDaemon, timeout time.Duration) {
	t.Helper()
	// A startup failure is observed through the real child-process exit rather
	// than a fixed sleep; this is the process-boundary behavior under test.
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-daemon.done:
	case <-timer.C:
		t.Fatalf("corrupt-recording daemon did not exit within %s\nstdout=%s\nstderr=%s", timeout, daemon.stdout.String(), daemon.stderr.String())
	}
}

func (daemon *boardPersistenceDaemon) kill(t *testing.T) {
	t.Helper()
	if daemon == nil {
		return
	}
	daemon.mu.Lock()
	if daemon.stopped {
		daemon.mu.Unlock()
		return
	}
	daemon.mu.Unlock()
	daemon.shutdownStartedAt = time.Now()
	if err := daemon.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("hard-kill isolated you daemon: %v", err)
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-daemon.done:
		daemon.stoppedAt = time.Now()
		daemon.mu.Lock()
		daemon.stopped = true
		daemon.mu.Unlock()
	case <-timer.C:
		t.Fatalf("hard-killed isolated you daemon did not exit within 20s\nstdout=%s\nstderr=%s", daemon.stdout.String(), daemon.stderr.String())
	}
}

func (daemon *boardPersistenceDaemon) cleanup() {
	if daemon == nil {
		return
	}
	daemon.mu.Lock()
	if daemon.stopped {
		daemon.mu.Unlock()
		return
	}
	daemon.stopped = true
	daemon.mu.Unlock()
	daemon.shutdownStartedAt = time.Now()
	select {
	case <-daemon.done:
	default:
		_ = daemon.cmd.Process.Kill()
		<-daemon.done
	}
	daemon.stoppedAt = time.Now()
}

func (daemon *boardPersistenceDaemon) stop(t *testing.T) {
	t.Helper()
	daemon.mu.Lock()
	if daemon.stopped {
		daemon.mu.Unlock()
		return
	}
	daemon.mu.Unlock()
	daemon.shutdownStartedAt = time.Now()
	if err := interruptBoardPersistenceProcess(daemon.cmd); err != nil {
		t.Fatalf("interrupt isolated you daemon: %v", err)
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-daemon.done:
		daemon.stoppedAt = time.Now()
		if err := daemon.waitError(); err != nil {
			if !boardPersistenceCleanExit(err) {
				t.Fatalf("isolated you daemon shutdown error = %v\nstdout=%s\nstderr=%s", err, daemon.stdout.String(), daemon.stderr.String())
			}
		}
		daemon.mu.Lock()
		daemon.stopped = true
		daemon.mu.Unlock()
	case <-timer.C:
		t.Fatalf("isolated you daemon did not stop within 20s\nstdout=%s\nstderr=%s", daemon.stdout.String(), daemon.stderr.String())
	}
}

func (daemon *boardPersistenceDaemon) waitError() error {
	daemon.mu.Lock()
	defer daemon.mu.Unlock()
	return daemon.waitErr
}

func reserveBoardPersistenceAddress(t *testing.T) string {
	t.Helper()
	for {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve isolated daemon loopback port: %v", err)
		}
		address := listener.Addr().String()
		port := listener.Addr().(*net.TCPAddr).Port
		if err := listener.Close(); err != nil {
			t.Fatalf("release isolated daemon loopback port: %v", err)
		}
		if port != boardPersistenceDefaultPort && port != boardPersistenceAdjacentReservedPort {
			return address
		}
	}
}
