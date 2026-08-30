package restart_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestBoardPersistenceWorkerHelper is launched by the real SCRIPT_WORKER child
// in TestBoardPersistenceCLIRestartRoundTrip. It exits only after the test has
// inspected the re-armed attempt, which makes the second dispatch observable
// without relying on a mock worker edge.
func TestBoardPersistenceWorkerHelper(t *testing.T) {
	if os.Getenv(boardPersistenceHelperEnv) != boardPersistenceHelperEnvValue {
		return
	}
	if builds := boardPersistenceBinaryBuilds.Load(); builds != 0 {
		t.Fatalf("SCRIPT_WORKER helper observed %d package CLI builds, want 0", builds)
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
	scenario := newBoardPersistenceScenario(t)
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
	scenario := newBoardPersistenceScenario(t)
	first := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	batchJSON := boardPersistenceBatchJSON(t, boardPersistenceRequestID, []boardPersistenceBatchWork{
		{Name: "board-init", WorkID: boardPersistenceInitialWorkID, State: "init", TraceID: "trace-board-init", Content: "durable init content"},
		{Name: "board-processing", WorkID: boardPersistenceProcessingWorkID, State: "processing", TraceID: "trace-board-processing", Content: "durable processing content"},
		{Name: "board-awaiting-ci", WorkID: boardPersistenceAwaitingWorkID, State: "awaiting-ci", TraceID: "trace-board-awaiting-ci", Content: "durable awaiting-ci content"},
	})
	submitBoardPersistenceBatchThroughCLI(t, first, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, batchJSON, boardPersistenceRequestID, 3)
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
	scenario := newBoardPersistenceScenario(t)
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
	var diagnostic factoryapi.ErrorResponse
	for _, line := range strings.Split(output, "\n") {
		var candidate factoryapi.ErrorResponse
		if err := json.Unmarshal([]byte(line), &candidate); err == nil && candidate.Code == factoryapi.ErrorResponseCode("CURRENT_BOARD_RECORDING_CORRUPT") {
			diagnostic = candidate
			break
		}
	}
	if diagnostic.Code != factoryapi.ErrorResponseCode("CURRENT_BOARD_RECORDING_CORRUPT") {
		t.Fatalf("corrupt recording startup output = %q, want structured corruption diagnostic", output)
	}
	expectedRecordPath := strconv.Quote(filepath.Clean(scenario.recordPath))
	if !strings.Contains(diagnostic.Message, expectedRecordPath) {
		t.Fatalf("corrupt recording diagnostic message = %q, want exact resolved path %q", diagnostic.Message, expectedRecordPath)
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
	expected              map[string]boardPersistenceExpectedWork
	activeDispatchID      string
	activeWorkerSessionID string
	second                *boardPersistenceDaemon
}

func newBoardPersistenceScenario(t *testing.T) *boardPersistenceScenario {
	t.Helper()
	if markerPath := strings.TrimSpace(os.Getenv(boardPersistenceScenarioMarkerEnv)); markerPath != "" {
		if err := os.WriteFile(markerPath, []byte("scenario started\n"), 0o600); err != nil {
			t.Fatalf("record scenario start for setup-failure probe: %v", err)
		}
	}
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
		releasePath: releasePath, recordPath: recordPath,
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
	submitBoardPersistenceBatchThroughCLI(t, first, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, batchJSON, boardPersistenceRequestID, 3)

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
	assertBoardCLIListAndShows(t, second, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)

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
	submitBoardPersistenceBatchThroughCLI(t, second, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, newBatchJSON, boardPersistenceNewRequestID, 1)
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
	assertBoardCLIListAndShows(t, third, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)

	finalDispatches := waitForBoardDispatchStates(t, third.baseURL, 30*time.Second)
	if got := activeBoardDispatches(finalDispatches, boardPersistenceProcessingWorkID); len(got) != 0 {
		t.Fatalf("second restart restored phantom active dispatches = %#v, want none", got)
	}
	workerSessions, err := readBoardWorkerSessions(t.Context(), third.baseURL, third.sessionID, boardPersistenceProcessingWorkID)
	if err != nil {
		t.Fatalf("read second-restart Worker Session observations: %v", err)
	}
	assertBoardCLIWorkerSessionsForWork(
		t,
		third,
		scenario.binaryPath,
		scenario.factoryDir,
		scenario.homeDir,
		boardPersistenceProcessingWorkID,
	)
	for _, observation := range workerSessions.Sessions {
		if observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting {
			t.Fatalf("second restart left Worker Session %q in %s", observation.WorkerSessionId, observation.State)
		}
	}
	third.stop(t)
}
