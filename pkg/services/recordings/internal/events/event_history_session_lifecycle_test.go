package events

import (
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type completedFlushWatermarkStub struct {
	cursor recordings.CanonicalEventCursor
	ok     bool
}

func (stub completedFlushWatermarkStub) CompletedFlushWatermark(string) (recordings.CanonicalEventCursor, bool) {
	return stub.cursor, stub.ok
}

func TestFactoryEventHistoryForwardsCompletedFlushWatermark(t *testing.T) {
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	expected := recordings.CanonicalEventCursor{StreamGenerationID: "generation", Sequence: 7}
	history.SetCompletedFlushWatermarkReader(completedFlushWatermarkStub{cursor: expected, ok: true})

	got, ok := history.CompletedFlushWatermark("generation")
	if !ok || got != expected {
		t.Fatalf("CompletedFlushWatermark() = %#v, %v; want %#v, true", got, ok, expected)
	}
}

func TestFactoryEventHistory_FiltersSensitiveFactoryPointerProvenance(t *testing.T) {
	snapshot, err := interfaces.NewFactorySnapshot(map[string]any{
		"name":   "factory-sensitive",
		"nested": map[string]any{"key": "value"},
		"items": []any{
			map[string]any{"name": "first", "a/b": "slash", "a~b": "tilde"},
			map[string]any{"name": "second"},
		},
	})
	if err != nil {
		t.Fatalf("NewFactorySnapshot: %v", err)
	}
	history := newTestFactoryEventHistory(nil, func() time.Time { return time.Unix(0, 0).UTC() })
	history.SetInitialStructureFactory(snapshot)
	history.SetInvocationSensitiveJSONPointers([]string{
		"",
		"/name",
		"/nested/key",
		"/items/0/name",
		"/items/0/a~1b",
		"/items/0/a~0b",
		"/items/1/name",
		"/missing",
		"/items/0/a~2b",
		"/items/0/a~",
	})

	var duringAppend []recordings.RecordingSecret
	history.AddEventRecorder(func(event interfaces.FactoryEvent) {
		duringAppend = history.SecretProvenanceDuringAppend(event)
	})
	history.RecordInitialStructure()

	events := history.CanonicalEvents()
	if len(events) != 1 {
		t.Fatalf("canonical events = %d, want one initial structure event", len(events))
	}
	provenance := history.SecretProvenanceForEvent(events[0])
	wantPointers := []string{
		"/factory",
		"/factory/name",
		"/factory/nested/key",
		"/factory/items/0/name",
		"/factory/items/0/a~1b",
		"/factory/items/0/a~0b",
		"/factory/items/1/name",
	}
	if len(provenance) != len(wantPointers) || len(duringAppend) != len(wantPointers) {
		t.Fatalf("filtered provenance = %#v, callback provenance = %#v, want %d entries", provenance, duringAppend, len(wantPointers))
	}
	for index, want := range wantPointers {
		if provenance[index].JSONPointer != want || provenance[index].Provenance != recordings.RecordingSecretProvenanceDeclared {
			t.Fatalf("provenance[%d] = %#v, want declared pointer %q", index, provenance[index], want)
		}
		if duringAppend[index] != provenance[index] {
			t.Fatalf("during-append provenance[%d] = %#v, want detached copy %#v", index, duringAppend[index], provenance[index])
		}
	}
}

func TestFactoryEventHistory_RecordSessionLifecycle_EmitsReconstructableBracketSequence(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 10, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-alpha", &interfaces.FactoryConfig{
		Name: "factory-alpha",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
			JavaScript: &interfaces.FactoryOrchestratorJavaScriptConfig{
				Dialect:    "workflow-v1",
				SourceRef:  "workflow/main.js",
				SourceHash: "sha256:source",
			},
		},
	}, 0, t0)
	history.RecordSessionLifecycleCompletion(
		"session-alpha",
		&interfaces.FactoryConfig{
			Orchestrator: &interfaces.FactoryOrchestratorConfig{
				Kind: interfaces.OrchestratorKindJavaScript,
			},
		},
		2,
		interfaces.FactoryStateCompleted,
		"",
		t0.Add(2*time.Second),
	)

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want started, result-updated, completed", len(events))
	}
	assertSessionLifecycleEventType(t, events[0], factoryapi.FactoryEventTypeSessionStarted, "factory-event/session-started")
	assertSessionLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeSessionResultUpdated, "factory-event/session-result-updated/1")
	assertSessionLifecycleEventType(t, events[2], factoryapi.FactoryEventTypeSessionCompleted, "factory-event/session-completed")

	for i, event := range events {
		if event.Context.SessionId == nil || *event.Context.SessionId != "session-alpha" {
			t.Fatalf("events[%d].context.sessionId = %#v, want session-alpha", i, event.Context.SessionId)
		}
		if event.Context.SessionSequence == nil || *event.Context.SessionSequence != i {
			t.Fatalf("events[%d].context.sessionSequence = %#v, want %d", i, event.Context.SessionSequence, i)
		}
	}

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(history.CanonicalEvents(), 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil || !worldState.SessionBracket.Terminal {
		t.Fatalf("session bracket = %#v, want terminal reconstructed lifecycle", worldState.SessionBracket)
	}
}

func TestFactoryEventHistory_AddEventTypeRecorder_ReplaysHistoryThenObservesCompletion(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 10, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-alpha", &interfaces.FactoryConfig{
		Name: "factory-alpha",
	}, 0, t0)

	var eventTypes []interfaces.FactoryEventType
	history.AddEventTypeRecorder(func(eventType interfaces.FactoryEventType) {
		eventTypes = append(eventTypes, eventType)
	})
	history.RecordSessionLifecycleCompletion(
		"session-alpha",
		&interfaces.FactoryConfig{},
		1,
		interfaces.FactoryStateCompleted,
		"",
		t0.Add(time.Second),
	)

	want := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeSessionResultUpdated,
		interfaces.FactoryEventTypeSessionCompleted,
	}
	if len(eventTypes) != len(want) {
		t.Fatalf("event types = %v, want %v", eventTypes, want)
	}
	for index := range want {
		if eventTypes[index] != want[index] {
			t.Fatalf("event types[%d] = %q, want %q", index, eventTypes[index], want[index])
		}
	}
}

func TestFactoryEventHistory_RecordSessionLifecycle_FailedRunEmitsFailedWithPartialResult(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 11, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-beta", &interfaces.FactoryConfig{
		Name: "factory-beta",
	}, 0, t0)
	history.RecordSessionLifecycleCompletion(
		"session-beta",
		&interfaces.FactoryConfig{},
		1,
		interfaces.FactoryStateFailed,
		"dispatch failed",
		t0.Add(time.Second),
	)

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want started, failed-with-partial result, completed", len(events))
	}
	resultPayload, err := events[1].Payload.AsSessionResultUpdatedEventPayload()
	if err != nil {
		t.Fatalf("result updated payload: %v", err)
	}
	if resultPayload.ResultStatus != factoryapi.FactoryEventSessionResultStatusFailedWithPartial {
		t.Fatalf("result status = %q, want FAILED_WITH_PARTIAL", resultPayload.ResultStatus)
	}
	completedPayload, err := events[2].Payload.AsSessionCompletedEventPayload()
	if err != nil {
		t.Fatalf("completed payload: %v", err)
	}
	if completedPayload.FailureDetail == nil || completedPayload.FailureDetail.Message != "dispatch failed" {
		t.Fatalf("completed failure detail = %#v, want dispatch failed message", completedPayload.FailureDetail)
	}
}

func TestFactoryEventHistory_RecordSessionLifecycleControl_EmitsPauseAndResume(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Source:           "runtime",
		Tick:             3,
		Operation:        interfaces.FactorySessionLifecycleControlPause,
		Outcome:          interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		PreviousStatus:   interfaces.FactorySessionLifecycleStatusRunning,
		NewStatus:        interfaces.FactorySessionLifecycleStatusPaused,
		Reason:           "pause requested",
	}, t0)
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Source:           "runtime",
		Tick:             4,
		Operation:        interfaces.FactorySessionLifecycleControlResume,
		Outcome:          interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		PreviousStatus:   interfaces.FactorySessionLifecycleStatusPaused,
		NewStatus:        interfaces.FactorySessionLifecycleStatusRunning,
		Reason:           "resume requested",
	}, t0.Add(time.Second))

	events := generatedHistoryEvents(t, history)
	if len(events) != 2 {
		t.Fatalf("events = %d, want pause and resume lifecycle controls", len(events))
	}
	assertSessionLifecycleEventType(t, events[0], factoryapi.FactoryEventTypeSessionLifecycleControl, "session-lifecycle-control/session-live/0")
	assertSessionLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeSessionLifecycleControl, "session-lifecycle-control/session-live/1")

	pausePayload, err := events[0].Payload.AsSessionLifecycleControlEventPayload()
	if err != nil {
		t.Fatalf("pause payload: %v", err)
	}
	if pausePayload.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pausePayload.PreviousStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning ||
		pausePayload.NewStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("pause payload = %#v, want RUNNING->PAUSED", pausePayload)
	}
}

func TestFactoryStateToDurableLifecycleStatus_MapsLiveFactoryStates(t *testing.T) {
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStatePaused); got != interfaces.FactorySessionLifecycleStatusPaused {
		t.Fatalf("paused = %q, want PAUSED", got)
	}
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStateCompleted); got != interfaces.FactorySessionLifecycleStatusSucceeded {
		t.Fatalf("completed = %q, want SUCCEEDED", got)
	}
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStateFailed); got != interfaces.FactorySessionLifecycleStatusFailed {
		t.Fatalf("failed = %q, want FAILED", got)
	}
	if got := FactoryStateToDurableLifecycleStatus(interfaces.FactoryStateRunning); got != interfaces.FactorySessionLifecycleStatusRunning {
		t.Fatalf("running = %q, want RUNNING", got)
	}
}

func TestFactoryEventHistory_SessionLifecycleGuardsAndOptionalPayloads(t *testing.T) {
	t.Parallel()

	t0 := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionPaused(SessionLifecycleControlInput{}, t0)
	history.RecordSessionResumed(SessionLifecycleControlInput{}, t0)
	history.RecordSessionStarted(SessionLifecycleStartInput{}, t0)
	history.RecordSessionResultUpdated(SessionLifecycleResultInput{SessionID: "session-rich"}, t0)
	history.RecordSessionCompleted(SessionLifecycleCompleteInput{}, t0)
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID: "session-rich",
		Outcome:   interfaces.FactorySessionLifecycleControlOutcome("REJECTED"),
	}, t0)
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID: "session-rich",
		Outcome:   interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		Operation: interfaces.FactorySessionLifecycleControlKind("unsupported"),
	}, t0)
	history.RecordSessionLifecycleControl(SessionLifecycleControlInput{
		SessionID:      "session-rich",
		Outcome:        interfaces.FactorySessionLifecycleControlOutcomeAccepted,
		Operation:      interfaces.FactorySessionLifecycleControlPause,
		PreviousStatus: interfaces.FactorySessionLifecycleStatusRunning,
		NewStatus:      interfaces.FactorySessionLifecycleStatusRunning,
	}, t0)

	history.RecordSessionStarted(SessionLifecycleStartInput{
		SessionID:           "session-rich",
		OrchestratorKind:    interfaces.OrchestratorKindJavaScript,
		OrchestratorDialect: "workflow-v1",
		Source:              "runtime",
		FactoryID:           "factory-rich",
		SourceRef:           "workflow/main.js",
		SourceHash:          "sha256:source",
		PolicyHash:          "sha256:policy",
		ArgsDigest:          "sha256:args",
		Tick:                1,
	}, t0)
	// SESSION_STARTED is idempotent for a recording.
	history.RecordSessionStarted(SessionLifecycleStartInput{SessionID: "session-rich"}, t0.Add(time.Second))
	history.RecordSessionResultUpdated(SessionLifecycleResultInput{
		SessionID:        "session-rich",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		PhaseID:          "phase-review",
		PhaseName:        "review",
		Source:           "runtime",
		Tick:             2,
		ResultStatus:     interfaces.FactorySessionResultStatusFinal,
		ResultSummary: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: "final result",
		}},
		ArtifactIDs: []string{"artifact-1"},
	}, t0.Add(2*time.Second))

	resultStatus := interfaces.FactorySessionResultStatusFailedWithPartial
	history.RecordSessionCompleted(SessionLifecycleCompleteInput{
		SessionID:        "session-rich",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Source:           "runtime",
		Tick:             3,
		FinalStatus:      interfaces.FactorySessionLifecycleStatusFailed,
		ResultStatus:     &resultStatus,
		ArtifactIDs:      []string{"artifact-2"},
		DispatchCounts:   &interfaces.FactorySessionChildDispatchCounts{Queued: 1, Running: 2, Completed: 3},
		FailureDetail: &workerexecution.FailureDetail{
			Reason:  workerexecution.WorkFailureTypeUnknown,
			Message: "partial failure",
		},
	}, t0.Add(-time.Second))
	// SESSION_COMPLETED is also idempotent.
	history.RecordSessionCompleted(SessionLifecycleCompleteInput{SessionID: "session-rich"}, t0.Add(4*time.Second))

	events := history.CanonicalEvents()
	if len(events) != 3 || events[0].Type != interfaces.FactoryEventTypeSessionStarted ||
		events[1].Type != interfaces.FactoryEventTypeSessionResultUpdated ||
		events[2].Type != interfaces.FactoryEventTypeSessionCompleted {
		t.Fatalf("session lifecycle events = %#v, want started/result/completed", events)
	}
	var completed interfaces.FactorySessionCompletedEventPayload
	if err := events[2].DecodePayload(&completed); err != nil {
		t.Fatalf("decode completed payload: %v", err)
	}
	if completed.DurationMillis == nil || *completed.DurationMillis != 0 || completed.DispatchCounts == nil ||
		completed.DispatchCounts.Queued != 1 || completed.FailureDetail == nil || completed.FailureDetail.Message != "partial failure" {
		t.Fatalf("completed payload = %#v, want clamped duration and optional fields", completed)
	}
}

func assertSessionLifecycleEventType(
	t *testing.T,
	event factoryapi.FactoryEvent,
	wantType factoryapi.FactoryEventType,
	wantID string,
) {
	t.Helper()
	if event.Type != wantType {
		t.Fatalf("event type = %q, want %q", event.Type, wantType)
	}
	if event.Id != wantID {
		t.Fatalf("event id = %q, want %q", event.Id, wantID)
	}
}

func TestFactoryEventHistory_RecordSessionPauseResume_EmitsReconstructableControlStatus(t *testing.T) {
	t0 := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-live", &interfaces.FactoryConfig{
		Name: "factory-live",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindPetri,
		},
	}, 0, t0)
	history.RecordSessionPaused(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Source:           "runtime",
		Tick:             1,
	}, t0.Add(time.Second))
	history.RecordSessionResumed(SessionLifecycleControlInput{
		SessionID:        "session-live",
		OrchestratorKind: interfaces.OrchestratorKindPetri,
		Source:           "runtime",
		Tick:             2,
	}, t0.Add(2*time.Second))

	events := generatedHistoryEvents(t, history)
	if len(events) != 3 {
		t.Fatalf("events = %d, want started, paused, resumed", len(events))
	}
	assertSessionLifecycleEventType(t, events[1], factoryapi.FactoryEventTypeSessionPaused, "factory-event/session-paused/1")
	assertSessionLifecycleEventType(t, events[2], factoryapi.FactoryEventTypeSessionResumed, "factory-event/session-resumed/2")

	worldState, err := projections.ReconstructCanonicalFactoryWorldState(history.CanonicalEvents(), 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	if worldState.SessionBracket == nil {
		t.Fatal("session bracket = nil, want pause/resume lifecycle")
	}
	if worldState.SessionBracket.LifecycleControlStatus != string(factoryapi.FactorySessionDurableLifecycleStatusRunning) {
		t.Fatalf("lifecycle control status = %q, want RUNNING", worldState.SessionBracket.LifecycleControlStatus)
	}
	if worldState.SessionBracket.PausedAt.IsZero() || worldState.SessionBracket.ResumedAt.IsZero() {
		t.Fatalf("paused/resumed timestamps = %#v, want both set", worldState.SessionBracket)
	}
}

// TestFactoryEventHistory_EventRecorderFollowsCanonicalAppendOrder protects
// the ordering contract consumed by the portable recording writer. The
// callback for the first committed event is held while a second append is
// attempted; the second append must not reach the recorder until the first
// callback has returned.
func TestFactoryEventHistory_EventRecorderFollowsCanonicalAppendOrder(t *testing.T) {
	history := newTestFactoryEventHistory(
		eventHistoryProjectionNet(),
		func() time.Time { return time.Unix(0, 0).UTC() },
	)
	firstCallbackStarted := make(chan struct{})
	releaseFirstCallback := make(chan struct{})
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	var recordedMu sync.Mutex
	var recorded []int
	history.AddEventRecorder(func(event interfaces.FactoryEvent) {
		if event.Context.Sequence == 0 {
			close(firstCallbackStarted)
			<-releaseFirstCallback
		}
		recordedMu.Lock()
		recorded = append(recorded, event.Context.Sequence)
		recordedMu.Unlock()
	})

	go func() {
		history.RecordFactoryStateChange(
			1, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning,
			"first", time.Unix(1, 0).UTC(),
		)
		close(firstDone)
	}()
	select {
	case <-firstCallbackStarted:
	case <-time.After(time.Second):
		t.Fatal("first event recorder callback did not start")
	}

	go func() {
		history.RecordFactoryStateChange(
			2, interfaces.FactoryStateIdle, interfaces.FactoryStateRunning,
			"second", time.Unix(2, 0).UTC(),
		)
		close(secondDone)
	}()
	select {
	case <-secondDone:
		close(releaseFirstCallback)
		<-firstDone
		t.Fatal("second append reached the recorder before the first callback returned")
	case <-time.After(time.Second):
	}

	close(releaseFirstCallback)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first append did not finish after releasing its recorder callback")
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second append did not finish after releasing the first callback")
	}

	recordedMu.Lock()
	defer recordedMu.Unlock()
	if len(recorded) != 2 || recorded[0] != 0 || recorded[1] != 1 {
		t.Fatalf("recorded callback sequence = %#v, want [0 1]", recorded)
	}
}

func TestFactoryEventHistory_CurrentSessionProjectionFactsAreDetachedAndIncremental(t *testing.T) {
	t0 := time.Date(2026, 8, 23, 17, 0, 0, 0, time.UTC)
	history := newTestFactoryEventHistory(nil, func() time.Time { return t0 })
	history.RecordSessionLifecycleFromFactoryConfig("session-js", &interfaces.FactoryConfig{
		Name: "factory-js",
		Orchestrator: &interfaces.FactoryOrchestratorConfig{
			Kind: interfaces.OrchestratorKindJavaScript,
		},
	}, 0, t0)
	history.RecordOrchestratorPhaseChanged(OrchestratorPhaseChangedInput{
		SessionID:        "session-js",
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		PhaseID:          "phase-plan",
		PhaseName:        "plan",
		Source:           "runtime",
		Tick:             1,
		PhaseStatus:      interfaces.OrchestratorPhaseStatusActive,
	}, t0.Add(time.Second))

	record := interfaces.FactoryDispatchRecord{
		DispatchID:    "dispatch-approval",
		HumanApproval: true,
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-approval",
			TransitionID:    "approval-workstation",
			WorkstationName: "Release Approval",
			Execution:       work.ExecutionMetadata{RequestID: "request-approval"},
			InputTokens: workerexecution.InputTokens(workerexecution.Token{
				ID:    "token-approval",
				Color: workerexecution.Color{WorkID: "work-approval", TraceID: "trace-approval"},
			}),
		},
	}
	history.RecordWorkstationRequest(2, record, t0.Add(2*time.Second))
	history.RecordHumanApprovalRequested(2, record, t0.Add(2*time.Second))

	facts := currentSessionProjectionFactsForTest(t, history, "initial")
	assertSessionProjectionFacts(t, facts)
	assertSessionProjectionFactsAreDetached(t, history, facts)

	history.RecordDispatchReconciled(DispatchReconciledInput{
		SessionID:            "session-js",
		DispatchID:           "dispatch-approval",
		Tick:                 3,
		ReconciledStatus:     interfaces.FactoryDispatchStatusCompleted,
		ReconciliationSource: interfaces.DispatchReconciliationSource("RUNTIME_RECONCILER"),
	}, t0.Add(3*time.Second))
	resolved := currentSessionProjectionFactsForTest(t, history, "resolution")
	if len(resolved.PendingHumanApprovals) != 0 {
		t.Fatalf("resolved approvals = %#v, want none", resolved.PendingHumanApprovals)
	}
}

func currentSessionProjectionFactsForTest(
	t *testing.T,
	history *FactoryEventHistory,
	phase string,
) recordings.SessionProjectionFacts {
	t.Helper()
	facts, err := history.CurrentSessionProjectionFacts()
	if err != nil {
		t.Fatalf("CurrentSessionProjectionFacts() %s error = %v", phase, err)
	}
	return facts
}

func assertSessionProjectionFacts(t *testing.T, facts recordings.SessionProjectionFacts) {
	t.Helper()
	if facts.SessionBracket == nil || facts.SessionBracket.SessionID != "session-js" {
		t.Fatalf("session bracket = %#v, want session-js", facts.SessionBracket)
	}
	if facts.JavaScriptRuntime == nil || facts.JavaScriptRuntime.Phase != "plan" {
		t.Fatalf("JavaScript runtime = %#v, want phase plan", facts.JavaScriptRuntime)
	}
	approval, ok := facts.PendingHumanApprovals["approval-dispatch-approval"]
	if !ok || approval.SessionID != "session-js" || approval.RequestID != "request-approval" ||
		approval.WorkstationID != "approval-workstation" || len(approval.WorkItemIDs) != 1 || approval.WorkItemIDs[0] != "work-approval" {
		t.Fatalf("pending approval = %#v, want stable correlated approval", facts.PendingHumanApprovals)
	}
}

func assertSessionProjectionFactsAreDetached(
	t *testing.T,
	history *FactoryEventHistory,
	facts recordings.SessionProjectionFacts,
) {
	t.Helper()
	approval := facts.PendingHumanApprovals["approval-dispatch-approval"]
	if len(approval.Decisions) == 0 || len(approval.WorkItemIDs) == 0 || facts.JavaScriptRuntime == nil || len(facts.JavaScriptRuntime.Phases) == 0 {
		t.Fatalf("projection facts missing detached values: %#v", facts)
	}
	approval.WorkItemIDs[0] = "mutated"
	approval.Decisions[0] = "MUTATED"
	facts.PendingHumanApprovals["approval-dispatch-approval"] = approval
	facts.JavaScriptRuntime.Phases[0] = "mutated"
	next := currentSessionProjectionFactsForTest(t, history, "detachment")
	nextApproval := next.PendingHumanApprovals["approval-dispatch-approval"]
	if len(nextApproval.WorkItemIDs) == 0 || len(nextApproval.Decisions) == 0 || next.JavaScriptRuntime == nil || len(next.JavaScriptRuntime.Phases) == 0 {
		t.Fatalf("detached projection facts lost values: %#v", next)
	}
	if nextApproval.WorkItemIDs[0] != "work-approval" || nextApproval.Decisions[0] != interfaces.HumanApprovalDecisionApprove ||
		next.JavaScriptRuntime.Phases[0] != "plan" {
		t.Fatalf("projection leaked mutable read state: %#v / %#v", nextApproval, next.JavaScriptRuntime)
	}
}
