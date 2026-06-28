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
