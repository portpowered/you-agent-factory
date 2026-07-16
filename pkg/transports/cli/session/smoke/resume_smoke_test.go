package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/runtimepersist"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/testharness"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"go.uber.org/zap"
)

func TestCLIResumeSmoke_InterruptedJavaScriptFactorySessionResumesThroughSharedSessionCommands(t *testing.T) {
	harness := newCLIResumeSmokeHarness(t)
	sessionID := harness.startInterruptedSession(t)

	before := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusInterrupted {
		t.Fatalf("pre-resume status = %q, want INTERRUPTED", before.Status)
	}
	if before.Lifecycle == nil || before.Lifecycle.InterruptedAt == nil {
		t.Fatalf("pre-resume lifecycle = %#v, want interruptedAt", before.Lifecycle)
	}

	resumeResponse := resumeSessionViaCLI(t, harness.serverURL, sessionID)
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
		harness.serverURL,
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
	harness := newCLIResumeSmokeHarness(t)
	sessionID := harness.startInterruptedSession(t)

	beforeShow := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
	assertDurableProgressCounts(t, beforeShow.Progress, 1, 2, 0)

	beforeDispatches := readDispatchesViaCLI(t, harness.serverURL, sessionID)
	dispatchOneBefore := requireDispatchSummary(t, beforeDispatches, "dispatch-1", factoryapi.FactoryDispatchStatusCOMPLETED)
	dispatchTwoBefore := requireDispatchSummary(t, beforeDispatches, "dispatch-2", factoryapi.FactoryDispatchStatusINTERRUPTED, factoryapi.FactoryDispatchStatusRUNNING)
	if len(beforeDispatches.Dispatches) != 2 {
		t.Fatalf("pre-resume dispatch count = %d, want 2", len(beforeDispatches.Dispatches))
	}

	resumeResponse := resumeSessionViaCLI(t, harness.serverURL, sessionID)
	if resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume outcome = %q, want ACCEPTED", resumeResponse.Outcome)
	}

	afterShow := waitForDurableSessionStatusViaCLI(
		t,
		harness.serverURL,
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

	afterDispatches := readDispatchesViaCLI(t, harness.serverURL, sessionID)
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
	harness := newCLIResumeSmokeSucceededHarness(t)
	sessionID := harness.startSucceededSession(t)

	before := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
	if before.SessionId != sessionID {
		t.Fatalf("pre-resume sessionId = %q, want %q", before.SessionId, sessionID)
	}
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("pre-resume status = %q, want SUCCEEDED", before.Status)
	}

	response, err := resumeSessionViaCLIExpectingRejection(t, harness.serverURL, sessionID)
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

	after := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
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
	harness := newCLIResumeSmokeRunningHarness(t)
	sessionID := harness.startRunningSession(t)

	before := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
	if before.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("pre-resume status = %q, want RUNNING", before.Status)
	}

	response := resumeSessionViaCLI(t, harness.serverURL, sessionID)
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

	after := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
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
	serverURL string,
	sessionID string,
) factoryapi.ListFactorySessionDispatchesResponse {
	t.Helper()

	var out bytes.Buffer
	if err := sessioncli.Dispatches(sessioncli.DispatchesConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &out,
	}); err != nil {
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
	serverURL string
	service   fse.Service
	provider  *cliResumeSmokeBlockingProvider
}

func newCLIResumeSmokeHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "resumable-two-step-fake-children.workflow.js", workflowName)
	provider := newCLIResumeSmokeBlockingProvider(workflowName)

	runtimeService := newCLIResumeRuntimeService(t, projectRoot, fse.ChildExecutorModeLive, provider)

	server := httptest.NewServer(api.NewServer(&testutil.MockFactory{
		DurableExecutionService: runtimeService,
	}, 0, zap.NewNop()).Handler())
	t.Cleanup(server.Close)

	return &cliResumeSmokeHarness{
		serverURL: server.URL,
		service:   runtimeService,
		provider:  provider,
	}
}

func newCLIResumeSmokeSucceededHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "simple-final"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "simple-final.workflow.js", workflowName)
	runtimeService := newCLIResumeRuntimeService(t, projectRoot, fse.ChildExecutorModeFake, nil)

	server := httptest.NewServer(api.NewServer(&testutil.MockFactory{
		DurableExecutionService: runtimeService,
	}, 0, zap.NewNop()).Handler())
	t.Cleanup(server.Close)

	return &cliResumeSmokeHarness{
		serverURL: server.URL,
		service:   runtimeService,
	}
}

func newCLIResumeSmokeRunningHarness(t *testing.T) *cliResumeSmokeHarness {
	t.Helper()

	const workflowName = "busy-loop"
	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "busy-loop.workflow.js", workflowName)
	runtimeService := newCLIResumeRuntimeService(t, projectRoot, fse.ChildExecutorModeFake, nil)

	server := httptest.NewServer(api.NewServer(&testutil.MockFactory{
		DurableExecutionService: runtimeService,
	}, 0, zap.NewNop()).Handler())
	t.Cleanup(server.Close)

	return &cliResumeSmokeHarness{
		serverURL: server.URL,
		service:   runtimeService,
	}
}

func newCLIResumeRuntimeService(
	t *testing.T,
	projectRoot string,
	childExecutorMode string,
	provider workers.Provider,
) fse.Service {
	t.Helper()

	service, err := testharness.New(testharness.Config{
		Mode:              testharness.ModeJavaScript,
		ProjectRoot:       projectRoot,
		Clock:             platformclock.Real{},
		Provider:          provider,
		Persistence:       runtimepersist.DirectoryStore{Dir: filepath.Join(t.TempDir(), "durable-sessions")},
		ChildExecutorMode: childExecutorMode,
	})
	if err != nil {
		t.Fatalf("compose CLI resume runtime service: %v", err)
	}
	return service
}

func (h *cliResumeSmokeHarness) startSucceededSession(t *testing.T) string {
	t.Helper()

	const workflowName = "simple-final"
	started, err := h.service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-cli-resume-smoke-succeeded-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: workflowName,
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	sessionID := started.SessionID
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	waitForCLIResumeSmokeSessionStatus(t, h.service, sessionID, fse.LifecycleStatusSucceeded, 15*time.Second)
	return sessionID
}

func (h *cliResumeSmokeHarness) startRunningSession(t *testing.T) string {
	t.Helper()

	const workflowName = "busy-loop"
	started, err := h.service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-cli-resume-smoke-running-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: workflowName,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	sessionID := started.SessionID
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}
	waitForCLIResumeSmokeSessionStatus(t, h.service, sessionID, fse.LifecycleStatusRunning, 5*time.Second)
	return sessionID
}

func (h *cliResumeSmokeHarness) startInterruptedSession(t *testing.T) string {
	t.Helper()

	const workflowName = "resumable-two-step-fake-children"
	started, err := h.service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-cli-resume-smoke-start-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: workflowName,
		},
		Args: map[string]any{
			"subject": "workflows",
		},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	sessionID := started.SessionID
	if sessionID == "" {
		t.Fatal("session id unexpectedly empty")
	}

	waitForCLIResumeSmokeDispatchStatus(
		t,
		h.service,
		sessionID,
		"dispatch-1",
		fse.DispatchStatusCompleted,
		5*time.Second,
	)
	waitForCLIResumeSmokeDispatchStatus(
		t,
		h.service,
		sessionID,
		"dispatch-2",
		fse.DispatchStatusRunning,
		5*time.Second,
	)

	_, err = h.service.InterruptDispatch(context.Background(), sessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "cli resume smoke interrupt"},
		DispatchID:     "dispatch-2",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	h.provider.waitForCanceledInfer(t, 5*time.Second)
	waitForCLIResumeSmokeSessionStatus(t, h.service, sessionID, fse.LifecycleStatusInterrupted, 5*time.Second)
	return sessionID
}

func readDurableSessionViaCLI(
	t *testing.T,
	serverURL string,
	sessionID string,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	var out bytes.Buffer
	if err := sessioncli.Show(sessioncli.ShowConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &out,
	}); err != nil {
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
	serverURL string,
	sessionID string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	var out bytes.Buffer
	if err := sessioncli.Resume(sessioncli.LifecycleControlConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &out,
	}); err != nil {
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
	serverURL string,
	sessionID string,
) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	t.Helper()

	var out bytes.Buffer
	err := sessioncli.Resume(sessioncli.LifecycleControlConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &out,
	})

	var response factoryapi.FactorySessionLifecycleControlResponse
	if decodeErr := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &response); decodeErr != nil {
		t.Fatalf("decode session resume JSON: %v\n%s", decodeErr, out.String())
	}
	return response, err
}

func waitForDurableSessionStatusViaCLI(
	t *testing.T,
	serverURL string,
	sessionID string,
	want factoryapi.FactorySessionDurableLifecycleStatus,
	timeout time.Duration,
) factoryapi.FactorySessionDurableReadModel {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session := readDurableSessionViaCLI(t, serverURL, sessionID)
		if session.Status == want {
			return session
		}
		time.Sleep(15 * time.Millisecond)
	}
	session := readDurableSessionViaCLI(t, serverURL, sessionID)
	t.Fatalf("session %s status = %q, want %q within %s", sessionID, session.Status, want, timeout)
	return session
}

func setupCLIResumeSmokeWorkflowFixture(t *testing.T, fixtureName, workflowName string) string {
	t.Helper()

	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "orchestrators", "javascript", "runtime", "testdata", fixtureName))
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
	svc fse.Service,
	sessionID string,
	dispatchID string,
	want fse.DispatchStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		listed, err := svc.ListDispatches(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("ListDispatches: %v", err)
		}
		for _, dispatch := range listed.Dispatches {
			if dispatch.ID != dispatchID {
				continue
			}
			if dispatch.Status == want {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("dispatch %s did not reach %s within %s", dispatchID, want, timeout)
}

func waitForCLIResumeSmokeSessionStatus(
	t *testing.T,
	svc fse.Service,
	sessionID string,
	want fse.LifecycleStatus,
	timeout time.Duration,
) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		read, err := svc.GetSession(context.Background(), sessionID)
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if read.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	read, err := svc.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	t.Fatalf("session status = %q, want %q within %s", read.Status, want, timeout)
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
