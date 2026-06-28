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
