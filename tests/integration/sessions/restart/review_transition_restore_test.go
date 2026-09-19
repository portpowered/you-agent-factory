package restart_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	restoredReviewTaskA       = "restored-review-task-a"
	restoredReviewTaskB       = "restored-review-task-b"
	restoredReviewWorkA       = "restored-review-work-a"
	restoredReviewWorkB       = "restored-review-work-b"
	restoredReviewTaskRequest = "restored-review-task-request"
	restoredReviewWorkRequest = "restored-review-work-request"
)

// TestRestoredReviewTransitionDispatchesEveryMigratedPair proves the real
// daemon restart, session resume, and operator migration path. Both review
// workers remain active so the public event ledger can prove one disjoint
// dispatch per matching pair before either result creates another scheduling
// opportunity.
func TestRestoredReviewTransitionDispatchesEveryMigratedPair(t *testing.T) {
	t.Parallel()
	binaryPath := requireRestartCLIArtifact(t)
	evidence := beginRestartBaselineEvidence(*restartCLIArtifact)
	factoryDir := scaffoldBoardPersistenceFactory(t, restoredReviewFactoryConfig())
	workerPath := currentRestartWorkerExecutable(t)
	homeDir := t.TempDir()
	releasePath := filepath.Join(t.TempDir(), "release-review-workers")
	recordPath := filepath.Join(factoryDir, "restored-review.recording.json")
	successorRecordPath := filepath.Join(factoryDir, "restored-review.successor.recording.json")
	writeBoardPersistenceAgentConfig(t, factoryDir, "review-blocker", boardPersistenceWorkerConfig(workerPath))
	if err := evidence.hashFixtureFiles(factoryDir,
		"factory.json",
		"workstations/hold-processing/AGENTS.md",
		"workers/review-blocker/AGENTS.md",
	); err != nil {
		t.Fatalf("hash complete immutable restart fixtures: %v", err)
	}

	first := startBoardPersistenceDaemon(t, binaryPath, factoryDir, homeDir, recordPath, releasePath)
	evidence.trackDaemon(t, "source-process", first)
	submitBoardPersistenceBatchThroughCLI(t, first, binaryPath, factoryDir, homeDir, restoredReviewBatchJSON(t, "task"), restoredReviewTaskRequest, 2)
	submitBoardPersistenceBatchThroughCLI(t, first, binaryPath, factoryDir, homeDir, restoredReviewBatchJSON(t, "review"), restoredReviewWorkRequest, 2)
	firstWorks := waitForBoardStates(t, first.baseURL, map[string]string{
		restoredReviewTaskA: "staged",
		restoredReviewTaskB: "staged",
		restoredReviewWorkA: "staged",
		restoredReviewWorkB: "staged",
	}, 30*time.Second)
	assertBoardList(t, firstWorks, restoredReviewExpectedWorks())
	firstObservation := evidence.capturePublicObservation(t, "source-before-stop", first.baseURL)
	assertRestartPublicCounts(t, firstObservation, 1, 4, 0, 0)
	first.stop(t)
	evidence.captureDaemon(0, first)
	if err := evidence.recordSourceRecording(recordPath); err != nil {
		t.Fatalf("hash source recording before resume: %v", err)
	}

	second := startBoardPersistenceResumeDaemon(t, binaryPath, factoryDir, homeDir, recordPath, successorRecordPath, releasePath)
	evidence.trackDaemon(t, "successor-process", second)
	secondWorks := waitForBoardStates(t, second.baseURL, map[string]string{
		restoredReviewTaskA: "staged",
		restoredReviewTaskB: "staged",
		restoredReviewWorkA: "staged",
		restoredReviewWorkB: "staged",
	}, 30*time.Second)
	assertBoardList(t, secondWorks, restoredReviewExpectedWorks())
	resumedObservation := evidence.capturePublicObservation(t, "successor-after-resume-before-migration", second.baseURL)
	assertRestartPublicCounts(t, resumedObservation, 1, 4, 0, 0)
	runRestoredReviewLifecycleCLI(t, second, binaryPath, factoryDir, homeDir, "pause")
	for _, migration := range []struct{ workID, state string }{
		{restoredReviewTaskA, "in-review"},
		{restoredReviewWorkA, "init"},
		{restoredReviewTaskB, "in-review"},
		{restoredReviewWorkB, "init"},
	} {
		evidence.recordManualWorkMove(migration.workID, migration.state)
		output, moveErr := runBoardPersistenceCLIWithFreshContext(
			t, binaryPath, factoryDir, homeDir, second.baseURL,
			"--json", "work", "move", migration.workID, migration.state,
		)
		if moveErr != nil {
			t.Fatalf("you work move %s %s: %v\noutput:\n%s", migration.workID, migration.state, moveErr, output)
		}
	}
	wantMigrated := map[string]string{
		restoredReviewTaskA: "in-review",
		restoredReviewTaskB: "in-review",
		restoredReviewWorkA: "init",
		restoredReviewWorkB: "init",
	}
	waitForBoardStates(t, second.baseURL, wantMigrated, 30*time.Second)
	runRestoredReviewLifecycleCLI(t, second, binaryPath, factoryDir, homeDir, "resume")

	dispatchA := waitForBoardActiveDispatch(t, second.baseURL, restoredReviewTaskA, 30*time.Second)
	dispatchB := waitForBoardActiveDispatch(t, second.baseURL, restoredReviewTaskB, 30*time.Second)
	if dispatchA == dispatchB {
		t.Fatalf("matching review pairs shared dispatch %q, want one distinct dispatch per pair", dispatchA)
	}
	states := waitForBoardDispatchStates(t, second.baseURL, 30*time.Second)
	assertRestoredReviewDispatch(t, states[dispatchA], dispatchA, restoredReviewTaskA, restoredReviewWorkA)
	assertRestoredReviewDispatch(t, states[dispatchB], dispatchB, restoredReviewTaskB, restoredReviewWorkB)
	if got := countActiveRestoredReviewDispatches(states, map[string]bool{
		restoredReviewTaskA: true,
		restoredReviewTaskB: true,
	}); got != 2 {
		t.Fatalf("active restored review dispatches = %d, want exactly 2", got)
	}
	for _, workID := range []string{restoredReviewTaskA, restoredReviewTaskB} {
		observation := waitForBoardWorkerObservation(t, second.baseURL, second.sessionID, workID, func(observation factoryapi.WorkerSessionObservation) bool {
			return observation.State == factoryapi.WorkerSessionObservationStateRunning || observation.State == factoryapi.WorkerSessionObservationStateStarting
		}, 30*time.Second)
		if observation.AttemptId == "" {
			t.Fatalf("active Worker Session for Work %q has empty attempt identity: %#v", workID, observation)
		}
	}
	states = waitForBoardDispatchStates(t, second.baseURL, 30*time.Second)
	for _, dispatchID := range []string{dispatchA, dispatchB} {
		if got := len(states[dispatchID].WorkerSessionIDs); got != 1 {
			t.Fatalf("active dispatch %q worker-session associations = %d, want exactly one: %#v", dispatchID, got, states[dispatchID])
		}
	}
	activeObservation := evidence.capturePublicObservation(t, "successor-after-migration-with-active-owners", second.baseURL)
	assertRestartPublicCounts(t, activeObservation, 1, 4, 2, 2)
	if activeObservation.WorkerSessionCount != 2 || activeObservation.ActiveWorkerSessionCount != 2 {
		t.Fatalf("active public Worker Session counts = total:%d active:%d, want 2/2", activeObservation.WorkerSessionCount, activeObservation.ActiveWorkerSessionCount)
	}
	evidence.addTiming("successor-ready-to-two-active-owners", time.Since(second.readyAt))
	second.stop(t)
	evidence.captureDaemon(1, second)
	if err := evidence.verifyFixtureFilesUnchanged(factoryDir); err != nil {
		t.Fatalf("verify immutable restart fixtures: %v", err)
	}
	if err := evidence.recordSuccessorRecordings(recordPath, successorRecordPath); err != nil {
		t.Fatalf("verify immutable source and hashed successor recordings: %v", err)
	}
}

func restoredReviewExpectedWorks() map[string]boardPersistenceExpectedWork {
	return map[string]boardPersistenceExpectedWork{
		restoredReviewTaskA: {
			Name: "pair-a", WorkID: restoredReviewTaskA, RequestID: restoredReviewTaskRequest,
			State: "staged", StateType: "INITIAL", TraceID: "trace-restored-review", CurrentTraceID: "trace-restored-review", Content: "task a",
		},
		restoredReviewTaskB: {
			Name: "pair-b", WorkID: restoredReviewTaskB, RequestID: restoredReviewTaskRequest,
			State: "staged", StateType: "INITIAL", TraceID: "trace-restored-review", CurrentTraceID: "trace-restored-review", Content: "task b",
		},
		restoredReviewWorkA: {
			Name: "pair-a", WorkID: restoredReviewWorkA, RequestID: restoredReviewWorkRequest,
			State: "staged", StateType: "INITIAL", TraceID: "trace-restored-review", CurrentTraceID: "trace-restored-review", Content: "review a",
		},
		restoredReviewWorkB: {
			Name: "pair-b", WorkID: restoredReviewWorkB, RequestID: restoredReviewWorkRequest,
			State: "staged", StateType: "INITIAL", TraceID: "trace-restored-review", CurrentTraceID: "trace-restored-review", Content: "review b",
		},
	}
}

func countActiveRestoredReviewDispatches(states map[string]boardPersistenceDispatchState, taskIDs map[string]bool) int {
	count := 0
	for _, state := range states {
		matchesTask := false
		for workID := range state.WorkIDs {
			matchesTask = matchesTask || taskIDs[workID]
		}
		if matchesTask && state.RequestEvents == 1 && state.ResponseEvents == 0 && state.InterruptedEvents == 0 && len(state.ReconciledStatuses) == 0 {
			count++
		}
	}
	return count
}

func runRestoredReviewLifecycleCLI(
	t *testing.T,
	daemon *boardPersistenceDaemon,
	binaryPath, factoryDir, homeDir, operation string,
) {
	t.Helper()
	output, err := runBoardPersistenceCLIWithFreshContext(
		t, binaryPath, factoryDir, homeDir, daemon.baseURL,
		"--remote", "--json", "session", operation, daemon.sessionID,
	)
	if err != nil {
		t.Fatalf("you session %s: %v\noutput:\n%s", operation, err, output)
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(bytes.TrimSpace(output), &response); err != nil {
		t.Fatalf("decode session %s: %v\noutput:\n%s", operation, err, output)
	}
	wantAccepted := response.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeAccepted
	resumeAlreadyRunning := operation == "resume" &&
		response.Outcome == factoryapi.FactorySessionLifecycleControlOutcomeNoOp &&
		response.Status == factoryapi.FactorySessionDurableLifecycleStatusRunning
	if !wantAccepted && !resumeAlreadyRunning {
		t.Fatalf("session %s response = %#v, want ACCEPTED or running resume NO_OP", operation, response)
	}
}

func assertRestoredReviewDispatch(
	t *testing.T,
	state boardPersistenceDispatchState,
	dispatchID, taskID, reviewID string,
) {
	t.Helper()
	if state.ID != dispatchID || state.RequestEvents != 1 || state.ResponseEvents != 0 || state.InterruptedEvents != 0 {
		t.Fatalf("restored review dispatch %q lifecycle = %#v, want one active request only", dispatchID, state)
	}
	if len(state.WorkIDs) != 2 {
		t.Fatalf("restored review dispatch %q Work IDs = %#v, want exactly task and review", dispatchID, state.WorkIDs)
	}
	for _, workID := range []string{taskID, reviewID} {
		if _, ok := state.WorkIDs[workID]; !ok {
			t.Fatalf("restored review dispatch %q Work IDs = %#v, missing %q", dispatchID, state.WorkIDs, workID)
		}
	}
}

func restoredReviewFactoryConfig() map[string]any {
	states := []map[string]string{
		{"name": "staged", "type": "INITIAL"},
		{"name": "in-review", "type": "PROCESSING"},
		{"name": "init", "type": "PROCESSING"},
		{"name": "complete", "type": "TERMINAL"},
		{"name": "failed", "type": "FAILED"},
	}
	return map[string]any{
		"name": "restored-review-transition",
		"workTypes": []map[string]any{
			{"name": "task", "states": states},
			{"name": "review", "states": states},
		},
		"resources": []map[string]any{{"name": "executor-slot", "capacity": 2}},
		"workers":   []map[string]string{{"name": "review-blocker"}},
		"workstations": []map[string]any{{
			"name":     "review",
			"behavior": "REPEATER",
			"worker":   "review-blocker",
			"inputs": []map[string]any{
				{"workType": "task", "state": "in-review"},
				{"workType": "review", "state": "init", "guards": []map[string]string{{"type": "SAME_NAME", "matchInput": "task"}}},
			},
			"resources": []map[string]any{{"name": "executor-slot", "capacity": 1}},
			"outputs": []map[string]string{
				{"workType": "task", "state": "complete"},
				{"workType": "review", "state": "complete"},
			},
			"onFailure": []map[string]string{
				{"workType": "task", "state": "failed"},
				{"workType": "review", "state": "failed"},
			},
		}},
	}
}

func restoredReviewBatchJSON(t *testing.T, workType string) string {
	t.Helper()
	requestID := restoredReviewTaskRequest
	workIDs := []string{restoredReviewTaskA, restoredReviewTaskB}
	if workType == "review" {
		requestID = restoredReviewWorkRequest
		workIDs = []string{restoredReviewWorkA, restoredReviewWorkB}
	}
	works := []map[string]any{
		{"name": "pair-a", "workId": workIDs[0], "workTypeName": workType, "state": "staged", "traceId": "trace-restored-review", "currentChainingTraceId": "trace-restored-review", "content": []map[string]string{{"type": "text", "text": workType + " a"}}},
		{"name": "pair-b", "workId": workIDs[1], "workTypeName": workType, "state": "staged", "traceId": "trace-restored-review", "currentChainingTraceId": "trace-restored-review", "content": []map[string]string{{"type": "text", "text": workType + " b"}}},
	}
	raw, err := json.Marshal(map[string]any{
		"requestId": requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works":     works,
		"relations": []map[string]string{},
	})
	if err != nil {
		t.Fatalf("marshal restored review batch: %v", err)
	}
	return string(raw)
}
