//go:build windows

package process_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	windowsGracefulStopReliabilityIterations = 10
	windowsGracefulStopReadinessTimeout      = 30 * time.Second
	windowsGracefulStopTimeout               = 20 * time.Second
	windowsGracefulStopScanTimeout           = 5 * time.Second
	windowsGracefulStopCleanupTimeout        = 5 * time.Second
)

type windowsGracefulStopTarget struct {
	command  *exec.Cmd
	server   string
	scanErr  <-chan error
	stderr   *bytes.Buffer
	waitDone chan error

	mu     sync.Mutex
	waited bool
}

type windowsGracefulStopScenario struct {
	name string
	args func(server string) []string
}

// TestWindowsGracefulStopReliability proves the shipped Windows executable
// can stop both supported long-running local commands through a separate CLI
// process. The listener readiness signal and child process exit are the only
// synchronization points; the test does not use a fixed startup or shutdown
// sleep.
func TestWindowsGracefulStopReliability(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process-boundary coverage requires Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildCtx, harness.RepoRoot)
	iterations := windowsGracefulStopReliabilityIterations
	if testing.Short() {
		// The short functional lane proves each supported process boundary once.
		// Repetition is reliability/stress evidence and remains in the non-short
		// lane without charging every pull request for twenty process cycles.
		iterations = 1
	}

	for _, scenario := range windowsGracefulStopScenarios() {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			passes := 0
			for iteration := 1; iteration <= iterations; iteration++ {
				result := runWindowsGracefulStopIteration(t, harness, binaryPath, scenario, iteration)
				passes++
				t.Logf(
					"PASS iteration=%d/%d target=%s pid=%d elapsed=%s stop_command=%q\n"+
						"tasklist before:\n%s\n"+
						"tasklist after:\n%s\n"+
						"stop stdout=%q stderr=%q",
					iteration,
					iterations,
					scenario.name,
					result.pid,
					result.elapsed.Round(time.Millisecond),
					result.stopCommand,
					result.tasklistBefore,
					result.tasklistAfter,
					result.stopStdout,
					result.stopStderr,
				)
			}
			t.Logf(
				"graceful-stop reliability target=%s passes=%d/%d",
				scenario.name,
				passes,
				iterations,
			)
		})
	}
}

// TestWindowsHardKillFallbackRemainsAvailable records the explicit forceful
// fallback separately from graceful-stop coverage. It intentionally uses the
// documented taskkill command shape and only targets the test-owned process.
func TestWindowsHardKillFallbackRemainsAvailable(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process-boundary coverage requires Windows")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildCtx, harness.RepoRoot)
	target := startWindowsGracefulStopTarget(t, harness, binaryPath, windowsGracefulStopScenarios()[0])

	tasklistBefore := windowsTasklist(t, target.command.Process.Pid)
	if !windowsTasklistContainsPID(tasklistBefore, target.command.Process.Pid) {
		t.Fatalf("hard-kill target missing from tasklist before kill: pid=%d output=%q", target.command.Process.Pid, tasklistBefore)
	}

	args := []string{"/PID", strconv.Itoa(target.command.Process.Pid), "/F"}
	hardKill := exec.Command("taskkill", args...)
	hardKillOutput, err := hardKill.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"hard-kill command %q failed: %v; output=%q; tasklist before=%q",
			"taskkill "+strings.Join(args, " "),
			err,
			string(hardKillOutput),
			tasklistBefore,
		)
	}

	waitErr := target.wait(windowsGracefulStopTimeout)
	if waitErr == nil {
		t.Fatalf("hard-killed target pid=%d returned success; want forceful termination", target.command.Process.Pid)
	}
	waitForScannerCompletion(t, target.scanErr, "hard-killed target", windowsGracefulStopScanTimeout)
	tasklistAfter := windowsTasklist(t, target.command.Process.Pid)
	if windowsTasklistContainsPID(tasklistAfter, target.command.Process.Pid) {
		t.Fatalf("hard-killed target remains in tasklist: pid=%d output=%q", target.command.Process.Pid, tasklistAfter)
	}
	t.Logf(
		"hard-kill fallback PASS command=%q pid=%d tasklist before:\n%s\ntaskkill output=%q\ntasklist after:\n%s",
		"taskkill "+strings.Join(args, " "),
		target.command.Process.Pid,
		tasklistBefore,
		string(hardKillOutput),
		tasklistAfter,
	)
}

type windowsGracefulStopIterationResult struct {
	pid            int
	elapsed        time.Duration
	stopCommand    string
	tasklistBefore string
	tasklistAfter  string
	stopStdout     string
	stopStderr     string
}

func runWindowsGracefulStopIteration(
	t *testing.T,
	harness *builtcliacceptance.Harness,
	binaryPath string,
	scenario windowsGracefulStopScenario,
	iteration int,
) windowsGracefulStopIterationResult {
	t.Helper()
	target := startWindowsGracefulStopTarget(t, harness, binaryPath, scenario)
	pid := target.command.Process.Pid
	tasklistBefore := windowsTasklist(t, pid)
	if !windowsTasklistContainsPID(tasklistBefore, pid) {
		t.Fatalf("iteration %d target missing from tasklist before stop: pid=%d output=%q", iteration, pid, tasklistBefore)
	}

	stopArgs := []string{"--server", target.server, "server", "stop"}
	stopCommand := exec.Command(binaryPath, stopArgs...)
	stopCommand.Dir = target.command.Dir
	stopCommand.Env = target.command.Env
	var stopStdout, stopStderr bytes.Buffer
	stopCommand.Stdout = &stopStdout
	stopCommand.Stderr = &stopStderr
	started := time.Now()
	if err := stopCommand.Run(); err != nil {
		t.Fatalf(
			"iteration %d stop command %q failed: %v; stdout=%q stderr=%q target_pid=%d tasklist_before=%q",
			iteration,
			"you "+strings.Join(stopArgs, " "),
			err,
			stopStdout.String(),
			stopStderr.String(),
			pid,
			tasklistBefore,
		)
	}

	waitErr := target.wait(windowsGracefulStopTimeout)
	if waitErr != nil {
		t.Fatalf(
			"iteration %d target pid=%d exit=%v, want clean graceful-stop exit; target stderr=%q",
			iteration,
			pid,
			waitErr,
			target.stderr.String(),
		)
	}
	waitForScannerCompletion(t, target.scanErr, scenario.name, windowsGracefulStopScanTimeout)
	tasklistAfter := windowsTasklist(t, pid)
	if windowsTasklistContainsPID(tasklistAfter, pid) {
		t.Fatalf("iteration %d target remains in tasklist after graceful stop: pid=%d output=%q", iteration, pid, tasklistAfter)
	}

	return windowsGracefulStopIterationResult{
		pid:            pid,
		elapsed:        time.Since(started),
		stopCommand:    "you " + strings.Join(stopArgs, " "),
		tasklistBefore: tasklistBefore,
		tasklistAfter:  tasklistAfter,
		stopStdout:     stopStdout.String(),
		stopStderr:     stopStderr.String(),
	}
}

func startWindowsGracefulStopTarget(
	t *testing.T,
	harness *builtcliacceptance.Harness,
	binaryPath string,
	scenario windowsGracefulStopScenario,
) *windowsGracefulStopTarget {
	t.Helper()
	session := harness.NewSession(t)
	writeIdleCurrentFactory(t, session.WorkDir)
	port := reserveWindowsGracefulStopPort(t)
	server := fmt.Sprintf("http://127.0.0.1:%d", port)
	command := exec.Command(binaryPath, scenario.args(server)...)
	command.Dir = session.WorkDir
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(session.HomeDir)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open %s stdout: %v", scenario.name, err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start %s target: %v", scenario.name, err)
	}
	target := &windowsGracefulStopTarget{
		command:  command,
		server:   server,
		stderr:   &stderr,
		waitDone: make(chan error, 1),
	}
	t.Cleanup(target.cleanup)

	lines := make(chan string, 128)
	scanErr := make(chan error, 1)
	target.scanErr = scanErr
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		scanErr <- scanner.Err()
	}()
	go func() { target.waitDone <- command.Wait() }()

	dashboardURL := waitForDashboardURL(t, lines, scanErr, &stderr, windowsGracefulStopReadinessTimeout)
	if scenario.name == "continuous run" {
		waitForStatus(t, server, windowsGracefulStopReadinessTimeout, func(status factoryapi.StatusResponse) bool {
			return status.FactoryState == "RUNNING"
		})
	}
	t.Logf("ready target=%s pid=%d server=%s dashboard=%s", scenario.name, command.Process.Pid, server, dashboardURL)
	return target
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
	cleanup := exec.Command("taskkill", args...)
	_ = cleanup.Run()
	select {
	case <-target.waitDone:
		target.mu.Lock()
		target.waited = true
		target.mu.Unlock()
	case <-time.After(windowsGracefulStopCleanupTimeout):
	}
}

func windowsGracefulStopScenarios() []windowsGracefulStopScenario {
	return []windowsGracefulStopScenario{
		{
			name: "server",
			args: func(server string) []string { return []string{"--server", server, "server"} },
		},
		{
			name: "continuous run",
			args: func(server string) []string {
				return []string{"--server", server, "run", "--continuously", "--with-server", "--no-record"}
			},
		},
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
