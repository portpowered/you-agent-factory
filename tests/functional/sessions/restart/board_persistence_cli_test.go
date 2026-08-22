package restart_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	boardPersistenceRequestID        = "board-persistence-round-trip-request"
	boardPersistenceNewRequestID     = "board-persistence-after-recovery-request"
	boardPersistenceHelperEnv        = "YOU_BOARD_PERSISTENCE_HELPER"
	boardPersistenceReleaseEnv       = "YOU_BOARD_PERSISTENCE_RELEASE"
	boardPersistenceHelperEnvValue   = "1"
	boardPersistenceWorkerSentinel   = "board-persistence-worker-result:PASS"
	boardPersistenceInitialWorkID    = "board-persistence-init"
	boardPersistenceProcessingWorkID = "board-persistence-processing"
	boardPersistenceAwaitingWorkID   = "board-persistence-awaiting-ci"
	boardPersistenceNewWorkID        = "board-persistence-new-work"
)

// TestBoardPersistenceWorkerHelper is launched by the real SCRIPT_WORKER child
// in TestBoardPersistenceCLIRestartRoundTrip. It exits only after the test has
// inspected the re-armed attempt, which makes the second dispatch observable
// without relying on a mock worker edge.
func TestBoardPersistenceWorkerHelper(t *testing.T) {
	if os.Getenv(boardPersistenceHelperEnv) != boardPersistenceHelperEnvValue {
		return
	}
	releasePath := strings.TrimSpace(os.Getenv(boardPersistenceReleaseEnv))
	if releasePath == "" {
		t.Fatal("board persistence worker helper release path is empty")
	}
	fmt.Fprintln(os.Stdout, boardPersistenceWorkerSentinel)

	// A child process has no test-owned event channel back into the daemon. The
	// bounded file observation is deliberately confined to this helper process;
	// the parent test synchronizes only through the public Work projection.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := os.Stat(releasePath)
		switch {
		case err == nil:
			return
		case errors.Is(err, os.ErrNotExist):
			<-ticker.C
		default:
			t.Fatalf("observe worker helper release file %q: %v", releasePath, err)
		}
	}
}

// TestBoardPersistenceCLIRestartRoundTrip proves the customer-visible board
// contract across real daemon processes. The in-process functional harnesses
// cover service composition; this test intentionally crosses the OS boundary
// because a daemon restart is the failure boundary being repaired.
func TestBoardPersistenceCLIRestartRoundTrip(t *testing.T) {
	cliContext, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	scenario := newBoardPersistenceScenario(t, cliContext)
	runBoardPersistenceInitialGeneration(t, scenario)
	runBoardPersistenceRecoveryGeneration(t, scenario)
	runBoardPersistenceSecondRestart(t, scenario)
}

// TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording proves
// that a valid durable snapshot survives an ungraceful daemon stop even when
// the current-board recording is absent at the next opening boundary. The
// process boundary is intentional: BuildProcess covers composition, while
// only a real child process can prove kill-and-reopen behavior.
func TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording(t *testing.T) {
	cliContext, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	scenario := newBoardPersistenceScenario(t, cliContext)
	first := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	batchJSON := boardPersistenceBatchJSON(t, boardPersistenceRequestID, []boardPersistenceBatchWork{
		{Name: "board-init", WorkID: boardPersistenceInitialWorkID, State: "init", TraceID: "trace-board-init", Content: "durable init content"},
		{Name: "board-processing", WorkID: boardPersistenceProcessingWorkID, State: "processing", TraceID: "trace-board-processing", Content: "durable processing content"},
		{Name: "board-awaiting-ci", WorkID: boardPersistenceAwaitingWorkID, State: "awaiting-ci", TraceID: "trace-board-awaiting-ci", Content: "durable awaiting-ci content"},
	})
	submitBatchThroughCLI(t, cliContext, first, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, batchJSON, boardPersistenceRequestID, 3)
	waitForBoardStates(t, first.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "processing",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
	}, 30*time.Second)
	if err := os.WriteFile(scenario.releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release worker helper before durable snapshot probe: %v", err)
	}
	waitForBoardStates(t, first.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "complete",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
	}, 30*time.Second)

	snapshotPath := filepath.Join(
		scenario.factoryDir,
		".you-agent-factory",
		"durable-sessions",
		factorysessions.DefaultSessionID+".json",
	)
	if strings.TrimSpace(first.sessionID) == "" {
		t.Fatal("hard-kill scenario session ID is empty")
	}
	snapshotBefore := waitForBoardPersistenceSnapshot(t, snapshotPath, factorysessions.DefaultSessionID, 30*time.Second)

	// Remove the selected board artifact immediately before the forceful stop so
	// the next process observes the same interrupted-write boundary as the
	// outage: durable state is already present, but board history is absent.
	if err := os.Remove(scenario.recordPath); err != nil {
		t.Fatalf("remove current-board recording before hard kill: %v", err)
	}
	first.kill(t)
	if _, err := os.Stat(scenario.recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current-board recording after hard kill = %v, want absent", err)
	}

	second := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	defer second.kill(t)
	restarted := waitForBoardStates(t, second.baseURL, map[string]string{}, 30*time.Second)
	if len(restarted.Results) != 0 {
		t.Fatalf("restarted board = %#v, want empty after unreconstructable board history", restarted.Results)
	}
	snapshotAfter, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read durable snapshot after recovery: %v", err)
	}
	if !bytes.Equal(snapshotAfter, snapshotBefore) {
		t.Fatalf("durable snapshot changed during missing-board recovery")
	}
	waitForBoardPersistenceLogMessage(t, second, []string{
		"board contents were lost",
		"empty board was initialized",
		"preserved durable state was not deleted",
		filepath.Base(scenario.recordPath),
	}, 30*time.Second)
}

// TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails proves that a
// present-but-invalid current-board artifact is not treated as an interrupted
// write. The child process is intentional so the assertion covers the actual
// operator-facing startup diagnostic emitted by the real CLI.
func TestBoardPersistenceCLIRestartWithCorruptBoardRecordingFails(t *testing.T) {
	cliContext, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	scenario := newBoardPersistenceScenario(t, cliContext)
	corruptPayload := []byte(`{"schemaVersion":"recordings.portable-artifact.v1","summary":{}}`)
	if err := os.WriteFile(scenario.recordPath, corruptPayload, 0o600); err != nil {
		t.Fatalf("write corrupt current-board recording: %v", err)
	}

	daemon := startBoardPersistenceDaemonProcess(
		t,
		scenario.binaryPath,
		scenario.factoryDir,
		scenario.homeDir,
		scenario.recordPath,
		scenario.releasePath,
	)
	defer daemon.cleanup()
	waitForBoardPersistenceDaemonExit(t, daemon, 20*time.Second)
	if daemon.waitError() == nil {
		t.Fatal("corrupt current-board recording process exited successfully")
	}
	output := daemon.stdout.String() + daemon.stderr.String()
	for _, fragment := range []string{
		"CURRENT_BOARD_RECORDING_CORRUPT",
		"CORRUPT_HISTORY",
		filepath.Base(scenario.recordPath),
		"preserve the artifact",
		"replace it from a trusted backup",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("corrupt recording startup output = %q, want fragment %q", output, fragment)
		}
	}
	if strings.Contains(output, "board contents were lost") || strings.Contains(output, "empty board was initialized") {
		t.Fatalf("corrupt recording was reported as recoverable absence: %q", output)
	}
	contents, err := os.ReadFile(scenario.recordPath)
	if err != nil {
		t.Fatalf("read corrupt current-board recording after failed startup: %v", err)
	}
	if !bytes.Equal(contents, corruptPayload) {
		t.Fatal("failed startup changed the corrupt recording; artifact must remain available for investigation")
	}
}

type boardPersistenceScenario struct {
	binaryPath            string
	factoryDir            string
	homeDir               string
	releasePath           string
	recordPath            string
	cliContext            context.Context
	expected              map[string]boardPersistenceExpectedWork
	activeDispatchID      string
	activeWorkerSessionID string
	second                *boardPersistenceDaemon
}

func newBoardPersistenceScenario(t *testing.T, cliContext context.Context) *boardPersistenceScenario {
	t.Helper()
	binaryPath := buildBoardPersistenceBinary(t)
	factoryDir := support.ScaffoldFactory(t, boardPersistenceFactoryConfig())
	homeDir := t.TempDir()
	releasePath := filepath.Join(t.TempDir(), "release-worker")
	recordPath := filepath.Join(factoryDir, "board-persistence.recording.json")
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve functional test binary: %v", err)
	}
	support.WriteAgentConfig(
		t,
		factoryDir,
		"restart-blocker",
		boardPersistenceWorkerConfig(testBinary),
	)
	return &boardPersistenceScenario{
		binaryPath: binaryPath, factoryDir: factoryDir, homeDir: homeDir,
		releasePath: releasePath, recordPath: recordPath, cliContext: cliContext,
		expected: boardPersistenceExpectedWorks(),
	}
}

func runBoardPersistenceInitialGeneration(t *testing.T, scenario *boardPersistenceScenario) {
	t.Helper()
	first := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	batchJSON := boardPersistenceBatchJSON(t, boardPersistenceRequestID, []boardPersistenceBatchWork{
		{Name: "board-init", WorkID: boardPersistenceInitialWorkID, State: "init", TraceID: "trace-board-init", Content: "durable init content"},
		{Name: "board-processing", WorkID: boardPersistenceProcessingWorkID, State: "processing", TraceID: "trace-board-processing", Content: "durable processing content"},
		{Name: "board-awaiting-ci", WorkID: boardPersistenceAwaitingWorkID, State: "awaiting-ci", TraceID: "trace-board-awaiting-ci", Content: "durable awaiting-ci content"},
	})
	submitBatchThroughCLI(t, scenario.cliContext, first, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, batchJSON, boardPersistenceRequestID, 3)

	beforeRestart := waitForBoardStates(t, first.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "processing",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
	}, 30*time.Second)
	assertBoardList(t, beforeRestart, scenario.expected)

	scenario.activeDispatchID = waitForBoardActiveDispatch(t, first.baseURL, boardPersistenceProcessingWorkID, 30*time.Second)
	if states, err := readBoardDispatchStates(t.Context(), first.baseURL); err == nil {
		t.Logf("initial active dispatch state: %#v", states[scenario.activeDispatchID])
	}
	activeObservation := waitForBoardWorkerObservation(t, first.baseURL, first.sessionID, boardPersistenceProcessingWorkID, func(observation factoryapi.WorkerSessionObservation) bool {
		return observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting
	}, 30*time.Second)
	if activeObservation.AttemptId == "" {
		t.Fatal("active Worker Session observation has empty attemptId")
	}
	scenario.activeWorkerSessionID = activeObservation.WorkerSessionId

	first.stop(t)
	if info, err := os.Stat(scenario.recordPath); err != nil || info.Size() == 0 {
		t.Fatalf("durable board recording after clean stop = %v, size=%d; want non-empty recording", err, fileSize(info))
	}
}

func runBoardPersistenceRecoveryGeneration(t *testing.T, scenario *boardPersistenceScenario) {
	t.Helper()
	second := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	afterFirstRestart := waitForBoardStates(t, second.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "processing",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
	}, 30*time.Second)
	assertBoardList(t, afterFirstRestart, scenario.expected)
	assertBoardCLIListAndShows(t, scenario.cliContext, second, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)

	rearmedDispatchID := waitForBoardRearmedDispatch(t, second.baseURL, boardPersistenceProcessingWorkID, scenario.activeDispatchID, 30*time.Second)
	rearmedObservation := waitForBoardWorkerObservation(t, second.baseURL, second.sessionID, boardPersistenceProcessingWorkID, func(observation factoryapi.WorkerSessionObservation) bool {
		return observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting
	}, 30*time.Second)
	if rearmedObservation.WorkerSessionId == scenario.activeWorkerSessionID {
		t.Fatalf("re-armed Worker Session reused original identity %q", scenario.activeWorkerSessionID)
	}
	if err := os.WriteFile(scenario.releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release re-armed worker helper: %v", err)
	}
	waitForBoardDispatchResponse(t, second.baseURL, boardPersistenceProcessingWorkID, rearmedDispatchID, 30*time.Second)
	scenario.expected[boardPersistenceProcessingWorkID] = boardPersistenceExpectedWork{
		Name:           "board-processing",
		WorkID:         boardPersistenceProcessingWorkID,
		RequestID:      boardPersistenceRequestID,
		State:          "complete",
		StateType:      "TERMINAL",
		TraceID:        "trace-board-processing",
		CurrentTraceID: "trace-board-processing",
		Content:        boardPersistenceWorkerSentinel,
		WorkerOutput:   true,
	}
	waitForBoardStates(t, second.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "complete",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
	}, 30*time.Second)

	newBatchJSON := boardPersistenceBatchJSON(t, boardPersistenceNewRequestID, []boardPersistenceBatchWork{{
		Name: "board-new-work", WorkID: boardPersistenceNewWorkID, State: "init", TraceID: "trace-board-new-work", Content: "new work after recovery",
	}})
	submitBatchThroughCLI(t, scenario.cliContext, second, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, newBatchJSON, boardPersistenceNewRequestID, 1)
	scenario.expected[boardPersistenceNewWorkID] = boardPersistenceExpectedWork{
		Name:           "board-new-work",
		WorkID:         boardPersistenceNewWorkID,
		RequestID:      boardPersistenceNewRequestID,
		State:          "init",
		StateType:      "INITIAL",
		TraceID:        "trace-board-new-work",
		CurrentTraceID: "trace-board-new-work",
		Content:        "new work after recovery",
	}

	second.stop(t)
	scenario.second = second
}

func runBoardPersistenceSecondRestart(t *testing.T, scenario *boardPersistenceScenario) {
	t.Helper()
	third := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	afterSecondRestart := waitForBoardStates(t, third.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "complete",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
		boardPersistenceNewWorkID:        "init",
	}, 30*time.Second)
	assertBoardList(t, afterSecondRestart, scenario.expected)
	assertBoardCLIListAndShows(t, scenario.cliContext, third, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)

	finalDispatches := waitForBoardDispatchStates(t, third.baseURL, 30*time.Second)
	if got := activeBoardDispatches(finalDispatches, boardPersistenceProcessingWorkID); len(got) != 0 {
		t.Fatalf("second restart restored phantom active dispatches = %#v, want none", got)
	}
	workerSessions, err := readBoardWorkerSessions(t.Context(), third.baseURL, third.sessionID, boardPersistenceProcessingWorkID)
	if err != nil {
		t.Fatalf("read second-restart Worker Session observations: %v", err)
	}
	for _, observation := range workerSessions.Sessions {
		if observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting {
			t.Fatalf("second restart left Worker Session %q in %s", observation.WorkerSessionId, observation.State)
		}
	}
	third.stop(t)
}

type boardPersistenceExpectedWork struct {
	Name           string
	WorkID         string
	RequestID      string
	State          string
	StateType      string
	TraceID        string
	CurrentTraceID string
	Content        string
	RelationTarget string
	WorkerOutput   bool
}

type boardPersistenceBatchWork struct {
	Name    string
	WorkID  string
	State   string
	TraceID string
	Content string
}

func boardPersistenceExpectedWorks() map[string]boardPersistenceExpectedWork {
	return map[string]boardPersistenceExpectedWork{
		boardPersistenceInitialWorkID: {
			Name: "board-init", WorkID: boardPersistenceInitialWorkID, RequestID: boardPersistenceRequestID,
			State: "init", StateType: "INITIAL", TraceID: "trace-board-init", CurrentTraceID: "trace-board-init", Content: "durable init content",
		},
		boardPersistenceProcessingWorkID: {
			Name: "board-processing", WorkID: boardPersistenceProcessingWorkID, RequestID: boardPersistenceRequestID,
			State: "processing", StateType: "PROCESSING", TraceID: "trace-board-processing", CurrentTraceID: "trace-board-processing", Content: "durable processing content",
		},
		boardPersistenceAwaitingWorkID: {
			Name: "board-awaiting-ci", WorkID: boardPersistenceAwaitingWorkID, RequestID: boardPersistenceRequestID,
			State: "awaiting-ci", StateType: "PROCESSING", TraceID: "trace-board-awaiting-ci", CurrentTraceID: "trace-board-awaiting-ci", Content: "durable awaiting-ci content",
			RelationTarget: boardPersistenceProcessingWorkID,
		},
	}
}

func boardPersistenceFactoryConfig() map[string]any {
	return map[string]any{
		"name": "board-persistence-restart",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "awaiting-ci", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "restart-blocker"}},
		"workstations": []map[string]any{{
			"name":      "hold-processing",
			"worker":    "restart-blocker",
			"inputs":    []map[string]string{{"workType": "task", "state": "processing"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func boardPersistenceWorkerConfig(testBinary string) string {
	return "---\n" +
		"type: SCRIPT_WORKER\n" +
		"command: " + strconv.Quote(testBinary) + "\n" +
		"args:\n" +
		"  - " + strconv.Quote("-test.run=^TestBoardPersistenceWorkerHelper$") + "\n" +
		"---\n" +
		"Hold the Work attempt until the restart test releases it.\n"
}

func boardPersistenceBatchJSON(t *testing.T, requestID string, works []boardPersistenceBatchWork) string {
	t.Helper()
	entries := make([]map[string]any, 0, len(works))
	for _, work := range works {
		entries = append(entries, map[string]any{
			"name":                     work.Name,
			"workId":                   work.WorkID,
			"workTypeName":             "task",
			"state":                    work.State,
			"traceId":                  work.TraceID,
			"currentChainingTraceId":   work.TraceID,
			"previousChainingTraceIds": []string{},
			"content":                  []map[string]any{{"type": "text", "text": work.Content}},
		})
	}
	request := map[string]any{
		"requestId": requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works":     entries,
		"relations": []map[string]string{{
			"type":           "PARENT_CHILD",
			"sourceWorkName": "board-awaiting-ci",
			"targetWorkName": "board-processing",
		}},
	}
	if requestID == boardPersistenceNewRequestID {
		request["relations"] = []map[string]string{}
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal board persistence batch: %v", err)
	}
	return string(raw)
}

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

func buildBoardPersistenceBinary(t *testing.T) string {
	t.Helper()
	binaryName := "you"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	output, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build fresh you binary: %v\n%s", err, output)
	}
	return binaryPath
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

func waitForBoardDaemonReady(t *testing.T, daemon *boardPersistenceDaemon, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// The real child process exposes no parent readiness channel. This bounded
	// public /status observation is the unavoidable process-bound synchronization
	// for the isolated-daemon acceptance test.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: 2 * time.Second}
	for {
		select {
		case <-daemon.done:
			dumpBoardPersistenceDiagnostics(t, daemon)
			t.Fatalf("isolated you daemon exited before readiness: %v\nstdout=%s\nstderr=%s", daemon.waitError(), daemon.stdout.String(), daemon.stderr.String())
		case <-ticker.C:
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, daemon.baseURL+"/status", nil)
			if err != nil {
				t.Fatalf("build daemon readiness request: %v", err)
			}
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			var status factoryapi.StatusResponse
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK && decodeErr == nil && status.RuntimeStatus != "" {
				return
			}
		case <-deadline.C:
			t.Fatalf("timed out waiting for isolated you daemon readiness at %s\nstdout=%s\nstderr=%s", daemon.baseURL, daemon.stdout.String(), daemon.stderr.String())
		}
	}
}

func dumpBoardPersistenceDiagnostics(t *testing.T, daemon *boardPersistenceDaemon) {
	t.Helper()
	t.Logf("daemon diagnostic paths: factory=%q home=%q record=%q", daemon.factoryDir, daemon.homeDir, daemon.recordPath)
	logsRoot := filepath.Join(daemon.homeDir, ".you-agent-factory", "logs")
	_ = filepath.WalkDir(logsRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			return nil
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if len(contents) > 8192 {
			contents = contents[len(contents)-8192:]
		}
		t.Logf("daemon runtime log tail %q (%d bytes): %s", path, len(contents), contents)
		return nil
	})
}

func waitForBoardPersistenceSnapshot(t *testing.T, path, wantSessionID string, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		contents, err := os.ReadFile(path)
		if err == nil && len(contents) > 0 {
			var snapshot struct {
				Session struct {
					SessionID string `json:"sessionId"`
				} `json:"session"`
			}
			if err := json.Unmarshal(contents, &snapshot); err == nil && snapshot.Session.SessionID == wantSessionID {
				return contents
			}
			lastErr = fmt.Errorf("durable snapshot was empty or session identity was not %q", wantSessionID)
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for valid durable snapshot %q: %v", path, lastErr)
		}
	}
}

func waitForBoardPersistenceLogMessage(
	t *testing.T,
	daemon *boardPersistenceDaemon,
	fragments []string,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		found := false
		_ = filepath.WalkDir(daemon.logDir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
				return nil
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			for _, fragment := range fragments {
				if !strings.Contains(string(contents), fragment) {
					return nil
				}
			}
			found = true
			return filepath.SkipAll
		})
		if found {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for recovery warning in runtime logs under %q", daemon.logDir)
		}
	}
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
		if port != 7437 {
			return address
		}
	}
}

func runBoardPersistenceCLI(
	ctx context.Context,
	binaryPath, factoryDir, homeDir, baseURL string,
	args ...string,
) ([]byte, error) {
	commandArgs := append([]string{"--server", baseURL}, args...)
	command := exec.CommandContext(ctx, binaryPath, commandArgs...)
	command.Dir = factoryDir
	command.Env = builtcliacceptance.ProcessEnvForIsolatedHome(homeDir)
	return command.CombinedOutput()
}

func submitBatchThroughCLI(
	t *testing.T,
	ctx context.Context,
	daemon *boardPersistenceDaemon,
	binaryPath, factoryDir, homeDir, batchJSON, requestID string,
	wantWorkCount int,
) {
	t.Helper()
	output, err := runBoardPersistenceCLI(ctx, binaryPath, factoryDir, homeDir, daemon.baseURL, "--json", "submit", "batch", batchJSON)
	if err != nil {
		t.Fatalf("you submit batch %q: %v\noutput:\n%s", requestID, err, output)
	}
	var acknowledgement struct {
		RequestID string `json:"requestId"`
		WorkCount int    `json:"workCount"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &acknowledgement); err != nil {
		t.Fatalf("decode you submit batch %q: %v\noutput:\n%s", requestID, err, output)
	}
	if acknowledgement.RequestID != requestID || acknowledgement.WorkCount != wantWorkCount {
		t.Fatalf("you submit batch acknowledgement = %#v, want requestId %q and workCount %d", acknowledgement, requestID, wantWorkCount)
	}
}

func waitForBoardStates(t *testing.T, baseURL string, want map[string]string, timeout time.Duration) factoryapi.ListWorkResponse {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	// Work has no process-parent notification, so state convergence is observed
	// through the public session list with a bounded ticker rather than a fixed
	// sleep. This is the process-bound wait required by this test only.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last factoryapi.ListWorkResponse
	var lastErr error
	for {
		listed, err := readBoardWorkList(t.Context(), baseURL)
		if err == nil {
			last = listed
			if boardStatesMatch(listed, want) {
				return listed
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for Work states %#v; last list=%#v, last error=%v", want, last.Results, lastErr)
		}
	}
}

func readBoardWorkList(ctx context.Context, baseURL string) (factoryapi.ListWorkResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, support.DefaultSessionWorkURL(baseURL, "/work"), nil)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.ListWorkResponse{}, fmt.Errorf("GET /work status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var listed factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		return factoryapi.ListWorkResponse{}, err
	}
	return listed, nil
}

func boardStatesMatch(listed factoryapi.ListWorkResponse, want map[string]string) bool {
	if len(listed.Results) != len(want) {
		return false
	}
	seen := make(map[string]struct{}, len(listed.Results))
	for _, item := range listed.Results {
		workID := support.StringPointerValue(item.WorkId)
		state := ""
		if item.State != nil {
			state = item.State.Name
		}
		if _, duplicate := seen[workID]; duplicate || want[workID] != state {
			return false
		}
		seen[workID] = struct{}{}
	}
	return len(seen) == len(want)
}

func assertBoardList(t *testing.T, listed factoryapi.ListWorkResponse, expected map[string]boardPersistenceExpectedWork) {
	t.Helper()
	if len(listed.Results) != len(expected) {
		t.Fatalf("Work list count = %d, want %d: %#v", len(listed.Results), len(expected), listed.Results)
	}
	seen := make(map[string]struct{}, len(listed.Results))
	for _, item := range listed.Results {
		workID := support.StringPointerValue(item.WorkId)
		if _, duplicate := seen[workID]; duplicate {
			t.Fatalf("Work list duplicated Work ID %q", workID)
		}
		want, ok := expected[workID]
		if !ok {
			t.Fatalf("Work list returned unexpected Work ID %q: %#v", workID, item)
		}
		seen[workID] = struct{}{}
		assertBoardWork(t, item, want)
	}
}

func assertBoardCLIListAndShows(
	t *testing.T,
	ctx context.Context,
	daemon *boardPersistenceDaemon,
	binaryPath, factoryDir, homeDir string,
	expected map[string]boardPersistenceExpectedWork,
) {
	t.Helper()
	listOutput, err := runBoardPersistenceCLI(ctx, binaryPath, factoryDir, homeDir, daemon.baseURL, "--json", "work", "list")
	if err != nil {
		t.Fatalf("you work list: %v\noutput:\n%s", err, listOutput)
	}
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal(bytes.TrimSpace(listOutput), &listed); err != nil {
		t.Fatalf("decode you work list: %v\noutput:\n%s", err, listOutput)
	}
	assertBoardList(t, listed, expected)

	for workID, want := range expected {
		showOutput, err := runBoardPersistenceCLI(ctx, binaryPath, factoryDir, homeDir, daemon.baseURL, "--json", "work", "show", workID)
		if err != nil {
			t.Fatalf("you work show %s: %v\noutput:\n%s", workID, err, showOutput)
		}
		var shown factoryapi.Work
		if err := json.Unmarshal(bytes.TrimSpace(showOutput), &shown); err != nil {
			t.Fatalf("decode you work show %s: %v\noutput:\n%s", workID, err, showOutput)
		}
		assertBoardWork(t, shown, want)
	}
}

func assertBoardWork(t *testing.T, item factoryapi.Work, want boardPersistenceExpectedWork) {
	t.Helper()
	if item.Name != want.Name || support.StringPointerValue(item.WorkId) != want.WorkID || support.StringPointerValue(item.RequestId) != want.RequestID {
		t.Fatalf("Work identity = %#v, want name=%q workId=%q requestId=%q", item, want.Name, want.WorkID, want.RequestID)
	}
	if item.State == nil || item.State.Name != want.State || string(item.State.Type) != want.StateType {
		t.Fatalf("Work %q state = %#v, want %s/%s", want.WorkID, item.State, want.State, want.StateType)
	}
	if support.StringPointerValue(item.TraceId) != want.TraceID || support.StringPointerValue(item.CurrentChainingTraceId) != want.CurrentTraceID {
		t.Fatalf("Work %q lineage = trace=%q current=%q, want %q/%q", want.WorkID, support.StringPointerValue(item.TraceId), support.StringPointerValue(item.CurrentChainingTraceId), want.TraceID, want.CurrentTraceID)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("Work %q content = %#v, want one text part", want.WorkID, item.Content)
	}
	part, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("Work %q content part = %#v, decode error=%v, want %q", want.WorkID, part, err, want.Content)
	}
	if want.WorkerOutput {
		assertBoardPersistenceWorkerOutput(t, part.Text, want.Content)
	} else if part.Text != want.Content {
		t.Fatalf("Work %q content part = %#v, want %q", want.WorkID, part, want.Content)
	}
	if want.RelationTarget == "" {
		if item.Relations != nil && len(*item.Relations) != 0 {
			t.Fatalf("Work %q relations = %#v, want none", want.WorkID, item.Relations)
		}
		return
	}
	if item.Relations == nil || len(*item.Relations) != 1 {
		t.Fatalf("Work %q relations = %#v, want one PARENT_CHILD relation", want.WorkID, item.Relations)
	}
	relation := (*item.Relations)[0]
	if relation.Type != factoryapi.RelationTypeParentChild || relation.SourceWorkName != want.Name || relation.TargetWorkId == nil || *relation.TargetWorkId != want.RelationTarget {
		t.Fatalf("Work %q relation = %#v, want source=%q target=%q PARENT_CHILD", want.WorkID, relation, want.Name, want.RelationTarget)
	}
}

func assertBoardPersistenceWorkerOutput(t *testing.T, got, sentinel string) {
	t.Helper()
	lines := strings.Split(got, "\n")
	if len(lines) < 2 || lines[0] != sentinel || lines[1] != "PASS" {
		t.Fatalf("worker output = %q, want exactly the sentinel and PASS worker lines", got)
	}

	suffix := lines[2:]
	if len(suffix) == 0 {
		return
	}
	if len(suffix) == 1 && suffix[0] == "coverage: [no statements]" {
		return
	}

	const coveragePrefix = "coverage: 0.0% of statements"
	if !strings.HasPrefix(suffix[0], coveragePrefix) {
		t.Fatalf("worker output = %q, want the sentinel/PASS lines plus a recognized Go coverage suffix", got)
	}
	coverageDetail := strings.TrimPrefix(suffix[0], coveragePrefix)
	if coverageDetail != "" && !strings.HasPrefix(coverageDetail, " in ") {
		t.Fatalf("worker output = %q, want Go coverage detail to use the standard ' in <package list>' form", got)
	}
	if strings.TrimSpace(coverageDetail) == "in" {
		t.Fatalf("worker output = %q, want a non-empty Go coverage package list", got)
	}
	for _, packageLine := range suffix[1:] {
		if !strings.Contains(packageLine, "github.com/portpowered/infinite-you/") {
			t.Fatalf("worker output = %q, want only the Go harness package-list suffix after coverage", got)
		}
	}
}

type boardPersistenceDispatchState struct {
	ID                 string
	WorkIDs            map[string]struct{}
	RequestEvents      int
	ResponseEvents     int
	InterruptedEvents  int
	ReconciledStatuses []factoryapi.FactoryDispatchStatus
	WorkerSessionIDs   []string
}

func waitForBoardActiveDispatch(t *testing.T, baseURL, workID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			active := activeBoardDispatches(states, workID)
			if len(active) == 1 {
				return active[0].ID
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for one active dispatch for Work %q; last=%#v, error=%v", workID, last, lastErr)
		}
	}
}

func waitForBoardRearmedDispatch(t *testing.T, baseURL, workID, oldDispatchID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			old, oldFound := states[oldDispatchID]
			active := activeBoardDispatches(states, workID)
			if oldFound && old.InterruptedEvents == 1 && len(active) == 1 && active[0].ID != oldDispatchID {
				if active[0].RequestEvents != 1 {
					t.Fatalf("re-armed dispatch %q request events = %d, want exactly one", active[0].ID, active[0].RequestEvents)
				}
				return active[0].ID
			}
			if oldFound && old.InterruptedEvents > 1 {
				t.Fatalf("original dispatch %q interruption events = %d, want exactly one", oldDispatchID, old.InterruptedEvents)
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for dispatch %q interruption and one re-armed dispatch for Work %q; last=%#v, error=%v", oldDispatchID, workID, last, lastErr)
		}
	}
}

func waitForBoardDispatchResponse(t *testing.T, baseURL, workID, dispatchID string, timeout time.Duration) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			if state, ok := states[dispatchID]; ok && state.ResponseEvents == 1 && len(activeBoardDispatches(states, workID)) == 0 {
				return
			}
		} else {
			lastErr = err
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for dispatch %q response for Work %q; last=%#v, error=%v", dispatchID, workID, last, lastErr)
		}
	}
}

func waitForBoardDispatchStates(t *testing.T, baseURL string, timeout time.Duration) map[string]boardPersistenceDispatchState {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	var lastErr error
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			return states
		}
		lastErr = err
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out reading public Factory Event dispatch history; last=%#v, error=%v", last, lastErr)
		}
	}
}

func readBoardDispatchStates(ctx context.Context, baseURL string) (map[string]boardPersistenceDispatchState, error) {
	events, err := readBoardEvents(ctx, baseURL)
	if err != nil {
		return nil, err
	}
	states := make(map[string]boardPersistenceDispatchState)
	for _, event := range events {
		if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
			continue
		}
		dispatchID := strings.TrimSpace(*event.Context.DispatchId)
		state := states[dispatchID]
		state.ID = dispatchID
		if event.Context.WorkIds != nil {
			if state.WorkIDs == nil {
				state.WorkIDs = make(map[string]struct{})
			}
			for _, workID := range *event.Context.WorkIds {
				if workID != "" {
					state.WorkIDs[workID] = struct{}{}
				}
			}
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, decodeErr := event.Payload.AsDispatchRequestEventPayload()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch request %q: %w", dispatchID, decodeErr)
			}
			state.RequestEvents++
			if state.WorkIDs == nil {
				state.WorkIDs = make(map[string]struct{})
			}
			for _, input := range payload.Inputs {
				if input.WorkId != "" {
					state.WorkIDs[input.WorkId] = struct{}{}
				}
			}
		case factoryapi.FactoryEventTypeDispatchResponse:
			if _, decodeErr := event.Payload.AsDispatchResponseEventPayload(); decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch response %q: %w", dispatchID, decodeErr)
			}
			state.ResponseEvents++
		case factoryapi.FactoryEventTypeDispatchInterrupted:
			if _, decodeErr := event.Payload.AsDispatchInterruptedEventPayload(); decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch interruption %q: %w", dispatchID, decodeErr)
			}
			state.InterruptedEvents++
		case factoryapi.FactoryEventTypeDispatchReconciled:
			payload, decodeErr := event.Payload.AsDispatchReconciledEventPayload()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode public dispatch reconciliation %q: %w", dispatchID, decodeErr)
			}
			state.ReconciledStatuses = append(state.ReconciledStatuses, payload.ReconciledStatus)
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			payload, decodeErr := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if decodeErr != nil {
				return nil, fmt.Errorf("decode public Worker Session association %q: %w", dispatchID, decodeErr)
			}
			if payload.WorkerSessionId != "" {
				state.WorkerSessionIDs = append(state.WorkerSessionIDs, payload.WorkerSessionId)
			}
		}
		states[dispatchID] = state
	}
	return states, nil
}

func activeBoardDispatches(states map[string]boardPersistenceDispatchState, workID string) []boardPersistenceDispatchState {
	ids := make([]string, 0, len(states))
	for dispatchID, state := range states {
		if state.RequestEvents == 0 || !boardPersistenceDispatchIncludesWork(state, workID) || state.ResponseEvents > 0 || state.InterruptedEvents > 0 || len(state.ReconciledStatuses) > 0 {
			continue
		}
		ids = append(ids, dispatchID)
	}
	sort.Strings(ids)
	active := make([]boardPersistenceDispatchState, 0, len(ids))
	for _, dispatchID := range ids {
		active = append(active, states[dispatchID])
	}
	return active
}

func boardPersistenceDispatchIncludesWork(state boardPersistenceDispatchState, workID string) bool {
	_, ok := state.WorkIDs[workID]
	return ok
}
