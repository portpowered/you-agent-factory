package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAppendDispatchInterruptedEvent_RecordsCanonicalMetadata(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	session := SessionReadResult{
		SessionID:        "dur-sess-interrupt-001",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		Dialect:          "you-workflow-v1",
		Phase:            "execute",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
	}
	base := BuildCanonicalRuntimeSessionEvents(session, ResultReadResult{
		SessionID:    session.SessionID,
		ResultStatus: ResultStatusNotReady,
	})
	dispatch := DispatchSummary{
		ID:     "disp-js-002",
		Status: DispatchStatusRunning,
		Phase:  "execute",
		Label:  "audit",
	}
	events := AppendDispatchInterruptedEvent(
		base,
		session,
		dispatch,
		InterruptDispatchRequest{
			ControlRequest: ControlRequest{Reason: "operator stop"},
			DispatchID:     "disp-js-002",
		},
		DispatchStatusRunning,
		canonicalEventSourceRuntimeService,
		startedAt,
	)
	if len(events) != len(base)+1 {
		t.Fatalf("events = %d, want %d", len(events), len(base)+1)
	}

	var envelope canonicalFactoryEvent
	if err := json.Unmarshal(events[len(events)-1], &envelope); err != nil {
		t.Fatalf("unmarshal interrupted event: %v", err)
	}
	if envelope.Type != "DISPATCH_INTERRUPTED" {
		t.Fatalf("type = %q, want DISPATCH_INTERRUPTED", envelope.Type)
	}
	if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != "disp-js-002" {
		t.Fatalf("dispatchId = %#v, want disp-js-002", envelope.Context.DispatchID)
	}

	var payload dispatchInterruptedEventPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Reason != "operator stop" {
		t.Fatalf("reason = %q, want operator stop", payload.Reason)
	}
	if payload.ObservedStatus != string(factoryapi.FactoryDispatchStatusRUNNING) {
		t.Fatalf("observedStatus = %q, want RUNNING", payload.ObservedStatus)
	}
	if payload.RetryPlanned {
		t.Fatal("retryPlanned = true, want false")
	}
}

func TestMarkDispatchInterrupted_UpdatesInspectionProjection(t *testing.T) {
	t.Parallel()
	dispatches := []DispatchSummary{{
		ID:     "disp-js-002",
		Status: DispatchStatusRunning,
	}}
	updated, _ := MarkDispatchInterrupted(
		dispatches,
		map[string][]DispatchStatus{},
		"disp-js-002",
		InterruptDispatchRequest{DispatchID: "disp-js-002"},
	)
	if updated[0].Status != DispatchStatusInterrupted {
		t.Fatalf("status = %q, want INTERRUPTED", updated[0].Status)
	}
	if updated[0].FailureDetail == nil || updated[0].FailureDetail.Reason != dispatchInterruptionFailureReasonCode {
		t.Fatalf("failureDetail = %#v, want DISPATCH_INTERRUPTED reason", updated[0].FailureDetail)
	}
	if updated[0].FailureDetail.Message != defaultDispatchInterruptionReason {
		t.Fatalf("failure message = %q", updated[0].FailureDetail.Message)
	}
}

func TestReplayDispatchProjection_DerivesInterruptedDispatchMetadata(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	session := SessionReadResult{
		SessionID:        "dur-sess-interrupt-002",
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
	}
	events := AppendDispatchInterruptedEvent(
		BuildCanonicalRuntimeSessionEvents(session, ResultReadResult{SessionID: session.SessionID}),
		session,
		DispatchSummary{ID: "disp-js-002", Status: DispatchStatusRunning, Phase: "execute"},
		InterruptDispatchRequest{
			ControlRequest: ControlRequest{Reason: "bad prompt"},
			DispatchID:     "disp-js-002",
		},
		DispatchStatusRunning,
		canonicalEventSourceRuntimeService,
		startedAt,
	)

	replayed, err := ReplayDispatchProjection(events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 {
		t.Fatalf("replayed dispatches = %#v, want one interrupted dispatch", replayed)
	}
	if replayed[0].Status != DispatchStatusInterrupted {
		t.Fatalf("status = %q, want INTERRUPTED", replayed[0].Status)
	}
	if replayed[0].FailureDetail == nil || replayed[0].FailureDetail.Message != "bad prompt" {
		t.Fatalf("failureDetail = %#v, want bad prompt", replayed[0].FailureDetail)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake-service regression keeps interrupt event payload assertions together on one scenario.
func TestFakeService_InterruptDispatch_RecordsDispatchInterruptedEvent(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-run-n-001")

	result, err := service.InterruptDispatch(context.Background(), started.SessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: "stop bad run"},
		DispatchID:     "disp-js-002",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if result.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}

	dispatch, err := service.GetDispatch(context.Background(), started.SessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "stop bad run" {
		t.Fatalf("failureDetail = %#v, want stop bad run", dispatch.FailureDetail)
	}

	events, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	found := false
	for _, raw := range events.Events {
		var envelope canonicalFactoryEvent
		if err := json.Unmarshal(raw, &envelope); err != nil {
			t.Fatalf("unmarshal event: %v", err)
		}
		if envelope.Type != "DISPATCH_INTERRUPTED" {
			continue
		}
		found = true
		if envelope.Context.DispatchID == nil || *envelope.Context.DispatchID != "disp-js-002" {
			t.Fatalf("dispatchId = %#v, want disp-js-002", envelope.Context.DispatchID)
		}
	}
	if !found {
		t.Fatal("DISPATCH_INTERRUPTED event missing from session events")
	}

	replayed, err := ReplayDispatchProjection(events.Events)
	if err != nil {
		t.Fatalf("ReplayDispatchProjection: %v", err)
	}
	if len(replayed) != 1 || replayed[0].Status != DispatchStatusInterrupted {
		t.Fatalf("replayed = %#v, want one interrupted dispatch", replayed)
	}
}

func TestFakeServiceDispatchListAndDetailDefaultToUnconfirmedTogether(t *testing.T) {
	t.Parallel()
	service := newContractFakeService(t)
	started := startAsyncByRequestID(t, service, "req-js-run-n-001")

	list, err := service.ListDispatches(context.Background(), started.SessionID)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	var listed DispatchSummary
	found := false
	for _, dispatch := range list.Dispatches {
		if dispatch.ID == "disp-js-002" {
			listed = dispatch
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("dispatch list = %#v, want disp-js-002", list.Dispatches)
	}
	if listed.ConfirmationState != ConfirmationStateUnconfirmed {
		t.Fatalf("listed dispatch confirmation = %#v, want UNCONFIRMED", listed)
	}

	detail, err := service.GetDispatch(context.Background(), started.SessionID, listed.ID)
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if detail.ConfirmationState != ConfirmationStateUnconfirmed || detail.StateSequence != listed.StateSequence || detail.StateSequenceKnown != listed.StateSequenceKnown {
		t.Fatalf("dispatch detail = %#v, want same unconfirmed cursor as list %#v", detail, listed)
	}
}

func TestRestoreInterruptedDispatchResultSuppression_LateCompletionDoesNotReactivateRouting(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 6, 20, 15, 0, 0, 0, time.UTC)
	interrupted := DispatchSummary{
		ID:     "disp-js-002",
		Status: DispatchStatusInterrupted,
		Phase:  "execute",
		Label:  "audit",
		FailureDetail: &DispatchFailureDetail{
			Reason:  dispatchInterruptionFailureReasonCode,
			Message: "operator stop",
		},
	}
	state := &runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-interrupt-late-001", Phase: "execute"},
		dispatches: []DispatchSummary{
			interrupted,
		},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"disp-js-002": {DispatchStatusQueued, DispatchStatusRunning, DispatchStatusInterrupted},
		},
	}
	preserved := snapshotInterruptedDispatches(state)

	lateRecords := []factory.JavaScriptRuntimeRecord{{
		Kind: factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &factory.JavaScriptChildDispatchRecord{
			DispatchID:         "disp-js-002",
			Status:             factory.JavaScriptChildDispatchStatusCompleted,
			Label:              "audit",
			ArtifactRef:        "artifact://child-artifact-late",
			ProviderSessionRef: "provider-session-late",
			Provider:           "fake",
		},
	}}
	applyRuntimeExecutionRecordProjection(state, "dur-sess-interrupt-late-001", lateRecords, observedAt)
	if state.dispatches[0].Status != DispatchStatusCompleted {
		t.Fatalf("projected status = %q, want COMPLETED before suppression", state.dispatches[0].Status)
	}
	if state.session.Progress == nil || state.session.Progress.CompletedDispatches != 1 {
		t.Fatalf("progress before suppression = %#v, want one completed dispatch", state.session.Progress)
	}

	restoreInterruptedDispatchResultSuppression(state, preserved)

	if state.dispatches[0].Status != DispatchStatusInterrupted {
		t.Fatalf("status after suppression = %q, want INTERRUPTED", state.dispatches[0].Status)
	}
	if len(state.dispatches[0].OutputArtifactIDs) != 0 {
		t.Fatalf("outputArtifactIds = %#v, want suppressed late output", state.dispatches[0].OutputArtifactIDs)
	}
	if len(state.dispatches[0].ProviderSessionRefs) != 1 || state.dispatches[0].ProviderSessionRefs[0].ID != "provider-session-late" {
		t.Fatalf("providerSessionRefs = %#v, want late diagnostic preserved", state.dispatches[0].ProviderSessionRefs)
	}
	if state.session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0 after suppression", state.session.Progress.CompletedDispatches)
	}
	for _, artifact := range state.artifacts {
		if artifact.DispatchID == "disp-js-002" && artifact.Kind == "CHILD_RESULT" {
			t.Fatalf("artifact = %#v, want late child output suppressed", artifact)
		}
	}
	transitions := state.dispatchStatusTransitions["disp-js-002"]
	if len(transitions) != 3 || transitions[2] != DispatchStatusInterrupted {
		t.Fatalf("statusTransitions = %#v, want queued/running/interrupted", transitions)
	}
}

func TestApplyTerminalRuntimeProjection_PreservesInterruptedDispatchAndEvents(t *testing.T) {
	t.Parallel()
	observedAt := time.Date(2026, 6, 20, 15, 30, 0, 0, time.UTC)
	sessionID := "dur-sess-interrupt-terminal-001"
	startedAt := observedAt.Add(-time.Minute)
	running := SessionReadResult{
		SessionID: sessionID,
		Status:    LifecycleStatusRunning,
		Phase:     "execute",
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt},
	}
	events := AppendDispatchInterruptedEvent(
		BuildCanonicalRuntimeSessionEvents(running, ResultReadResult{SessionID: sessionID}),
		running,
		DispatchSummary{ID: "disp-js-002", Status: DispatchStatusRunning, Phase: "execute"},
		InterruptDispatchRequest{DispatchID: "disp-js-002", ControlRequest: ControlRequest{Reason: "operator stop"}},
		DispatchStatusRunning,
		canonicalEventSourceRuntimeService,
		observedAt,
	)
	prior := &runtimeSessionState{
		session: running,
		dispatches: []DispatchSummary{{
			ID:            "disp-js-002",
			Status:        DispatchStatusInterrupted,
			Phase:         "execute",
			FailureDetail: dispatchInterruptionFailureDetail("operator stop"),
		}},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			"disp-js-002": {DispatchStatusQueued, DispatchStatusRunning, DispatchStatusInterrupted},
		},
		events: events,
	}
	lateRecords := []factory.JavaScriptRuntimeRecord{{
		Kind: factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &factory.JavaScriptChildDispatchRecord{
			DispatchID:  "disp-js-002",
			Status:      factory.JavaScriptChildDispatchStatusCompleted,
			Label:       "audit",
			ArtifactRef: "artifact://child-artifact-late",
		},
	}}
	terminal := runtimeSessionState{session: running}
	applyRuntimeSuccessProjection(&terminal, sessionID, factory.JavaScriptRuntimeOutcome{
		OK:      true,
		Records: lateRecords,
		Value:   factory.TypedValue{JSON: json.RawMessage("null")},
	}, observedAt)

	applyTerminalRuntimeProjection(prior, terminal, factory.JavaScriptRuntimeOutcome{
		OK:      true,
		Records: lateRecords,
		Value:   factory.TypedValue{JSON: json.RawMessage("null")},
	})

	if prior.dispatches[0].Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", prior.dispatches[0].Status)
	}
	foundInterruptedEvent := false
	for _, raw := range prior.events {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if envelope.Type == "DISPATCH_INTERRUPTED" {
			foundInterruptedEvent = true
			break
		}
	}
	if !foundInterruptedEvent {
		t.Fatal("DISPATCH_INTERRUPTED event missing after terminal projection")
	}
	if prior.session.Progress != nil && prior.session.Progress.CompletedDispatches != 0 {
		t.Fatalf("completedDispatches = %d, want 0", prior.session.Progress.CompletedDispatches)
	}
	if prior.session.Status != LifecycleStatusInterrupted {
		t.Fatalf("session status = %q, want INTERRUPTED", prior.session.Status)
	}
	if prior.session.ResultSummary == nil || prior.session.ResultSummary.ResultStatus != string(ResultStatusUnavailable) {
		t.Fatalf("resultSummary = %#v, want UNAVAILABLE", prior.session.ResultSummary)
	}
	if prior.result.SessionStatus != LifecycleStatusInterrupted || prior.result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("result = status %q session %q, want UNAVAILABLE/INTERRUPTED", prior.result.ResultStatus, prior.result.SessionStatus)
	}
}

func TestReplaySessionProjection_PauseResumeLifecycleEventsDeriveStatus(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	pausedAt := time.Date(2026, 6, 11, 12, 0, 5, 0, time.UTC)
	resumedAt := time.Date(2026, 6, 11, 12, 0, 10, 0, time.UTC)
	sessionID := "dur-sess-replay-pause-resume-001"

	baseSession := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		SourceHash:       "sha256:fixture",
		Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
		ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
	}
	baseResult := ResultReadResult{
		SessionID:    sessionID,
		ResultStatus: ResultStatusNotReady,
	}

	events := BuildCanonicalRuntimeSessionEvents(baseSession, baseResult)
	events = AppendSessionLifecycleControlEvent(
		events,
		SessionReadResult{SessionID: sessionID, Status: LifecycleStatusPaused, OrchestratorKind: "JAVASCRIPT", Dialect: "you-workflow-v1"},
		LifecycleStatusRunning,
		LifecycleControlPause,
		LifecycleControlOutcomeAccepted,
		pausedAt,
		canonicalEventSourceRuntimeService,
		"",
	)
	events = AppendSessionLifecycleControlEvent(
		events,
		SessionReadResult{SessionID: sessionID, Status: LifecycleStatusRunning, OrchestratorKind: "JAVASCRIPT", Dialect: "you-workflow-v1"},
		LifecycleStatusPaused,
		LifecycleControlResume,
		LifecycleControlOutcomeAccepted,
		resumedAt,
		canonicalEventSourceRuntimeService,
		"",
	)

	session, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.Status != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", session.Status)
	}
	if result.SessionStatus != LifecycleStatusRunning {
		t.Fatalf("result sessionStatus = %q, want RUNNING", result.SessionStatus)
	}
	if session.Lifecycle == nil || session.Lifecycle.PausedAt == nil || !session.Lifecycle.PausedAt.Equal(pausedAt) {
		t.Fatalf("pausedAt = %#v, want %s", session.Lifecycle, pausedAt)
	}
	if session.Lifecycle.ResumedAt == nil || !session.Lifecycle.ResumedAt.Equal(resumedAt) {
		t.Fatalf("resumedAt = %#v, want %s", session.Lifecycle.ResumedAt, resumedAt)
	}

	var lifecycleEnvelope canonicalFactoryEvent
	if err := json.Unmarshal(events[2], &lifecycleEnvelope); err != nil {
		t.Fatalf("unmarshal lifecycle event: %v", err)
	}
	if lifecycleEnvelope.Type != "SESSION_LIFECYCLE_CONTROL" {
		t.Fatalf("event type = %q, want SESSION_LIFECYCLE_CONTROL", lifecycleEnvelope.Type)
	}
}

func TestReplaySessionProjection_LegacyPauseResumeEventsDeriveStatus(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	pausedAt := time.Date(2026, 6, 11, 12, 0, 5, 0, time.UTC)
	resumedAt := time.Date(2026, 6, 11, 12, 0, 10, 0, time.UTC)
	sessionID := "dur-sess-replay-legacy-pause-resume-001"
	sessionSequence := 1
	source := canonicalEventSourceRuntimeService
	mustMarshalEvent := func(event canonicalFactoryEvent) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("json.Marshal event: %v", err)
		}
		return raw
	}

	baseSession := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: "JAVASCRIPT",
		SourceHash:       "sha256:fixture",
		Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
		ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
	}
	baseResult := ResultReadResult{
		SessionID:    sessionID,
		ResultStatus: ResultStatusNotReady,
	}

	events := BuildCanonicalRuntimeSessionEvents(baseSession, baseResult)
	events = append(events,
		mustMarshalEvent(canonicalFactoryEvent{
			SchemaVersion: "v1alpha",
			ID:            "event-session-paused",
			Type:          "SESSION_PAUSED",
			Context: canonicalFactoryEventContext{
				Sequence:        3,
				Tick:            3,
				EventTime:       pausedAt,
				SessionID:       &sessionID,
				SessionSequence: &sessionSequence,
				Source:          &source,
			},
			Payload: mustMarshalPayload(map[string]any{
				"status":   string(LifecycleStatusPaused),
				"pausedAt": pausedAt.Format(time.RFC3339),
			}),
		}),
		mustMarshalEvent(canonicalFactoryEvent{
			SchemaVersion: "v1alpha",
			ID:            "event-session-resumed",
			Type:          "SESSION_RESUMED",
			Context: canonicalFactoryEventContext{
				Sequence:        4,
				Tick:            4,
				EventTime:       resumedAt,
				SessionID:       &sessionID,
				SessionSequence: &sessionSequence,
				Source:          &source,
			},
			Payload: mustMarshalPayload(map[string]any{
				"status":    string(LifecycleStatusRunning),
				"resumedAt": resumedAt.Format(time.RFC3339),
			}),
		}),
	)

	session, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.Status != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", session.Status)
	}
	if result.SessionStatus != LifecycleStatusRunning {
		t.Fatalf("result sessionStatus = %q, want RUNNING", result.SessionStatus)
	}
	if session.Lifecycle == nil || session.Lifecycle.PausedAt == nil || !session.Lifecycle.PausedAt.Equal(pausedAt) {
		t.Fatalf("pausedAt = %#v, want %s", session.Lifecycle, pausedAt)
	}
	if session.Lifecycle.ResumedAt == nil || !session.Lifecycle.ResumedAt.Equal(resumedAt) {
		t.Fatalf("resumedAt = %#v, want %s", session.Lifecycle.ResumedAt, resumedAt)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this regression keeps pause/resume append and no-op event immutability assertions together.
func TestFakeService_PauseResumeAppendsLifecycleControlEventsWithoutNoOpMutation(t *testing.T) {
	t.Parallel()
	service, err := NewFakeServiceFromContractFixtures(contractFixturesPath(t), fakeServiceTestClock(), fileeffects.ContractFixtureReader(os.ReadFile))
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	started, err := service.StartAsync(context.Background(), StartRequest{
		RequestID: "req-js-run-n-001",
		Source: Source{
			Kind:      factory.WorkflowSourceKindFactoryID,
			FactoryID: "customer-support-triage",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}

	beforeEvents, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents before pause: %v", err)
	}
	beforeCount := len(beforeEvents.Events)

	paused, err := service.Pause(context.Background(), started.SessionID, ControlRequest{RequestID: "ctrl-pause-events-001"})
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if paused.Outcome != LifecycleControlOutcomeAccepted || paused.Status != LifecycleStatusPaused {
		t.Fatalf("pause = %#v, want ACCEPTED/PAUSED", paused)
	}

	afterPause, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after pause: %v", err)
	}
	if len(afterPause.Events) != beforeCount+1 {
		t.Fatalf("event count after pause = %d, want %d", len(afterPause.Events), beforeCount+1)
	}
	assertCanonicalEventEnvelope(t, afterPause.Events[len(afterPause.Events)-1], "SESSION_LIFECYCLE_CONTROL", "session-lifecycle-control/"+started.SessionID+"/2")

	pauseNoOp, err := service.Pause(context.Background(), started.SessionID, ControlRequest{})
	if err != nil {
		t.Fatalf("Pause no-op: %v", err)
	}
	if pauseNoOp.Outcome != LifecycleControlOutcomeNoOp {
		t.Fatalf("pause no-op outcome = %q, want NO_OP", pauseNoOp.Outcome)
	}
	afterNoOp, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after no-op: %v", err)
	}
	if len(afterNoOp.Events) != len(afterPause.Events) {
		t.Fatalf("event count after no-op = %d, want unchanged %d", len(afterNoOp.Events), len(afterPause.Events))
	}

	resumed, err := service.Resume(context.Background(), started.SessionID, ControlRequest{RequestID: "ctrl-resume-events-001"})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if resumed.Outcome != LifecycleControlOutcomeAccepted || resumed.Status != LifecycleStatusRunning {
		t.Fatalf("resume = %#v, want ACCEPTED/RUNNING", resumed)
	}

	afterResume, err := service.ReadEvents(context.Background(), started.SessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents after resume: %v", err)
	}
	if len(afterResume.Events) != len(afterPause.Events)+1 {
		t.Fatalf("event count after resume = %d, want %d", len(afterResume.Events), len(afterPause.Events)+1)
	}

	replayed, _, err := ReplaySessionProjection(afterResume.Events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if replayed.Status != LifecycleStatusRunning {
		t.Fatalf("replayed status = %q, want RUNNING", replayed.Status)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this fake-service race regression keeps interrupt and replay assertions together on one scenario.
func TestFakeService_InterruptAcceptedBeforeCompletion_ObservableDispatchAndEventOutcomes(t *testing.T) {
	t.Parallel()
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

type terminalWorkerBehavior string

const (
	terminalWorkerSuccess      terminalWorkerBehavior = "success"
	terminalWorkerFailure      terminalWorkerBehavior = "failure"
	terminalWorkerCancellation terminalWorkerBehavior = "cancellation"
	terminalWorkerTimeout      terminalWorkerBehavior = "timeout"
)

type childTerminalResponseCase struct {
	id            string
	name          string
	behavior      terminalWorkerBehavior
	progress      []workers.ProgressFragment
	wantKind      responseevents.Kind
	wantPhase     responseevents.Phase
	wantErrorCode string
}

func TestChildWorkerExecutor_PublishesExactlyOneDurableTerminalResponseForEveryOutcome(t *testing.T) {
	t.Parallel()
	for _, test := range childTerminalResponseCases() {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runChildTerminalResponseCase(t, test)
		})
	}
}

func childTerminalResponseCases() []childTerminalResponseCase {
	return []childTerminalResponseCase{
		{
			id:       "success",
			name:     "success with provider terminal progress",
			behavior: terminalWorkerSuccess,
			progress: []workers.ProgressFragment{
				{Kind: workers.ProgressFragmentKind, Payload: "provider progress"},
				{Kind: workers.CompletedFragmentKind, Type: "COMPLETED"},
			},
			wantKind: responseevents.KindRun, wantPhase: responseevents.PhaseCompleted,
		},
		{
			id:            "failure",
			name:          "failure without provider terminal progress",
			behavior:      terminalWorkerFailure,
			progress:      []workers.ProgressFragment{{Kind: workers.ProgressFragmentKind, Payload: "failure progress"}},
			wantKind:      responseevents.KindError,
			wantPhase:     responseevents.PhaseFailed,
			wantErrorCode: "stream_failed",
		},
		{
			id:            "cancellation",
			name:          "cancellation without provider terminal progress",
			behavior:      terminalWorkerCancellation,
			wantKind:      responseevents.KindError,
			wantPhase:     responseevents.PhaseFailed,
			wantErrorCode: "stream_canceled",
		},
		{
			id:            "timeout",
			name:          "timeout without provider terminal progress",
			behavior:      terminalWorkerTimeout,
			wantKind:      responseevents.KindError,
			wantPhase:     responseevents.PhaseFailed,
			wantErrorCode: "timeout",
		},
	}
}

func runChildTerminalResponseCase(t *testing.T, test childTerminalResponseCase) {
	t.Helper()
	sessionID := "dur-sess-terminal-" + test.id
	service := newDurableResponseEventsService(t)
	state := seedResponseEventSession(t, service, sessionID)
	if err := service.ensureSessionResponseEvents(sessionID, state); err != nil {
		t.Fatalf("ensure response events: %v", err)
	}

	barriers := newTerminalWorkerBarriers()
	provider := newTerminalWorkerProvider(test.behavior, barriers)
	workerService := newTerminalWorkersService(t, provider)
	execution := terminalWorkerExecution{service: workerService, progress: test.progress}
	executor := newChildWorkerExecutor(
		sessionID, execution, newChildRecordSink(), childTestValues{},
		service.observeWorkerDispatch, "/project", 0,
	)
	if test.behavior == terminalWorkerTimeout {
		// Codex has no native attempt timeout in this test service. Keep the
		// configured attempt bound meaningful, then release it through the
		// controlled factory after the provider start barrier.
		executor.maxWorkerDuration = time.Second
		executor.timeoutContextFactory = barriers.timeoutContext
	}
	executor.publish = func(_ string, fragment workers.ProgressFragment) {
		service.PublishWorkerProgress(fragment)
	}
	executionErr := executeTerminalChild(t, executor, test.behavior, barriers)

	cursor, err := service.SubscribeResponseEvents(context.Background(), sessionID, factorysessions.ResponseEventSubscriptionRequest{SessionID: sessionID})
	if err != nil {
		t.Fatalf("SubscribeResponseEvents: %v", err)
	}
	defer cursor.Detach()
	events, err := cursor.Drain()
	if err != nil {
		t.Fatalf("Drain response events: %v", err)
	}
	assertTerminalResponse(t, events, test, executionErr)
}

// terminalWorkerBarrierWatchdog is diagnostic only. Normal outcome progression
// is driven by the case-owned channels; this timer turns a stuck barrier into
// an actionable test failure and does not determine any outcome.
const terminalWorkerBarrierWatchdog = time.Second

type terminalWorkerBarriers struct {
	started       chan struct{}
	deadline      chan struct{}
	canceled      chan struct{}
	completed     chan error
	startOnce     sync.Once
	deadlineOnce  sync.Once
	cancelOnce    sync.Once
	cancelMu      sync.Mutex
	cancelAttempt context.CancelFunc
}

func newTerminalWorkerBarriers() *terminalWorkerBarriers {
	return &terminalWorkerBarriers{
		started:   make(chan struct{}),
		deadline:  make(chan struct{}),
		canceled:  make(chan struct{}),
		completed: make(chan error, 1),
	}
}

func (barriers *terminalWorkerBarriers) markStarted() {
	barriers.startOnce.Do(func() { close(barriers.started) })
}

func (barriers *terminalWorkerBarriers) cancelParent(cancel context.CancelFunc) {
	barriers.cancelOnce.Do(func() {
		close(barriers.canceled)
		cancel()
	})
}

func (barriers *terminalWorkerBarriers) timeoutContext(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	base, cancel := context.WithCancel(parent)
	barriers.cancelMu.Lock()
	barriers.cancelAttempt = cancel
	deadlineReleased := false
	select {
	case <-barriers.deadline:
		deadlineReleased = true
	default:
	}
	barriers.cancelMu.Unlock()
	if deadlineReleased {
		cancel()
	}
	return terminalWorkerDeadlineContext{
		Context:          base,
		deadline:         time.Now().Add(timeout),
		deadlineReleased: barriers.deadline,
	}, cancel
}

func (barriers *terminalWorkerBarriers) releaseDeadline() {
	barriers.deadlineOnce.Do(func() {
		close(barriers.deadline)
		barriers.cancelMu.Lock()
		cancel := barriers.cancelAttempt
		barriers.cancelMu.Unlock()
		if cancel != nil {
			cancel()
		}
	})
}

type terminalWorkerDeadlineContext struct {
	context.Context
	deadline         time.Time
	deadlineReleased <-chan struct{}
}

func (ctx terminalWorkerDeadlineContext) Deadline() (time.Time, bool) {
	return ctx.deadline, true
}

func (ctx terminalWorkerDeadlineContext) Err() error {
	select {
	case <-ctx.deadlineReleased:
		return context.DeadlineExceeded
	default:
		return ctx.Context.Err()
	}
}

func executeTerminalChild(
	t *testing.T,
	executor *childWorkerExecutor,
	behavior terminalWorkerBehavior,
	barriers *terminalWorkerBarriers,
) error {
	t.Helper()
	request := factory.JavaScriptChildExecutionRequest{
		Prompt: "run", Preset: "agent", ModelProvider: "codex", Model: "terminal-model",
	}
	parentCtx, cancelParent := context.WithCancel(context.Background())
	defer barriers.cancelParent(cancelParent)
	joined := false
	go func() {
		_, err := executor.Execute(parentCtx, request)
		barriers.completed <- err
	}()
	t.Cleanup(func() {
		barriers.cancelParent(cancelParent)
		barriers.releaseDeadline()
		if joined {
			return
		}
		timer := time.NewTimer(terminalWorkerBarrierWatchdog)
		defer timer.Stop()
		select {
		case <-barriers.completed:
			joined = true
		case <-timer.C:
			t.Errorf("terminal worker completion barrier did not close")
		}
	})
	select {
	case <-barriers.started:
	case <-time.After(terminalWorkerBarrierWatchdog):
		t.Fatal("terminal Workers provider start barrier did not close")
	}
	if behavior == terminalWorkerCancellation {
		barriers.cancelParent(cancelParent)
	} else if behavior == terminalWorkerTimeout {
		barriers.releaseDeadline()
	}
	var executionErr error
	select {
	case executionErr = <-barriers.completed:
		joined = true
	case <-time.After(terminalWorkerBarrierWatchdog):
		t.Fatal("terminal worker completion barrier did not close")
	}
	if behavior == terminalWorkerTimeout && parentCtx.Err() != nil {
		t.Fatalf("timeout canceled parent context: %v", parentCtx.Err())
	}
	return executionErr
}

func assertTerminalResponse(
	t *testing.T,
	events []responseevents.FactoryResponseEvent,
	test childTerminalResponseCase,
	executionErr error,
) {
	t.Helper()
	if test.behavior == terminalWorkerSuccess && executionErr != nil {
		t.Fatalf("success child execution error = %v", executionErr)
	}
	if test.behavior != terminalWorkerSuccess && executionErr == nil {
		t.Fatal("unhappy child execution error = nil")
	}
	terminals := terminalResponseEvents(events)
	if len(terminals) != 1 {
		t.Fatalf("response events = %#v, want exactly one terminal event", events)
	}
	expectedProgressEvents := 0
	for _, fragment := range test.progress {
		if !isChildTerminalProgress(fragment) {
			expectedProgressEvents++
		}
	}
	if len(events) != expectedProgressEvents+1 {
		t.Fatalf("response event count = %d, want %d progress events plus one terminal", len(events), expectedProgressEvents+1)
	}
	terminal := terminals[0]
	if terminal.Kind != test.wantKind || terminal.Phase != test.wantPhase {
		t.Fatalf("terminal event = %#v, want kind=%q phase=%q", terminal, test.wantKind, test.wantPhase)
	}
	if test.wantErrorCode != "" {
		var payload responseevents.ErrorPayload
		if err := json.Unmarshal(terminal.Payload, &payload); err != nil {
			t.Fatalf("decode terminal error payload: %v", err)
		}
		if payload.Code != test.wantErrorCode {
			t.Fatalf("terminal error code = %q, want %q", payload.Code, test.wantErrorCode)
		}
	}
	if len(events) == 0 || events[len(events)-1].EventID != terminal.EventID {
		t.Fatalf("events = %#v, want progress before the terminal event", events)
	}
	for _, event := range events[:len(events)-1] {
		if isTerminalResponseEvent(event) {
			t.Fatalf("events = %#v, want no terminal response before the final event", events)
		}
	}
}

func newTerminalWorkerProvider(behavior terminalWorkerBehavior, barriers *terminalWorkerBarriers) providers.Service {
	provider := testutil.NativeProvider{
		ExecuteFunc: func(ctx context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
			barriers.markStarted()
			switch behavior {
			case terminalWorkerSuccess:
				return providers.ExecuteResult{
					Outcome: providers.ExecuteOutcomeAccepted, Content: "completed",
				}, nil
			case terminalWorkerFailure:
				return providers.ExecuteResult{}, errors.New("provider failed")
			default:
				<-ctx.Done()
				return providers.ExecuteResult{}, ctx.Err()
			}
		},
	}
	return provider
}

type terminalWorkerExecution struct {
	service  WorkerExecution
	progress []workers.ProgressFragment
}

func (execution terminalWorkerExecution) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	if request.Input.ProgressPublisher == nil {
		return workers.ExecuteResult{}, errors.New("progress publisher is required")
	}
	for _, fragment := range execution.progress {
		request.Input.ProgressPublisher(fragment)
	}
	return execution.service.Execute(ctx, request)
}

func terminalResponseEvents(events []responseevents.FactoryResponseEvent) []responseevents.FactoryResponseEvent {
	terminals := make([]responseevents.FactoryResponseEvent, 0, 2)
	for _, event := range events {
		if isTerminalResponseEvent(event) {
			terminals = append(terminals, event)
		}
	}
	return terminals
}

func isTerminalResponseEvent(event responseevents.FactoryResponseEvent) bool {
	isRunTerminal := event.Kind == responseevents.KindRun && event.Phase == responseevents.PhaseCompleted
	isErrorTerminal := event.Kind == responseevents.KindError &&
		(event.Phase == responseevents.PhaseFailed || event.Phase == responseevents.PhaseCanceled)
	return isRunTerminal || isErrorTerminal
}

func TestJavaScriptRuntimeService_CloseReportsNonCooperativeRunTimeout(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	service := newConfiguredJavaScriptRuntimeService(javaScriptRuntimeServiceConfig{
		ProjectRoot: t.TempDir(),
		Workflows: scriptedRuntimeWorkflows(func(
			context.Context,
			factory.JavaScriptRuntimeRequest,
			factory.JavaScriptRuntimeHooks,
		) (factory.JavaScriptRuntimeOutcome, error) {
			close(entered)
			<-release
			return factory.JavaScriptRuntimeOutcome{OK: true}, nil
		}),
	})
	defer func() {
		close(release)
		service.runWaitGroup.Wait()
	}()
	if _, err := service.StartAsync(context.Background(), inlineWorkflowStartRequest(
		"req-runtime-close-timeout-001",
		busyLoopWorkflowSource,
		nil,
		nil,
	)); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the non-cooperative workflow to start")
	}

	if err := service.closeWithTimeout(time.Millisecond); !errors.Is(err, ErrDurableExecutionShutdownTimeout) {
		t.Fatalf("closeWithTimeout error = %v, want ErrDurableExecutionShutdownTimeout", err)
	}
}

// TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession pins what makes
// those routing keys distinct in the first place.
//
// Child dispatch identities are minted per session and start again at
// dispatch-1 for each, while the Workers pool they share treats a dispatch ID
// as single-use for its whole life. Two sessions submitting an unqualified
// dispatch-1 would leave the second refused outright.
func TestChildWorkerExecutor_ScopesTheWorkersIdentityToItsSession(t *testing.T) {
	first := newChildWorkerExecutor("dur-sess-first", nil, nil, nil, nil, "", 0)
	second := newChildWorkerExecutor("dur-sess-second", nil, nil, nil, nil, "", 0)

	firstID := first.workerDispatchIdentity("dispatch-1")
	secondID := second.workerDispatchIdentity("dispatch-1")
	if firstID == secondID {
		t.Fatalf("two sessions submitted the same Workers dispatch identity %q", firstID)
	}
	if firstID != "dur-sess-first/dispatch-1" {
		t.Fatalf("Workers dispatch identity = %q, want the session-scoped identity", firstID)
	}
}

func TestCompactPetriTokenHistory_PreservesReachableAndRetiresConsumedTerminalHistory(t *testing.T) {
	reachable := largeTerminalTokenMutations(1, "active-output")
	reachable[2].TransitionReachable = true
	if retained, summaries := compactPetriTokenHistory(reachable, nil); len(retained) != len(reachable) || len(summaries) != 0 {
		t.Fatalf("reachable terminal history = %d mutations, %d summaries, want lossless history", len(retained), len(summaries))
	}
	consumed := append(largeTerminalTokenMutations(2, "output"), interfaces.TokenMutationRecord{
		DispatchID: "dispatch-2", TransitionID: "consume", Outcome: workers.OutcomeAccepted, Type: interfaces.MutationConsume, TokenID: "token-2", FromPlace: "task:done", Terminal: true,
	})
	retained, summaries := compactPetriTokenHistory(consumed, nil)
	if len(retained) != 0 || len(summaries) != 1 || !summaries[0].Retired || summaries[0].WorkID != "work-2" || summaries[0].State != "done" {
		t.Fatalf("consumed terminal history = %d mutations, %#v, want one retired work-2 summary", len(retained), summaries)
	}
}
