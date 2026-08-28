package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestAppendDispatchInterruptedEvent_RecordsCanonicalMetadata(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	_, base, events := interruptedDispatchFixture(
		"dur-sess-interrupt-001", startedAt, startedAt,
		"JAVASCRIPT", "you-workflow-v1", "execute", "audit", "operator stop", ResultStatusNotReady,
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
	_, _, events := interruptedDispatchFixture(
		"dur-sess-interrupt-002", startedAt, startedAt,
		"JAVASCRIPT", "", "", "", "bad prompt", "",
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

	dispatch, events := interruptFakeDispatch(t, service, started.SessionID, "stop bad run")
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Message != "stop bad run" {
		t.Fatalf("failureDetail = %#v, want stop bad run", dispatch.FailureDetail)
	}
	findDispatchInterruptedEventPayload(t, events.Events, "disp-js-002")

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
	listed := findDispatchByID(list.Dispatches, "disp-js-002")
	if listed == nil {
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
	interrupted := interruptedDispatchSummary("audit", "operator stop")
	state := &runtimeSessionState{
		session: SessionReadResult{SessionID: "dur-sess-interrupt-late-001", Phase: "execute"},
		dispatches: []DispatchSummary{
			interrupted,
		},
		dispatchStatusTransitions: interruptedDispatchTransitions(),
	}
	preserved := snapshotInterruptedDispatches(state)

	lateRecords := lateChildCompletionRecords(
		"disp-js-002", "audit", "artifact://child-artifact-late", "provider-session-late", "fake",
	)
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
	running, _, events := interruptedDispatchFixture(
		sessionID, startedAt, observedAt, "", "", "execute", "", "operator stop", "",
	)
	prior := &runtimeSessionState{
		session:                   running,
		dispatches:                []DispatchSummary{interruptedDispatchSummary("", "operator stop")},
		dispatchStatusTransitions: interruptedDispatchTransitions(),
		events:                    events,
	}
	lateRecords := lateChildCompletionRecords("disp-js-002", "audit", "artifact://child-artifact-late", "", "")
	terminal := runtimeSessionState{session: running}
	applyRuntimeSuccessProjection(&terminal, sessionID, successfulRuntimeOutcome(lateRecords), observedAt)

	applyTerminalRuntimeProjection(prior, terminal, successfulRuntimeOutcome(lateRecords))

	if prior.dispatches[0].Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", prior.dispatches[0].Status)
	}
	if !containsEventType(prior.events, "DISPATCH_INTERRUPTED") {
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

	baseSession := javascriptReplaySession(sessionID, LifecycleStatusRunning, &startedAt)
	baseResult := javascriptReplayResult(sessionID)

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

	session, result := replaySessionProjection(t, events)
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

	baseSession := javascriptReplaySession(sessionID, LifecycleStatusRunning, &startedAt)
	baseResult := javascriptReplayResult(sessionID)

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

	session, result := replaySessionProjection(t, events)
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

	replayed, _ := replaySessionProjection(t, afterResume.Events)
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

	_, events := interruptFakeDispatch(t, service, started.SessionID, "stop before provider completion")
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

func TestBuildCanonicalSessionEvents_RunningAndTerminalSessions(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 8, 14, 10, 0, 0, time.UTC)

	runningSession := replaySession("dur-sess-js-run-n-001", "JAVASCRIPT", LifecycleStatusRunning, &startedAt, "", "", "verify")
	runningSession.Dialect = "you-workflow-v1"
	runningEvents := canonicalReplayEvents(runningSession, ResultStatusPartial)
	if len(runningEvents) != 2 {
		t.Fatalf("running events = %d, want 2", len(runningEvents))
	}
	assertCanonicalEventEnvelope(t, runningEvents[0], "SESSION_STARTED", "session-started/dur-sess-js-run-n-001")
	assertCanonicalEventEnvelope(t, runningEvents[1], "SESSION_RESULT_UPDATED", "session-result-updated/dur-sess-js-run-n-001")

	terminalSession := replaySession("dur-sess-js-success-002", "JAVASCRIPT", LifecycleStatusSucceeded, &startedAt, "", "", "")
	terminalSession.Lifecycle.FinishedAt = &finishedAt
	terminalEvents := canonicalReplayEvents(terminalSession, ResultStatusFinal)
	if len(terminalEvents) != 3 {
		t.Fatalf("terminal events = %d, want 3", len(terminalEvents))
	}
	assertCanonicalEventEnvelope(t, terminalEvents[2], "SESSION_COMPLETED", "session-completed/dur-sess-js-success-002")
}

func TestPetriTokenSummary_RoundTripsThroughTaggedDurableHistory(t *testing.T) {
	snapshot := PersistedRuntimeSessionState{Records: []DurableSessionRecord{{Kind: DurableRecordKindPetriTokenSummary, PetriSummary: &PetriTokenSummary{TokenID: "token-summary", WorkID: "work-summary", WorkTypeID: "task", PlaceID: "task:done", State: "done"}}}}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal summary snapshot: %v", err)
	}
	var decoded PersistedRuntimeSessionState
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal summary snapshot: %v", err)
	}
	hydrated := runtimeStateFromPersistedSnapshot(decoded)
	if len(hydrated.petriMutations) != 0 || len(hydrated.petriSummaries) != 1 || hydrated.petriSummaries[0].WorkID != "work-summary" {
		t.Fatalf("hydrated summary state = %d mutations, %#v", len(hydrated.petriMutations), hydrated.petriSummaries)
	}
	resaved := persistedSnapshotFromRuntimeStateWithFailureLogCapacity(hydrated, 0)
	if len(resaved.Records) != 1 || resaved.Records[0].PetriSummary == nil {
		t.Fatalf("resaved summary records = %#v", resaved.Records)
	}
}

func TestProjectRuntimeExecutionRecords_LiveChildDispatch_ProjectsLifecycleArtifactsAndProviderSession(t *testing.T) {
	t.Parallel()
	artifactRef := factory.FormatArtifactURI("session-live-child", "child-artifact-1")
	records := []factory.JavaScriptRuntimeRecord{
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-1",
				Status:        factory.JavaScriptChildDispatchStatusQueued,
				Label:         "summarize-findings",
				ExecutionMode: ChildExecutorModeLive,
				ArtifactRef:   artifactRef,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-1",
				Status:        factory.JavaScriptChildDispatchStatusRunning,
				Label:         "summarize-findings",
				ExecutionMode: ChildExecutorModeLive,
				ArtifactRef:   artifactRef,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:         "dispatch-1",
				Status:             factory.JavaScriptChildDispatchStatusCompleted,
				Label:              "summarize-findings",
				ExecutionMode:      ChildExecutorModeLive,
				Provider:           "mock",
				ProviderSessionRef: "provider-session-42",
				ArtifactRef:        artifactRef,
			},
		},
	}

	observedAt := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	projection := ProjectRuntimeExecutionRecords("session-live-child", records, observedAt)
	if len(projection.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one dispatch", projection.Dispatches)
	}
	dispatch := projection.Dispatches[0]
	if dispatch.Status != DispatchStatusCompleted {
		t.Fatalf("dispatch status = %q, want COMPLETED", dispatch.Status)
	}
	if len(dispatch.ProviderSessionRefs) != 1 || dispatch.ProviderSessionRefs[0].ID != "provider-session-42" {
		t.Fatalf("providerSessionRefs = %#v", dispatch.ProviderSessionRefs)
	}
	if len(dispatch.OutputArtifactIDs) != 1 || dispatch.OutputArtifactIDs[0] != "child-artifact-1" {
		t.Fatalf("outputArtifactIds = %#v", dispatch.OutputArtifactIDs)
	}
	transitions := projection.DispatchStatusTransitions["dispatch-1"]
	if len(transitions) != 3 {
		t.Fatalf("statusTransitions = %#v, want queued/running/completed", transitions)
	}
	if len(projection.Artifacts) != 1 || projection.Artifacts[0].ID != "child-artifact-1" {
		t.Fatalf("artifacts = %#v", projection.Artifacts)
	}
}

func TestReplaySessionProjection_TerminalSessionBracket(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-001"
	session := replaySession(
		sessionID, "JAVASCRIPT", LifecycleStatusSucceeded, &startedAt,
		"sha256:fixture", "sha256:policy", "",
	)
	session.Dialect = "you-workflow-v1"
	session.ResolvedSource.SourceRef = "workflow/simple-final"
	session.ResultSummary = &ResultSummary{
		ResultStatus: string(ResultStatusFinal),
		Summary:      "Completed simple workflow.",
	}
	session.Lifecycle.FinishedAt = &finishedAt
	events := BuildCanonicalRuntimeSessionEvents(session, ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusFinal})

	session, result := replaySessionProjection(t, events)
	if session.Status != LifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", session.Status)
	}
	if session.SourceHash != "sha256:fixture" {
		t.Fatalf("sourceHash = %q", session.SourceHash)
	}
	if session.Policy.EffectiveHash != "sha256:policy" {
		t.Fatalf("policyHash = %q", session.Policy.EffectiveHash)
	}
	if session.ResultSummary == nil || session.ResultSummary.ResultStatus != string(ResultStatusFinal) {
		t.Fatalf("resultSummary = %#v, want FINAL", session.ResultSummary)
	}
	if session.Links.Session == "" || session.Links.Events == "" {
		t.Fatal("expected inspection links")
	}
	if result.ResultStatus != ResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", result.ResultStatus)
	}
}

func TestReplaySessionProjection_IdempotentOnDuplicateSequence(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-002"
	events := javascriptReplayEvents(sessionID, LifecycleStatusRunning, &startedAt, javascriptReplayResult(sessionID))

	firstSession, firstResult := replaySessionProjection(t, events)
	secondSession, secondResult := replaySessionProjection(t, events)
	if firstSession.SessionID != secondSession.SessionID ||
		firstSession.Status != secondSession.Status ||
		firstSession.SourceHash != secondSession.SourceHash ||
		firstSession.Policy.EffectiveHash != secondSession.Policy.EffectiveHash ||
		firstSession.Links.Session != secondSession.Links.Session {
		t.Fatalf("session projection changed on replay: %#v vs %#v", firstSession, secondSession)
	}
	if firstResult.ResultStatus != secondResult.ResultStatus ||
		firstResult.SessionStatus != secondResult.SessionStatus {
		t.Fatalf("result projection changed on replay: %#v vs %#v", firstResult, secondResult)
	}
}

func TestReplaySessionProjection_ReplacesArtifactStubsWithoutDuplication(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-003"
	session := javascriptReplaySession(sessionID, LifecycleStatusSucceeded, &startedAt)
	session.Lifecycle.FinishedAt = &finishedAt
	events := BuildCanonicalRuntimeSessionEvents(session, replayResult(sessionID, ResultStatusFinal, "art-1", "art-2"))

	session, _ = replaySessionProjection(t, events)
	if len(session.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs = %d, want 2", len(session.ArtifactRefs))
	}

	// Replaying the same events again must not duplicate artifact stubs.
	sessionAgain, _ := replaySessionProjection(t, events)
	if len(sessionAgain.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs after second replay = %d, want 2", len(sessionAgain.ArtifactRefs))
	}
}

func TestReplaySessionProjection_FirstTerminalOutcomeWinsCompetingRace(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	sessionID := "dur-sess-replay-terminal-race"
	base := orchestratorReplaySession(sessionID, "JAVASCRIPT", LifecycleStatusCanceled, &startedAt)
	base.Lifecycle.FinishedAt = &finishedAt
	canceled := BuildCanonicalRuntimeSessionEvents(base, ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusUnavailable, SessionStatus: LifecycleStatusCanceled})
	base.Status = LifecycleStatusFailed
	failed := BuildCanonicalRuntimeSessionEvents(base, ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusFailedWithPartial, SessionStatus: LifecycleStatusFailed})

	session, result := replaySessionProjection(t, append(canceled, failed...))
	if session.Status != LifecycleStatusCanceled || result.SessionStatus != LifecycleStatusCanceled || result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("late terminal outcome overwrote cancellation: session=%#v result=%#v", session, result)
	}
}

func TestReplaySessionProjection_PreservesSyncTimeoutAvailability(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-timeout-availability"
	events := javascriptReplayEvents(sessionID, LifecycleStatusRunning, &startedAt, syncTimeoutReplayResult(sessionID))

	_, result := replaySessionProjection(t, events)
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestReplaySessionProjection_IgnoresUnknownEventTypes(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-004"
	base := javascriptReplayEvents(sessionID, LifecycleStatusRunning, &startedAt, javascriptReplayResult(sessionID))
	events := append(append([]json.RawMessage(nil), base...), json.RawMessage(`{"type":"DISPATCH_QUEUED","id":"dispatch-queued/1","context":{"sequence":99},"payload":{}}`))

	session, _ := replaySessionProjection(t, events)
	if session.SessionID != sessionID {
		t.Fatalf("sessionId = %q, want %q", session.SessionID, sessionID)
	}
}

func interruptedDispatchFixture(
	sessionID string,
	startedAt, observedAt time.Time,
	orchestratorKind, dialect, sessionPhase, label, reason string,
	resultStatus ResultStatus,
) (SessionReadResult, []json.RawMessage, []json.RawMessage) {
	session := SessionReadResult{
		SessionID:        sessionID,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: orchestratorKind,
		Dialect:          dialect,
		Phase:            sessionPhase,
		Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
	}
	base := BuildCanonicalRuntimeSessionEvents(session, ResultReadResult{
		SessionID:    sessionID,
		ResultStatus: resultStatus,
	})
	events := AppendDispatchInterruptedEvent(
		base,
		session,
		DispatchSummary{ID: "disp-js-002", Status: DispatchStatusRunning, Phase: "execute", Label: label},
		InterruptDispatchRequest{
			ControlRequest: ControlRequest{Reason: reason},
			DispatchID:     "disp-js-002",
		},
		DispatchStatusRunning,
		canonicalEventSourceRuntimeService,
		observedAt,
	)
	return session, base, events
}

func interruptedDispatchSummary(label, reason string) DispatchSummary {
	return DispatchSummary{
		ID: "disp-js-002", Status: DispatchStatusInterrupted, Phase: "execute", Label: label,
		FailureDetail: dispatchInterruptionFailureDetail(reason),
	}
}

func javascriptReplayResult(sessionID string) ResultReadResult {
	return ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusNotReady}
}

func interruptedDispatchTransitions() map[string][]DispatchStatus {
	return map[string][]DispatchStatus{"disp-js-002": {DispatchStatusQueued, DispatchStatusRunning, DispatchStatusInterrupted}}
}

func interruptFakeDispatch(
	t *testing.T,
	service *FakeService,
	sessionID, reason string,
) (DispatchDetail, EventReadResult) {
	t.Helper()
	result, err := service.InterruptDispatch(context.Background(), sessionID, InterruptDispatchRequest{
		ControlRequest: ControlRequest{Reason: reason},
		DispatchID:     "disp-js-002",
	})
	if err != nil {
		t.Fatalf("InterruptDispatch: %v", err)
	}
	if result.Outcome != LifecycleControlOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	dispatch, err := service.GetDispatch(context.Background(), sessionID, "disp-js-002")
	if err != nil {
		t.Fatalf("GetDispatch: %v", err)
	}
	if dispatch.Status != DispatchStatusInterrupted {
		t.Fatalf("dispatch status = %q, want INTERRUPTED", dispatch.Status)
	}
	events, err := service.ReadEvents(context.Background(), sessionID, EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	return dispatch, events
}

func lateChildCompletionRecords(
	dispatchID, label, artifactRef, providerSessionRef, provider string,
) []factory.JavaScriptRuntimeRecord {
	return []factory.JavaScriptRuntimeRecord{{
		Kind: factory.JavaScriptRecordKindChildDispatch,
		ChildDispatch: &factory.JavaScriptChildDispatchRecord{
			DispatchID:         dispatchID,
			Status:             factory.JavaScriptChildDispatchStatusCompleted,
			Label:              label,
			ArtifactRef:        artifactRef,
			ProviderSessionRef: providerSessionRef,
			Provider:           provider,
		},
	}}
}

func successfulRuntimeOutcome(records []factory.JavaScriptRuntimeRecord) factory.JavaScriptRuntimeOutcome {
	return factory.JavaScriptRuntimeOutcome{
		OK: true, Records: records, Value: factory.TypedValue{JSON: json.RawMessage("null")},
	}
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
