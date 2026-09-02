//go:build windows

package recordingsprocess_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWindowsRecordingFlushBeforeGracefulStopReturns proves the shipped
// Windows executable waits for cancellation-induced recording failure to be
// durably published before the stop command can observe a clean exit. The
// artifact is decoded here with encoding/json helpers from the sibling test,
// never through the product replay/read path.
func TestWindowsRecordingFlushBeforeGracefulStopReturns(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process-boundary coverage requires Windows")
	}
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildCtx, harness.RepoRoot)
	run := startWindowsRecordingRun(t, harness, binaryPath)
	submitWindowsRecordingShutdownWork(t, binaryPath, run.target)
	waitForRecordingWorkerReady(t, run.readyPath, run.target)

	before, err := waitForStandaloneRecordingEvent(run.recordingPath, "DISPATCH_REQUEST", recordingShutdownObservationTimeout)
	if err != nil {
		t.Fatalf("wait for durable Windows DISPATCH_REQUEST: %v", err)
	}
	beforeInfo, err := os.Stat(run.recordingPath)
	if err != nil {
		t.Fatalf("stat Windows recording before graceful stop: %v", err)
	}
	beforeBytes := beforeInfo.Size()

	stopArgs := []string{"--server", run.target.server, "server", "stop"}
	stop := recordingProcessCLICommand(binaryPath, stopArgs...)
	stop.Dir = run.target.command.Dir
	stop.Env = append(run.target.command.Env, recordingProcessCLIHelperEnv+"=1")
	var stopStdout, stopStderr bytes.Buffer
	stop.Stdout = &stopStdout
	stop.Stderr = &stopStderr
	if err := stop.Run(); err != nil {
		t.Fatalf("Windows graceful stop %q failed: %v; stdout=%q stderr=%q", strings.Join(stopArgs, " "), err, stopStdout.String(), stopStderr.String())
	}
	if err := run.target.wait(windowsGracefulStopTimeout); err != nil {
		t.Fatalf("Windows recorded run exited with %v after graceful stop; target stderr=%q", err, run.target.stderr.String())
	}
	waitForScannerCompletion(t, run.target.scanErr, "recorded graceful-stop target", windowsGracefulStopScanTimeout)

	after, err := readStandaloneRecording(run.recordingPath)
	if err != nil {
		t.Fatalf("standalone parse after Windows graceful stop: %v", err)
	}
	afterInfo, err := os.Stat(run.recordingPath)
	if err != nil {
		t.Fatalf("stat Windows recording after graceful stop: %v", err)
	}
	failed, err := standaloneFailureEvents(after)
	if err != nil {
		t.Fatalf("inspect Windows standalone failure events: %v", err)
	}
	if len(failed) == 0 {
		t.Fatalf("Windows graceful-stop recording has no failure event: event_types=%v", standaloneRecordingEventTypes(after))
	}
	if len(after.Events) <= len(before.Events) {
		t.Fatalf("Windows graceful-stop event count = %d, want > baseline %d", len(after.Events), len(before.Events))
	}
	if !afterInfo.ModTime().After(beforeInfo.ModTime()) {
		t.Fatalf("Windows graceful-stop recording mtime = %s, want later than baseline %s", afterInfo.ModTime(), beforeInfo.ModTime())
	}
	if afterInfo.Size() <= beforeBytes {
		t.Fatalf("Windows graceful-stop recording bytes = %d, want > baseline %d", afterInfo.Size(), beforeBytes)
	}

	t.Logf(
		"Windows orderly recording durability stop_command=%q before_mtime=%s after_mtime=%s before_events=%d after_events=%d before_bytes=%d after_bytes=%d failure_events=%v stdout=%q stderr=%q",
		"you "+strings.Join(stopArgs, " "),
		beforeInfo.ModTime().UTC().Format(time.RFC3339Nano),
		afterInfo.ModTime().UTC().Format(time.RFC3339Nano),
		len(before.Events),
		len(after.Events),
		beforeBytes,
		afterInfo.Size(),
		failed,
		stopStdout.String(),
		stopStderr.String(),
	)
}

// TestWindowsRecordingTaskkillLeavesPartialValidSnapshot proves the forceful
// fallback does not acquire synchronous final-flush semantics. It kills only
// the test-owned process tree, then parses the last atomic snapshot directly.
func TestWindowsRecordingTaskkillLeavesPartialValidSnapshot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process-boundary coverage requires Windows")
	}
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildCtx, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildCtx, harness.RepoRoot)
	run := startWindowsRecordingRun(t, harness, binaryPath)
	submitWindowsRecordingShutdownWork(t, binaryPath, run.target)
	waitForRecordingWorkerReady(t, run.readyPath, run.target)
	before, err := waitForStandaloneRecordingEvent(run.recordingPath, "DISPATCH_REQUEST", recordingShutdownObservationTimeout)
	if err != nil {
		t.Fatalf("wait for durable Windows DISPATCH_REQUEST before taskkill: %v", err)
	}
	beforeInfo, err := os.Stat(run.recordingPath)
	if err != nil {
		t.Fatalf("stat Windows recording before taskkill: %v", err)
	}
	dispatchID := standaloneDispatchRequestID(before)
	if dispatchID == "" {
		t.Fatalf("durable baseline has no dispatch identity: event_types=%v", standaloneRecordingEventTypes(before))
	}

	pid := run.target.command.Process.Pid
	tasklistBefore := windowsTasklist(t, pid)
	if !windowsTasklistContainsPID(tasklistBefore, pid) {
		t.Fatalf("taskkill target missing from tasklist before kill: pid=%d output=%q", pid, tasklistBefore)
	}
	killArgs := []string{"/PID", strconv.Itoa(pid), "/T", "/F"}
	kill := exec.Command("taskkill", killArgs...)
	killOutput, killErr := kill.CombinedOutput()
	if killErr != nil {
		t.Fatalf("taskkill %q failed: %v; output=%q; tasklist before=%q", strings.Join(killArgs, " "), killErr, string(killOutput), tasklistBefore)
	}
	waitErr := run.target.wait(windowsGracefulStopTimeout)
	if waitErr == nil {
		t.Fatalf("taskkill target pid=%d returned success; want forceful termination", pid)
	}
	waitForScannerCompletion(t, run.target.scanErr, "taskkilled recorded target", windowsGracefulStopScanTimeout)
	tasklistAfter := windowsTasklist(t, pid)
	if windowsTasklistContainsPID(tasklistAfter, pid) {
		t.Fatalf("taskkilled recorded target remains in tasklist: pid=%d output=%q", pid, tasklistAfter)
	}

	after, err := readStandaloneRecording(run.recordingPath)
	if err != nil {
		t.Fatalf("standalone parse after taskkill: %v", err)
	}
	afterInfo, err := os.Stat(run.recordingPath)
	if err != nil {
		t.Fatalf("stat Windows recording after taskkill: %v", err)
	}
	if len(after.Events) == 0 {
		t.Fatalf("standalone recording after taskkill has no events")
	}
	if !standaloneHasDispatchRequest(after, dispatchID) {
		t.Fatalf("taskkill snapshot lost durable dispatch request %q: event_types=%v", dispatchID, standaloneRecordingEventTypes(after))
	}
	if standaloneHasDispatchResponse(after, dispatchID) {
		t.Fatalf("taskkill snapshot contains a terminal response for dispatch %q; forceful termination should retain the partial prefix", dispatchID)
	}

	t.Logf(
		"Windows taskkill recording fallback command=%q pid=%d baseline_mtime=%s after_mtime=%s baseline_events=%d after_events=%d baseline_bytes=%d after_bytes=%d dispatch_id=%q taskkill_output=%q tasklist_before=%q tasklist_after=%q",
		"taskkill "+strings.Join(killArgs, " "),
		pid,
		beforeInfo.ModTime().UTC().Format(time.RFC3339Nano),
		afterInfo.ModTime().UTC().Format(time.RFC3339Nano),
		len(before.Events),
		len(after.Events),
		beforeInfo.Size(),
		afterInfo.Size(),
		dispatchID,
		string(killOutput),
		tasklistBefore,
		tasklistAfter,
	)
}

type windowsRecordingRun struct {
	target        *windowsGracefulStopTarget
	recordingPath string
	readyPath     string
}

func startWindowsRecordingRun(
	t *testing.T,
	harness *builtcliacceptance.Harness,
	binaryPath string,
) windowsRecordingRun {
	t.Helper()
	session := harness.NewSession(t)
	writeIdleCurrentFactory(t, session.WorkDir)
	factoryDir := filepath.Join(session.WorkDir, "factory")
	support.WriteAgentConfig(t, factoryDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.WriteWorkstationConfig(t, factoryDir, "process", "---\ntype: MODEL_WORKSTATION\n---\nHold the Windows dispatch.\n")
	recordingPath := filepath.Join(session.WorkDir, "shutdown.replay.json")
	readyPath := filepath.Join(session.WorkDir, "worker-started.signal")
	mockWorkersPath := writeWindowsBlockingMockWorkers(t, readyPath)
	server := fmt.Sprintf("http://127.0.0.1:%d", reserveWindowsGracefulStopPort(t))
	args := []string{
		"--server", server,
		"run",
		"--dir", filepath.Join(session.WorkDir, "factory"),
		"--continuously",
		"--with-server",
		"--with-mock-workers=" + mockWorkersPath,
		"--record", recordingPath,
	}
	command := recordingProcessCLICommand(binaryPath, args...)
	command.Dir = session.WorkDir
	command.Env = append(
		builtcliacceptance.ProcessEnvForIsolatedHome(session.HomeDir),
		recordingProcessCLIHelperEnv+"=1",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("open recorded Windows run stdout: %v", err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start recorded Windows run: %v", err)
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
	_ = waitForDashboardURL(t, lines, scanErr, &stderr, windowsGracefulStopReadinessTimeout)
	support.WaitForStatus(t, server, windowsGracefulStopReadinessTimeout, func(status factoryapi.StatusResponse) bool {
		return status.FactoryState == "RUNNING"
	})
	t.Logf("ready recorded Windows run pid=%d server=%s recording=%s", command.Process.Pid, server, recordingPath)
	return windowsRecordingRun{target: target, recordingPath: recordingPath, readyPath: readyPath}
}

func writeWindowsBlockingMockWorkers(t *testing.T, readyPath string) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "block-worker.ps1")
	script := `param([string]$ReadyPath)
$ErrorActionPreference = 'Stop'
New-Item -ItemType File -Path $ReadyPath -Force | Out-Null
while ($true) { Start-Sleep -Seconds 60 }
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write blocking Windows worker script: %v", err)
	}

	// Functional-test construction exception: this test invokes a separately
	// built `you.exe` to prove Windows process, pipe, signal, and taskkill
	// behavior. ProviderCommandRunner is an edges.Edges dependency in the
	// parent test process and cannot cross that executable boundary, so this
	// narrowly scoped serialized mock-worker fixture is the child-process seam.
	// The reusable in-process scenario above uses ProviderCommandRunner instead.
	payload := fmt.Sprintf(`{"mockWorkers":[{"runType":"script","scriptConfig":{"command":"powershell.exe","args":["-NoLogo","-NoProfile","-NonInteractive","-ExecutionPolicy","Bypass","-File",%q,"-ReadyPath",%q]}}]}`, scriptPath, readyPath)
	mockWorkersPath := filepath.Join(t.TempDir(), "blocking-mock-workers.json")
	if err := os.WriteFile(mockWorkersPath, []byte(payload), 0o600); err != nil {
		t.Fatalf("write blocking Windows mock-workers fixture: %v", err)
	}
	return mockWorkersPath
}

func submitWindowsRecordingShutdownWork(t testing.TB, binaryPath string, target *windowsGracefulStopTarget) {
	t.Helper()
	name := "windows-recording-shutdown-work"
	payloadPath := filepath.Join(t.TempDir(), "recording-shutdown-work.json")
	if err := os.WriteFile(payloadPath, []byte(`{"title":"hold the Windows dispatch"}`), 0o600); err != nil {
		t.Fatalf("write Windows recording Work payload: %v", err)
	}
	args := []string{
		"--server", target.server, "--json", "submit",
		"--name", name, "--work-type-name", "task", "--payload", payloadPath,
	}
	command := recordingProcessCLICommand(binaryPath, args...)
	command.Dir = target.command.Dir
	command.Env = append(target.command.Env, recordingProcessCLIHelperEnv+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("Windows CLI submit %q failed: %v; output=%q", strings.Join(args, " "), err, string(output))
	}
	var response struct {
		WorkID *string `json:"workId"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatalf("decode Windows CLI submit response: %v; output=%q", err, string(output))
	}
	if response.WorkID == nil || strings.TrimSpace(*response.WorkID) == "" {
		t.Fatalf("Windows CLI submit response missing workId: %q", string(output))
	}
}

func waitForRecordingWorkerReady(t testing.TB, path string, target *windowsGracefulStopTarget) {
	t.Helper()
	// The signal is the only OS-visible observation that the built CLI has
	// crossed into the blocking child process; bounded polling avoids a fixed
	// startup sleep while keeping this process-boundary synchronization narrow.
	deadline := time.NewTimer(recordingShutdownObservationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stat Windows worker readiness signal %q: %v", path, err)
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Windows worker readiness signal %q; target stderr=%q", path, target.stderr.String())
		}
	}
}

func standaloneDispatchRequestID(recording standaloneRecording) string {
	for _, event := range recording.Events {
		if event.Type == "DISPATCH_REQUEST" && event.Context.DispatchID != nil {
			return *event.Context.DispatchID
		}
	}
	return ""
}

func standaloneHasDispatchRequest(recording standaloneRecording, dispatchID string) bool {
	for _, event := range recording.Events {
		if event.Type == "DISPATCH_REQUEST" && event.Context.DispatchID != nil && *event.Context.DispatchID == dispatchID {
			return true
		}
	}
	return false
}

func standaloneHasDispatchResponse(recording standaloneRecording, dispatchID string) bool {
	for _, event := range recording.Events {
		if event.Type == "DISPATCH_RESPONSE" && event.Context.DispatchID != nil && *event.Context.DispatchID == dispatchID {
			return true
		}
	}
	return false
}

func standaloneRecordingEventTypes(recording standaloneRecording) []string {
	types := make([]string, 0, len(recording.Events))
	for _, event := range recording.Events {
		types = append(types, event.Type)
	}
	return types
}
