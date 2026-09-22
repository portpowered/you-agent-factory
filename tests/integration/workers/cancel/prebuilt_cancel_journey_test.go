package cancel_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type cancelJourney struct {
	ctx                  context.Context
	binaryPath           string
	fixture              cancelFixture
	daemon               *cancelDaemon
	manifest             cancelEvidenceManifest
	sessionID            string
	targetWorkID         string
	unrelatedWorkID      string
	targetObservation    factoryapi.WorkerSessionObservation
	unrelatedObservation factoryapi.WorkerSessionObservation
	targetTree           workerProcessTree
	unrelatedTree        workerProcessTree
	successorTree        workerProcessTree
	beforeCancel         factoryapi.Work
}

type canceledAttempt struct {
	applied              factoryapi.WorkerSessionControlResponse
	sessionsBeforeRepeat factoryapi.ListWorkerSessionsResponse
	successorBefore      factoryapi.WorkerSessionObservation
	eventsAfterRepeat    []factoryapi.FactoryEvent
}

// TestPrebuiltWorkerSessionCancelStopsExactTree proves the cancel contract on
// one already-built you CLI, a real running Factory, a declared resource, and
// two operating-system process trees. Evidence is written before test cleanup;
// cleanup may protect the host after a failure but cannot turn it into a pass.
func TestPrebuiltWorkerSessionCancelStopsExactTree(t *testing.T) {
	journey := newCancelJourney(t)
	defer func() { saveCancelEvidence(t, journey.manifest) }()
	startCancelJourney(t, journey)

	attempt := canceledAttempt{applied: cancelTargetAndSampleTrees(t, journey)}
	captureCanceledAttempt(t, journey, &attempt)
	repeatCancelWithoutEvents(t, journey, &attempt)
	verifyCanceledWorkerSession(t, journey, attempt)
	verifyCanceledDispatchAndWork(t, journey, attempt)
	verifyUnrelatedWork(t, journey)
	verifyUnknownTargetParity(t, journey)
	stopAndCensusCancelJourney(t, journey)

	journey.manifest.CleanupMethod = "public Factory server stop; no validator process kill on passing path"
	journey.manifest.Outcome = "PASS"
	logCancelEvidence(t, journey.manifest)
}

func newCancelJourney(t *testing.T) *cancelJourney {
	t.Helper()
	binaryPath := resolveCancelArtifact(t)
	ctx, cancel := context.WithTimeout(t.Context(), cancelFactorySessionTimeout)
	t.Cleanup(cancel)
	fixture, err := writeCancelFixture(t)
	if err != nil {
		t.Fatalf("write controlled cancel fixture: %v", err)
	}
	binaryHash, err := fileSHA256(binaryPath)
	if err != nil {
		t.Fatalf("hash prebuilt CLI: %v", err)
	}
	fixtureHash, err := cancelFixtureSHA256(fixture.factoryDir)
	if err != nil {
		t.Fatalf("hash controlled cancel fixture: %v", err)
	}
	return &cancelJourney{
		ctx: ctx, binaryPath: binaryPath, fixture: fixture,
		manifest: cancelEvidenceManifest{
			Schema: "factory-reliability-cancel-v1", Outcome: "INCOMPLETE",
			ArtifactPath: binaryPath, BinarySHA256: binaryHash, FixtureSHA256: fixtureHash,
			GitHead: strings.TrimSpace(os.Getenv(cancelGitHeadEnvironment)), GoVersion: runtime.Version(),
			OS: runtime.GOOS, Arch: runtime.GOARCH,
			SampleInterval: cancelProcessSampleInterval.String(), ProcessBound: cancelProcessBound.String(),
		},
	}
}

func startCancelJourney(t *testing.T, journey *cancelJourney) {
	t.Helper()
	journey.daemon = startCancelDaemon(t, journey.ctx, journey.binaryPath, journey.fixture)
	journey.manifest.Listener = journey.fixture.serverURL
	journey.sessionID = waitForCancelFactorySession(t, journey.ctx, journey.fixture.serverURL, journey.daemon)
	journey.manifest.FactorySessionID = journey.sessionID
	journey.targetWorkID = submitCancelWork(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, "cancel-target")
	journey.unrelatedWorkID = submitCancelWork(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, "cancel-unrelated")
	journey.manifest.TargetWorkID = journey.targetWorkID
	journey.manifest.UnrelatedWorkID = journey.unrelatedWorkID
	journey.targetObservation = waitForRunningWorkerSession(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetWorkID, journey.daemon)
	journey.unrelatedObservation = waitForRunningWorkerSession(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.unrelatedWorkID, journey.daemon)
	journey.manifest.TargetWorkerSessionID = journey.targetObservation.WorkerSessionId
	journey.manifest.TargetDispatchID = journey.targetObservation.AttemptId
	journey.manifest.UnrelatedWorkerSessionID = journey.unrelatedObservation.WorkerSessionId
	journey.targetTree = waitForFixtureProcessTree(t, journey.ctx, journey.fixture.stateDir, journey.targetWorkID)
	registerFailedTreeCleanup(t, journey.targetTree)
	journey.unrelatedTree = waitForFixtureProcessTree(t, journey.ctx, journey.fixture.stateDir, journey.unrelatedWorkID)
	registerFailedTreeCleanup(t, journey.unrelatedTree)
	journey.manifest.TargetTreeBefore = append([]int(nil), journey.targetTree.PIDs...)
	journey.manifest.UnrelatedTreeBefore = append([]int(nil), journey.unrelatedTree.PIDs...)
	assertObservedTreeAncestry(t, journey.targetTree)
	assertObservedTreeAncestry(t, journey.unrelatedTree)
	journey.beforeCancel = readCancelWork(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.unrelatedWorkID)
	if journey.beforeCancel.State == nil || journey.beforeCancel.State.Type != factoryapi.WorkStateTypePROCESSING {
		t.Fatalf("unrelated Work before cancel = %#v, want PROCESSING", journey.beforeCancel)
	}
}

func registerFailedTreeCleanup(t *testing.T, tree workerProcessTree) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() {
			cleanupCancelProcessTree(tree)
		}
	})
}

func cancelTargetAndSampleTrees(t *testing.T, journey *cancelJourney) factoryapi.WorkerSessionControlResponse {
	t.Helper()
	manifest := &journey.manifest
	apiStarted := time.Now()
	apiStatus, apiBody, apiErr := postWorkerSessionCancel(journey.ctx, journey.fixture.serverURL, journey.targetObservation.WorkerSessionId)
	apiReturned := time.Now()
	manifest.APIControlStartedAt, manifest.APIAppliedAt = apiStarted.UTC(), apiReturned.UTC()
	if apiErr != nil {
		t.Fatalf("API cancel exact Worker Session %q: %v", journey.targetObservation.WorkerSessionId, apiErr)
	}
	manifest.APIStatus = apiStatus
	var applied factoryapi.WorkerSessionControlResponse
	if err := json.Unmarshal(apiBody, &applied); err != nil {
		t.Fatalf("decode API cancel response: %v; body=%s", err, strings.TrimSpace(string(apiBody)))
	}
	manifest.APIOutcome, manifest.APIState = string(applied.Outcome), string(applied.State)
	if apiStatus != http.StatusOK || applied.WorkerSessionId != journey.targetObservation.WorkerSessionId ||
		applied.Action != factoryapi.WorkerSessionControlResponseActionCancel ||
		applied.Outcome != factoryapi.WorkerSessionControlResponseOutcomeApplied ||
		applied.State != factoryapi.WorkerSessionControlResponseStateCanceled || applied.DispatchId != journey.targetObservation.AttemptId {
		manifest.Outcome = "FAIL_API_CONTROL"
		t.Fatalf("API cancel = HTTP %d %#v; want APPLIED/CANCELED for exact dispatch %q", apiStatus, applied, journey.targetObservation.AttemptId)
	}
	manifest.Samples = sampleCancelProcessTrees(t, journey.ctx, journey.targetTree, journey.unrelatedTree, apiReturned)
	manifest.TargetGoneAfterAPIMillis = firstTargetAbsenceMillis(manifest.Samples)
	assertCancelProcessSamples(t, journey)
	return applied
}

func assertCancelProcessSamples(t *testing.T, journey *cancelJourney) {
	t.Helper()
	manifest := &journey.manifest
	if !validCancelSampleSchedule(manifest.Samples) {
		manifest.Outcome = "FAIL_SAMPLE_SCHEDULE"
		t.Fatalf("process observations do not cover the 10 second bound at one-second intervals: %#v", manifest.Samples)
	}
	if !allTargetSamplesAbsent(manifest.Samples) {
		manifest.Outcome = "FAIL_TARGET_PROCESS_BOUND"
		t.Fatalf("owned target process tree survived the 10 second bound after APPLIED; samples=%#v", manifest.Samples)
	}
	if !allUnrelatedSamplesUnchanged(manifest.Samples, journey.unrelatedTree.PIDs) {
		manifest.Outcome = "FAIL_UNRELATED_PROCESS_CHANGED"
		t.Fatalf("unrelated process tree changed during target cancel; initial=%v samples=%#v", journey.unrelatedTree.PIDs, manifest.Samples)
	}
}

func captureCanceledAttempt(t *testing.T, journey *cancelJourney, attempt *canceledAttempt) {
	t.Helper()
	manifest := &journey.manifest
	if err := waitForCanceledDispatchEvent(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, attempt.applied.DispatchId); err != nil {
		manifest.Outcome = "FAIL_DISPATCH_EVENT"
		t.Fatalf("wait for canonical canceled dispatch event: %v", err)
	}
	attempt.sessionsBeforeRepeat = readWorkerSessions(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetWorkID)
	if len(attempt.sessionsBeforeRepeat.Sessions) != 2 {
		manifest.Outcome = "FAIL_CANCEL_SESSION_COUNT"
		t.Fatalf("continuously running target Work has %d Worker Sessions before the stable repeat; want the canceled attempt and its one authorized successor: %#v", len(attempt.sessionsBeforeRepeat.Sessions), attempt.sessionsBeforeRepeat.Sessions)
	}
	manifest.AuthorizedSuccessorCount = 1
	attempt.successorBefore = cancelSuccessorObservation(t, attempt.sessionsBeforeRepeat, journey.targetObservation.WorkerSessionId, journey.targetWorkID)
	journey.successorTree = waitForFixtureProcessTree(t, journey.ctx, journey.fixture.stateDir, journey.targetWorkID, journey.targetTree.RootPID)
	registerFailedTreeCleanup(t, journey.successorTree)
	assertObservedTreeAncestry(t, journey.successorTree)
	manifest.TargetSuccessorTreeBefore = append([]int(nil), journey.successorTree.PIDs...)
	eventsBeforeRepeat, err := readFactoryEvents(journey.ctx, journey.fixture.serverURL, journey.sessionID)
	if err != nil {
		manifest.Outcome = "FAIL_EVENT_READ"
		t.Fatalf("read canonical events before CLI repeat: %v", err)
	}
	manifest.DispatchResponseAt, err = dispatchResponseTime(eventsBeforeRepeat, attempt.applied.DispatchId)
	if err != nil {
		manifest.Outcome = "FAIL_DISPATCH_COMPLETION_TIME"
		t.Fatalf("read canceled dispatch command completion time: %v", err)
	}
	manifest.CommandCompletionAfterAPIMillis = manifest.DispatchResponseAt.Sub(manifest.APIAppliedAt).Milliseconds()
	if manifest.CommandCompletionAfterAPIMillis > cancelProcessBound.Milliseconds() {
		manifest.Outcome = "FAIL_COMMAND_COMPLETION_BOUND"
		t.Fatalf("canceled command completion observed %dms after APPLIED; bound is %dms", manifest.CommandCompletionAfterAPIMillis, cancelProcessBound.Milliseconds())
	}
	manifest.EventIDsBeforeRepeat = factoryEventIDs(eventsBeforeRepeat)
	listedTarget := observationByID(t, attempt.sessionsBeforeRepeat, journey.targetObservation.WorkerSessionId)
	if err := assertCanceledWorkerObservation(listedTarget, journey.targetObservation.WorkerSessionId, journey.targetWorkID, attempt.applied.DispatchId); err != nil {
		manifest.Outcome = "FAIL_LIST_TRUTH_BEFORE_REPEAT"
		t.Fatalf("canceled Worker Session list truth before CLI repeat: %v", err)
	}
}

func repeatCancelWithoutEvents(t *testing.T, journey *cancelJourney, attempt *canceledAttempt) {
	t.Helper()
	manifest := &journey.manifest
	repeat := runCancelCLI(journey.ctx, journey.binaryPath, journey.fixture, "worker-sessions", "cancel", journey.targetObservation.WorkerSessionId)
	manifest.CLIRepeatExit = exitCode(repeat.err)
	if repeat.err != nil {
		manifest.Outcome = "FAIL_CLI_REPEAT"
		t.Fatalf("CLI repeat cancel: %v; stdout=%s stderr=%s", repeat.err, repeat.stdout, repeat.stderr)
	}
	var repeated factoryapi.WorkerSessionControlResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(repeat.stdout)), &repeated); err != nil {
		manifest.Outcome = "FAIL_CLI_REPEAT_DECODE"
		t.Fatalf("decode CLI repeat cancel: %v; stdout=%q", err, repeat.stdout)
	}
	manifest.CLIRepeatOutcome, manifest.CLIRepeatState = string(repeated.Outcome), string(repeated.State)
	if repeated.WorkerSessionId != journey.targetObservation.WorkerSessionId ||
		repeated.Action != factoryapi.WorkerSessionControlResponseActionCancel ||
		repeated.Outcome != factoryapi.WorkerSessionControlResponseOutcomeNoop ||
		repeated.State != factoryapi.WorkerSessionControlResponseStateCanceled || repeated.DispatchId != attempt.applied.DispatchId {
		manifest.Outcome = "FAIL_CLI_REPEAT_TRUTH"
		t.Fatalf("CLI repeat cancel = %#v; want NOOP/CANCELED for exact dispatch %q", repeated, attempt.applied.DispatchId)
	}
	eventsAfterRepeat, err := readFactoryEvents(journey.ctx, journey.fixture.serverURL, journey.sessionID)
	if err != nil {
		manifest.Outcome = "FAIL_EVENT_READ_AFTER_REPEAT"
		t.Fatalf("read canonical events after CLI repeat: %v", err)
	}
	attempt.eventsAfterRepeat = eventsAfterRepeat
	manifest.EventIDsAfterRepeat = factoryEventIDs(eventsAfterRepeat)
	if !reflect.DeepEqual(manifest.EventIDsBeforeRepeat, manifest.EventIDsAfterRepeat) {
		manifest.Outcome = "FAIL_REPEAT_ADDED_EVENT"
		t.Fatalf("CLI NOOP changed canonical Factory Event identities: before=%v after=%v", manifest.EventIDsBeforeRepeat, manifest.EventIDsAfterRepeat)
	}
}

func verifyCanceledWorkerSession(t *testing.T, journey *cancelJourney, attempt canceledAttempt) {
	t.Helper()
	manifest := &journey.manifest
	workerEvents, err := readWorkerSessionEvents(journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetObservation.WorkerSessionId)
	if err != nil {
		manifest.Outcome = "FAIL_WORKER_EVENT_READ"
		t.Fatalf("read target Worker Session events: %v", err)
	}
	manifest.WorkerTerminalCount, manifest.WorkerTerminalPhase = terminalWorkerEventFacts(workerEvents)
	if manifest.WorkerTerminalCount != 1 || manifest.WorkerTerminalPhase != "CANCELED" {
		manifest.Outcome = "FAIL_WORKER_TERMINAL_EVENT"
		t.Fatalf("Worker Session terminal events = %d/%q; want one CANCELED terminal: %#v", manifest.WorkerTerminalCount, manifest.WorkerTerminalPhase, workerEvents)
	}
	var scriptOutput string
	manifest.ScriptResponseCount, manifest.ScriptResponseOutcome, scriptOutput = scriptResponseFacts(workerEvents)
	manifest.LateOutputRetained = strings.Contains(scriptOutput, cancelFixtureLateOutput)
	if manifest.ScriptResponseCount != 1 || manifest.ScriptResponseOutcome != "CANCELED" || !manifest.LateOutputRetained {
		manifest.Outcome = "FAIL_LATE_OUTPUT_CLASSIFICATION"
		t.Fatalf("canceled script response count/outcome/late-output=%d/%q/%t stdout=%q; want one CANCELED response retaining the late output", manifest.ScriptResponseCount, manifest.ScriptResponseOutcome, manifest.LateOutputRetained, scriptOutput)
	}
	verifyCanceledWorkerProjections(t, journey, attempt)
}

func verifyCanceledWorkerProjections(t *testing.T, journey *cancelJourney, attempt canceledAttempt) {
	t.Helper()
	manifest := &journey.manifest
	listed := readWorkerSessions(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetWorkID)
	listedTarget := observationByID(t, listed, journey.targetObservation.WorkerSessionId)
	detailedTarget := readWorkerSessionDetail(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetObservation.WorkerSessionId)
	manifest.TargetFailureKind = failureKind(listedTarget)
	if err := assertCanceledWorkerObservation(listedTarget, journey.targetObservation.WorkerSessionId, journey.targetWorkID, attempt.applied.DispatchId); err != nil {
		manifest.Outcome = "FAIL_LIST_TRUTH"
		t.Fatalf("canceled Worker Session list truth: %v; observation=%#v", err, listedTarget)
	}
	if err := assertCanceledWorkerObservation(detailedTarget, journey.targetObservation.WorkerSessionId, journey.targetWorkID, attempt.applied.DispatchId); err != nil {
		manifest.Outcome = "FAIL_DETAIL_TRUTH"
		t.Fatalf("canceled Worker Session detail truth: %v; observation=%#v", err, detailedTarget)
	}
	if err := assertTranscriptUnavailable(journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetObservation.WorkerSessionId); err != nil {
		manifest.Outcome = "FAIL_TRANSCRIPT_CLASSIFICATION"
		t.Fatalf("terminal script Worker Session transcript classification: %v", err)
	}
	manifest.TargetTranscript = string(detailedTarget.Transcript)
	if !reflect.DeepEqual(workerSessionIDs(attempt.sessionsBeforeRepeat), workerSessionIDs(listed)) {
		manifest.Outcome = "FAIL_NOOP_ADMITTED_SUCCESSOR"
		t.Fatalf("CLI NOOP changed target Work Worker Session identities: before=%v after=%v", workerSessionIDs(attempt.sessionsBeforeRepeat), workerSessionIDs(listed))
	}
	if len(listed.Sessions) != 2 {
		manifest.Outcome = "FAIL_UNEXPECTED_SUCCESSOR"
		t.Fatalf("target Work has %d Worker Sessions after cancel/repeat, want the canceled attempt and one authorized successor: %#v", len(listed.Sessions), listed.Sessions)
	}
	verifySuccessorUnchanged(t, journey, attempt, listed)
}

func verifySuccessorUnchanged(t *testing.T, journey *cancelJourney, attempt canceledAttempt, listed factoryapi.ListWorkerSessionsResponse) {
	t.Helper()
	manifest := &journey.manifest
	successorAfter := observationByID(t, listed, attempt.successorBefore.WorkerSessionId)
	if !sameActiveWorkerObservation(attempt.successorBefore, successorAfter) {
		manifest.Outcome = "FAIL_NOOP_CHANGED_SUCCESSOR"
		t.Fatalf("CLI NOOP changed the one authorized successor: before=%#v after=%#v", attempt.successorBefore, successorAfter)
	}
	currentPIDs, err := currentCancelTreePIDs(journey.successorTree)
	if err != nil || !reflect.DeepEqual(currentPIDs, journey.successorTree.PIDs) {
		manifest.Outcome = "FAIL_NOOP_CHANGED_SUCCESSOR_PROCESS"
		t.Fatalf("CLI NOOP changed the one authorized successor process tree: pids=%v error=%v, want %v", currentPIDs, err, journey.successorTree.PIDs)
	}
}

func verifyCanceledDispatchAndWork(t *testing.T, journey *cancelJourney, attempt canceledAttempt) {
	t.Helper()
	manifest := &journey.manifest
	facts, err := canceledDispatchFacts(attempt.eventsAfterRepeat, journey.targetWorkID, attempt.applied.DispatchId)
	if err != nil {
		manifest.Outcome = "FAIL_RESOURCE_EVENT"
		t.Fatalf("canceled dispatch/resource evidence: %v", err)
	}
	manifest.CanceledDispatchRequestCount, manifest.CanceledDispatchResponseCount = facts.Requests, facts.Responses
	manifest.CancellationReason, manifest.ResourceReleaseCount = facts.CancellationReason, facts.ResourceReleases
	manifest.LateOutputRetained = manifest.LateOutputRetained || facts.LateOutputReturned
	manifest.CanceledWorkState = facts.OutputWorkState
	if facts.Requests != 1 || facts.Responses != 1 || facts.ResourceReleases != 1 ||
		facts.CancellationReason != "CANCELED" || facts.OutputWorkState != "init" {
		manifest.Outcome = "FAIL_CANCELLED_DISPATCH_FACTS"
		t.Fatalf("canceled dispatch facts = %#v; want one request, response, resource release, and restored init Work", facts)
	}
	targetWork := readCancelWork(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.targetWorkID)
	manifest.CanceledWorkState = workStateName(targetWork)
	if targetWork.State == nil || targetWork.State.Name != "init" ||
		targetWork.State.Type == factoryapi.WorkStateTypeTERMINAL || strings.Contains(workContentText(targetWork), cancelFixtureLateOutput) {
		manifest.Outcome = "FAIL_WORK_ADVANCED"
		t.Fatalf("canceled Work after control = %#v, want restored init state with no late result", targetWork)
	}
}

func verifyUnrelatedWork(t *testing.T, journey *cancelJourney) {
	t.Helper()
	manifest := &journey.manifest
	afterCancel := readCancelWork(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.unrelatedWorkID)
	if !sameWorkStateAndContent(journey.beforeCancel, afterCancel) {
		manifest.Outcome = "FAIL_UNRELATED_WORK_CHANGED"
		t.Fatalf("unrelated Work changed during target cancel: before=%#v after=%#v", journey.beforeCancel, afterCancel)
	}
	unrelatedAfter := observationByID(t, readWorkerSessions(t, journey.ctx, journey.fixture.serverURL, journey.sessionID, journey.unrelatedWorkID), journey.unrelatedObservation.WorkerSessionId)
	if !sameActiveWorkerObservation(journey.unrelatedObservation, unrelatedAfter) {
		manifest.Outcome = "FAIL_UNRELATED_SESSION_CHANGED"
		t.Fatalf("unrelated Worker Session changed during target cancel: before=%#v after=%#v", journey.unrelatedObservation, unrelatedAfter)
	}
	manifest.UnrelatedWorkStateAfter, manifest.UnrelatedSessionStateAfter = workStateName(afterCancel), string(unrelatedAfter.State)
}

func verifyUnknownTargetParity(t *testing.T, journey *cancelJourney) {
	t.Helper()
	manifest := &journey.manifest
	unknownAPIStatus, unknownAPIBody, unknownAPIErr := postWorkerSessionCancel(journey.ctx, journey.fixture.serverURL, "cancel-child-unknown-target")
	if unknownAPIErr != nil {
		manifest.Outcome = "FAIL_UNKNOWN_API"
		t.Fatalf("API cancel unknown target: %v", unknownAPIErr)
	}
	manifest.UnknownAPIStatus, manifest.UnknownAPICode = unknownAPIStatus, errorResponseCode(unknownAPIBody)
	unknownCLI := runCancelCLI(journey.ctx, journey.binaryPath, journey.fixture, "worker-sessions", "cancel", "cancel-child-unknown-target")
	manifest.UnknownCLIExit, manifest.UnknownCLICode = exitCode(unknownCLI.err), firstErrorCode(unknownCLI.stdout, unknownCLI.stderr)
	if unknownAPIStatus != http.StatusNotFound || manifest.UnknownAPICode != string(factoryapi.ErrorResponseCodeNOTFOUND) ||
		unknownCLI.err == nil || manifest.UnknownCLICode != string(factoryapi.ErrorResponseCodeNOTFOUND) {
		manifest.Outcome = "FAIL_UNKNOWN_TARGET_PARITY"
		t.Fatalf("unknown-target API/CLI = HTTP %d %q and exit %d %q; want NOT_FOUND parity; stdout=%s stderr=%s", unknownAPIStatus, manifest.UnknownAPICode, manifest.UnknownCLIExit, manifest.UnknownCLICode, unknownCLI.stdout, unknownCLI.stderr)
	}
}

func stopAndCensusCancelJourney(t *testing.T, journey *cancelJourney) {
	t.Helper()
	manifest := &journey.manifest
	stopCancelDaemon(t, journey.binaryPath, journey.fixture, journey.daemon)
	census, err := collectCancelCleanupCensus(
		[]workerProcessTree{journey.targetTree, journey.successorTree},
		[]workerProcessTree{journey.unrelatedTree},
		journey.daemon.cmd.Process.Pid,
		journey.fixture.port,
	)
	manifest.CleanupCensus = census
	if err != nil {
		manifest.Outcome = "FAIL_CLEANUP_CENSUS"
		t.Fatalf("collect post-cleanup process census: %v", err)
	}
	if len(census.TargetPIDsRemaining) != 0 || len(census.UnrelatedPIDsRemaining) != 0 ||
		census.DaemonPIDRemaining || !census.ListenerAvailable {
		manifest.Outcome = "FAIL_CLEANUP_CENSUS"
		t.Fatalf("public cleanup left task-owned process/listener evidence: %#v", census)
	}
}

func logCancelEvidence(t *testing.T, manifest cancelEvidenceManifest) {
	t.Helper()
	t.Logf("compiled cancel evidence: binary_sha256=%s fixture_sha256=%s target=%s/%s dispatch=%s gone_ms=%d resource_releases=%d unrelated=%s/%s samples=%d head=%s", manifest.BinarySHA256, manifest.FixtureSHA256, manifest.TargetWorkID, manifest.TargetWorkerSessionID, manifest.TargetDispatchID, manifest.TargetGoneAfterAPIMillis, manifest.ResourceReleaseCount, manifest.UnrelatedWorkID, manifest.UnrelatedWorkerSessionID, len(manifest.Samples), manifest.GitHead)
}
