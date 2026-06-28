package fixtures_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/runtimepersist"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

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
