package factorysessionexecution

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

func TestJavaScriptRuntimeService_InterruptAcceptedBeforeChildCompletion_RecordsObservedCancellation(t *testing.T) {
	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-race-runtime-001"
	dispatchID := "dispatch-1"
	if err := SeedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop before provider completion"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}
	if interruptResult.DispatchID != dispatchID {
		t.Fatalf("dispatchId = %q, want %q", interruptResult.DispatchID, dispatchID)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "operator stop before provider completion" {
		t.Fatalf("failureDetail = %#v, want operator stop before provider completion", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, dispatchID)
	if payload.Reason != "operator stop before provider completion" {
		t.Fatalf("event reason = %q, want operator stop before provider completion", payload.Reason)
	}
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}

	replayed, err := ReplayDispatchProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 || replayed[0].Status != DispatchStatusInterrupted {
		t.Fatalf("replayed dispatches = %#v, want one interrupted dispatch", replayed)
	}
}

func TestJavaScriptRuntimeService_LateChildResultAfterInterrupt_SuppressesNormalRouting(t *testing.T) {
	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-late-runtime-001"
	dispatchID := "dispatch-1"
	if err := SeedRuntimeSessionWithRunningDispatch(service, sessionID, dispatchID, "summarize-findings"); err != nil {
		t.Fatalf("SeedRuntimeSessionWithRunningDispatch: %v", err)
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "operator stop"},
		DispatchID:     dispatchID,
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	lateRecords := []workflowruntime.RuntimeRecord{{
		Kind: workflowruntime.RecordKindChildDispatch,
		ChildDispatch: &workflowruntime.ChildDispatchRecord{
			DispatchID:         dispatchID,
			Status:             workflowruntime.ChildDispatchStatusCompleted,
			Label:              "summarize-findings",
			ArtifactRef:        workflowresult.FormatArtifactURI(sessionID, "child-artifact-late"),
			ProviderSessionRef: "provider-session-late",
			Provider:           "mock",
		},
	}}
	if err := ApplyRuntimeTerminalOutcomeForTests(service, sessionID, workflowruntime.Outcome{
		OK:      true,
		Records: lateRecords,
		Value:   workflowresult.TypedValue{JSON: json.RawMessage(`{"label":"agent-run-fake-child"}`)},
	}); err != nil {
		t.Fatalf("ApplyRuntimeTerminalOutcomeForTests: %v", err)
	}

	dispatch, err := service.GetDispatch(context.Background(), sessionID, dispatchID)
	if err != nil {
		t.Fatalf("GetDispatch after late completion: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED after late completion", dispatch.Status)
	}
	if len(dispatch.OutputArtifactIDs) != 0 {
		t.Fatalf("outputArtifactIds = %#v, want suppressed late child output", dispatch.OutputArtifactIDs)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "provider-session-late" {
		t.Fatalf("providerSessionRefs = %#v, want late diagnostic preserved", dispatch.ProviderSessionRefs)
	}

	session, err := service.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("GetSession after late completion: %v", err)
	}
	if session.Progress != nil && session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after suppression", session.Progress.CompletedDispatches)
	}

	artifacts, err := service.ListArtifacts(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListArtifacts: %v", err)
	}
	for _, artifact := range artifacts.Artifacts {
		if artifact.DispatchID == dispatchID && artifact.Kind == "CHILD_OUTPUT" {
			t.Fatalf("artifact = %#v, want late child output suppressed", artifact)
		}
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if !containsEventType(events.Events, "DISPATCH_INTERRUPTED") {
		t.Fatal("DISPATCH_INTERRUPTED event missing after late completion merge")
	}
}

func TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes(t *testing.T) {
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-run-n-001")

	dispatches, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches before interrupt: %v", err)
	}
	if len(dispatches.Dispatches) < 2 {
		t.Fatalf("dispatches = %#v, want at least two fixture dispatches", dispatches.Dispatches)
	}
	runningBefore := findDispatchByID(dispatches.Dispatches, "disp-js-002")
	if runningBefore == nil || runningBefore.Status != DispatchStatusRunning {
		t.Fatalf("dispatch disp-js-002 = %#v, want RUNNING before interrupt", runningBefore)
	}

	interruptResult, err := service.InterruptDispatch(context.Background(), started.SessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "stop before provider completion"},
		DispatchID:     "disp-js-002",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if interruptResult.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", interruptResult.Outcome)
	}

	dispatch, err := service.GetDispatch(context.Background(), started.SessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}

	events, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "disp-js-002")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "stop before provider completion" {
		t.Fatalf("reason = %q, want stop before provider completion", payload.Reason)
	}

	session, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	updated, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches after interrupt: %v", err)
	}
	interrupted := findDispatchByID(updated.Dispatches, "disp-js-002")
	if interrupted == nil || interrupted.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch disp-js-002 after interrupt = %#v, want INTERRUPTED", interrupted)
	}
	if err := ValidateDispatchListMatchesSessionProgress(session, updated.Dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}
}

func findDispatchInterruptedEventPayload(t *testing.T, events []json.RawMessage, dispatchID string) dispatchInterruptedEventPayload {
	t.Helper()
	for _, raw := range events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != dispatchID {
			continue
		}
		var payload dispatchInterruptedEventPayload
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			t.Fatalf("unmarshal DISPATCH_INTERRUPTED payload: %v", err)
		}
		return payload
	}
	t.Fatalf("DISPATCH_INTERRUPTED event for %s not found in %#v", dispatchID, events)
	return dispatchInterruptedEventPayload{}
}

func containsEventType(events []json.RawMessage, eventType string) bool {
	for _, raw := range events {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == eventType {
			return true
		}
	}
	return false
}

func findDispatchByID(dispatches []DispatchSummary, dispatchID string) *DispatchSummary {
	for index := range dispatches {
		if dispatches[index].ID == dispatchID {
			return &dispatches[index]
		}
	}
	return nil
}

func TestJavaScriptRuntimeService_InterruptRunningDispatch_PreservesObservedCancellationAtRecordTime(t *testing.T) {
	service := NewJavaScriptRuntimeService(JavaScriptRuntimeServiceConfig{ProjectRoot: t.TempDir()})
	sessionID := "dur-sess-interrupt-observed-status-001"
	now := time.Date(2026, 6, 20, 18, 0, 0, 0, time.UTC)
	service.sessions[sessionID] = &runtimeSessionState{
		session: SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: interfaces.OrchestratorKindJavaScript,
			Phase:            "execute",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
			Links:            InspectionLinksForSession(sessionID, true),
		},
		result: ResultReadResult{
			SessionID:     sessionID,
			SessionStatus: LifecycleStatusRunning,
			ResultStatus:  ResultStatusNotReady,
		},
		dispatches: []DispatchSummary{{
			ID:     "dispatch-1",
			Status: DispatchStatusRunning,
			Phase:  "execute",
			Label:  "summarize-findings",
		}},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"dispatch-1": {DispatchStatusQueued, DispatchStatusRunning},
		},
		events: BuildCanonicalRuntimeSessionEvents(
			SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning, OrchestratorKind: interfaces.OrchestratorKindJavaScript},
			ResultReadResult{SessionID: sessionID, SessionStatus: LifecycleStatusRunning, ResultStatus: ResultStatusNotReady},
		),
	}

	_, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "cancellation observed while running"},
		DispatchID:     "dispatch-1",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}

	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	payload := findDispatchInterruptedEventPayload(t, events.Events, "dispatch-1")
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.Reason != "cancellation observed while running" {
		t.Fatalf("reason = %q, want cancellation observed while running", payload.Reason)
	}
}
