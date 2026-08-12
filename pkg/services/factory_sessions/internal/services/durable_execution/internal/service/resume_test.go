package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/checkpointfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

const resumeOwnerSessionID = "dur-sess-resume-owner-aaaaaaaaaaaaaaaaaaaaaaaa"

func TestDurableResumeInterruptedSessionReturnsPublishedSuccessShape(t *testing.T) {
	t.Parallel()

	store := newRestartMemoryStore()
	seedInterruptedSessionInStore(t, store, resumeOwnerSessionID, true)

	owner, err := New(newResumeBackedExecution(t, store))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resumed, err := owner.ResumeInterruptedSession(
		context.Background(),
		resumeOwnerSessionID,
		factorysessions.DurableResumeRequest{RequestID: "req-resume-owner-success-001"},
	)
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}
	if resumed.SessionID != resumeOwnerSessionID {
		t.Fatalf("sessionId = %q, want %q", resumed.SessionID, resumeOwnerSessionID)
	}
	switch resumed.Status {
	case string(factorysessions.LifecycleStatusResuming):
		final := waitUntilOwnerSessionStatus(
			t,
			owner,
			resumeOwnerSessionID,
			factorysessions.LifecycleStatusSucceeded,
			5*time.Second,
		)
		if final.SessionID != resumeOwnerSessionID {
			t.Fatalf("final sessionId = %q, want %q", final.SessionID, resumeOwnerSessionID)
		}
	case string(factorysessions.LifecycleStatusSucceeded):
		// Under full-suite parallel load the resume goroutine can finish before
		// the published async-start snapshot is observed as RESUMING.
	default:
		t.Fatalf("status = %q, want RESUMING or SUCCEEDED", resumed.Status)
	}
}

func TestDurableResumeMissingCheckpointReturnsTypedFailure(t *testing.T) {
	t.Parallel()

	store := newRestartMemoryStore()
	seedInterruptedSessionInStore(t, store, resumeOwnerSessionID, false)

	owner, err := New(newResumeBackedExecution(t, store))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()

	before, err := owner.GetSession(ctx, resumeOwnerSessionID)
	if err != nil {
		t.Fatalf("GetSession before resume: %v", err)
	}
	if before.Status != factorysessions.LifecycleStatusInterrupted {
		t.Fatalf("status before resume = %q, want INTERRUPTED", before.Status)
	}

	_, err = owner.ResumeInterruptedSession(
		ctx,
		resumeOwnerSessionID,
		factorysessions.DurableResumeRequest{RequestID: "req-resume-owner-missing-checkpoint-001"},
	)
	var missingCheckpoint *factorysessions.DurableResumeError
	if !errors.As(err, &missingCheckpoint) {
		t.Fatalf("ResumeInterruptedSession = %v, want *DurableResumeError", err)
	}
	if missingCheckpoint.Outcome != factorysessions.DurableResumeOutcomeMissingCheckpoint {
		t.Fatalf("outcome = %q, want MISSING_CHECKPOINT", missingCheckpoint.Outcome)
	}
	if errors.Is(err, factorysessions.ErrDurableSessionNotFound) {
		t.Fatal("missing checkpoint must stay distinct from ErrDurableSessionNotFound")
	}

	after, err := owner.GetSession(ctx, resumeOwnerSessionID)
	if err != nil {
		t.Fatalf("GetSession after resume: %v", err)
	}
	if after.Status != factorysessions.LifecycleStatusInterrupted {
		t.Fatalf("status after failed resume = %q, want INTERRUPTED", after.Status)
	}
	if after.SessionID != before.SessionID {
		t.Fatalf("session changed after failed resume: before=%#v after=%#v", before, after)
	}
}

func seedInterruptedSessionInStore(
	t *testing.T,
	store *restartMemoryStore,
	sessionID string,
	withCheckpoint bool,
) {
	t.Helper()

	startedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	startRequest := factorysessions.StartRequest{
		RequestID: "req-resume-owner-seed-001",
		Source: factorysessions.Source{
			Kind:         factoryruntime.WorkflowSourceKindWorkflowName,
			WorkflowName: "resumable-two-step-fake-children",
		},
		Args: map[string]any{"subject": "workflows"},
	}
	session := factorysessions.SessionReadResult{
		SessionID:        sessionID,
		Status:           factorysessions.LifecycleStatusInterrupted,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Dialect:          "you-workflow-v1",
		SourceHash:       "sha256:scripted-resume",
		Lifecycle: &factorysessions.LifecycleTimestamps{
			StartedAt:     &startedAt,
			InterruptedAt: &startedAt,
		},
	}
	result := factorysessions.ResultReadResult{
		SessionID:     sessionID,
		SessionStatus: factorysessions.LifecycleStatusInterrupted,
		ResultStatus:  factorysessions.ResultStatusPartial,
	}
	dispatches := []factorysessions.DispatchSummary{{
		ID: "dispatch-1", Status: factorysessionexecution.DispatchStatusCompleted, Attempt: 1,
	}}

	var checkpointSummary *factoryruntime.JavaScriptCheckpointSummary
	if withCheckpoint {
		checkpointSummary = checkpointfixtures.ResumableCheckpointSummaryResult()
		checkpointSummary.SessionID = sessionID
	}

	snapshot := factorysessionexecution.PersistedRuntimeSessionState{
		Session:    session,
		Result:     result,
		Dispatches: dispatches,
		Events: factorysessionexecution.BuildCanonicalRuntimeSessionEvents(session, result, factorysessionexecution.RuntimeDispatchEventInput{
			Dispatches: dispatches,
			CheckpointEvents: []factorysessionexecution.RuntimeCheckpointEventProjection{{
				CheckpointID: "checkpoint-1", Label: "after-step-one",
				SourceHash: "sha256:scripted-resume", ResumabilityStatus: "RESUMABLE",
				Timestamp: startedAt,
			}},
		}),
		RuntimeRecords: []factoryruntime.JavaScriptRuntimeRecord{
			{
				Sequence: 1, Kind: factoryruntime.JavaScriptRecordKindChildDispatch,
				ChildDispatch: &factoryruntime.JavaScriptChildDispatchRecord{
					DispatchID: "dispatch-1", ChildIndex: 1,
					Status: factoryruntime.JavaScriptChildDispatchStatusCompleted,
					Output: map[string]any{"label": "step-one"},
				},
			},
			{
				Sequence: 2, Kind: factoryruntime.JavaScriptRecordKindCheckpoint,
				Checkpoint: &factoryruntime.JavaScriptCheckpointRecord{
					ID: "checkpoint-1", Label: "after-step-one",
				},
			},
		},
		CheckpointSummary: checkpointSummary,
		StartRequest:      &startRequest,
		ResolvedSource: factorysessions.ResolvedSource{
			Kind:       factoryruntime.WorkflowSourceKindWorkflowName,
			SourceRef:  "resumable-two-step-fake-children.workflow.js",
			SourceHash: "sha256:scripted-resume",
			Dialect:    "you-workflow-v1",
		},
		SourceContent: "scripted resumable source",
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal interrupted snapshot: %v", err)
	}
	if err := store.Save(sessionID, encoded); err != nil {
		t.Fatalf("persist interrupted snapshot: %v", err)
	}
}

func newResumeBackedExecution(
	t *testing.T,
	store *restartMemoryStore,
) durableexecution.Service {
	t.Helper()

	clock := restartClock{now: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}
	workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{
		ResumeContextFunc: func(
			summary factoryruntime.JavaScriptCompletedCheckpointSummary,
			_ []factoryruntime.JavaScriptRuntimeRecord,
		) factoryruntime.JavaScriptResumeContext {
			if len(summary.CompletedDispatchIDs) != 1 {
				t.Fatalf("resume completed dispatch ids = %#v, want one dispatch", summary.CompletedDispatchIDs)
			}
			return factoryruntime.JavaScriptResumeContext{CompletedDispatchIDs: []string{"dispatch-1"}}
		},
		RunFunc: func(
			_ context.Context,
			request factoryruntime.JavaScriptRuntimeRequest,
			_ factoryruntime.JavaScriptRuntimeHooks,
		) (factoryruntime.JavaScriptRuntimeOutcome, error) {
			if request.Resume == nil || len(request.Resume.CompletedDispatchIDs) != 1 {
				t.Fatalf("runtime resume context = %#v", request.Resume)
			}
			encoded, err := json.Marshal(map[string]any{"status": "resumed"})
			if err != nil {
				return factoryruntime.JavaScriptRuntimeOutcome{}, err
			}
			return factoryruntime.JavaScriptRuntimeOutcome{
				OK:    true,
				Value: factoryruntime.TypedValue{JSON: encoded},
				Records: []factoryruntime.JavaScriptRuntimeRecord{{
					Sequence: 3, Kind: factoryruntime.JavaScriptRecordKindChildDispatch,
					ChildDispatch: &factoryruntime.JavaScriptChildDispatchRecord{
						DispatchID: "dispatch-2", ChildIndex: 2,
						Status: factoryruntime.JavaScriptChildDispatchStatusCompleted,
						Output: map[string]any{"label": "step-two"},
					},
				}},
			}, nil
		},
	}
	return factorysessionexecution.NewJavaScriptRuntimeService(
		t.TempDir(),
		factorysessionexecution.ChildExecutorModeFake,
		nil,
		store,
		clock,
		restartSyncWaitScheduler{},
		checkpointfixtures.CheckpointSummariesFixture{
			BuildResult:  checkpointfixtures.ResumableCheckpointSummaryResult(),
			LatestResult: checkpointfixtures.ResumableCheckpointSummaryResult(),
		},
		workflows,
		workflows,
		workflows,
		nil,
		factoryruntime.JavaScriptWorkerSettings{},
		restartRecordingWriter{},
		func() string { return resumeOwnerSessionID },
		nil, nil, nil,
	)
}

func waitUntilOwnerSessionStatus(
	t *testing.T,
	owner *Service,
	sessionID string,
	want factorysessions.LifecycleStatus,
	timeout time.Duration,
) factorysessions.SessionReadResult {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		session, err := owner.GetSession(context.Background(), sessionID)
		if err == nil && session.Status == want {
			return session
		}
		time.Sleep(25 * time.Millisecond)
	}
	session, err := owner.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession while waiting for %s: %v", want, err)
	}
	t.Fatalf("session status = %q, want %q within %s", session.Status, want, timeout)
	return session
}
