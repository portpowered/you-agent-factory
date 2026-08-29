package process_test

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

const builtBrowserSuppressionTimeout = 30 * time.Second

// TestBuiltCLINoBrowserOpenSuppressesLauncherAcrossLifecycleCases crosses the
// compiled CLI boundary with a controlled launcher at the front of PATH. The
// launcher writes a marker and exits non-zero if selected, so an unexpected
// fallback is observable without permitting a real browser command to run.
func TestBuiltCLINoBrowserOpenSuppressesLauncherAcrossLifecycleCases(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" {
		t.Skip("built browser suppression coverage requires Windows or Linux")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildContext, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildContext, harness.RepoRoot)

	t.Run("success-and-cancellation", func(t *testing.T) {
		sentinel := newBrowserLauncherSentinel(t)
		session := harness.NewSession(t).WithNoExternalServer(t)
		server := startBuiltBrowserServer(t, binaryPath, session, sentinel)
		t.Cleanup(func() { server.cleanup(t) })

		dashboardURL := waitForDashboardURL(
			t,
			server.lines,
			server.scanErr,
			&server.stderr,
			builtBrowserSuppressionTimeout,
		)
		if want := session.ServerURL + "/dashboard/ui"; dashboardURL != want {
			t.Fatalf("dashboard URL = %q, want %q", dashboardURL, want)
		}
		assertBrowserLauncherNotObserved(t, sentinel)

		server.stop(t)
		assertServerPortReusable(t, session.ServerURL)
		assertBrowserLauncherNotObserved(t, sentinel)
	})

	t.Run("startup-failure-and-recovery", func(t *testing.T) {
		sentinel := newBrowserLauncherSentinel(t)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve occupied server listener: %v", err)
		}
		defer listener.Close()

		failedSession := harness.NewSession(t)
		failedSession.ServerURL = "http://" + listener.Addr().String()
		writeIdleCurrentFactory(t, failedSession.WorkDir)
		failedOutput, failedErr := runBuiltBrowserServerUntilExit(t, binaryPath, failedSession, sentinel)
		if failedErr == nil {
			t.Fatalf("occupied-port server result = success; output=%q", failedOutput)
		}
		if strings.Contains(failedOutput, "Dashboard URL: ") {
			t.Fatalf("occupied-port server claimed dashboard readiness: %q", failedOutput)
		}
		if strings.TrimSpace(failedOutput) == "" {
			t.Fatalf("occupied-port server emitted no diagnostic; error=%v", failedErr)
		}
		assertBrowserLauncherNotObserved(t, sentinel)

		recoverySentinel := newBrowserLauncherSentinel(t)
		recoverySession := harness.NewSession(t).WithNoExternalServer(t)
		recoveryServer := startBuiltBrowserServer(t, binaryPath, recoverySession, recoverySentinel)
		t.Cleanup(func() { recoveryServer.cleanup(t) })
		_ = waitForDashboardURL(
			t,
			recoveryServer.lines,
			recoveryServer.scanErr,
			&recoveryServer.stderr,
			builtBrowserSuppressionTimeout,
		)
		recoveryServer.stop(t)
		assertServerPortReusable(t, recoverySession.ServerURL)
		assertBrowserLauncherNotObserved(t, recoverySentinel)
	})

	t.Run("concurrent-isolated-children", func(t *testing.T) {
		firstSentinel := newBrowserLauncherSentinel(t)
		secondSentinel := newBrowserLauncherSentinel(t)
		firstSession := harness.NewSession(t).WithNoExternalServer(t)
		secondSession := harness.NewSession(t).WithNoExternalServer(t)
		first := startBuiltBrowserServer(t, binaryPath, firstSession, firstSentinel)
		second := startBuiltBrowserServer(t, binaryPath, secondSession, secondSentinel)
		t.Cleanup(func() {
			first.cleanup(t)
			second.cleanup(t)
		})

		firstURL := waitForDashboardURL(t, first.lines, first.scanErr, &first.stderr, builtBrowserSuppressionTimeout)
		secondURL := waitForDashboardURL(t, second.lines, second.scanErr, &second.stderr, builtBrowserSuppressionTimeout)
		if firstURL == secondURL {
			t.Fatalf("concurrent dashboard URLs both resolved to %q; want isolated ports", firstURL)
		}
		first.stop(t)
		second.stop(t)
		assertServerPortReusable(t, firstSession.ServerURL)
		assertServerPortReusable(t, secondSession.ServerURL)
		assertBrowserLauncherNotObserved(t, firstSentinel)
		assertBrowserLauncherNotObserved(t, secondSentinel)
	})
}

type browserLauncherSentinel struct {
	directory string
	marker    string
}

func newBrowserLauncherSentinel(t *testing.T) browserLauncherSentinel {
	t.Helper()
	directory := t.TempDir()
	marker := filepath.Join(directory, "launcher-invoked.marker")
	var launcherPath string
	var launcher []byte
	var mode os.FileMode
	switch runtime.GOOS {
	case "windows":
		launcherPath = filepath.Join(directory, "rundll32.cmd")
		launcher = []byte("@echo off\r\n>\"%YOU_TEST_BROWSER_MARKER%\" echo launcher-invoked\r\nexit /b 97\r\n")
		mode = 0o755
	case "linux":
		launcherPath = filepath.Join(directory, "xdg-open")
		launcher = []byte("#!/bin/sh\nprintf '%s\\n' launcher-invoked > \"$YOU_TEST_BROWSER_MARKER\"\nexit 97\n")
		mode = 0o755
	default:
		t.Fatalf("unsupported browser launcher sentinel OS %q", runtime.GOOS)
	}
	if err := os.WriteFile(launcherPath, launcher, mode); err != nil {
		t.Fatalf("write %s launcher sentinel: %v", runtime.GOOS, err)
	}
	return browserLauncherSentinel{directory: directory, marker: marker}
}

func (sentinel browserLauncherSentinel) environment(
	t testing.TB,
	session *builtcliacceptance.Session,
) []string {
	t.Helper()
	env := session.ProcessEnvWith("YOU_TEST_BROWSER_MARKER=" + sentinel.marker)
	for index, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.EqualFold(key, "PATH") {
			continue
		}
		env[index] = key + "=" + sentinel.directory + string(os.PathListSeparator) + value
		return env
	}
	return append(env, "PATH="+sentinel.directory)
}

type builtBrowserServer struct {
	command  *exec.Cmd
	lines    chan string
	scanErr  chan error
	scanDone chan struct{}
	stderr   bytes.Buffer
	waited   bool
}

func startBuiltBrowserServer(
	t *testing.T,
	binaryPath string,
	session *builtcliacceptance.Session,
	sentinel browserLauncherSentinel,
) *builtBrowserServer {
	t.Helper()
	writeIdleCurrentFactory(t, session.WorkDir)
	args := []string{"server", "--listen", serverListenAddress(t, session.ServerURL)}
	command := exec.Command(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = sentinel.environment(t, session)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open built server stdout: %v", err)
	}
	server := &builtBrowserServer{
		command:  command,
		lines:    make(chan string, 32),
		scanErr:  make(chan error, 1),
		scanDone: make(chan struct{}),
	}
	command.Stderr = &server.stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start built server: %v", err)
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			server.lines <- scanner.Text()
		}
		server.scanErr <- scanner.Err()
		close(server.scanDone)
	}()
	return server
}

func runBuiltBrowserServerUntilExit(
	t *testing.T,
	binaryPath string,
	session *builtcliacceptance.Session,
	sentinel browserLauncherSentinel,
) (string, error) {
	t.Helper()
	args := []string{"server", "--listen", serverListenAddress(t, session.ServerURL)}
	ctx, cancel := context.WithTimeout(t.Context(), builtBrowserSuppressionTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = sentinel.environment(t, session)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil && !errors.Is(ctx.Err(), context.Canceled) {
		return string(output), fmt.Errorf("run occupied-port server: %w", ctx.Err())
	}
	return string(output), err
}

func (server *builtBrowserServer) stop(t testing.TB) {
	t.Helper()
	if server == nil || server.waited {
		return
	}
	if server.command.ProcessState == nil || !server.command.ProcessState.Exited() {
		if runtime.GOOS == "windows" {
			_ = server.command.Process.Kill()
		} else {
			_ = server.command.Process.Signal(os.Interrupt)
		}
	}
	waitErr := make(chan error, 1)
	go func() { waitErr <- server.command.Wait() }()
	select {
	case <-waitErr:
	case <-time.After(builtBrowserSuppressionTimeout):
		_ = server.command.Process.Kill()
		<-waitErr
		t.Errorf("built server did not stop within %s", builtBrowserSuppressionTimeout)
	}
	server.waited = true
	waitForBrowserServerScanner(t, server, "built browser suppression server")
}

func (server *builtBrowserServer) cleanup(t testing.TB) {
	t.Helper()
	if server == nil || server.waited {
		return
	}
	if server.command.ProcessState == nil || !server.command.ProcessState.Exited() {
		_ = server.command.Process.Kill()
	}
	_ = server.command.Wait()
	server.waited = true
	waitForBrowserServerScanner(t, server, "built browser suppression cleanup")
}

func waitForBrowserServerScanner(t testing.TB, server *builtBrowserServer, role string) {
	t.Helper()
	select {
	case <-server.scanDone:
	case <-time.After(5 * time.Second):
		t.Errorf("%s stdout scanner did not finish within 5s", role)
	}
}

func serverListenAddress(t testing.TB, serverURL string) string {
	t.Helper()
	parsed, err := url.Parse(serverURL)
	if err != nil || parsed.Host == "" {
		t.Fatalf("parse server URL %q: %v", serverURL, err)
	}
	return parsed.Host
}

func assertBrowserLauncherNotObserved(t testing.TB, sentinel browserLauncherSentinel) {
	t.Helper()
	if data, err := os.ReadFile(sentinel.marker); err == nil {
		t.Fatalf("controlled browser launcher marker exists at %s: %q", sentinel.marker, data)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect controlled browser launcher marker %s: %v", sentinel.marker, err)
	}
}

func assertServerPortReusable(t testing.TB, serverURL string) {
	t.Helper()
	listener, err := net.Listen("tcp", serverListenAddress(t, serverURL))
	if err != nil {
		t.Fatalf("rebind server port for %s: %v", serverURL, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close server port rebind for %s: %v", serverURL, err)
	}
}
