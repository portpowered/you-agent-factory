package fixtures_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

func TestJavaScriptRuntimeService_ResumeInterruptedSession_ExposesReadSurfacesAndEventLineage(t *testing.T) {
	harness := startInterruptedResumableSession(t, "req-runtime-resume-reads-001")
	if harness.interrupted.Lifecycle == nil || harness.interrupted.Lifecycle.InterruptedAt == nil {
		t.Fatalf("interrupted lifecycle = %#v, want interruptedAt", harness.interrupted.Lifecycle)
	}

	resumedService := newResumedRuntimeService(harness)
	_, err := resumedService.ResumeInterruptedSession(context.Background(), harness.sessionID, fse.ResumeSessionRequest{
		RequestID: "req-runtime-resume-reads-resume-001",
	})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession: %v", err)
	}

	success := waitUntilSessionStatus(t, resumedService, harness.sessionID, fse.LifecycleStatusSucceeded, 5*time.Second)
	if success.Lifecycle == nil || success.Lifecycle.InterruptedAt == nil || success.Lifecycle.ResumedAt == nil {
		t.Fatalf("resumed lifecycle = %#v, want interruptedAt and resumedAt", success.Lifecycle)
	}
	if success.Lifecycle.FinishedAt == nil {
		t.Fatal("expected finishedAt on succeeded resumed session")
	}
	if success.Progress == nil || success.Progress.CompletedDispatches != 2 {
		t.Fatalf("progress = %#v, want 2 completed dispatches", success.Progress)
	}
	if success.ResultSummary == nil || success.ResultSummary.ResultStatus != string(fse.ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", success.ResultSummary)
	}

	result, err := resumedService.GetResult(context.Background(), harness.sessionID, fse.ResultRequest{
		Mode: fse.ResultModeFinal,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	if result.ResultStatus != fse.ResultStatusFinal || result.SessionStatus != fse.LifecycleStatusSucceeded {
		t.Fatalf("result = %#v, want FINAL/SUCCEEDED", result)
	}

	dispatches, err := resumedService.ListDispatches(context.Background(), harness.sessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	if len(dispatches.Dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want 2", len(dispatches.Dispatches))
	}
	for _, dispatch := range dispatches.Dispatches {
		if dispatch.Status != fse.DispatchStatusCompleted {
			t.Fatalf("dispatch %s = %#v, want COMPLETED", dispatch.ID, dispatch)
		}
	}

	events, err := resumedService.ReadEvents(context.Background(), harness.sessionID, fse.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	assertResumeEventLineage(t, events.Events, harness.sessionID)

	replayedSession, replayedResult, err := fse.ReplaySessionProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if replayedSession.Status != fse.LifecycleStatusSucceeded {
		t.Fatalf("replayed status = %q, want SUCCEEDED", replayedSession.Status)
	}
	if replayedResult.ResultStatus != fse.ResultStatusFinal {
		t.Fatalf("replayed result = %q, want FINAL", replayedResult.ResultStatus)
	}
	if replayedSession.Lifecycle == nil || replayedSession.Lifecycle.ResumedAt == nil {
		t.Fatalf("replayed lifecycle = %#v, want resumedAt", replayedSession.Lifecycle)
	}

	reconnect, err := resumedService.ReadEvents(context.Background(), harness.sessionID, reconnectAfterFirstEvent(t, events.Events))
	if err != nil {
		t.Fatalf("ReadEvents reconnect: %v", err)
	}
	if len(reconnect.Events) == 0 {
		t.Fatal("expected reconnect-filtered events after first event id")
	}
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
	foundCheckpoint := false
	foundResumed := false
	foundCompleted := false
	for _, raw := range events {
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
			continue
		}
		switch envelope.Type {
		case "ORCHESTRATOR_CHECKPOINT_WRITTEN":
			foundCheckpoint = true
			if envelope.Context.CheckpointID == nil || strings.TrimSpace(*envelope.Context.CheckpointID) == "" {
				t.Fatalf("checkpoint event missing checkpointId: %s", string(raw))
			}
		case "SESSION_RESUMED":
			foundResumed = true
			var payload struct {
				Status string `json:"status"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatalf("unmarshal SESSION_RESUMED payload: %v", err)
			}
			if payload.Status != string(fse.LifecycleStatusResuming) {
				t.Fatalf("SESSION_RESUMED status = %q, want RESUMING", payload.Status)
			}
		case "SESSION_COMPLETED":
			foundCompleted = true
			var payload struct {
				FinalStatus string `json:"finalStatus"`
			}
			if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
				t.Fatalf("unmarshal SESSION_COMPLETED payload: %v", err)
			}
			if payload.FinalStatus != string(fse.LifecycleStatusSucceeded) {
				t.Fatalf("SESSION_COMPLETED finalStatus = %q, want SUCCEEDED", payload.FinalStatus)
			}
		}
	}
	if !foundCheckpoint {
		t.Fatal("ORCHESTRATOR_CHECKPOINT_WRITTEN event missing from resumed session history")
	}
	if !foundResumed {
		t.Fatal("SESSION_RESUMED event missing from resumed session history")
	}
	if !foundCompleted {
		t.Fatal("SESSION_COMPLETED event missing from resumed session history")
	}
}
