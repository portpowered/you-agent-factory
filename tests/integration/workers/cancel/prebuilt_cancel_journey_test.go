package cancel_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// A real host death and joined restart prove the delivered transport repair.
// Default CLI control has its own owner-placement cases below; no synthetic
// capture or provider files stand in for this read/restart witness.
func TestPrebuiltRecordedReadControlParity(t *testing.T) {
	binary := resolveCancelArtifact(t)
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact sha256=%x", sha256.Sum256(artifact))
	f := writeRecordedParityFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	initialize := exec.CommandContext(ctx, binary, "init", "--provider", "codex")
	initialize.Dir, initialize.Env = f.factoryDir, f.environment
	if output, err := initialize.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, output)
	}
	first := startCancelDaemon(t, ctx, binary, f)
	waitForCancelFactorySession(t, ctx, f.serverURL, first)
	admitRecordedParityWorker(t, ctx, f, "completed")
	waitRecordedParityState(t, ctx, f, "completed", "COMPLETED")
	transcript := readRecordedParityTranscript(t, ctx, f, "completed", http.StatusOK)
	if !bytes.Contains(transcript, []byte("recorded parity completed marker")) {
		t.Fatalf("completed output missing: %s", transcript)
	}
	second := recoverRecordedParityOwner(t, ctx, binary, f, first)
	assertRecordedParityUnavailable(t, ctx, binary, f)
	after := readRecordedParityTranscript(t, ctx, f, "completed", http.StatusOK)
	assertRecordedParityJSON(t, transcript, after)
	cli := runCancelCLI(ctx, binary, f, "worker-sessions", "read", "--worker-session-id", "completed", "--view", "transcript")
	if cli.err != nil {
		t.Fatalf("completed CLI: %+v", cli)
	}
	assertRecordedParityJSON(t, after, []byte(cli.stdout))
	assertRecordedParityMCP(t, ctx, binary, f, after)
	cleanupCancelDaemon(second)
	select {
	case <-second.done:
	case <-ctx.Done():
		t.Fatal("recovered host was not joined")
	}
}

func recoverRecordedParityOwner(t *testing.T, ctx context.Context, binary string, f cancelFixture, first *cancelDaemon) *cancelDaemon {
	t.Helper()
	admitRecordedParityWorker(t, ctx, f, "lost")
	waitRecordedParityState(t, ctx, f, "lost", "RUNNING")
	prefix, err := getJSON[factoryapi.WorkerSessionLogPage](ctx, http.DefaultClient, f.serverURL+"/worker-sessions/lost/logs")
	if err != nil || len(prefix.Events) == 0 {
		t.Fatalf("committed opening: %+v %v", prefix, err)
	}
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first.done:
	case <-ctx.Done():
		t.Fatal("killed owner was not joined")
	}
	second := startCancelDaemon(t, ctx, binary, f)
	waitForCancelFactorySession(t, ctx, f.serverURL, second)
	row := waitRecordedParityState(t, ctx, f, "lost", "FAILED")
	if row.RecordingHealth == nil || *row.RecordingHealth != "INCOMPLETE" {
		t.Fatalf("owner loss health: %+v", row)
	}
	recoveredLogs, err := getJSON[factoryapi.WorkerSessionLogPage](ctx, http.DefaultClient, f.serverURL+"/worker-sessions/lost/logs")
	if err != nil || recoveredLogs.Health != "INCOMPLETE" || !reflect.DeepEqual(prefix.Events, recoveredLogs.Events) {
		t.Fatalf("lost capture prefix changed: before=%+v after=%+v err=%v", prefix, recoveredLogs, err)
	}
	return second
}

func assertRecordedParityUnavailable(t *testing.T, ctx context.Context, binary string, f cancelFixture) {
	t.Helper()
	lostBody := readRecordedParityTranscript(t, ctx, f, "lost", http.StatusInternalServerError)
	if errorResponseCode(lostBody) != "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE" {
		t.Fatalf("HTTP error: %s", lostBody)
	}
	cli := runCancelCLI(ctx, binary, f, "worker-sessions", "read", "--worker-session-id", "lost", "--view", "transcript")
	if cli.err == nil || firstErrorCode(cli.stdout, cli.stderr) != "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE" {
		t.Fatalf("CLI unavailable: %+v", cli)
	}
}

func admitRecordedParityWorker(t *testing.T, ctx context.Context, f cancelFixture, id string) {
	t.Helper()
	payload := map[string]any{"requestId": "request-" + id, "workerSessionId": id, "execution": map[string]any{
		"workstationName": "process", "workerType": id, "runnerId": "codex", "modelProvider": "codex", "model": "test-model", "userMessage": "safe recorded parity prompt",
		"dispatch": map[string]any{"dispatchId": "attempt-" + id, "workstationName": "process", "workerType": id},
	}}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.serverURL+"/worker-sessions", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("admit %s: %d %s", id, response.StatusCode, body)
	}
}

func waitRecordedParityState(t *testing.T, ctx context.Context, f cancelFixture, id, state string) factoryapi.WorkerSessionObservation {
	t.Helper()
	// The compiled host has no injected readiness channel. Poll its public
	// observation; this timer is cadence only and the context bounds failure.
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		row, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, f.serverURL+"/worker-sessions/"+id)
		ready := state != "COMPLETED" || (row.RecordingHealth != nil && *row.RecordingHealth == "COMPLETE" && row.Transcript == "AVAILABLE")
		if err == nil && ready && row.State == factoryapi.WorkerSessionObservationState(state) {
			return row
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("%s state %s: %+v %v", id, state, row, err)
		}
	}
}

func readRecordedParityTranscript(t *testing.T, ctx context.Context, f cancelFixture, id string, status int) []byte {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.serverURL+"/worker-sessions/"+id+"/transcript", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status {
		t.Fatalf("transcript %s: %d %s %v", id, response.StatusCode, body, err)
	}
	return body
}

func assertRecordedParityJSON(t *testing.T, want, got []byte) {
	t.Helper()
	// Generated optional members serialize as null through CLI/MCP and may be
	// absent from HTTP. Compare the complete public transcript contract.
	var a, b factoryapi.WorkerSessionTranscriptResponse
	if json.Unmarshal(want, &a) != nil || json.Unmarshal(got, &b) != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("JSON mismatch: %s / %s", want, got)
	}
}

func assertRecordedParityMCP(t *testing.T, ctx context.Context, binary string, f cancelFixture, transcript []byte) {
	t.Helper()
	command := exec.CommandContext(ctx, binary, "--server", f.serverURL, "server", "mcp")
	command.Dir, command.Env = f.factoryDir, f.environment
	client := mcp.NewClient(&mcp.Implementation{Name: "recorded-parity", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, id := range []string{"lost", "completed"} {
		result, encoded := readRecordedParityMCP(t, ctx, session, id)
		if id == "lost" {
			assertRecordedParityMCPUnavailable(t, result, encoded, id)
		} else {
			assertRecordedParityMCPCompleted(t, result, encoded, transcript)
		}
	}
}

func readRecordedParityMCP(t *testing.T, ctx context.Context, session *mcp.ClientSession, id string) (*mcp.CallToolResult, []byte) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "workerSessionId": id, "view": "transcript"}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if id == "completed" {
		if len(result.Content) != 1 {
			t.Fatalf("MCP completed content: %+v", result)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("MCP completed content type: %T", result.Content[0])
		}
		encoded = []byte(content.Text)
	}
	return result, encoded
}

func assertRecordedParityMCPUnavailable(t *testing.T, result *mcp.CallToolResult, encoded []byte, id string) {
	t.Helper()
	var envelope struct {
		Error struct {
			Code            string
			Retryable       bool
			WorkerSessionID string `json:"workerSessionId"`
			Details         struct {
				Status       int
				UpstreamCode string `json:"upstreamCode"`
			}
		}
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if !result.IsError || envelope.Error.Code != "worker_session.unavailable" || envelope.Error.Retryable || envelope.Error.WorkerSessionID != id || envelope.Error.Details.Status != 500 || envelope.Error.Details.UpstreamCode != "WORKER_SESSION_TRANSCRIPT_UNAVAILABLE" {
		t.Fatalf("MCP unavailable: %s", encoded)
	}
}

func assertRecordedParityMCPCompleted(t *testing.T, result *mcp.CallToolResult, encoded, transcript []byte) {
	t.Helper()
	var envelope struct {
		Result struct{ Transcript json.RawMessage }
	}
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("MCP completed: %s", encoded)
	}
	if len(envelope.Result.Transcript) == 0 {
		t.Fatalf("MCP transcript envelope missing: %s", encoded)
	}
	assertRecordedParityJSON(t, transcript, envelope.Result.Transcript)
}

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

// Two small delivered-artifact scenarios cross real CLI/host process boundaries.
// Declarative gates supply worker effects without provider calls or scripts.
func TestPrebuiltDefaultWorkerSessionCancelOwner(t *testing.T) {
	binary := resolveCancelArtifact(t)
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact sha256=%x OS=%s", sha256.Sum256(artifact), runtime.GOOS)
	for _, scenario := range []string{"terminal-restart", "owner-loss"} {
		t.Run(scenario, func(t *testing.T) {
			f := writeRecordedParityFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			initialize := exec.CommandContext(ctx, binary, "init", "--provider", "codex")
			initialize.Dir, initialize.Env = f.factoryDir, f.environment
			if output, err := initialize.CombinedOutput(); err != nil {
				t.Fatalf("init: %v %s", err, output)
			}
			host := startCancelDaemon(t, ctx, binary, f)
			waitForCancelFactorySession(t, ctx, f.serverURL, host)
			admitRecordedParityWorker(t, ctx, f, "lost")
			waitRecordedParityState(t, ctx, f, "lost", "RUNNING")
			admitRecordedParityWorker(t, ctx, f, "peer")
			waitRecordedParityState(t, ctx, f, "peer", "RUNNING")
			if scenario == "terminal-restart" {
				result := runDefaultOwnerCancel(ctx, binary, f, "lost", false)
				assertDefaultOwnerCancelResult(t, result, "APPLIED")
				assertDefaultOwnerPeer(t, ctx, f)
				cleanupCancelDaemon(host)
				select {
				case <-host.done:
				case <-ctx.Done():
					t.Fatal("host not joined")
				}
			} else {
				if err := host.cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				select {
				case <-host.done:
				case <-ctx.Done():
					t.Fatal("lost owner not joined")
				}
				assertDefaultOwnerUnreachable(t, ctx, binary, f)
			}
			host = startCancelDaemon(t, ctx, binary, f)
			waitForCancelFactorySession(t, ctx, f.serverURL, host)
			assertDefaultOwnerAfterRestart(t, ctx, binary, f, scenario)
		})
	}
}

func assertDefaultOwnerAfterRestart(t *testing.T, ctx context.Context, binary string, f cancelFixture, scenario string) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		result := runDefaultOwnerCancel(ctx, binary, f, "lost", remote)
		if scenario == "terminal-restart" {
			assertDefaultOwnerCancelResult(t, result, "NOOP")
		} else if exitCode(result.err) != 1 || firstErrorCode(result.stdout, result.stderr) != "WORKER_SESSION_CONTROL_FAILED" || strings.TrimSpace(result.stdout) != "" {
			t.Fatalf("owner-loss fabricated success: %+v", result)
		}
	}
	if scenario == "terminal-restart" {
		assertDefaultOwnerTransportParity(t, ctx, binary, f)
	}
}

func runDefaultOwnerCancel(ctx context.Context, binary string, f cancelFixture, id string, remote bool) cancelCommandResult {
	args := []string{"--server", f.serverURL, "--json", "worker-sessions", "cancel", id}
	if remote {
		args = append(args, "--remote")
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = f.factoryDir, f.environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return cancelCommandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func assertDefaultOwnerCancelResult(t *testing.T, result cancelCommandResult, outcome string) {
	t.Helper()
	var response factoryapi.WorkerSessionControlResponse
	if result.err != nil || json.Unmarshal([]byte(result.stdout), &response) != nil || response.WorkerSessionId != "lost" || string(response.Action) != "CANCEL" || string(response.Outcome) != outcome || string(response.State) != "CANCELED" || response.DispatchId != "attempt-lost" {
		t.Fatalf("cancel want %s/CANCELED/attempt-lost: %+v %+v", outcome, result, response)
	}
}

func assertDefaultOwnerPeer(t *testing.T, ctx context.Context, f cancelFixture) {
	t.Helper()
	row, err := getJSON[factoryapi.WorkerSessionObservation](ctx, http.DefaultClient, f.serverURL+"/worker-sessions/peer")
	if err != nil || string(row.State) != "RUNNING" || row.AttemptId != "attempt-peer" {
		t.Fatalf("peer changed: %+v %v", row, err)
	}
}

func assertDefaultOwnerTransportParity(t *testing.T, ctx context.Context, binary string, f cancelFixture) {
	t.Helper()
	status, body, err := postWorkerSessionCancel(ctx, f.serverURL, "lost")
	if err != nil || status != http.StatusOK {
		t.Fatalf("HTTP repeat=%d/%s/%v", status, body, err)
	}
	assertDefaultOwnerCancelResult(t, cancelCommandResult{stdout: string(body)}, "NOOP")
	command := exec.CommandContext(ctx, binary, "--server", f.serverURL, "server", "mcp")
	command.Dir, command.Env = f.factoryDir, f.environment
	client := mcp.NewClient(&mcp.Implementation{Name: "default-owner", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "CONTROL", "workerSessionId": "lost", "operation": "CANCEL"}})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP repeat: %+v %v", result, err)
	}
	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("MCP content=%T", result.Content[0])
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(content.Text), &envelope); err != nil {
		t.Fatal(err)
	}
	assertDefaultOwnerCancelResult(t, cancelCommandResult{stdout: string(envelope.Result)}, "NOOP")
}

func assertDefaultOwnerUnreachable(t *testing.T, ctx context.Context, binary string, f cancelFixture) {
	t.Helper()
	for _, remote := range []bool{false, true} {
		result := runDefaultOwnerCancel(ctx, binary, f, "lost", remote)
		code := "NOT_FOUND"
		if remote {
			code = "FACTORY_UNREACHABLE"
		}
		if exitCode(result.err) != 1 || firstErrorCode(result.stdout, result.stderr) != code {
			t.Fatalf("unreachable remote=%v: %+v", remote, result)
		}
	}
}
