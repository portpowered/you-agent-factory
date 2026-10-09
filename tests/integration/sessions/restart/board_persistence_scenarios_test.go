package restart_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// TestBoardPersistenceWorkerHelper is launched by real SCRIPT_WORKER children
// in restart process tests. Test cases hold the helper at a parent-owned gate
// or release file while they inspect public runtime state.
func TestBoardPersistenceWorkerHelper(t *testing.T) {
	if os.Getenv(boardPersistenceHelperEnv) != boardPersistenceHelperEnvValue {
		return
	}
	if readyEndpoint := strings.TrimSpace(os.Getenv(boardPersistenceWorkerReadyEnv)); readyEndpoint != "" {
		fmt.Fprintln(os.Stdout, boardPersistenceWorkerSentinel)
		signalBoardPersistenceWorkerReady(t, readyEndpoint)
		return
	}
	releasePath := strings.TrimSpace(os.Getenv(boardPersistenceReleaseEnv))
	if releasePath == "" {
		t.Fatal("board persistence worker helper release path is empty")
	}
	fmt.Fprintln(os.Stdout, boardPersistenceWorkerSentinel)

	// The process-level test may provide a parent-owned barrier endpoint so it
	// can capture public dispatch state while this child is held. Other restart
	// scenarios use the bounded release-file gate below.
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

func signalBoardPersistenceWorkerReady(t *testing.T, readyEndpoint string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, readyEndpoint, nil)
	if err != nil {
		t.Fatalf("build worker readiness request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("signal worker readiness: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("worker readiness response status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

// TestBoardPersistenceCLIRestartRoundTrip proves the customer-visible board
// contract across real daemon processes. The in-process functional harnesses
// cover service composition; this test intentionally crosses the OS boundary
// because a daemon restart is the failure boundary being repaired.
func TestBoardPersistenceCLIRestartRoundTrip(t *testing.T) {
	t.Parallel()
	scenario := newBoardPersistenceScenario(t)
	t.Run("source-to-successor", func(t *testing.T) {
		// This cell owns exactly two Factory process generations: the original
		// owner and its externally started successor.
		runBoardPersistenceInitialGeneration(t, scenario)
		runBoardPersistenceRecoveryGeneration(t, scenario)
	})
	t.Run("successor-recording-restart", func(t *testing.T) {
		// The next serialized cell consumes the flushed successor recording and
		// owns only that final process generation.
		runBoardPersistenceSecondRestart(t, scenario)
	})
}

// TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording proves
// that a valid durable snapshot survives an ungraceful daemon stop even when
// the current-board recording is absent at the next opening boundary. The
// process boundary is intentional: BuildProcess covers composition, while
// only a real child process can prove kill-and-reopen behavior.
func TestBoardPersistenceCLIRestartAfterHardKillWithMissingBoardRecording(t *testing.T) {
	t.Parallel()
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
	}, 120*time.Second)
	if err := os.WriteFile(scenario.releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release worker helper before durable snapshot probe: %v", err)
	}
	waitForBoardStates(t, first.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "complete",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
	}, 120*time.Second)

	snapshotPath := filepath.Join(
		scenario.factoryDir,
		".you-agent-factory",
		"durable-sessions",
		factorysessions.DefaultSessionID+".json",
	)
	if strings.TrimSpace(first.sessionID) == "" {
		t.Fatal("hard-kill scenario session ID is empty")
	}
	snapshotBefore := waitForBoardPersistenceSnapshot(t, snapshotPath, factorysessions.DefaultSessionID, 120*time.Second)

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
	restarted := waitForBoardStates(t, second.baseURL, map[string]string{}, 120*time.Second)
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
	}, 120*time.Second)
}

// TestRecordStartupSafetyFailedRestore proves that a
// present-but-invalid current-board artifact is not treated as an interrupted
// write. The child process is intentional so the assertion covers the actual
// operator-facing startup diagnostic emitted by the real CLI.
func TestRecordStartupSafetyFailedRestore(t *testing.T) {
	t.Parallel()
	scenario := newBoardPersistenceScenario(t)
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "RF-I1-failed-restore", t.Name())
	evidence.StoryID = "record-flag-never-destroys-an-existing-recording-001"
	t.Cleanup(func() { evidence.publishRestartScenario(t) })
	const privateMarker = "private-recording-decoder-marker"
	corruptPayload := []byte(`{"schemaVersion":"replay.v1","private":"` + privateMarker + `","events":[`)
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
	evidence.trackDaemon(t, "failed-restore-after-cleanup", daemon)
	waitForBoardPersistenceDaemonExit(t, daemon, 90*time.Second)
	var exitError *exec.ExitError
	if !errors.As(daemon.waitError(), &exitError) || exitError.ExitCode() != 1 {
		t.Fatalf("failed restore exit = %v, want CLI exit 1", daemon.waitError())
	}
	output := daemon.stdout.String() + daemon.stderr.String()
	assertRecordStartupSafetyRestoreDiagnostic(t, output, scenario.recordPath)
	contents, err := os.ReadFile(scenario.recordPath)
	if err != nil {
		t.Fatalf("read corrupt current-board recording after failed startup: %v", err)
	}
	if !bytes.Equal(contents, corruptPayload) {
		t.Fatal("failed startup changed the corrupt recording; artifact must remain available for investigation")
	}
	logs := collectRestartRuntimeLogs(daemon.logDir)
	encodedLogs, err := json.Marshal(logs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output, privateMarker) || bytes.Contains(encodedLogs, []byte(privateMarker)) {
		t.Fatal("startup diagnostic exposed recording payload")
	}
	if len(logs) == 0 || !bytes.Contains(encodedLogs, []byte("runtime_startup_failed")) ||
		!bytes.Contains(encodedLogs, []byte("invalid JSON at byte 84")) {
		t.Fatalf("failed restore runtime logs lack the safe startup cause: %s", encodedLogs)
	}
	t.Logf("RF-I1 failed restore: exit=1, retained bytes=%d, safe JSON cause in ErrorResponse, payload withheld; runtime log files=%d", len(contents), len(logs))
}

// This cell consumes the same prebuilt CLI as the failed-restore cell. Work
// stays in states without a workstation, so recovery needs no worker process.
func TestRecordStartupSafetyResumeCopy(t *testing.T) {
	t.Parallel()
	scenario := newBoardPersistenceScenario(t)
	delete(scenario.expected, boardPersistenceProcessingWorkID)
	for id, want := range scenario.expected {
		want.RequestID = boardPersistenceNewRequestID
		want.RelationTarget = ""
		scenario.expected[id] = want
	}
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "RF-I1-resume-copy", t.Name())
	evidence.StoryID = "record-flag-never-destroys-an-existing-recording-001"
	t.Cleanup(func() { evidence.publishRestartScenario(t) })
	first := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	evidence.trackDaemon(t, "source-graceful-stop", first)
	batch := boardPersistenceBatchJSON(t, boardPersistenceNewRequestID, []boardPersistenceBatchWork{
		{Name: "board-init", WorkID: boardPersistenceInitialWorkID, State: "init", TraceID: "trace-board-init", Content: "durable init content"},
		{Name: "board-awaiting-ci", WorkID: boardPersistenceAwaitingWorkID, State: "awaiting-ci", TraceID: "trace-board-awaiting-ci", Content: "durable awaiting-ci content"},
	})
	submitBoardPersistenceBatchThroughCLI(t, first, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, batch, boardPersistenceNewRequestID, 2)
	assertBoardCLIListAndShows(t, first, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)
	first.stop(t)
	source, err := os.ReadFile(scenario.recordPath)
	if err != nil || len(source) == 0 {
		t.Fatalf("read gracefully stopped source: bytes=%d, error=%v", len(source), err)
	}
	copyPath := filepath.Join(t.TempDir(), "board-backup.json")
	if err := os.WriteFile(copyPath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := evidence.recordSourceRecording(copyPath); err != nil {
		t.Fatal(err)
	}
	successorPath := filepath.Join(t.TempDir(), "successor.json")
	assertRecordStartupSafetyCopyGeneration(t, scenario, evidence, "resume-copy", copyPath, successorPath)
	assertRecordStartupSafetyUnchangedFile(t, scenario.recordPath, source)
	assertRecordStartupSafetyUnchangedFile(t, copyPath, source)
	successor, err := os.ReadFile(successorPath)
	if err != nil || len(successor) == 0 {
		t.Fatalf("read flushed successor: bytes=%d, error=%v", len(successor), err)
	}
	// A fresh customer profile rules out recovering Work from the source's
	// durable snapshot instead of reconstructing the selected successor.
	assertRecordStartupSafetyCopyGeneration(t, scenario, evidence, "resume-successor", successorPath, filepath.Join(t.TempDir(), "next.json"))
	assertRecordStartupSafetyUnchangedFile(t, successorPath, successor)
	if err := evidence.verifySourceRecordingUnchanged(copyPath); err != nil {
		t.Fatal(err)
	}
	if err := evidence.recordSuccessorRecordings(copyPath, successorPath); err != nil {
		t.Fatal(err)
	}
	t.Logf("RF-I1 resume copy: original and backup unchanged (%d bytes); successor readable (%d bytes); Work IDs/states retained", len(source), len(successor))
}

func assertRecordStartupSafetyCopyGeneration(t *testing.T, scenario *boardPersistenceScenario, evidence *restartBaselineEvidence, name, source, target string) {
	t.Helper()
	home := t.TempDir()
	daemon := startBoardPersistenceResumeDaemon(t, scenario.binaryPath, scenario.factoryDir, home, source, target, scenario.releasePath)
	evidence.trackDaemon(t, name, daemon)
	assertBoardCLIListAndShows(t, daemon, scenario.binaryPath, scenario.factoryDir, home, scenario.expected)
	daemon.stop(t)
}

func assertRecordStartupSafetyUnchangedFile(t *testing.T, path string, expected []byte) {
	t.Helper()
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("resume changed retained source %q", path)
	}
}

func assertRecordStartupSafetyRestoreDiagnostic(t *testing.T, output, recordPath string) {
	t.Helper()
	for _, fragment := range []string{
		"CURRENT_BOARD_RECORDING_CORRUPT",
		"CORRUPT_HISTORY",
		filepath.Base(recordPath),
		"preserve the artifact",
		"replace it from a trusted backup",
		"invalid JSON at byte",
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
	expectedRecordPath := strconv.Quote(filepath.Clean(recordPath))
	if !strings.Contains(diagnostic.Message, expectedRecordPath) {
		t.Fatalf("corrupt recording diagnostic message = %q, want exact resolved path %q", diagnostic.Message, expectedRecordPath)
	}
	if strings.Contains(output, "board contents were lost") || strings.Contains(output, "empty board was initialized") {
		t.Fatalf("corrupt recording was reported as recoverable absence: %q", output)
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
}

func newBoardPersistenceScenario(t *testing.T) *boardPersistenceScenario {
	t.Helper()
	binaryPath := requireRestartCLIArtifact(t)
	workerPath := currentRestartWorkerExecutable(t)
	factoryDir := scaffoldBoardPersistenceFactory(t, boardPersistenceFactoryConfig())
	homeDir := t.TempDir()
	releasePath := filepath.Join(t.TempDir(), "release-worker")
	recordPath := filepath.Join(factoryDir, "board-persistence.recording.json")
	writeBoardPersistenceAgentConfig(
		t,
		factoryDir,
		"restart-blocker",
		boardPersistenceWorkerConfig(workerPath),
	)
	return &boardPersistenceScenario{
		binaryPath: binaryPath, factoryDir: factoryDir, homeDir: homeDir,
		releasePath: releasePath, recordPath: recordPath,
		expected: boardPersistenceExpectedWorks(),
	}
}

func scaffoldBoardPersistenceFactory(t *testing.T, config map[string]any) string {
	t.Helper()
	dir := t.TempDir()
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal board-persistence Factory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), raw, 0o644); err != nil {
		t.Fatalf("write board-persistence Factory: %v", err)
	}
	workstationPath := filepath.Join(dir, "workstations", "hold-processing", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(workstationPath), 0o755); err != nil {
		t.Fatalf("create board-persistence workstation directory: %v", err)
	}
	if err := os.WriteFile(workstationPath, []byte("---\ntype: MODEL_WORKSTATION\n---\nHold the Work attempt.\n"), 0o644); err != nil {
		t.Fatalf("write board-persistence workstation: %v", err)
	}
	return dir
}

func writeBoardPersistenceAgentConfig(t *testing.T, dir, workerName, content string) {
	t.Helper()
	path := filepath.Join(dir, "workers", workerName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create board-persistence worker directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write board-persistence worker config: %v", err)
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
	}, 120*time.Second)
	assertBoardList(t, beforeRestart, scenario.expected)

	scenario.activeDispatchID = waitForBoardActiveDispatch(t, first.baseURL, boardPersistenceProcessingWorkID, 120*time.Second)
	if states, err := readBoardDispatchStates(t.Context(), first.baseURL); err == nil {
		t.Logf("initial active dispatch state: %#v", states[scenario.activeDispatchID])
	}
	activeObservation := waitForBoardWorkerObservation(t, first.baseURL, first.sessionID, boardPersistenceProcessingWorkID, func(observation factoryapi.WorkerSessionObservation) bool {
		return observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting
	}, 120*time.Second)
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
	}, 120*time.Second)
	assertBoardList(t, afterFirstRestart, scenario.expected)
	assertBoardCLIListAndShows(t, second, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)

	rearmedDispatchID := waitForBoardRearmedDispatch(t, second.baseURL, boardPersistenceProcessingWorkID, scenario.activeDispatchID, 120*time.Second)
	rearmedObservation := waitForBoardWorkerObservation(t, second.baseURL, second.sessionID, boardPersistenceProcessingWorkID, func(observation factoryapi.WorkerSessionObservation) bool {
		return observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting
	}, 120*time.Second)
	if rearmedObservation.WorkerSessionId == scenario.activeWorkerSessionID {
		t.Fatalf("re-armed Worker Session reused original identity %q", scenario.activeWorkerSessionID)
	}
	if err := os.WriteFile(scenario.releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release re-armed worker helper: %v", err)
	}
	waitForBoardDispatchResponse(t, second.baseURL, boardPersistenceProcessingWorkID, rearmedDispatchID, 120*time.Second)
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
	}, 120*time.Second)

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
}

func runBoardPersistenceSecondRestart(t *testing.T, scenario *boardPersistenceScenario) {
	t.Helper()
	third := startBoardPersistenceDaemon(t, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.recordPath, scenario.releasePath)
	afterSecondRestart := waitForBoardStates(t, third.baseURL, map[string]string{
		boardPersistenceInitialWorkID:    "init",
		boardPersistenceProcessingWorkID: "complete",
		boardPersistenceAwaitingWorkID:   "awaiting-ci",
		boardPersistenceNewWorkID:        "init",
	}, 120*time.Second)
	assertBoardList(t, afterSecondRestart, scenario.expected)
	assertBoardCLIListAndShows(t, third, scenario.binaryPath, scenario.factoryDir, scenario.homeDir, scenario.expected)

	finalDispatches := waitForBoardDispatchStates(t, third.baseURL, 120*time.Second)
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
