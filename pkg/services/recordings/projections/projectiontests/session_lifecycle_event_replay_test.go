package projections_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/recordings/projections"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestReconstructFactoryWorldState_SuccessfulSessionBracketReconstructsLifecycle(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	events := successfulSessionBracketEvents(t, t0)

	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	assertSuccessfulSessionBracketReplay(t, worldState)

	view := BuildFactoryWorldView(worldState)
	if view.Runtime.Session.Bracket == nil {
		t.Fatal("session bracket projection = nil, want lifecycle bracket")
	}
	if !view.Runtime.Session.Bracket.Terminal || view.Runtime.Session.Bracket.FinalStatus != string(factoryapi.FactorySessionDurableLifecycleStatusSucceeded) {
		t.Fatalf("session bracket projection = %#v, want terminal FINISHED", view.Runtime.Session.Bracket)
	}
	if view.Runtime.Session.Bracket.ResultStatus != string(factoryapi.FactoryEventSessionResultStatusFinal) {
		t.Fatalf("session bracket result status = %q, want FINAL", view.Runtime.Session.Bracket.ResultStatus)
	}
}

func TestReconstructFactoryWorldState_RunningSessionNotReadyResultStatusRoundTrips(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 2, 0, 0, time.UTC)
	sessionID := "session-running"
	kind := factoryapi.JAVASCRIPT
	source := "api"
	events := []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionStarted, "event-session-started-running", 1, t0, factoryapi.FactoryEventContext{
			Sequence:         1,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(0),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionStartedEventPayload{
			FactoryId: stringPointer("factory-running"),
			StartedAt: t0,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "event-session-result-not-ready", 2, t0.Add(time.Second), factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(1),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResultUpdatedEventPayload{
			ResultStatus: factoryapi.FactoryEventSessionResultStatusNotReady,
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 2)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	bracket := worldState.SessionBracket
	if bracket == nil || bracket.Terminal {
		t.Fatalf("session bracket = %#v, want non-terminal running lifecycle", bracket)
	}
	if bracket.ResultStatus != string(factoryapi.FactoryEventSessionResultStatusNotReady) {
		t.Fatalf("result status = %q, want NOT_READY", bracket.ResultStatus)
	}
}

func TestReconstructFactoryWorldState_CanceledSessionUnavailableResultStatusRoundTrips(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 3, 0, 0, time.UTC)
	sessionID := "session-canceled"
	kind := factoryapi.PETRI
	source := "api"
	events := []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionStarted, "event-session-started-canceled", 1, t0, factoryapi.FactoryEventContext{
			Sequence:         1,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(0),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionStartedEventPayload{
			FactoryId: stringPointer("factory-canceled"),
			StartedAt: t0,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "event-session-result-unavailable", 2, t0.Add(time.Second), factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(1),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResultUpdatedEventPayload{
			ResultStatus: factoryapi.FactoryEventSessionResultStatusUnavailable,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionCompleted, "event-session-completed-canceled", 3, t0.Add(2*time.Second), factoryapi.FactoryEventContext{
			Sequence:         3,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(2),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionCompletedEventPayload{
			FinalStatus:    factoryapi.FactorySessionDurableLifecycleStatusCanceled,
			CompletedAt:    t0.Add(2 * time.Second),
			DurationMillis: int64PtrForProjectionTest(2000),
			ResultStatus:   factoryEventSessionResultStatusPtr(factoryapi.FactoryEventSessionResultStatusUnavailable),
		}),
	}

	worldState, err := ReconstructFactoryWorldState(events, 3)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	bracket := worldState.SessionBracket
	if bracket == nil || !bracket.Terminal {
		t.Fatalf("session bracket = %#v, want terminal canceled lifecycle", bracket)
	}
	if bracket.FinalStatus != string(factoryapi.FactorySessionDurableLifecycleStatusCanceled) {
		t.Fatalf("final status = %q, want CANCELED", bracket.FinalStatus)
	}
	if bracket.ResultStatus != string(factoryapi.FactoryEventSessionResultStatusUnavailable) {
		t.Fatalf("result status = %q, want UNAVAILABLE", bracket.ResultStatus)
	}
}

func TestReconstructFactoryWorldState_FailedWithPartialSessionBracketReconstructsLifecycle(t *testing.T) {
	t0 := time.Date(2026, 6, 9, 12, 5, 0, 0, time.UTC)
	events := failedWithPartialSessionBracketEvents(t, t0)

	worldState, err := ReconstructFactoryWorldState(events, 4)
	if err != nil {
		t.Fatalf("ReconstructFactoryWorldState: %v", err)
	}
	bracket := worldState.SessionBracket
	if bracket == nil {
		t.Fatal("session bracket = nil, want failed-with-partial lifecycle")
	}
	if bracket.ResultStatus != string(factoryapi.FactoryEventSessionResultStatusFailedWithPartial) {
		t.Fatalf("result status = %q, want FAILED_WITH_PARTIAL", bracket.ResultStatus)
	}
	if len(bracket.ResultSummary) != 1 || bracket.ResultSummary[0].Text != "Partial findings before failure" {
		t.Fatalf("result summary = %#v, want partial text summary", bracket.ResultSummary)
	}
	if !bracket.Terminal || bracket.FailureDetail == nil || bracket.FailureDetail.Reason != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("terminal failure = %#v, want terminal with normalized unknown reason", bracket)
	}
}

func assertSuccessfulSessionBracketReplay(t *testing.T, worldState interfaces.FactoryWorldState) {
	t.Helper()
	bracket := worldState.SessionBracket
	if bracket == nil {
		t.Fatal("session bracket = nil, want started lifecycle")
	}
	if bracket.SessionID != "session-alpha" || bracket.OrchestratorKind != string(factoryapi.JAVASCRIPT) {
		t.Fatalf("session identity = %#v, want session-alpha JAVASCRIPT", bracket)
	}
	if bracket.FactoryID != "factory-alpha" || bracket.SourceRef != "workflow/main.js" {
		t.Fatalf("session source = %#v, want factory-alpha workflow/main.js", bracket)
	}
	if bracket.ResultStatus != string(factoryapi.FactoryEventSessionResultStatusFinal) {
		t.Fatalf("latest result status = %q, want FINAL from terminal completion", bracket.ResultStatus)
	}
	if !bracket.Terminal || bracket.FinalStatus != string(factoryapi.FactorySessionDurableLifecycleStatusSucceeded) {
		t.Fatalf("terminal state = %#v, want SUCCEEDED terminal marker", bracket)
	}
	if bracket.DispatchCounts == nil || bracket.DispatchCounts.Completed != 2 {
		t.Fatalf("dispatch counts = %#v, want completed=2", bracket.DispatchCounts)
	}
}

func successfulSessionBracketEvents(t *testing.T, t0 time.Time) []factoryapi.FactoryEvent {
	t.Helper()
	sessionID := "session-alpha"
	kind := factoryapi.JAVASCRIPT
	dialect := "workflow-v1"
	source := "api"
	return []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionStarted, "event-session-started", 1, t0, factoryapi.FactoryEventContext{
			Sequence:            1,
			SessionId:           &sessionID,
			SessionSequence:     intPtrForProjectionTest(0),
			OrchestratorKind:    &kind,
			OrchestratorDialect: &dialect,
			Source:              &source,
		}, factoryapi.SessionStartedEventPayload{
			FactoryId:  stringPointer("factory-alpha"),
			SourceRef:  stringPointer("workflow/main.js"),
			SourceHash: stringPointer("sha256:source"),
			PolicyHash: stringPointer("sha256:policy"),
			ArgsDigest: stringPointer("sha256:args"),
			StartedAt:  t0,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "event-session-result-updated", 2, t0.Add(time.Second), factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(1),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResultUpdatedEventPayload{
			ResultStatus: factoryapi.FactoryEventSessionResultStatusPartial,
			ArtifactIds:  &[]string{"artifact-partial-1"},
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionCompleted, "event-session-completed", 3, t0.Add(2*time.Second), factoryapi.FactoryEventContext{
			Sequence:         3,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(2),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionCompletedEventPayload{
			FinalStatus:    factoryapi.FactorySessionDurableLifecycleStatusSucceeded,
			CompletedAt:    t0.Add(2 * time.Second),
			DurationMillis: int64PtrForProjectionTest(2000),
			ResultStatus:   factoryEventSessionResultStatusPtr(factoryapi.FactoryEventSessionResultStatusFinal),
			ArtifactIds:    &[]string{"artifact-result-1"},
			DispatchCounts: &factoryapi.FactorySessionJavaScriptChildDispatchCounts{
				Queued:    0,
				Running:   0,
				Completed: 2,
			},
		}),
	}
}

func failedWithPartialSessionBracketEvents(t *testing.T, t0 time.Time) []factoryapi.FactoryEvent {
	t.Helper()
	sessionID := "session-beta"
	kind := factoryapi.PETRI
	source := "api"
	summary := []factoryapi.WorkContentPart{}
	textPart := factoryapi.WorkContentPart{}
	if err := textPart.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: "Partial findings before failure",
	}); err != nil {
		t.Fatalf("build text result summary: %v", err)
	}
	summary = append(summary, textPart)
	return []factoryapi.FactoryEvent{
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionStarted, "event-session-started-beta", 1, t0, factoryapi.FactoryEventContext{
			Sequence:         1,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(0),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionStartedEventPayload{
			FactoryId: stringPointer("factory-beta"),
			StartedAt: t0,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "event-session-result-updated-beta", 2, t0.Add(time.Second), factoryapi.FactoryEventContext{
			Sequence:         2,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(1),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResultUpdatedEventPayload{
			ResultStatus:  factoryapi.FactoryEventSessionResultStatusPartial,
			ResultSummary: &summary,
			ArtifactIds:   &[]string{"artifact-partial-beta"},
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionResultUpdated, "event-session-result-updated-beta-final", 3, t0.Add(2*time.Second), factoryapi.FactoryEventContext{
			Sequence:         3,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(2),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionResultUpdatedEventPayload{
			ResultStatus:  factoryapi.FactoryEventSessionResultStatusFailedWithPartial,
			ResultSummary: &summary,
		}),
		generatedProjectionEvent(factoryapi.FactoryEventTypeSessionCompleted, "event-session-completed-beta", 4, t0.Add(3*time.Second), factoryapi.FactoryEventContext{
			Sequence:         4,
			SessionId:        &sessionID,
			SessionSequence:  intPtrForProjectionTest(3),
			OrchestratorKind: &kind,
			Source:           &source,
		}, factoryapi.SessionCompletedEventPayload{
			FinalStatus:    factoryapi.FactorySessionDurableLifecycleStatusFailed,
			CompletedAt:    t0.Add(3 * time.Second),
			DurationMillis: int64PtrForProjectionTest(3000),
			ResultStatus:   factoryEventSessionResultStatusPtr(factoryapi.FactoryEventSessionResultStatusFailedWithPartial),
			FailureDetail: &factoryapi.FailureDetail{
				Reason:  factoryapi.WorkFailureTypeUnknown,
				Message: "workflow execution failed after partial results",
			},
		}),
	}
}

func factoryEventSessionResultStatusPtr(value factoryapi.FactoryEventSessionResultStatus) *factoryapi.FactoryEventSessionResultStatus {
	return &value
}
