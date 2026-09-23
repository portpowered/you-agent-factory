package restart_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	restartRecoveryRequestID      = "restart-recovery-request"
	restartRecoveryEligibleWorkID = "restart-recovery-eligible"
	restartRecoveryCompleteWorkID = "restart-recovery-terminal-complete"
	restartRecoveryFailedWorkID   = "restart-recovery-terminal-failed"
	restartRecoveryWorkerName     = "recorded-recovery-worker"
	restartRecoverySecretMarker   = "restart-recovery-private-fixture-value"
	restartRecoveryProcessTimeout = 20 * time.Second
)

type restartRecoveryFailureFixture struct {
	id         string
	resumePath func(string) string
	contents   []byte
}

// TestRestartRecoveryRestoresRecordedDefinitionAndSingleOwner is the H-01
// compiled-artifact witness. The authored Factory B cannot consume the
// processing Work. A new dispatch therefore demonstrates that resume selected
// the recorded Factory A snapshot after a real process stop.
func TestRestartRecoveryRestoresRecordedDefinitionAndSingleOwner(t *testing.T) {
	t.Parallel()
	artifactPath := requireRestartCLIArtifact(t)
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "H-01", t.Name())
	t.Cleanup(func() { evidence.publishRestartScenario(t) })
	fixture := prepareRestartRecoveryH01Fixture(t, artifactPath, evidence)
	source := runRestartRecoveryH01SourceGeneration(t, fixture)
	verifyRestartRecoveryH01SuccessorGeneration(t, fixture, source)
}

func verifyRestartRecoveryH01SuccessorGeneration(
	t *testing.T,
	fixture restartRecoveryH01Fixture,
	source restartRecoveryH01SourceState,
) {
	t.Helper()
	second := startBoardPersistenceObservedResumeDaemon(
		t, fixture.artifactPath, fixture.factoryB, fixture.homeDir,
		fixture.sourceRecordPath, fixture.successorRecordPath, fixture.releasePath,
	)
	fixture.evidence.trackDaemon(t, "successor-process-recorded-definition-a", second)
	secondWorks := waitForBoardStates(t, second.baseURL, map[string]string{
		restartRecoveryEligibleWorkID: "processing",
		restartRecoveryCompleteWorkID: "complete",
		restartRecoveryFailedWorkID:   "failed",
	}, 30*time.Second)
	assertBoardList(t, secondWorks, source.expectedWorks)
	assertRestartWorkIDsUnique(t, secondWorks, source.expectedWorks)
	if got := restartWorkHistoryFingerprint(t, second.baseURL, restartRecoveryWorkIDs()); !equalStringSlices(got, source.workEventsBefore) {
		t.Fatalf("admitted and terminal Work history changed during resume:\nbefore=%v\nafter=%v", source.workEventsBefore, got)
	}
	newDispatchID := waitForBoardRearmedDispatch(t, second.baseURL, restartRecoveryEligibleWorkID, source.oldDispatchID, 30*time.Second)
	newWorker := waitForBoardWorkerObservation(t, second.baseURL, second.sessionID, restartRecoveryEligibleWorkID, func(observation factoryapi.WorkerSessionObservation) bool {
		return observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting
	}, 30*time.Second)
	if newWorker.WorkerSessionId == source.oldWorkerSessionID {
		t.Fatalf("successor reused source Worker Session %q", source.oldWorkerSessionID)
	}
	newOwnerDispatch, newOwnerID := waitForBoardActiveOwner(t, second.baseURL, restartRecoveryEligibleWorkID, 30*time.Second)
	if newOwnerDispatch != newDispatchID || newOwnerID != newWorker.WorkerSessionId || newOwnerID == source.oldOwnerID {
		t.Fatalf("successor active owner = %q/%q; dispatch=%q worker=%q oldOwner=%q", newOwnerDispatch, newOwnerID, newDispatchID, newWorker.WorkerSessionId, source.oldOwnerID)
	}
	resumedObservation := fixture.evidence.capturePublicObservation(t, "successor-before-worker-release", second.baseURL)
	assertRestartPublicCounts(t, resumedObservation, 1, 3, 1, 1)
	assertRestartActiveWorkerCount(t, resumedObservation, 1)
	completeRestartRecoveryDispatch(
		t, fixture.evidence, second, fixture.releasePath,
		source.oldDispatchID, newDispatchID, source.terminalHistoryBefore,
	)
	finalizeRestartRecoveryEvidence(
		t, fixture.evidence, second, fixture.sourceRecordPath,
		fixture.successorRecordPath, fixture.factoryA, fixture.factoryB,
	)
}

func completeRestartRecoveryDispatch(t *testing.T, evidence *restartBaselineEvidence, daemon *boardPersistenceDaemon, releasePath, oldDispatchID, newDispatchID string, terminalHistoryBefore []string) {
	t.Helper()
	if err := os.WriteFile(releasePath, []byte("release\n"), 0o600); err != nil {
		t.Fatalf("release successor worker: %v", err)
	}
	waitForBoardDispatchResponse(t, daemon.baseURL, restartRecoveryEligibleWorkID, newDispatchID, 30*time.Second)
	finalWorks := waitForBoardStates(t, daemon.baseURL, map[string]string{
		restartRecoveryEligibleWorkID: "complete",
		restartRecoveryCompleteWorkID: "complete",
		restartRecoveryFailedWorkID:   "failed",
	}, 30*time.Second)
	expectedWorks := restartRecoveryExpectedWorks(true)
	assertBoardList(t, finalWorks, expectedWorks)
	assertRestartWorkIDsUnique(t, finalWorks, expectedWorks)
	finalEvents, err := readBoardEvents(t.Context(), daemon.baseURL)
	if err != nil {
		t.Fatalf("read final Factory Event history: %v", err)
	}
	assertRestartReconciledBeforeRearm(t, finalEvents, oldDispatchID, newDispatchID)
	assertSingleTerminalCompletion(t, finalEvents, restartRecoveryEligibleWorkID, "complete")
	if got := restartWorkHistoryFingerprint(t, daemon.baseURL, restartRecoveryTerminalWorkIDs()); !equalStringSlices(got, terminalHistoryBefore) {
		t.Fatalf("terminal Work admission/failure history changed after recovered completion:\nbefore=%v\nafter=%v", terminalHistoryBefore, got)
	}
	dispatchStates, err := readBoardDispatchStates(t.Context(), daemon.baseURL)
	if err != nil {
		t.Fatalf("read final dispatch ownership history: %v", err)
	}
	assertRestartDispatchHistory(t, dispatchStates, oldDispatchID, newDispatchID, restartRecoveryEligibleWorkID)
	finalObservation := evidence.capturePublicObservation(t, "successor-after-one-completion", daemon.baseURL)
	assertRestartPublicCounts(t, finalObservation, 1, 3, 0, 0)
}

func finalizeRestartRecoveryEvidence(t *testing.T, evidence *restartBaselineEvidence, daemon *boardPersistenceDaemon, sourceRecordPath, successorRecordPath, factoryA, factoryB string) {
	t.Helper()
	if _, err := waitForRestartRecoveryRecord(t, daemon, "success", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	assertRestartRecoveryRecordSafe(t, daemon, "success")
	daemon.stop(t)
	evidence.captureDaemon(1, daemon)
	if err := verifyRestartFixtureHashes(evidence, map[string]string{
		"factory-a/factory.json":          filepath.Join(factoryA, "factory.json"),
		"factory-b/factory.json":          filepath.Join(factoryB, "factory.json"),
		"factory-a/workstation/AGENTS.md": filepath.Join(factoryA, "workstations", "hold-processing", "AGENTS.md"),
		"factory-a/worker/AGENTS.md":      filepath.Join(factoryA, "workers", restartRecoveryWorkerName, "AGENTS.md"),
		"factory-b/workstation/AGENTS.md": filepath.Join(factoryB, "workstations", "hold-processing", "AGENTS.md"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := evidence.verifyFixtureFilesUnchangedFromHashes(map[string]string{"source.recording.json": sourceRecordPath}); err != nil {
		t.Fatal(err)
	}
	if err := evidence.recordSuccessorRecordings(sourceRecordPath, successorRecordPath); err != nil {
		t.Fatalf("verify source/successor recording identity and hashes: %v", err)
	}
	if evidence.SourceRecordingSHA256 == evidence.SuccessorRecordingSHA256 {
		t.Fatal("successor recording hash equals source recording hash after one recovered completion")
	}
}

func assertRestartActiveWorkerCount(t *testing.T, observation restartPublicObservation, want int) {
	t.Helper()
	if observation.ActiveWorkerSessionCount != want {
		t.Fatalf("active Worker Sessions = %d, want %d", observation.ActiveWorkerSessionCount, want)
	}
}

// TestRestartRecoveryTerminalHistoryRemainsIdle is the H-02 prebuilt witness.
// A clean stop followed by resume preserves terminal Work without making any
// dispatch eligible.
func TestRestartRecoveryTerminalHistoryRemainsIdle(t *testing.T) {
	t.Parallel()
	artifactPath := requireRestartCLIArtifact(t)
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "H-02", t.Name())
	t.Cleanup(func() { evidence.publishRestartScenario(t) })

	factoryDir := scaffoldBoardPersistenceFactory(t, restartRecoveryFactoryConfig(true))
	workerPath := currentRestartWorkerExecutable(t)
	writeBoardPersistenceAgentConfig(t, factoryDir, restartRecoveryWorkerName, boardPersistenceWorkerConfig(workerPath))
	if err := recordRestartFixtureHash(evidence, "factory/factory.json", filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatal(err)
	}
	if err := recordRestartFixtureHash(evidence, "workstation/AGENTS.md", filepath.Join(factoryDir, "workstations", "hold-processing", "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := recordRestartFixtureHash(evidence, "worker/AGENTS.md", filepath.Join(factoryDir, "workers", restartRecoveryWorkerName, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	homeDir := t.TempDir()
	releasePath := filepath.Join(t.TempDir(), "unused-release")
	sourceRecordPath := filepath.Join(t.TempDir(), "terminal-only-source.recording.json")
	successorRecordPath := filepath.Join(t.TempDir(), "terminal-only-successor.recording.json")

	first := startBoardPersistenceDaemon(t, artifactPath, factoryDir, homeDir, sourceRecordPath, releasePath)
	evidence.trackDaemon(t, "terminal-only-source-process", first)
	batch := restartRecoveryTerminalOnlyBatchJSON(t)
	submitBoardPersistenceBatchThroughCLI(t, first, artifactPath, factoryDir, homeDir, batch, restartRecoveryRequestID, 2)
	want := restartRecoveryTerminalOnlyExpectedWorks()
	before := waitForBoardStates(t, first.baseURL, map[string]string{
		restartRecoveryCompleteWorkID: "complete",
		restartRecoveryFailedWorkID:   "failed",
	}, 30*time.Second)
	assertBoardList(t, before, want)
	assertRestartWorkIDsUnique(t, before, want)
	beforeEvents := restartWorkHistoryFingerprint(t, first.baseURL, restartRecoveryTerminalWorkIDs())
	beforeObservation := evidence.capturePublicObservation(t, "terminal-only-before-stop", first.baseURL)
	assertRestartPublicCounts(t, beforeObservation, 1, 2, 0, 0)
	if beforeObservation.DispatchCount != 0 {
		t.Fatalf("terminal-only source created %d dispatches before stop, want zero", beforeObservation.DispatchCount)
	}
	first.stop(t)
	evidence.captureDaemon(0, first)
	if err := evidence.recordSourceRecording(sourceRecordPath); err != nil {
		t.Fatalf("hash terminal-only source recording: %v", err)
	}

	second := startBoardPersistenceObservedResumeDaemon(t, artifactPath, factoryDir, homeDir, sourceRecordPath, successorRecordPath, releasePath)
	evidence.trackDaemon(t, "terminal-only-successor-process", second)
	after := waitForBoardStates(t, second.baseURL, map[string]string{
		restartRecoveryCompleteWorkID: "complete",
		restartRecoveryFailedWorkID:   "failed",
	}, 30*time.Second)
	assertBoardList(t, after, want)
	assertRestartWorkIDsUnique(t, after, want)
	if got := restartWorkHistoryFingerprint(t, second.baseURL, restartRecoveryTerminalWorkIDs()); !equalStringSlices(got, beforeEvents) {
		t.Fatalf("terminal Work event history changed across restart:\nbefore=%v\nafter=%v", beforeEvents, got)
	}
	afterObservation := evidence.capturePublicObservation(t, "terminal-only-after-resume", second.baseURL)
	assertRestartPublicCounts(t, afterObservation, 1, 2, 0, 0)
	if afterObservation.DispatchCount != 0 {
		t.Fatalf("terminal-only successor created %d dispatches, want zero", afterObservation.DispatchCount)
	}
	if _, err := waitForRestartRecoveryRecord(t, second, "success", 10*time.Second); err != nil {
		t.Fatal(err)
	}
	assertRestartRecoveryRecordSafe(t, second, "success")
	second.stop(t)
	evidence.captureDaemon(1, second)
	if err := verifyRestartFixtureHashes(evidence, map[string]string{
		"factory/factory.json":  filepath.Join(factoryDir, "factory.json"),
		"workstation/AGENTS.md": filepath.Join(factoryDir, "workstations", "hold-processing", "AGENTS.md"),
		"worker/AGENTS.md":      filepath.Join(factoryDir, "workers", restartRecoveryWorkerName, "AGENTS.md"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := evidence.recordSuccessorRecordings(sourceRecordPath, successorRecordPath); err != nil {
		t.Fatalf("verify terminal-only source/successor recording identity: %v", err)
	}
	if evidence.SourceRecordingSHA256 == evidence.SuccessorRecordingSHA256 {
		t.Fatal("terminal-only successor recording hash equals its source")
	}
}

// TestRestartRecoveryInvalidSourcesFailFast exercises missing, truncated,
// corrupt, incompatible, and internally invalid Factory-definition sources
// through the exact prebuilt CLI. Each failure is a separate process cell;
// the source bytes are read-only and all paths, homes, logs, and ports are
// test-owned.
func TestRestartRecoveryInvalidSourcesFailFast(t *testing.T) {
	t.Parallel()
	artifactPath := requireRestartCLIArtifact(t)
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "F-01..F-05", t.Name())
	t.Cleanup(func() { evidence.publishRestartScenario(t) })

	factoryDir := scaffoldBoardPersistenceFactory(t, restartRecoveryFactoryConfig(true))
	workerPath := currentRestartWorkerExecutable(t)
	writeBoardPersistenceAgentConfig(t, factoryDir, restartRecoveryWorkerName, boardPersistenceWorkerConfig(workerPath))
	if err := recordRestartFixtureHash(evidence, "factory/factory.json", filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatal(err)
	}
	homeDir := t.TempDir()
	sourceRecordPath := filepath.Join(t.TempDir(), "valid-source.recording.json")
	seed := startBoardPersistenceDaemon(t, artifactPath, factoryDir, homeDir, sourceRecordPath, filepath.Join(t.TempDir(), "unused-release"))
	evidence.trackDaemon(t, "valid-source-fixture-process", seed)
	seed.stop(t)
	evidence.captureDaemon(0, seed)
	if err := evidence.recordSourceRecording(sourceRecordPath); err != nil {
		t.Fatalf("hash valid immutable source recording: %v", err)
	}
	validSource, err := os.ReadFile(sourceRecordPath)
	if err != nil {
		t.Fatalf("read valid source recording fixture: %v", err)
	}
	validSourceSHA := sha256Hex(validSource)
	if validSourceSHA != evidence.SourceRecordingSHA256 {
		t.Fatalf("source fixture hash = %s, evidence hash = %s", validSourceSHA, evidence.SourceRecordingSHA256)
	}

	fixtures := []restartRecoveryFailureFixture{
		{id: "F-01-missing", resumePath: func(root string) string { return filepath.Join(root, "missing.recording.json") }},
		{id: "F-02-truncated", resumePath: func(root string) string { return filepath.Join(root, "truncated.recording.json") }, contents: truncateReplayRecording(validSource)},
		{id: "F-03-corrupt", resumePath: func(root string) string { return filepath.Join(root, "corrupt.recording.json") }, contents: corruptReplayRecording(validSource)},
		{id: "F-04-incompatible", resumePath: func(root string) string { return filepath.Join(root, "incompatible.recording.json") }, contents: incompatibleReplayRecording(t, validSource)},
		{id: "F-05-wrong-definition", resumePath: func(root string) string { return filepath.Join(root, "wrong-definition.recording.json") }, contents: wrongDefinitionReplayRecording(t, validSource)},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.id, func(t *testing.T) {
			runRestartRecoveryFailureCase(t, evidence, artifactPath, workerPath, fixture)
		})
	}
	if err := evidence.verifyFixtureFilesUnchangedFromHashes(map[string]string{"valid-source.recording.json": sourceRecordPath}); err != nil {
		t.Fatal(err)
	}
}

func runRestartRecoveryFailureCase(t *testing.T, evidence *restartBaselineEvidence, artifactPath, workerPath string, fixture restartRecoveryFailureFixture) {
	t.Helper()
	fixtureRoot := t.TempDir()
	resumePath := fixture.resumePath(fixtureRoot)
	var candidateHash string
	if fixture.contents != nil {
		if err := os.WriteFile(resumePath, fixture.contents, 0o600); err != nil {
			t.Fatalf("write %s immutable source copy: %v", fixture.id, err)
		}
		candidateHash = sha256Hex(fixture.contents)
	}
	failureEvidence := restartFailureCaseEvidence{ID: fixture.id, SourceSHA256: candidateHash}
	t.Cleanup(func() {
		if failureEvidence.Outcome == "" {
			failureEvidence.Outcome = "FAIL"
			if !t.Failed() {
				failureEvidence.Outcome = "PASS"
			}
		}
		evidence.FailureCases = append(evidence.FailureCases, failureEvidence)
	})
	factoryCopy := scaffoldBoardPersistenceFactory(t, restartRecoveryFactoryConfig(true))
	writeBoardPersistenceAgentConfig(t, factoryCopy, restartRecoveryWorkerName, boardPersistenceWorkerConfig(workerPath))
	failureHome := t.TempDir()
	successorPath := filepath.Join(fixtureRoot, "must-not-start.recording.json")
	daemon := startBoardPersistenceJSONResumeProcess(t, artifactPath, factoryCopy, failureHome, resumePath, successorPath, filepath.Join(t.TempDir(), "unused-release"))
	evidence.trackDaemon(t, "failed-startup-"+fixture.id, daemon)
	startedAt := time.Now()
	waitForBoardPersistenceDaemonExit(t, daemon, restartRecoveryProcessTimeout)
	failureEvidence.DurationNanoseconds = int64(time.Since(startedAt))
	daemon.cleanup()
	evidence.captureDaemon(len(evidence.Generations)-1, daemon)
	if daemon.waitError() == nil {
		t.Fatalf("%s startup exited successfully; want one typed failure", fixture.id)
	}
	failureEvidence.ExitCode = daemonExitCode(daemon)
	output := append(append([]byte(nil), daemon.stdout.Bytes()...), daemon.stderr.Bytes()...)
	diagnostics := restartCLIErrorResponses(output)
	failureEvidence.DiagnosticCount = len(diagnostics)
	if len(diagnostics) != 1 {
		t.Fatalf("%s emitted %d structured CLI errors; want exactly one:\n%s", fixture.id, len(diagnostics), output)
	}
	failureEvidence.ErrorCode = string(diagnostics[0].Code)
	if !isKnownReplayDiagnostic(failureEvidence.ErrorCode) || failureEvidence.ErrorCode == string(factoryapi.ErrorResponseCode("SERVER_START_FAILED")) {
		t.Fatalf("%s startup code = %q, want an existing typed Recordings code; output:\n%s", fixture.id, failureEvidence.ErrorCode, output)
	}
	if strings.TrimSpace(diagnostics[0].Message) == "" || strings.Contains(diagnostics[0].Message, resumePath) || strings.Contains(diagnostics[0].Message, restartRecoverySecretMarker) {
		t.Fatalf("%s startup diagnostic is empty or exposes source details: %#v", fixture.id, diagnostics[0])
	}
	assertRestartRecoveryFailureIsSafe(t, daemon, fixture.id, resumePath, successorPath, candidateHash, &failureEvidence)
	failureEvidence.Outcome = "PASS"
}

func assertRestartRecoveryFailureIsSafe(t *testing.T, daemon *boardPersistenceDaemon, fixtureID, resumePath, successorPath, candidateHash string, evidence *restartFailureCaseEvidence) {
	t.Helper()
	recoveryRecords := readRestartRecoveryRecordsForDaemon(daemon)
	evidence.RecoveryRecordCount = len(recoveryRecords)
	for _, record := range recoveryRecords {
		if record["outcome"] == "success" {
			evidence.RecoverySuccessCount++
		}
	}
	if evidence.RecoveryRecordCount != 1 || evidence.RecoverySuccessCount != 0 {
		t.Fatalf("%s recovery records = %#v, want one failed-startup record and no success", fixtureID, recoveryRecords)
	}
	if recoveryRecords[0]["outcome"] != "failed_startup" || recoveryRecords[0]["host_observation"] != "UNAVAILABLE_WHILE_STOPPED" || recoveryRecords[0]["supervisor"] != "external" || recoveryRecords[0]["error_code"] != evidence.ErrorCode {
		t.Fatalf("%s failed recovery record = %#v, want typed failed_startup/unavailable/external fields", fixtureID, recoveryRecords[0])
	}
	if _, err := os.Stat(successorPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s created a successor recording before readiness: %v", fixtureID, err)
	}
	if candidateHash != "" {
		after, err := os.ReadFile(resumePath)
		if err != nil {
			t.Fatalf("read %s source after failed startup: %v", fixtureID, err)
		}
		evidence.SourceAfterSHA256 = sha256Hex(after)
		if evidence.SourceAfterSHA256 != candidateHash {
			t.Fatalf("%s source changed from %s to %s", fixtureID, candidateHash, evidence.SourceAfterSHA256)
		}
	} else if _, err := os.Stat(resumePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s missing source was created during failed startup: %v", fixtureID, err)
	}
}

// TestRestartRecoveryCancellationBeforeReadinessDoesNotClaimSuccess observes
// runtime opening before public readiness, then interrupts that successor.
// The success record is emitted only after host readiness, and the source stays intact.
func TestRestartRecoveryCancellationBeforeReadinessDoesNotClaimSuccess(t *testing.T) {
	t.Parallel()
	artifactPath := requireRestartCLIArtifact(t)
	evidence := newRestartScenarioEvidence(*restartCLIArtifact, "B-03", t.Name())
	t.Cleanup(func() { evidence.publishRestartScenario(t) })

	factoryDir := scaffoldBoardPersistenceFactory(t, restartRecoveryFactoryConfig(true))
	workerPath := currentRestartWorkerExecutable(t)
	writeBoardPersistenceAgentConfig(t, factoryDir, restartRecoveryWorkerName, boardPersistenceWorkerConfig(workerPath))
	if err := recordRestartFixtureHash(evidence, "factory/factory.json", filepath.Join(factoryDir, "factory.json")); err != nil {
		t.Fatal(err)
	}
	homeDir := t.TempDir()
	sourcePath := filepath.Join(t.TempDir(), "cancel-source.recording.json")
	successorPath := filepath.Join(t.TempDir(), "cancel-successor.recording.json")
	first := startBoardPersistenceDaemon(t, artifactPath, factoryDir, homeDir, sourcePath, filepath.Join(t.TempDir(), "unused-release"))
	evidence.trackDaemon(t, "cancellation-source-process", first)
	first.stop(t)
	evidence.captureDaemon(0, first)
	if err := evidence.recordSourceRecording(sourcePath); err != nil {
		t.Fatalf("hash cancellation source recording: %v", err)
	}

	successorHomeDir := t.TempDir()
	logDir := filepath.Join(successorHomeDir, ".you-agent-factory", "logs")
	logWatcher := newBoardPersistenceLogWatcher(t, logDir)
	second := startBoardPersistenceDaemonProcessWithResumeOutput(
		t, artifactPath, factoryDir, successorHomeDir, sourcePath, successorPath,
		filepath.Join(t.TempDir(), "unused-release"), true, true,
	)
	evidence.trackDaemon(t, "cancelled-successor-process", second)
	startupLogPath := waitForBoardStartupLogBeforeReadiness(t, second, logWatcher, restartRecoveryProcessTimeout)
	if err := interruptBoardPersistenceProcess(second.cmd); err != nil {
		t.Fatalf("cancel successor before readiness: %v", err)
	}
	waitForBoardPersistenceDaemonExit(t, second, restartRecoveryProcessTimeout)
	second.cleanup()
	assertBoardResumeStartupWasCancelled(t, second, startupLogPath)
	evidence.captureDaemon(1, second)
	if waitErr := second.waitError(); waitErr == nil {
		t.Fatal("cancelled successor exited successfully after interrupt; want a nonzero interrupted-process result")
	}
	for _, record := range readRestartRecoveryRecordsForDaemon(second) {
		if record["outcome"] == "success" {
			t.Fatalf("pre-readiness cancellation emitted recovery success: %#v", record)
		}
	}
	if _, err := waitForRestartRecoveryRecord(t, second, "failed_startup", 100*time.Millisecond); err == nil {
		t.Fatal("pre-readiness cancellation emitted a failed-startup recovery record; cancellation is an unavailable observation")
	}
	if err := evidence.verifySourceRecordingUnchanged(sourcePath); err != nil {
		t.Fatalf("verify cancellation source recording remains unchanged: %v", err)
	}
}

func restartRecoveryFactoryConfig(consumeProcessing bool) map[string]any {
	inputState := "processing"
	factoryName := "recorded-definition-a"
	if !consumeProcessing {
		inputState = "staged"
		factoryName = "authored-definition-b"
	}
	states := []map[string]string{
		{"name": "staged", "type": "INITIAL"},
		{"name": "processing", "type": "PROCESSING"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
	return map[string]any{
		"name":      factoryName,
		"workTypes": []map[string]any{{"name": "task", "states": states}},
		"workers":   []map[string]string{{"name": restartRecoveryWorkerName}},
		"workstations": []map[string]any{{
			"name":      "hold-processing",
			"worker":    restartRecoveryWorkerName,
			"inputs":    []map[string]string{{"workType": "task", "state": inputState}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

func restartRecoveryBatchJSON(t *testing.T) string {
	t.Helper()
	works := []map[string]any{
		restartRecoveryWork("eligible-recovery", restartRecoveryEligibleWorkID, "processing", "eligible after restart"),
		restartRecoveryWork("terminal-complete", restartRecoveryCompleteWorkID, "complete", restartRecoverySecretMarker),
		restartRecoveryWork("terminal-failed", restartRecoveryFailedWorkID, "failed", "historical failure is preserved"),
	}
	request := map[string]any{
		"requestId": restartRecoveryRequestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works":     works,
		"relations": []map[string]string{{
			"type": "PARENT_CHILD", "sourceWorkName": "terminal-complete", "targetWorkName": "eligible-recovery",
		}},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal restart recovery batch: %v", err)
	}
	return string(encoded)
}

func restartRecoveryTerminalOnlyBatchJSON(t *testing.T) string {
	t.Helper()
	request := map[string]any{
		"requestId": restartRecoveryRequestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{
			restartRecoveryWork("terminal-complete", restartRecoveryCompleteWorkID, "complete", restartRecoverySecretMarker),
			restartRecoveryWork("terminal-failed", restartRecoveryFailedWorkID, "failed", "historical failure is preserved"),
		},
		"relations": []map[string]string{},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal terminal-only restart batch: %v", err)
	}
	return string(encoded)
}

func restartRecoveryWork(name, workID, state, content string) map[string]any {
	return map[string]any{
		"name": name, "workId": workID, "workTypeName": "task", "state": state,
		"traceId": "trace-restart-recovery", "currentChainingTraceId": "trace-restart-recovery",
		"previousChainingTraceIds": []string{}, "content": []map[string]string{{"type": "text", "text": content}},
	}
}

func restartRecoveryExpectedWorks(completed bool) map[string]boardPersistenceExpectedWork {
	activeState := "processing"
	activeType := "PROCESSING"
	activeContent := "eligible after restart"
	workerOutput := false
	if completed {
		activeState = "complete"
		activeType = "TERMINAL"
		activeContent = boardPersistenceWorkerSentinel
		workerOutput = true
	}
	return map[string]boardPersistenceExpectedWork{
		restartRecoveryEligibleWorkID: {
			Name: "eligible-recovery", WorkID: restartRecoveryEligibleWorkID, RequestID: restartRecoveryRequestID,
			State: activeState, StateType: activeType, TraceID: "trace-restart-recovery", CurrentTraceID: "trace-restart-recovery",
			Content: activeContent, WorkerOutput: workerOutput,
		},
		restartRecoveryCompleteWorkID: {
			Name: "terminal-complete", WorkID: restartRecoveryCompleteWorkID, RequestID: restartRecoveryRequestID,
			State: "complete", StateType: "TERMINAL", TraceID: "trace-restart-recovery", CurrentTraceID: "trace-restart-recovery",
			Content: restartRecoverySecretMarker, RelationTarget: restartRecoveryEligibleWorkID,
		},
		restartRecoveryFailedWorkID: {
			Name: "terminal-failed", WorkID: restartRecoveryFailedWorkID, RequestID: restartRecoveryRequestID,
			State: "failed", StateType: "FAILED", TraceID: "trace-restart-recovery", CurrentTraceID: "trace-restart-recovery",
			Content: "historical failure is preserved",
		},
	}
}

func restartRecoveryTerminalOnlyExpectedWorks() map[string]boardPersistenceExpectedWork {
	all := restartRecoveryExpectedWorks(false)
	delete(all, restartRecoveryEligibleWorkID)
	complete := all[restartRecoveryCompleteWorkID]
	complete.RelationTarget = ""
	all[restartRecoveryCompleteWorkID] = complete
	return all
}

func restartRecoveryWorkIDs() map[string]struct{} {
	return map[string]struct{}{
		restartRecoveryEligibleWorkID: {}, restartRecoveryCompleteWorkID: {}, restartRecoveryFailedWorkID: {},
	}
}

func restartRecoveryTerminalWorkIDs() map[string]struct{} {
	return map[string]struct{}{
		restartRecoveryCompleteWorkID: {}, restartRecoveryFailedWorkID: {},
	}
}

func assertRestartWorkIDsUnique(t *testing.T, listed factoryapi.ListWorkResponse, expected map[string]boardPersistenceExpectedWork) {
	t.Helper()
	seen := make(map[string]struct{}, len(listed.Results))
	for _, item := range listed.Results {
		id := boardPersistenceStringPointerValue(item.WorkId)
		if id == "" {
			t.Fatal("public Work list contains an empty Work ID")
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("public Work list contains duplicate historical Work ID %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != len(expected) {
		t.Fatalf("public unique Work IDs = %v, want %d IDs from expected board %v", sortedWorkIDSet(seen), len(expected), expected)
	}
	for id := range expected {
		if _, exists := seen[id]; !exists {
			t.Fatalf("public Work list lost historical Work ID %q", id)
		}
	}
}

func sortedWorkIDSet(values map[string]struct{}) []string {
	ids := make([]string, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func waitForBoardActiveOwner(t *testing.T, baseURL, workID string, timeout time.Duration) (string, string) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	var last map[string]boardPersistenceDispatchState
	for {
		states, err := readBoardDispatchStates(t.Context(), baseURL)
		if err == nil {
			last = states
			active := activeBoardDispatches(states, workID)
			if len(active) == 1 && len(active[0].WorkerSessionIDs) == 1 {
				return active[0].ID, active[0].WorkerSessionIDs[0]
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for one public dispatch owner for Work %q; last=%#v", workID, last)
		}
	}
}

func restartWorkHistoryFingerprint(t *testing.T, baseURL string, workIDs map[string]struct{}) []string {
	t.Helper()
	events, err := readBoardEvents(t.Context(), baseURL)
	if err != nil {
		t.Fatalf("read Work admission/state history: %v", err)
	}
	fingerprint := make([]string, 0)
	for _, event := range events {
		var includesWork bool
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			payload, decodeErr := event.Payload.AsWorkRequestEventPayload()
			if decodeErr != nil {
				t.Fatalf("decode Work Request history %q: %v", event.Id, decodeErr)
			}
			if payload.Works != nil {
				for _, item := range *payload.Works {
					if _, exists := workIDs[boardPersistenceStringPointerValue(item.WorkId)]; exists {
						includesWork = true
					}
				}
			}
		case factoryapi.FactoryEventTypeWorkStateChange:
			payload, decodeErr := event.Payload.AsWorkStateChangeEventPayload()
			if decodeErr != nil {
				t.Fatalf("decode Work State Change history %q: %v", event.Id, decodeErr)
			}
			_, includesWork = workIDs[payload.WorkId]
		}
		if includesWork {
			encoded, encodeErr := json.Marshal(event)
			if encodeErr != nil {
				t.Fatalf("encode immutable Work history event %q: %v", event.Id, encodeErr)
			}
			var semanticEvent any
			if err := json.Unmarshal(encoded, &semanticEvent); err != nil {
				t.Fatalf("decode immutable Work history event %q for canonical comparison: %v", event.Id, err)
			}
			canonical, err := json.Marshal(semanticEvent)
			if err != nil {
				t.Fatalf("canonicalize immutable Work history event %q: %v", event.Id, err)
			}
			fingerprint = append(fingerprint, string(canonical))
		}
	}
	return fingerprint
}

func assertSingleTerminalCompletion(t *testing.T, events []factoryapi.FactoryEvent, workID, terminalState string) {
	t.Helper()
	completions := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode Dispatch Response %q: %v", event.Id, err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.OutputWork == nil {
			continue
		}
		for _, output := range *payload.OutputWork {
			if boardPersistenceStringPointerValue(output.WorkId) == workID && output.State != nil && output.State.Name == terminalState {
				completions++
			}
		}
	}
	if completions != 1 {
		summary := make([]string, 0, len(events))
		for _, event := range events {
			if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
				continue
			}
			payload, _ := json.Marshal(event.Payload)
			summary = append(summary, fmt.Sprintf("%s/%s workIds=%v payload=%s", event.Id, event.Type, event.Context.WorkIds, payload))
		}
		t.Fatalf("Work %q terminal dispatch completions to %q = %d, want exactly one; relevant public history=%v", workID, terminalState, completions, summary)
	}
}

func assertRestartReconciledBeforeRearm(t *testing.T, events []factoryapi.FactoryEvent, oldDispatchID, successorDispatchID string) {
	t.Helper()
	oldInterruptedAt, successorRequestedAt := -1, -1
	for index, event := range events {
		if event.Context.DispatchId == nil {
			continue
		}
		dispatchID := *event.Context.DispatchId
		if dispatchID == oldDispatchID && event.Type == factoryapi.FactoryEventTypeDispatchInterrupted && oldInterruptedAt < 0 {
			oldInterruptedAt = index
		}
		if dispatchID == successorDispatchID && event.Type == factoryapi.FactoryEventTypeDispatchRequest && successorRequestedAt < 0 {
			successorRequestedAt = index
		}
	}
	if oldInterruptedAt < 0 || successorRequestedAt < 0 || oldInterruptedAt >= successorRequestedAt {
		t.Fatalf("public dispatch history order = interrupted-old:%d requested-successor:%d; want the historical owner closed before the successor request", oldInterruptedAt, successorRequestedAt)
	}
}

func assertRestartDispatchHistory(t *testing.T, states map[string]boardPersistenceDispatchState, oldID, successorID, workID string) {
	t.Helper()
	if len(states) != 2 {
		t.Fatalf("dispatch history contains %d dispatches, want source and one successor: %#v", len(states), states)
	}
	old, ok := states[oldID]
	if !ok || old.RequestEvents != 1 || old.ResponseEvents != 0 || old.InterruptedEvents != 1 {
		t.Fatalf("source dispatch history = %#v, want one request, one interruption, and no response", old)
	}
	newDispatch, ok := states[successorID]
	if !ok || newDispatch.RequestEvents != 1 || newDispatch.ResponseEvents != 1 || newDispatch.InterruptedEvents != 0 {
		t.Fatalf("successor dispatch history = %#v, want one request and one response", newDispatch)
	}
	if _, ok := newDispatch.WorkIDs[workID]; !ok {
		t.Fatalf("successor dispatch %q does not own eligible Work %q: %#v", successorID, workID, newDispatch.WorkIDs)
	}
	if len(newDispatch.WorkerSessionIDs) != 1 || newDispatch.WorkerSessionIDs[0] == "" {
		t.Fatalf("successor dispatch worker-session associations = %#v, want exactly one owner", newDispatch.WorkerSessionIDs)
	}
}

func recordRestartFixtureHash(evidence *restartBaselineEvidence, name, path string) error {
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read fixture %q: %w", name, err)
	}
	evidence.FixtureSHA256[name] = sha256Hex(contents)
	return nil
}

func verifyRestartFixtureHashes(evidence *restartBaselineEvidence, paths map[string]string) error {
	for name, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read fixture %q after recovery: %w", name, err)
		}
		want, exists := evidence.FixtureSHA256[name]
		if !exists {
			return fmt.Errorf("fixture %q was not hashed before recovery", name)
		}
		if got := sha256Hex(contents); got != want {
			return fmt.Errorf("fixture %q changed from %s to %s", name, want, got)
		}
	}
	return nil
}

func (evidence *restartBaselineEvidence) verifyFixtureFilesUnchangedFromHashes(paths map[string]string) error {
	for name, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read immutable fixture %q: %w", name, err)
		}
		want, exists := evidence.FixtureSHA256[name]
		if !exists {
			want = evidence.SourceRecordingSHA256
		}
		if got := sha256Hex(contents); got != want {
			return fmt.Errorf("fixture %q changed from %s to %s", name, want, got)
		}
	}
	return nil
}

func waitForRestartRecoveryRecord(t *testing.T, daemon *boardPersistenceDaemon, outcome string, timeout time.Duration) (map[string]any, error) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, record := range readRestartRecoveryRecordsForDaemon(daemon) {
			if record["outcome"] == outcome {
				return record, nil
			}
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return nil, fmt.Errorf("timed out waiting for run.recovery outcome %q in %q", outcome, daemon.logDir)
		}
	}
}

func assertRestartRecoveryRecordSafe(t *testing.T, daemon *boardPersistenceDaemon, outcome string) {
	t.Helper()
	records := readRestartRecoveryRecordsForDaemon(daemon)
	if len(records) != 1 {
		t.Fatalf("run.recovery records = %d, want exactly one: %#v", len(records), records)
	}
	record := records[0]
	if record["event"] != "run.recovery" || record["outcome"] != outcome ||
		record["host_observation"] != "UNAVAILABLE_WHILE_STOPPED" || record["supervisor"] != "external" {
		t.Fatalf("run.recovery record = %#v, want %q with unavailable interval and external supervisor", record, outcome)
	}
	for _, forbidden := range []string{daemon.recordPath, restartRecoverySecretMarker, "credential=", "prompt="} {
		encoded, _ := json.Marshal(record)
		if forbidden != "" && bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("run.recovery record leaks %q: %s", forbidden, encoded)
		}
	}
}

func truncateReplayRecording(source []byte) []byte {
	headerEnd := bytes.IndexByte(source, '\n')
	if headerEnd < 2 {
		return []byte("{")
	}
	// Keep a genuinely incomplete first record. Replay V2 deliberately accepts
	// a truncated final event tail, so cutting the whole file in half could
	// instead exercise the supported partial-history recovery path.
	return append([]byte(nil), source[:headerEnd/2]...)
}

func corruptReplayRecording(source []byte) []byte {
	lines := bytes.Split(source, []byte("\n"))
	if len(lines) < 3 {
		return []byte("{corrupt replay fixture\n")
	}
	var header map[string]json.RawMessage
	if err := json.Unmarshal(lines[0], &header); err != nil {
		return []byte("{corrupt replay fixture\n")
	}
	corruptLine, _ := json.Marshal(map[string]any{"recordType": "unknown-corrupt-record", "payload": "bad"})
	lines[1] = corruptLine
	return bytes.Join(lines, []byte("\n"))
}

func incompatibleReplayRecording(t *testing.T, source []byte) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(source, &document); err == nil && len(document["schemaVersion"]) > 0 {
		version, _ := json.Marshal("agent-factory.replay.v99")
		document["schemaVersion"] = version
		encoded, encodeErr := json.MarshalIndent(document, "", "  ")
		if encodeErr != nil {
			t.Fatalf("encode incompatible replay artifact: %v", encodeErr)
		}
		return encoded
	}

	lines := bytes.Split(source, []byte("\n"))
	if len(lines) < 2 {
		t.Fatal("valid replay source does not have a header line")
	}
	var header map[string]json.RawMessage
	if err := json.Unmarshal(lines[0], &header); err != nil {
		t.Fatalf("decode valid replay header: %v", err)
	}
	version, _ := json.Marshal("agent-factory.replay.v99")
	header["schemaVersion"] = version
	lines[0], _ = json.Marshal(header)
	return bytes.Join(lines, []byte("\n"))
}

func wrongDefinitionReplayRecording(t *testing.T, source []byte) []byte {
	t.Helper()
	var document map[string]json.RawMessage
	if err := json.Unmarshal(source, &document); err == nil && len(document["events"]) > 0 {
		var events []json.RawMessage
		if err := json.Unmarshal(document["events"], &events); err != nil {
			t.Fatalf("decode replay event list: %v", err)
		}
		for index, encodedEvent := range events {
			var event map[string]json.RawMessage
			if err := json.Unmarshal(encodedEvent, &event); err != nil {
				continue
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(event["payload"], &payload); err != nil || len(payload["factory"]) == 0 {
				continue
			}
			mutateWrongDefinitionSnapshot(t, payload)
			event["payload"], _ = json.Marshal(payload)
			events[index], _ = json.Marshal(event)
			document["events"], _ = json.Marshal(events)
			encoded, encodeErr := json.MarshalIndent(document, "", "  ")
			if encodeErr != nil {
				t.Fatalf("encode wrong-definition replay artifact: %v", encodeErr)
			}
			return encoded
		}
	}

	lines := bytes.Split(source, []byte("\n"))
	found := false
	for index := 1; index < len(lines); index++ {
		if len(bytes.TrimSpace(lines[index])) == 0 {
			continue
		}
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(lines[index], &wrapper); err != nil {
			continue
		}
		if string(wrapper["recordType"]) != `"event"` {
			continue
		}
		var event map[string]json.RawMessage
		if err := json.Unmarshal(wrapper["event"], &event); err != nil {
			continue
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event["payload"], &payload); err != nil || len(payload["factory"]) == 0 {
			continue
		}
		mutateWrongDefinitionSnapshot(t, payload)
		event["payload"], _ = json.Marshal(payload)
		wrapper["event"], _ = json.Marshal(event)
		lines[index], _ = json.Marshal(wrapper)
		found = true
		break
	}
	if !found {
		t.Fatal("valid replay source has no embedded Factory Definition snapshot to corrupt")
	}
	return bytes.Join(lines, []byte("\n"))
}

func mutateWrongDefinitionSnapshot(t *testing.T, payload map[string]json.RawMessage) {
	t.Helper()
	var factory map[string]json.RawMessage
	if err := json.Unmarshal(payload["factory"], &factory); err != nil {
		t.Fatalf("decode recorded Factory snapshot: %v", err)
	}
	wrongName, _ := json.Marshal("internally-wrong-definition")
	wrongID, _ := json.Marshal("internally-wrong-definition-id")
	wrongWorkstations, _ := json.Marshal("not-a-workstation-list")
	factory["name"] = wrongName
	factory["id"] = wrongID
	factory["workstations"] = wrongWorkstations
	payload["factory"], _ = json.Marshal(factory)
}
