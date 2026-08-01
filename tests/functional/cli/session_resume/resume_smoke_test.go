package session_resume_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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
	if harness.provider.callCount() < 3 {
		t.Fatalf("provider infer calls = %d, want at least 3 after resume completion", harness.provider.callCount())
	}
}

func TestCLIResumeSmoke_DurableResumeContinuityPreservesCompletedChildDispatchesWithoutReplay(t *testing.T) {
	t.Parallel()

	harness := newCLIResumeSmokeHarness(t)
	sessionID := harness.startInterruptedSession(t)

	beforeShow := readDurableSessionViaCLI(t, harness, sessionID)
	assertDurableProgressCounts(t, beforeShow.Progress, 1, 2, 0)

	beforeDispatches := readDispatchesViaCLI(t, harness, sessionID)
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

	afterDispatches := readDispatchesViaCLI(t, harness, sessionID)
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

	if harness.provider.callCount() != 3 {
		t.Fatalf("provider infer calls = %d, want exactly 3 (step-one once, blocked step-two once, resumed step-two once)", harness.provider.callCount())
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

func readDispatchesViaCLI(
	t *testing.T,
	harness *cliResumeSmokeHarness,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	out, err := harness.executeCLI(t, "session", "dispatches", sessionID)
	if err != nil {
		t.Fatalf("session dispatches: %v", err)
	}

	var listed factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &listed); err != nil {
		t.Fatalf("decode session dispatches JSON: %v\n%s", err, out.String())
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

type cliResumeSmokeHarness struct {
	serverURL   string
	projectRoot string
	process     cliResumeProcess
	provider    *cliResumeSmokeBlockingProvider
}

type cliResumeProcess interface {
	Execute(root.Input) error
}

func newCLIResumeSmokeHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "resumable-two-step-fake-children.workflow.js", workflowName)
	provider := newCLIResumeSmokeBlockingProvider(workflowName)

	serverURL, process := startRootCLIResumeAPIServer(t, projectRoot, provider)
	return &cliResumeSmokeHarness{serverURL: serverURL, projectRoot: projectRoot, process: process, provider: provider}
}

func newCLIResumeSmokeSucceededHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "simple-final"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "simple-final.workflow.js", workflowName)
	serverURL, process := startRootCLIResumeAPIServer(t, projectRoot, nil)
	return &cliResumeSmokeHarness{serverURL: serverURL, projectRoot: projectRoot, process: process}
}

func newCLIResumeSmokeRunningHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "busy-loop"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "busy-loop.workflow.js", workflowName)
	serverURL, process := startRootCLIResumeAPIServer(t, projectRoot, nil)
	return &cliResumeSmokeHarness{serverURL: serverURL, projectRoot: projectRoot, process: process}
}

func startRootCLIResumeAPIServer(
	t *testing.T,
	projectRoot string,
	provider workerprovider.Provider,
) (string, cliResumeProcess) {
	t.Helper()

	ready := make(chan struct{})
	var server *httptest.Server
	startServer := func(ctx context.Context, request platformhttpserver.StartRequest) error {
		server = httptest.NewServer(request.Handler)
		close(ready)
		if request.OnBound != nil {
			request.OnBound(platformhttpserver.Binding{Port: request.Port})
		}
		<-ctx.Done()
		server.Close()
		return nil
	}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderOverride: provider,
		APIServerStarter: startServer,
	})
	if err != nil {
		t.Fatalf("BuildProcess: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	homeDir := t.TempDir()
	env := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	stdinIsTTY := true
	runErr := make(chan error, 1)
	go func() {
		runErr <- process.Execute(root.Input{
			Args: []string{
				"you", "run", "--dir", projectRoot, "--continuously", "--with-server", "--quiet", "--no-record",
			},
			Env:              env,
			Context:          ctx,
			WorkingDirectory: projectRoot,
			StdinIsTTY:       &stdinIsTTY,
		})
	}()
	select {
	case <-ready:
	case err := <-runErr:
		cancel()
		t.Fatalf("start root CLI resume API server: %v", err)
	case <-time.After(15 * time.Second):
		cancel()
		t.Fatal("root CLI resume API server did not become ready")
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-runErr:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("root CLI resume API shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("root CLI resume API shutdown timed out")
		}
	})
	return server.URL, process
}

func (h *cliResumeSmokeHarness) executeCLI(t *testing.T, args ...string) (bytes.Buffer, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	inputArgs := []string{"you", "--json", "--server", h.serverURL}
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

func strPtr(value string) *string {
	return &value
}

func (h *cliResumeSmokeHarness) startSucceededSession(t *testing.T) string {
	t.Helper()

	const workflowName = "simple-final"
	started := startDurableSessionViaHTTP(t, h.serverURL, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-cli-resume-smoke-succeeded-001",
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
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	waitForDurableSessionStatusViaCLI(
		t, h, sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusSucceeded, 15*time.Second,
	)
	return sessionID
}

func (h *cliResumeSmokeHarness) startRunningSession(t *testing.T) string {
	t.Helper()

	const workflowName = "busy-loop"
	started := startDurableSessionViaHTTP(t, h.serverURL, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-cli-resume-smoke-running-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
	})
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	waitForDurableSessionStatusViaCLI(
		t, h, sessionID,
		factoryapi.FactorySessionDurableLifecycleStatusRunning, 5*time.Second,
	)
	return sessionID
}

func (h *cliResumeSmokeHarness) startInterruptedSession(t *testing.T) string {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	started := startDurableSessionViaHTTP(t, h.serverURL, factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-cli-resume-smoke-start-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:         factoryapi.FactorySessionExecutionSourceKindWorkflowName,
			WorkflowName: strPtr(workflowName),
		},
		Args: &map[string]any{
			"subject": "workflows",
		},
	})
	sessionID := started.SessionId
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}

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
	postCLIResumeJSON(t, h.serverURL+"/factory-sessions/"+sessionID+"/interrupt-dispatch",
		factoryapi.FactorySessionInterruptDispatchRequest{
			DispatchId: "dispatch-2",
			Reason:     &reason,
		},
	)
	h.provider.waitForCanceledInfer(t, 5*time.Second)
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

func setupCLIResumeSmokeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()

	projectRoot := support.ScaffoldSingleStepFactory(t, "cli-resume-smoke")
	workflowDir := filepath.Join(projectRoot, ".claude", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "fixtures", "javascript_runtime", fixtureName))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixtureName, err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, workflowName+".js"), raw, 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return projectRoot
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
		listed := readDispatchesViaCLI(t, harness, sessionID)
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

type cliResumeSmokeBlockingProvider struct {
	mu              sync.Mutex
	calls           int
	blockedOnce     bool
	contextCanceled int
	workflowName    string
}

func newCLIResumeSmokeBlockingProvider(workflowName string) *cliResumeSmokeBlockingProvider {
	return &cliResumeSmokeBlockingProvider{workflowName: workflowName}
}

func (p *cliResumeSmokeBlockingProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func (p *cliResumeSmokeBlockingProvider) Infer(ctx context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	alreadyBlocked := p.blockedOnce
	p.mu.Unlock()

	if call == 1 {
		return workerexecution.InferenceResponse{
			Content: fmt.Sprintf(`{"text":"live:%s:step-one:step-one:workflows","label":"step-one"}`, p.workflowName),
			ProviderSession: &workerexecution.ProviderSessionMetadata{
				Provider: "mock",
				Kind:     "session_id",
				ID:       "live-provider-session-1",
			},
		}, nil
	}

	if !alreadyBlocked {
		p.mu.Lock()
		p.blockedOnce = true
		p.mu.Unlock()

		<-ctx.Done()
		p.mu.Lock()
		p.contextCanceled++
		p.mu.Unlock()
		return workerexecution.InferenceResponse{}, ctx.Err()
	}

	return workerexecution.InferenceResponse{
		Content: fmt.Sprintf(`{"text":"live:%s:step-two:step-two:workflows","label":"step-two"}`, p.workflowName),
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "mock",
			Kind:     "session_id",
			ID:       "live-provider-session-2",
		},
	}, nil
}

func (p *cliResumeSmokeBlockingProvider) waitForCanceledInfer(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		canceled := p.contextCanceled
		p.mu.Unlock()
		if canceled > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("provider Infer did not observe canceled workflow context")
}
