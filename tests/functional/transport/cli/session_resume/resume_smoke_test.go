package session_resume_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCLIResumeSmoke_InterruptedJavaScriptFactorySessionResumesThroughSharedSessionCommands(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeHarness(t)
	sessionID := harness.startInterruptedSession(t)

	before := readDurableSessionViaCLI(t, harness, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", before.Status)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatalf("pre-resume lifecycle = %#v, want interruptedAt", before.Lifecycle)
	}

	resumeResponse := resumeSessionViaCLI(t, harness, sessionID)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", resumeResponse.Operation)
	}
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}
	if resumeResponse.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", resumeResponse.SessionId, sessionID)
	}

	after := waitForDurableSessionStatusViaCLI(
		t,
		harness,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Lifecycle == nil || after.Lifecycle.InterruptedAt == nil || after.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt", after.Lifecycle)
	}
	if after.ResultSummary == nil || after.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("post-resume resultSummary = %#v, want FINAL", after.ResultSummary)
	}
	if harness.provider.callCount(sessionID) < 3 {
		t.Fatalf("provider execute calls = %d, want at least 3 after resume completion", harness.provider.callCount(sessionID))
	}
}

func TestCLIResumeSmoke_DurableResumeContinuityPreservesCompletedChildDispatchesWithoutReplay(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeHarness(t)
	sessionID := harness.startInterruptedSession(t)

	beforeShow := readDurableSessionViaCLI(t, harness, sessionID)
	assertDurableProgressCounts(t, beforeShow.Progress, 1, 2, 0)

	beforeDispatches := readDispatchesViaHTTP(t, harness, sessionID)
	dispatchOneBefore := requireDispatchSummary(t, beforeDispatches, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	dispatchTwoBefore := requireDispatchSummary(t, beforeDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusINTERRUPTED, factoryapi.FactoryDispatchStatusRUNNING)
	if len(beforeDispatches.Dispatches) != 2 {
		t.Fatalf("pre-resume dispatch count = %d, want 2", len(beforeDispatches.Dispatches))
	}

	resumeResponse := resumeSessionViaCLI(t, harness, sessionID)
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}

	afterShow := waitForDurableSessionStatusViaCLI(
		t,
		harness,
		sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
		8*time.Second,
	)
	assertDurableProgressCounts(t, afterShow.Progress, 2, 2, 0)
	if afterShow.Lifecycle == nil || afterShow.Lifecycle.InterruptedAt == nil || afterShow.Lifecycle.ResumedAt == nil {
		t.Fatalf("post-resume lifecycle = %#v, want interruptedAt and resumedAt continuity", afterShow.Lifecycle)
	}
	if beforeShow.Lifecycle == nil || beforeShow.Lifecycle.InterruptedAt == nil {
		t.Fatal("pre-resume lifecycle missing interruptedAt")
	}
	if !afterShow.Lifecycle.InterruptedAt.Equal(*beforeShow.Lifecycle.InterruptedAt) {
		t.Fatalf(
			"interruptedAt changed across resume: before=%s after=%s",
			beforeShow.Lifecycle.InterruptedAt,
			afterShow.Lifecycle.InterruptedAt,
		)
	}

	afterDispatches := readDispatchesViaHTTP(t, harness, sessionID)
	if len(afterDispatches.Dispatches) != 2 {
		t.Fatalf("post-resume dispatch count = %d, want 2 (no replayed child dispatches)", len(afterDispatches.Dispatches))
	}
	dispatchOneAfter := requireDispatchSummary(t, afterDispatches, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	requireDispatchSummary(t, afterDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusCOMPLETED)
	assertDispatchSummaryParity(t, dispatchOneBefore, dispatchOneAfter)
	if dispatchTwoBefore.Status == factoryapi.FactoryDispatchStatusINTERRUPTED {
		// Interrupted dispatch-2 should finish as the same dispatch id, not spawn a third dispatch.
		dispatchTwoAfter := requireDispatchSummary(t, afterDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusCOMPLETED)
		if dispatchTwoAfter.Id != dispatchTwoBefore.Id {
			t.Fatalf("dispatch-2 id changed across resume: %q -> %q", dispatchTwoBefore.Id, dispatchTwoAfter.Id)
		}
	}

	if harness.provider.callCount(sessionID) != 3 {
		t.Fatalf("provider execute calls = %d, want exactly 3 (step-one once, blocked step-two once, resumed step-two once)", harness.provider.callCount(sessionID))
	}
}

func TestCLIResumeSmoke_TerminalSessionResumeReturnsTypedRejectionAndPreservesSessionRead(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeSucceededHarness(t)
	sessionID := harness.startSucceededSession(t)

	before := readDurableSessionViaCLI(t, harness, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("pre-resume status = %q, want SUCCEEDED", before.Status)
	}

	response, err := resumeSessionViaCLIExpectingRejection(t, harness, sessionID)
	var rejected *sessioncli.LifecycleControlRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("resume error = %v, want LifecycleControlRejectedError", err)
	}
	if rejected.Response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("resume outcome = %q, want TERMINAL_SESSION", rejected.Response.Outcome)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", response.Operation)
	}
	if response.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", response.SessionId, sessionID)
	}
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("stdout outcome = %q, want TERMINAL_SESSION", response.Outcome)
	}

	after := readDurableSessionViaCLI(t, harness, sessionID)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("post-resume status = %q, want SUCCEEDED unchanged", after.Status)
	}
	if before.ResultSummary == nil || after.ResultSummary == nil {
		t.Fatal("expected result summary before and after rejected resume")
	}
	if after.ResultSummary.ResultStatus != before.ResultSummary.ResultStatus {
		t.Fatalf(
			"result status changed after rejected resume: before=%q after=%q",
			before.ResultSummary.ResultStatus,
			after.ResultSummary.ResultStatus,
		)
	}
}

func TestCLIResumeSmoke_RunningSessionResumeReturnsTypedNoOpAndPreservesSessionRead(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeRunningHarness(t)
	sessionID := harness.startRunningSession(t)

	before := readDurableSessionViaCLI(t, harness, sessionID)
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("pre-resume status = %q, want RUNNING", before.Status)
	}

	response := resumeSessionViaCLI(t, harness, sessionID)
	if response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("resume outcome = %q, want NO_OP", response.Outcome)
	}
	if response.Operation != factoryapi.FactorySessionLifecycleControlKindResume {
		t.Fatalf("resume operation = %q, want RESUME", response.Operation)
	}
	if response.SessionId != sessionID {
		t.Fatalf("resume sessionId = %q, want %q", response.SessionId, sessionID)
	}
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("resume status = %q, want RUNNING", response.Status)
	}

	after := readDurableSessionViaCLI(t, harness, sessionID)
	if after.SessionId != sessionID {
		t.Fatalf("post-resume sessionId = %q, want %q", after.SessionId, sessionID)
	}
	if after.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("post-resume status = %q, want RUNNING unchanged", after.Status)
	}
}

func assertDurableProgressCounts(
	t *testing.T,
	progress *factoryapi.FactorySessionDurableProgressCounts,
	wantCompleted, wantTotal, wantInFlight int,
) {
	t.Helper()
	if progress == nil {
		t.Fatalf("progress = nil, want completed=%d total=%d inFlight=%d", wantCompleted, wantTotal, wantInFlight)
	}
	if intValueOrZero(progress.CompletedDispatches) != wantCompleted {
		t.Fatalf("completedDispatches = %#v, want %d", progress.CompletedDispatches, wantCompleted)
	}
	if intValueOrZero(progress.TotalDispatches) != wantTotal {
		t.Fatalf("totalDispatches = %#v, want %d", progress.TotalDispatches, wantTotal)
	}
	if intValueOrZero(progress.InFlightDispatches) != wantInFlight {
		t.Fatalf("inFlightDispatches = %#v, want %d", progress.InFlightDispatches, wantInFlight)
	}
}

func intValueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func readDispatchesViaHTTP(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	body := getCLIResumeJSON(t, harness.serverURL+"/factory-sessions/"+url.PathEscape(sessionID)+"/dispatches")
	var listed factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(body), &listed); err != nil {
		t.Fatalf("decode REST dispatches JSON: %v\n%s", err, body)
	}
	if listed.SessionId != sessionID {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.SessionId, sessionID)
	}
	if listed.Dispatches == nil {
		t.Fatal("dispatch list unexpectedly missing")
	}
	return listed
}

func requireDispatchSummary(
	t *testing.T,
	listed factoryapi.ListFactorySessionDispatchesResponse,
	dispatchID string,
	allowedStatuses ...factoryapi.FactoryDispatchStatus,
) factoryapi.FactorySessionDispatchSummary {
	t.Helper()

	for _, dispatch := range listed.Dispatches {
		if dispatch.Id != dispatchID {
			continue
		}
		for _, want := range allowedStatuses {
			if dispatch.Status == want {
				return dispatch
			}
		}
		t.Fatalf("dispatch %s status = %q, want one of %#v", dispatchID, dispatch.Status, allowedStatuses)
	}
	t.Fatalf("dispatch %s missing from %#v", dispatchID, listed.Dispatches)
	return factoryapi.FactorySessionDispatchSummary{}
}

func assertDispatchSummaryParity(
	t *testing.T,
	before, after factoryapi.FactorySessionDispatchSummary,
) {
	t.Helper()

	if before.Id != after.Id {
		t.Fatalf("dispatch id changed: %q -> %q", before.Id, after.Id)
	}
	if before.Status != after.Status && after.Status != factoryapi.FactoryDispatchStatusCOMPLETED {
		t.Fatalf("dispatch %s status drifted unexpectedly: before=%q after=%q", before.Id, before.Status, after.Status)
	}
	if before.DispatchKind != after.DispatchKind {
		t.Fatalf("dispatch %s kind changed: %q -> %q", before.Id, before.DispatchKind, after.DispatchKind)
	}
	if before.OutputArtifactIds != nil && after.OutputArtifactIds != nil {
		if len(*before.OutputArtifactIds) != len(*after.OutputArtifactIds) {
			t.Fatalf(
				"dispatch %s outputArtifactIds length changed: before=%#v after=%#v",
				before.Id,
				*before.OutputArtifactIds,
				*after.OutputArtifactIds,
			)
		}
		for i := range *before.OutputArtifactIds {
			if (*before.OutputArtifactIds)[i] != (*after.OutputArtifactIds)[i] {
				t.Fatalf(
					"dispatch %s outputArtifactIds changed: before=%#v after=%#v",
					before.Id,
					*before.OutputArtifactIds,
					*after.OutputArtifactIds,
				)
			}
		}
	}
}

func (h *cliResumeSmokeHarness) executeCLI(t *testing.T, args ...string) (bytes.Buffer, error) {
	t.Helper()
	h.fixture.executeMu.Lock()
	defer h.fixture.executeMu.Unlock()
	var stdout, stderr bytes.Buffer
	inputArgs := []string{"you", "--json"}
	if len(args) >= 2 && args[0] == "session" {
		switch args[1] {
		case "pause", "resume", "cancel", "terminate":
			inputArgs = append(inputArgs, "--remote")
		}
	}
	inputArgs = append(inputArgs, "--server", h.serverURL)
	inputArgs = append(inputArgs, args...)
	err := h.process.Execute(root.Input{
		Args:             inputArgs,
		Env:              os.Environ(),
		Context:          t.Context(),
		WorkingDirectory: h.projectRoot,
		Stdout:           &stdout,
		Stderr:           &stderr,
	})
	if err != nil && stderr.Len() > 0 {
		return stdout, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout, err
}

func startDurableSessionViaHTTP(
	t *testing.T,
	serverURL string,
	request factoryapi.FactorySessionExecutionRequest,
) factoryapi.FactorySessionExecutionResponse {
	t.Helper()
	body := postCLIResumeJSON(t, serverURL+"/factory-sessions/async", request)
	var response factoryapi.FactorySessionExecutionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode async Factory Session response: %v\n%s", err, body)
	}
	return response
}

func postCLIResumeJSON(t *testing.T, endpoint string, request any) []byte {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal CLI resume request: %v", err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("POST %s status = %d\n%s", endpoint, response.StatusCode, body)
	}
	return body
}

func getCLIResumeJSON(t *testing.T, endpoint string) []byte {
	t.Helper()
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read GET %s response: %v", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("GET %s status = %d\n%s", endpoint, response.StatusCode, body)
	}
	return body
}

func strPtr(value string) *string {
	return &value
}

func (h *cliResumeSmokeHarness) startSucceededSession(t *testing.T) string {
	t.Helper()

	const workflowName = "simple-final"
	started := h.startDurableSession(t, factoryapi.FactorySessionExecutionRequest{
		RequestId: h.fixture.nextRequestID("succeeded"),
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
		Args: &map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	sessionID := started.SessionId
	waitForDurableSessionStatusViaCLI(
		t, h, sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded, 15*time.Second,
	)
	return sessionID
}

func (h *cliResumeSmokeHarness) startRunningSession(t *testing.T) string {
	t.Helper()

	const workflowName = "busy-loop"
	started := h.startDurableSession(t, factoryapi.FactorySessionExecutionRequest{
		RequestId: h.fixture.nextRequestID("running"),
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
	})
	sessionID := started.SessionId
	waitForDurableSessionStatusViaCLI(
		t, h, sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning, 5*time.Second,
	)
	return sessionID
}

func (h *cliResumeSmokeHarness) startInterruptedSession(t *testing.T) string {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	started := h.startDurableSession(t, factoryapi.FactorySessionExecutionRequest{
		RequestId: h.fixture.nextRequestID("interrupted"),
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
		Args: &map[string]any{
			"subject": "workflows",
		},
	})
	sessionID := started.SessionId

	waitForCLIResumeSmokeDispatchStatus(
		t,
		h,
		sessionID,
		"dispatch-1",
		factoryapi.FactoryDispatchStatusCOMPLETED,
		15*time.Second,
	)
	waitForCLIResumeSmokeDispatchStatus(
		t,
		h,
		sessionID,
		"dispatch-2",
		factoryapi.FactoryDispatchStatusRUNNING,
		15*time.Second,
	)

	reason := "cli resume smoke interrupt"
	h.provider.waitForExecuteBlocked(t, sessionID, 5*time.Second)
	postCLIResumeJSON(t, h.serverURL+"/factory-sessions/"+url.PathEscape(sessionID)+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{
			DispatchId: "dispatch-2",
			Reason:     &reason,
		},
	)
	h.provider.waitForCanceledExecute(t, sessionID, 5*time.Second)
	waitForDurableSessionStatusViaCLI(
		t, h, sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusInterrupted, 5*time.Second,
	)
	return sessionID
}

func readDurableSessionViaCLI(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	out, err := harness.executeCLI(t, "session", "show", sessionID)
	if err != nil {
		t.Fatalf("session show: %v", err)
	}

	var session factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &session); err != nil {
		t.Fatalf("decode session show JSON: %v\n%s", err, out.String())
	}
	return session
}

func resumeSessionViaCLI(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	out, err := harness.executeCLI(t, "session", "resume", sessionID)
	if err != nil {
		t.Fatalf("session resume: %v", err)
	}

	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); err != nil {
		t.Fatalf("decode session resume JSON: %v\n%s", err, out.String())
	}
	return response
}

func resumeSessionViaCLIExpectingRejection(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	t.Helper()

	out, err := harness.executeCLI(t, "session", "resume", sessionID)

	var response factoryapi.FactorySessionLifecycleControlResponse
	if decodeErr := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); decodeErr != nil {
		t.Fatalf("decode session resume JSON: %v\n%s", decodeErr, out.String())
	}
	return response, err
}

func waitForDurableSessionStatusViaCLI(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableSessionViaCLI(t, harness, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableSessionViaCLI(t, harness, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func waitForCLIResumeSmokeDispatchStatus(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
	dispatchID string,
	want factoryapi.FactoryDispatchStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var last factoryapi.ListFactorySessionDispatchesResponse
	for time.Now().Before(deadline) {
		listed := readDispatchesViaHTTP(t, harness, sessionID)
		last = listed
		for _, dispatch := range listed.Dispatches {
			if dispatch.Id != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	lastJSON, _ := json.Marshal(last.Dispatches)
	t.Fatalf("dispatch %s did not reach %s within %s; last dispatches = %s", dispatchID, want, timeout, lastJSON)
}
