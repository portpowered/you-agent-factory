package restart_test

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	boardPersistenceDefaultPort          = 7437
	boardPersistenceAdjacentReservedPort = 7438
)

type boardPersistenceDaemon struct {
	cmd        *exec.Cmd
	baseURL    string
	sessionID  string
	factoryDir string
	homeDir    string
	logDir     string
	recordPath string
	stdout     *bytes.Buffer
	stderr     *bytes.Buffer
	done       chan struct{}
	mu         sync.Mutex
	waitErr    error
	stopped    bool
}

// The checked-out you executable is immutable for the lifetime of one package
// process while every mutable scenario input stays isolated per test, so all
// real-CLI scenarios share exactly one current-checkout build. The shared
// artifact lives in a dedicated package-scoped directory (never a global or
// cross-invocation cache) that TestMain removes after all daemon and helper
// children have joined.
var (
	boardPersistenceBinaryOnce   sync.Once
	boardPersistenceBinaryDir    string
	boardPersistenceBinaryPath   string
	boardPersistenceBinaryErr    error
	boardPersistenceBinaryOutput []byte
	boardPersistenceBinaryBuilds atomic.Int64
)

func TestMain(m *testing.M) {
	code := m.Run()
	if boardPersistenceBinaryDir != "" {
		if err := os.RemoveAll(boardPersistenceBinaryDir); err != nil {
			fmt.Fprintf(os.Stderr, "remove package-scoped you build directory %q: %v\n", boardPersistenceBinaryDir, err)
			if code == 0 {
				code = 1
			}
		}
	}
	os.Exit(code)
}

func buildBoardPersistenceBinary(t *testing.T) string {
	t.Helper()
	if os.Getenv(boardPersistenceHelperEnv) == boardPersistenceHelperEnvValue {
		t.Fatal("SCRIPT_WORKER helper must not build the package CLI binary")
	}
	boardPersistenceBinaryOnce.Do(func() {
		boardPersistenceBinaryBuilds.Add(1)
		binaryName := "you"
		if runtime.GOOS == "windows" {
			binaryName += ".exe"
		}
		dir, err := os.MkdirTemp("", "you-restart-package-build-")
		if err != nil {
			boardPersistenceBinaryErr = fmt.Errorf("create package-scoped build directory: %w", err)
			return
		}
		boardPersistenceBinaryDir = dir
		path := filepath.Join(dir, binaryName)
		build := exec.CommandContext(t.Context(), "go", "build", "-o", path, "./cmd/factory")
		build.Dir = testutil.MustRepoRoot(t)
		boardPersistenceBinaryOutput, err = build.CombinedOutput()
		if err != nil {
			boardPersistenceBinaryErr = fmt.Errorf("build fresh you binary: %w", err)
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			boardPersistenceBinaryErr = fmt.Errorf("stat built you binary: %w", err)
			return
		}
		if info.Size() == 0 {
			boardPersistenceBinaryErr = errors.New("built you binary is empty")
			return
		}
		boardPersistenceBinaryPath = path
	})
	if boardPersistenceBinaryErr != nil {
		t.Fatalf("build shared you binary: %v\n%s", boardPersistenceBinaryErr, boardPersistenceBinaryOutput)
	}
	buildCount := boardPersistenceBinaryBuilds.Load()
	if buildCount != 1 {
		t.Fatalf("package-scoped you binary build count = %d, want exactly 1", buildCount)
	}
	t.Logf("reusing package-scoped you binary built once for this process: %q (builds=%d)", boardPersistenceBinaryPath, buildCount)
	return boardPersistenceBinaryPath
}

func startBoardPersistenceDaemon(
	t *testing.T,
	binaryPath, factoryDir, homeDir, recordPath, releasePath string,
) *boardPersistenceDaemon {
	t.Helper()
	daemon := startBoardPersistenceDaemonProcess(t, binaryPath, factoryDir, homeDir, recordPath, releasePath)
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
	address := reserveBoardPersistenceAddress(t)
	command := exec.CommandContext(
		t.Context(),
		binaryPath,
		"run", "--dir", factoryDir,
		"--continuously", "--with-server",
		"--listen", address,
		"--record", recordPath,
	)
	command.Dir = factoryDir
	command.Env = append(
		builtcliacceptance.ProcessEnvForIsolatedHome(homeDir),
		boardPersistenceHelperEnv+"="+boardPersistenceHelperEnvValue,
		boardPersistenceReleaseEnv+"="+releasePath,
	)
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
	if err := daemon.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Fatalf("hard-kill isolated you daemon: %v", err)
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-daemon.done:
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
	select {
	case <-daemon.done:
	default:
		_ = daemon.cmd.Process.Kill()
		<-daemon.done
	}
}

func (daemon *boardPersistenceDaemon) stop(t *testing.T) {
	t.Helper()
	daemon.mu.Lock()
	if daemon.stopped {
		daemon.mu.Unlock()
		return
	}
	daemon.mu.Unlock()
	if err := interruptBoardPersistenceProcess(daemon.cmd); err != nil {
		t.Fatalf("interrupt isolated you daemon: %v", err)
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	select {
	case <-daemon.done:
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
