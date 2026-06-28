package fixtures_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_ResumeInterruptedSession_ReconstructsFromCheckpointSummary(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-interrupted-001")
	assertInterruptedResumePreconditions(t, harness)

	firstDispatchBeforeResume := getCompletedDispatch(t, harness.initial, harness.sessionID, "dispatch-1")
	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-interrupted-resume-001")
	waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)

	assertProviderCallCount(t, harness.provider, 3)
	assertResumedDispatchParity(t, resumedService, harness.sessionID, firstDispatchBeforeResume)
	assertFinalResult(t, resumedService, harness.sessionID)
}

func assertInterruptedResumePreconditions(t *testing.T, harness interruptedResumableHarness) {
	t.Helper()
	if harness.provider.CallCount() < 2 {
		t.Fatalf("provider infer calls = %d, want at least 2 before interrupt", harness.provider.CallCount())
	}
	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(harness.projectRoot), harness.sessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("interrupted snapshot must be durable before cross-instance resume: %v", err)
	}
	getCompletedDispatch(t, harness.initial, harness.sessionID, "dispatch-1")
}

func getCompletedDispatch(t *testing.T, service fse.Service, sessionID, dispatchID string) fse.DispatchDetail {
	t.Helper()
	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch %s: %v", dispatchID, err)
	}
	if dispatch.Status != fse.DispatchStatusCompleted {
		t.Fatalf("dispatch %s = %#v, want COMPLETED", dispatchID, dispatch)
	}
	return dispatch
}

func assertProviderCallCount(t *testing.T, provider *sequentialBlockingProvider, want int) {
	t.Helper()
	if provider.CallCount() != want {
		t.Fatalf("provider infer calls = %d, want %d", provider.CallCount(), want)
	}
}

func assertResumedDispatchParity(
	t *testing.T,
	service *fse.JavaScriptRuntimeService,
	sessionID string,
	firstDispatchBeforeResume fse.DispatchDetail,
) {
	t.Helper()
	firstDispatchAfterResume := getCompletedDispatch(t, service, sessionID, "dispatch-1")
	if firstDispatchAfterResume.ID != firstDispatchBeforeResume.ID {
		t.Fatalf("dispatch-1 id changed across resume: %q -> %q", firstDispatchBeforeResume.ID, firstDispatchAfterResume.ID)
	}
	getCompletedDispatch(t, service, sessionID, "dispatch-2")
}

func assertFinalResult(t *testing.T, service *fse.JavaScriptRuntimeService, sessionID string) {
	t.Helper()
	result, err := service.GetResult(context.Background(), sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("result status = %q, want FINAL", result.ResultStatus)
	}
}
func TestJavaScriptRuntimeService_ResumeInterruptedSession_ExposesReadSurfacesAndEventLineage(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-reads-001")
	assertInterruptedLifecycleHasTimestamp(t, harness.interrupted.Lifecycle)

	resumedService := resumeInterruptedHarness(t, harness, "req-runtime-resume-reads-resume-001")
	success := waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)
	assertResumedSessionReadSurfaces(t, success)
	assertResumedResultAndDispatches(t, resumedService, harness.sessionID)

	events, err := resumedService.ReadEvents(context.Background(), harness.sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	assertResumeEventLineage(t, events.Events, harness.sessionID)
	assertResumedReplayProjection(t, events.Events)
	assertResumedReconnectEvents(t, resumedService, harness.sessionID, events.Events)
}

func reconnectAfterFirstEvent(t *testing.T, events []json.RawMessage) fse.EventReconnectRequest {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("expected at least one event for reconnect cursor")
	}
	var envelope struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(events[0], &envelope); err != nil {
		t.Fatalf("parse first event: %v", err)
	}
	return fse.EventReconnectRequest{AfterEventID: envelope.ID}
}

func assertResumeEventLineage(t *testing.T, events []json.RawMessage, sessionID string) {
	t.Helper()
	flags := resumeEventLineageFlags{}
	for _, raw := range events {
		applyResumeEventLineage(t, raw, sessionID, &flags)
	}
	if !flags.checkpoint {
		t.Fatal("ORCHESTRATOR_CHECKPOINT_WRITTEN event missing from resumed session history")
	}
	if !flags.resumed {
		t.Fatal("SESSION_RESUMED event missing from resumed session history")
	}
	if !flags.completed {
		t.Fatal("SESSION_COMPLETED event missing from resumed session history")
	}
}

type resumeEventLineageFlags struct {
	checkpoint bool
	resumed    bool
	completed  bool
}

func applyResumeEventLineage(t *testing.T, raw json.RawMessage, sessionID string, flags *resumeEventLineageFlags) {
	t.Helper()
	var envelope struct {
		Type    string `json:"type"`
		Context struct {
			SessionID    *string `json:"sessionId"`
			CheckpointID *string `json:"checkpointId"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if envelope.Context.SessionID == nil || *envelope.Context.SessionID != sessionID {
		return
	}
	switch envelope.Type {
	case "ORCHESTRATOR_CHECKPOINT_WRITTEN":
		flags.checkpoint = true
		if envelope.Context.CheckpointID == nil || strings.TrimSpace(*envelope.Context.CheckpointID) == "" {
			t.Fatalf("checkpoint event missing checkpointId: %s", string(raw))
		}
	case "SESSION_RESUMED":
		flags.resumed = true
		assertSessionResumedEventPayload(t, envelope.Payload)
	case "SESSION_COMPLETED":
		flags.completed = true
		assertSessionCompletedEventPayload(t, envelope.Payload)
	}
}

func assertSessionResumedEventPayload(t *testing.T, payload json.RawMessage) {
	t.Helper()
	var decoded struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal SESSION_RESUMED payload: %v", err)
	}
	if decoded.Status != string(fse.LifecycleStatusResuming) {
		t.Fatalf("SESSION_RESUMED status = %q, want RESUMING", decoded.Status)
	}
}

func assertSessionCompletedEventPayload(t *testing.T, payload json.RawMessage) {
	t.Helper()
	var decoded struct {
		FinalStatus string `json:"finalStatus"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal SESSION_COMPLETED payload: %v", err)
	}
	if decoded.FinalStatus != string(fse.LifecycleStatusSucceeded) {
		t.Fatalf("SESSION_COMPLETED finalStatus = %q, want SUCCEEDED", decoded.FinalStatus)
	}
}
func TestJavaScriptRuntimeService_ResumeInterruptedSession_MissingCheckpointReturnsTypedFailure(t *testing.T) {
	sessionID, projectRoot := seedInterruptedCheckpointedSession(t)
	rewritePersistedSnapshot(t, projectRoot, sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		snapshot.CheckpointSummary = nil
	})

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})

	before, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession before resume: %v", err)
	}
	beforeEvents, err := service.ReadEvents(context.Background(), sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before resume: %v", err)
	}

	_, err = service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-missing-checkpoint-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeMissingCheckpoint, "checkpointSummary")

	after, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after resume: %v", err)
	}
	if after.Status != fse.LifecycleStatusInterrupted {
		t.Fatalf("status after failed resume = %q, want INTERRUPTED", after.Status)
	}
	if after.SessionID != before.SessionID || after.Phase != before.Phase {
		t.Fatalf("session read changed after failed resume: before=%#v after=%#v", before, after)
	}

	afterEvents, err := service.ReadEvents(context.Background(), sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after resume: %v", err)
	}
	if len(afterEvents.Events) != len(beforeEvents.Events) {
		t.Fatalf("event count changed after failed resume: before=%d after=%d", len(beforeEvents.Events), len(afterEvents.Events))
	}
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_CorruptedPersistenceReturnsTypedFailure(t *testing.T) {
	sessionID, projectRoot := seedInterruptedCheckpointedSession(t)
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupted snapshot: %v", err)
	}

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})
	_, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-corrupted-persistence-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeCorruptedPersistence, "")
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_InvalidCheckpointSummaryReturnsTypedFailure(t *testing.T) {
	sessionID, projectRoot := seedInterruptedCheckpointedSession(t)
	rewritePersistedSnapshot(t, projectRoot, sessionID, func(snapshot *fse.PersistedRuntimeSessionState) {
		if snapshot.CheckpointSummary != nil {
			snapshot.CheckpointSummary.Kind = "invalid-checkpoint-kind"
		}
	})

	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})
	_, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-invalid-checkpoint-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "checkpointSummary.kind")
}

func TestJavaScriptRuntimeService_ResumeInterruptedSession_NonInterruptedSessionReturnsTypedFailure(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(
		t,
		"simple-final.workflow.js",
		"simple-final",
	)
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})
	started, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-resume-non-interrupted-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	_, err = service.ResumeInterruptedSession(context.Background(), started.SessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-non-interrupted-resume-001",
	})
	assertResumeError(t, err, fse.ResumeOutcomeInvalidState, "")
}

func seedInterruptedCheckpointedSession(t *testing.T) (string, string) {
	t.Helper()
	provider := newSequentialBlockingProvider()
	projectRoot := setupRuntimeWorkflowFixture(
		t,
		"resumable-two-step-fake-children.workflow.js",
		"resumable-two-step-fake-children",
	)
	initial := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:       projectRoot,
		ChildExecutorMode: fse.ChildExecutorModeLive,
		Provider:          provider,
		PersistSessions:   true,
	})
	started, err := initial.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-resume-failure-seed-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{"subject": "workflows"},
		Runtime: &fse.RuntimeOptions{
			ChildExecutorMode: fse.ChildExecutorModeLive,
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-1", fse.DispatchStatusCompleted, 3*time.Second)
	waitForDispatchStatus(t, initial, started.SessionID, "dispatch-2", fse.DispatchStatusRunning, 3*time.Second)
	if _, err := initial.InterruptDispatch(context.Background(), started.SessionID, fse.InterruptDispatchRequest{
		ControlRequest: fse.ControlRequest{Reason: "resume failure seed"},
		DispatchID:     "dispatch-2",
	}); err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	provider.waitForCanceledInfer(t, 3*time.Second)
	interrupted := waitUntilSessionStatus(t, initial, started.SessionID, fse.LifecycleStatusInterrupted, 5*time.Second)
	if interrupted.Status != fse.LifecycleStatusInterrupted {
		t.Fatalf("seed status = %q, want INTERRUPTED", interrupted.Status)
	}
	waitForPersistedInterruptedSnapshot(t, projectRoot, started.SessionID)

	snapshotPath := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), started.SessionID+".json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("persisted snapshot missing at %s: %v", snapshotPath, err)
	}
	raw, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatalf("read persisted snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal persisted snapshot: %v", err)
	}
	if snapshot.CheckpointSummary == nil {
		t.Fatal("seed snapshot missing checkpoint summary")
	}
	return started.SessionID, projectRoot
}

func rewritePersistedSnapshot(
	t *testing.T,
	projectRoot, sessionID string,
	mutate func(*fse.PersistedRuntimeSessionState),
) {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	mutate(&snapshot)
	encoded, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}

func assertResumeError(t *testing.T, err error, wantOutcome fse.ResumeOutcome, wantField string) {
	t.Helper()
	var resumeErr *fse.ResumeError
	if !errors.As(err, &resumeErr) {
		t.Fatalf("error = %T %v, want *fse.ResumeError", err, err)
	}
	if resumeErr.Outcome != wantOutcome {
		t.Fatalf("outcome = %q, want %q", resumeErr.Outcome, wantOutcome)
	}
	if wantField != "" && resumeErr.Field != wantField {
		t.Fatalf("field = %q, want %q", resumeErr.Field, wantField)
	}
}

func waitForPersistedInterruptedSnapshot(t *testing.T, projectRoot, sessionID string) {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var snapshot fse.PersistedRuntimeSessionState
			if json.Unmarshal(raw, &snapshot) == nil && snapshot.CheckpointSummary != nil {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("persisted interrupted snapshot not ready at %s", path)
}
func TestJavaScriptRuntimeService_NonResumedFakeChild_PreservesShippedTransportSemantics(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-non-resumed-fake-child-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	read, err := service.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	wantDispatch := assertAgentRunFakeChildSessionRead(t, read)
	assertNonResumedLifecycleLineage(t, read)
	assertAgentRunFakeChildDispatch(t, service, completed.SessionID, wantDispatch)
	assertAgentRunFakeChildArtifact(t, service, completed.SessionID)

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, completed.SessionID)
	assertRuntimeEventSource(t, events.Events)
	assertNonResumeRestartEventLineage(t, events.Events, completed.SessionID)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)

	reconnect, err := service.ReadEvents(context.Background(), completed.SessionID, reconnectAfterFirstEvent(t, events.Events))
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(reconnect.Events) == 0 {
		t.Fatal("expected reconnect-filtered events after first event id")
	}

	_, err = service.Pause(context.Background(), completed.SessionID, fse.ControlRequest{})
	var controlErr *fse.ControlError
	if !errors.As(err, &controlErr) || controlErr.Outcome != fse.LifecycleControlOutcomeTerminalSession {
		t.Fatalf("pause on terminal = %v, want TERMINAL_SESSION ControlError", err)
	}

	assertRuntimeInspectionExcludesForbiddenVocabulary(t, liveSession, liveResult, events.Events)
}

func TestJavaScriptRuntimeService_NonResumedSimpleFinal_PreservesReplayReconnectAndTerminalResult(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})

	completed, err := service.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-non-resumed-simple-final-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	liveSession, liveResult, events := readRuntimeSessionEvents(t, service, completed.SessionID)
	assertNonResumedLifecycleLineage(t, liveSession)
	assertNonResumeRestartEventLineage(t, events.Events, completed.SessionID)

	dispatches, err := service.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("dispatches = %#v, want empty for simple-final", dispatches.Dispatches)
	}

	artifacts, err := service.ListArtifacts(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	if len(artifacts.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want empty for simple-final", artifacts.Artifacts)
	}

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	assertReplayedSessionMatchesLive(t, liveSession, replayedSession)
	assertReplayedResultStatusMatchesLive(t, liveResult, replayedResult)
	if replayedResult.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("replayed resultStatus = %q, want FINAL", replayedResult.ResultStatus)
	}

	reconnect, err := service.ReadEvents(context.Background(), completed.SessionID, reconnectAfterFirstEvent(t, events.Events))
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(reconnect.Events) == 0 {
		t.Fatal("expected reconnect-filtered events after first event id")
	}

	assertRuntimeInspectionExcludesForbiddenVocabulary(t, liveSession, liveResult, events.Events)
}

func TestJavaScriptRuntimeService_NonResumedTerminalSnapshot_OmitsCheckpointSummaryAndReloadsAcrossFreshServices(t *testing.T) {
	projectRoot := setupRuntimeWorkflowFixture(t, "agent-run-fake-child.workflow.js", "agent-run-fake-child")
	initial := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})

	completed, err := initial.StartSync(context.Background(), fse.StartRequest{
		RequestID: "req-runtime-non-resumed-persisted-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "agent-run-fake-child",
		},
		Args: map[string]any{
			"subject": "workflows",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	waitForPersistedTerminalSnapshot(t, projectRoot, completed.SessionID)
	snapshot := readPersistedRuntimeSnapshot(t, projectRoot, completed.SessionID)
	if snapshot.CheckpointSummary != nil {
		t.Fatalf("checkpointSummary = %#v, want nil for non-interrupted terminal session", snapshot.CheckpointSummary)
	}
	if snapshot.Session.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("persisted status = %q, want SUCCEEDED", snapshot.Session.Status)
	}

	reloaded := fse.NewJavaScriptRuntimeService(fse.JavaScriptRuntimeServiceConfig{
		ProjectRoot:     projectRoot,
		PersistSessions: true,
	})
	read, err := reloaded.GetSession(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("GetSession after reload: %v", err)
	}
	if read.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("reloaded status = %q, want SUCCEEDED", read.Status)
	}
	assertNonResumedLifecycleLineage(t, read)

	result, err := reloaded.GetResult(context.Background(), completed.SessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult after reload: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("reloaded resultStatus = %q, want FINAL", result.ResultStatus)
	}

	dispatches, err := reloaded.ListDispatches(context.Background(), completed.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches after reload: %v", err)
	}
	if len(dispatches.Dispatches) != 1 {
		t.Fatalf("reloaded dispatches = %#v, want one fake child", dispatches.Dispatches)
	}
}

func TestNewExecutionService_FakeProvider_PublishedScenarios_RemainAdditiveAfterRestartResumeLane(t *testing.T) {
	service := newFakeExecutionServiceFromContractFixtures(t)

	successRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeSyncSuccess)
	terminal, err := service.StartSync(context.Background(), startRequestForPublished(successRow))
	if err != nil {
		t.Fatalf("StartSync success: %v", err)
	}
	terminalHash, err := fixtures.SyncStartResultHash(terminal)
	if err != nil {
		t.Fatalf("SyncStartResultHash: %v", err)
	}
	if terminalHash != "sha256:89b3a278be3192017c6fcd9fbd4ca57154fb84ab6154ce961e4a597ba5fa6c05" {
		t.Fatalf("sync success hash = %q, want published fixture digest", terminalHash)
	}

	runningRow := publishedScenarioByPurpose(t, fixtures.FixturePurposeAsyncRunning)
	if _, err := service.StartAsync(context.Background(), startRequestForPublished(runningRow)); err != nil {
		t.Fatalf("StartAsync running: %v", err)
	}
	session, err := service.GetSession(context.Background(), runningRow.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if session.Status != fse.LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", session.Status)
	}
	if session.Lifecycle != nil && (session.Lifecycle.InterruptedAt != nil || session.Lifecycle.ResumedAt != nil) {
		t.Fatalf("running lifecycle = %#v, want no restart-resume lineage", session.Lifecycle)
	}
}

func assertNonResumedLifecycleLineage(t *testing.T, read fse.SessionReadResult) {
	t.Helper()
	if read.Lifecycle == nil {
		return
	}
	if read.Lifecycle.InterruptedAt != nil {
		t.Fatalf("interruptedAt = %v, want nil for non-resumed session", read.Lifecycle.InterruptedAt)
	}
	if read.Lifecycle.ResumedAt != nil {
		t.Fatalf("resumedAt = %v, want nil for non-resumed session", read.Lifecycle.ResumedAt)
	}
}

func assertNonResumeRestartEventLineage(t *testing.T, events []json.RawMessage, sessionID string) {
	t.Helper()
	for index, raw := range events {
		var envelope struct {
			Type    string `json:"type"`
			Context struct {
				SessionID *string `json:"sessionId"`
			} `json:"context"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event[%d]: %v", index, err)
		}
		if envelope.Context.SessionID == nil || *envelope.Context.SessionID != sessionID {
			continue
		}
		switch envelope.Type {
		case "ORCHESTRATOR_CHECKPOINT_WRITTEN":
			t.Fatalf("event[%d] = ORCHESTRATOR_CHECKPOINT_WRITTEN, want absent for non-resumed session", index)
		case "SESSION_RESUMED":
			var payload struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatalf("unmarshal SESSION_RESUMED payload: %v", err)
			}
			if payload.Status == string(fse.LifecycleStatusResuming) {
				t.Fatalf("event[%d] = SESSION_RESUMED with RESUMING, want absent for non-resumed session", index)
			}
		}
	}
}

func assertRuntimeInspectionExcludesForbiddenVocabulary(
	t *testing.T,
	session fse.SessionReadResult,
	result fse.ResultReadResult,
	events []json.RawMessage,
) {
	t.Helper()
	encoded, err := json.Marshal(struct {
		Session fse.SessionReadResult `json:"session"`
		Result  fse.ResultReadResult  `json:"result"`
		Events  []json.RawMessage     `json:"events"`
	}{
		Session: session,
		Result:  result,
		Events:  events,
	})
	if err != nil {
		t.Fatalf("marshal inspection snapshot: %v", err)
	}
	responseText := string(encoded)
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(responseText, term) {
			t.Fatalf("inspection response contained forbidden vocabulary %q:\n%s", term, responseText)
		}
	}
	if strings.Contains(responseText, "DynamicWorkflowRunResume") {
		t.Fatalf("inspection response leaked restart-resume-only resource:\n%s", responseText)
	}
}

func waitForPersistedTerminalSnapshot(t *testing.T, projectRoot, sessionID string) {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			var snapshot fse.PersistedRuntimeSessionState
			if json.Unmarshal(raw, &snapshot) == nil &&
				fse.IsTerminalLifecycleStatus(snapshot.Session.Status) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("persisted terminal snapshot not ready at %s", path)
}

func readPersistedRuntimeSnapshot(t *testing.T, projectRoot, sessionID string) fse.PersistedRuntimeSessionState {
	t.Helper()
	path := filepath.Join(runtimepersist.DirForProjectRoot(projectRoot), sessionID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read persisted snapshot: %v", err)
	}
	var snapshot fse.PersistedRuntimeSessionState
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatalf("unmarshal persisted snapshot: %v", err)
	}
	return snapshot
}
