// backendsizecheck:ignore-file consolidated fake-service contract and projection tests remain together until dedicated execution test seams split.
// pkgmaintcheck:ignore-file-lines consolidated fake-service contract and projection tests remain together until dedicated execution test seams split.
package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/fileeffects"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestBuildCanonicalSessionEvents_RunningAndTerminalSessions(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 8, 14, 10, 0, 0, time.UTC)

	runningEvents := BuildCanonicalSessionEvents(
		SessionReadResult{
			SessionID:        "dur-sess-js-run-n-001",
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			Phase:            "verify",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    "dur-sess-js-run-n-001",
			ResultStatus: ResultStatusPartial,
		},
	)
	if len(runningEvents) != 2 {
		t.Fatalf("running events = %d, want 2", len(runningEvents))
	}
	assertCanonicalEventEnvelope(t, runningEvents[0], "SESSION_STARTED", "session-started/dur-sess-js-run-n-001")
	assertCanonicalEventEnvelope(t, runningEvents[1], "SESSION_RESULT_UPDATED", "session-result-updated/dur-sess-js-run-n-001")

	terminalEvents := BuildCanonicalSessionEvents(
		SessionReadResult{
			SessionID:        "dur-sess-js-success-002",
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
		},
		ResultReadResult{
			SessionID:    "dur-sess-js-success-002",
			ResultStatus: ResultStatusFinal,
		},
	)
	if len(terminalEvents) != 3 {
		t.Fatalf("terminal events = %d, want 3", len(terminalEvents))
	}
	assertCanonicalEventEnvelope(t, terminalEvents[2], "SESSION_COMPLETED", "session-completed/dur-sess-js-success-002")
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

// pkgmaintcheck:ignore-cyclomatic-complexity this focused projection regression keeps live and persisted-record assertions together.
func TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail(t *testing.T) {
	t.Parallel()
	retryable := true
	records := []factory.JavaScriptRuntimeRecord{
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        factory.JavaScriptChildDispatchStatusQueued,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        factory.JavaScriptChildDispatchStatusRunning,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
			},
		},
		{
			Kind: factory.JavaScriptRecordKindChildDispatch,
			ChildDispatch: &factory.JavaScriptChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        factory.JavaScriptChildDispatchStatusFailed,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
				FailureDetail: &workerexecution.FailureDetail{
					Reason:  workerexecution.WorkFailureTypeUnknown,
					Message: "live child failed: simulated child error",
				},
				Attempt:               3,
				Retryable:             &retryable,
				FailureClassification: workerexecution.WorkFailureTypeTimeout,
			},
		},
	}

	observedAt := time.Date(2026, 6, 16, 14, 0, 0, 0, time.UTC)
	projection := ProjectRuntimeExecutionRecords("session-live-child-failure", records, observedAt)
	if len(projection.Dispatches) != 1 {
		t.Fatalf("dispatches = %#v, want one failed dispatch", projection.Dispatches)
	}
	dispatch := projection.Dispatches[0]
	if dispatch.Status != DispatchStatusFailed {
		t.Fatalf("dispatch status = %q, want FAILED", dispatch.Status)
	}
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Reason != string(workerexecution.WorkFailureTypeUnknown) {
		t.Fatalf("failureDetail = %#v", dispatch.FailureDetail)
	}
	if dispatch.FailureDetail.Message != "live child failed: simulated child error" {
		t.Fatalf("failure message = %q", dispatch.FailureDetail.Message)
	}
	if dispatch.Attempt != 3 || dispatch.Retryable == nil || !*dispatch.Retryable || dispatch.FailureClassification != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("retry diagnostics = attempt:%d retryable:%v classification:%q", dispatch.Attempt, dispatch.Retryable, dispatch.FailureClassification)
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal records: %v", err)
	}
	var replayedRecords []factory.JavaScriptRuntimeRecord
	if err := json.Unmarshal(encoded, &replayedRecords); err != nil {
		t.Fatalf("unmarshal records: %v", err)
	}
	replayed := ProjectRuntimeExecutionRecords("session-live-child-failure", replayedRecords, observedAt).Dispatches[0]
	if replayed.Attempt != dispatch.Attempt || replayed.Retryable == nil || *replayed.Retryable != *dispatch.Retryable || replayed.FailureClassification != dispatch.FailureClassification {
		t.Fatalf("replayed retry diagnostics = %#v, want %#v", replayed, dispatch)
	}
	if len(projection.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want none for failed child", projection.Artifacts)
	}
	transitions := projection.DispatchStatusTransitions["dispatch-2"]
	if len(transitions) != 3 || transitions[2] != DispatchStatusFailed {
		t.Fatalf("statusTransitions = %#v, want queued/running/failed", transitions)
	}
}

func TestFilterEventsAfterReconnect_AfterEventIDAndSequence(t *testing.T) {
	t.Parallel()
	events := []json.RawMessage{
		json.RawMessage(`{"id":"session-started/s1","context":{"sequence":1,"sessionSequence":0}}`),
		json.RawMessage(`{"id":"session-result-updated/s1","context":{"sequence":2,"sessionSequence":1}}`),
		json.RawMessage(`{"id":"session-completed/s1","context":{"sequence":3,"sessionSequence":2}}`),
	}

	all, err := FilterEventsAfterReconnect(events, EventReconnectRequest{}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("all events = %d, want 3", len(all))
	}

	afterStart, err := FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterEventID: "session-started/s1",
	}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect after start: %v", err)
	}
	if len(afterStart) != 2 {
		t.Fatalf("after start events = %d, want 2", len(afterStart))
	}

	sequence := 1
	afterSequence, err := FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterSequence: &sequence,
	}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect after sequence: %v", err)
	}
	if len(afterSequence) != 1 {
		t.Fatalf("after sequence events = %d, want 1", len(afterSequence))
	}

	eventsWithoutSessionSequence := append([]json.RawMessage(nil), events...)
	eventsWithoutSessionSequence[1] = json.RawMessage(
		`{"id":"session-result-updated/s1","context":{"sequence":42}}`,
	)
	canonicalSequence := 42
	afterCanonicalSequence, err := FilterEventsAfterReconnect(eventsWithoutSessionSequence, EventReconnectRequest{
		AfterSequence: &canonicalSequence,
	}, "s1")
	if err != nil {
		t.Fatalf("FilterEventsAfterReconnect canonical sequence fallback: %v", err)
	}
	if len(afterCanonicalSequence) != 1 {
		t.Fatalf("after canonical sequence events = %d, want 1", len(afterCanonicalSequence))
	}

	_, err = FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterEventID: "missing-event",
	}, "s1")
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("missing cursor error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func TestReplaySessionProjection_TerminalSessionBracket(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2026, 6, 11, 12, 0, 2, 0, time.UTC)
	sessionID := "dur-sess-replay-001"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			Dialect:          "you-workflow-v1",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource: ResolvedSource{
				SourceRef:  "workflow/simple-final",
				SourceHash: "sha256:fixture",
			},
			ResultSummary: &ResultSummary{
				ResultStatus: string(ResultStatusFinal),
				Summary:      "Completed simple workflow.",
			},
			Lifecycle: &LifecycleTimestamps{
				StartedAt:  &startedAt,
				FinishedAt: &finishedAt,
			},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
		},
	)

	session, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
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
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		},
	)

	firstSession, firstResult, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection first: %v", err)
	}
	secondSession, secondResult, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection second: %v", err)
	}
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
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusSucceeded,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusFinal,
			ArtifactIDs:  []string{"art-1", "art-2"},
		},
	)

	session, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if len(session.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs = %d, want 2", len(session.ArtifactRefs))
	}

	// Replaying the same events again must not duplicate artifact stubs.
	sessionAgain, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection again: %v", err)
	}
	if len(sessionAgain.ArtifactRefs) != 2 {
		t.Fatalf("artifact refs after second replay = %d, want 2", len(sessionAgain.ArtifactRefs))
	}
}

func TestReplaySessionProjection_FirstTerminalOutcomeWinsCompetingRace(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 12, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)
	sessionID := "dur-sess-replay-terminal-race"
	base := SessionReadResult{
		SessionID: sessionID, Status: LifecycleStatusCanceled, OrchestratorKind: "JAVASCRIPT",
		Lifecycle: &LifecycleTimestamps{StartedAt: &startedAt, FinishedAt: &finishedAt},
	}
	canceled := BuildCanonicalRuntimeSessionEvents(base, ResultReadResult{
		SessionID: sessionID, ResultStatus: ResultStatusUnavailable, SessionStatus: LifecycleStatusCanceled,
	})
	base.Status = LifecycleStatusFailed
	failed := BuildCanonicalRuntimeSessionEvents(base, ResultReadResult{
		SessionID: sessionID, ResultStatus: ResultStatusFailedWithPartial, SessionStatus: LifecycleStatusFailed,
	})

	session, result, err := ReplaySessionProjection(append(canceled, failed...))
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.Status != LifecycleStatusCanceled || result.SessionStatus != LifecycleStatusCanceled || result.ResultStatus != ResultStatusUnavailable {
		t.Fatalf("late terminal outcome overwrote cancellation: session=%#v result=%#v", session, result)
	}
}

func TestReplaySessionProjection_PreservesSyncTimeoutAvailability(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-timeout-availability"
	events := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
			Availability: &ResultAvailabilityDetail{
				Reason:    "SYNC_WAIT_TIMED_OUT",
				Message:   "Sync wait ended before a terminal result was available.",
				Retryable: true,
			},
		},
	)

	_, result, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if result.Availability == nil || result.Availability.Reason != "SYNC_WAIT_TIMED_OUT" {
		t.Fatalf("availability = %#v, want SYNC_WAIT_TIMED_OUT", result.Availability)
	}
}

func TestReplaySessionProjection_IgnoresUnknownEventTypes(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	sessionID := "dur-sess-replay-004"
	base := BuildCanonicalRuntimeSessionEvents(
		SessionReadResult{
			SessionID:        sessionID,
			Status:           LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT",
			SourceHash:       "sha256:fixture",
			Policy:           PolicyProjection{EffectiveHash: "sha256:policy"},
			ResolvedSource:   ResolvedSource{SourceHash: "sha256:fixture"},
			Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
		},
		ResultReadResult{
			SessionID:    sessionID,
			ResultStatus: ResultStatusNotReady,
		},
	)
	events := append(append([]json.RawMessage(nil), base...), json.RawMessage(`{"type":"DISPATCH_QUEUED","id":"dispatch-queued/1","context":{"sequence":99},"payload":{}}`))

	session, _, err := ReplaySessionProjection(events)
	if err != nil {
		t.Fatalf("ReplaySessionProjection: %v", err)
	}
	if session.SessionID != sessionID {
		t.Fatalf("sessionId = %q, want %q", session.SessionID, sessionID)
	}
}

func TestReplaySessionProjection_EquivalentOrchestratorsSharePublicSessionProjection(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 18, 30, 0, 0, time.UTC)
	sessionID := "dur-sess-orchestrator-parity-001"
	baseSession := SessionReadResult{
		SessionID:      sessionID,
		Status:         LifecycleStatusRunning,
		SourceHash:     "sha256:equivalent-source",
		Policy:         PolicyProjection{EffectiveHash: "sha256:equivalent-policy"},
		ResolvedSource: ResolvedSource{SourceHash: "sha256:equivalent-source"},
		Lifecycle:      &LifecycleTimestamps{StartedAt: &startedAt},
		Phase:          "execute",
	}
	result := ResultReadResult{SessionID: sessionID, ResultStatus: ResultStatusNotReady}

	petriSession := baseSession
	petriSession.OrchestratorKind = "PETRI"
	petriEvents := BuildCanonicalRuntimeSessionEvents(petriSession, result)
	petriPayload := factoryapi.DispatchRequestEventPayload{
		Inputs:       []factoryapi.DispatchConsumedWorkRef{},
		TransitionId: "transition-review",
	}
	petriEvents = append(petriEvents, canonicalTypedInternalEvent(t, "DISPATCH_REQUEST", sessionID, petriPayload))

	javascriptSession := baseSession
	javascriptSession.OrchestratorKind = "JAVASCRIPT"
	javascriptSession.Dialect = "you-workflow-v1"
	javascriptEvents := BuildCanonicalRuntimeSessionEvents(javascriptSession, result)
	javascriptPayload := factoryapi.OrchestratorCheckpointWrittenEventPayload{
		Label:              "after-review",
		ResumabilityStatus: factoryapi.RESUMABLE,
		Timestamp:          &startedAt,
	}
	javascriptEvents = append(javascriptEvents, canonicalTypedInternalEvent(t, "ORCHESTRATOR_CHECKPOINT_WRITTEN", sessionID, javascriptPayload))

	petriProjection, _, err := ReplaySessionProjection(petriEvents)
	if err != nil {
		t.Fatalf("ReplaySessionProjection(PETRI): %v", err)
	}
	javascriptProjection, _, err := ReplaySessionProjection(javascriptEvents)
	if err != nil {
		t.Fatalf("ReplaySessionProjection(JAVASCRIPT): %v", err)
	}

	type sharedPublicSession struct {
		SessionID     string
		Status        LifecycleStatus
		SourceHash    string
		PolicyHash    string
		Phase         string
		StartedAt     time.Time
		ResultStatus  string
		ArtifactCount int
		Links         InspectionLinks
	}
	sharedProjection := func(session SessionReadResult) sharedPublicSession {
		t.Helper()
		if session.Lifecycle == nil || session.Lifecycle.StartedAt == nil {
			t.Fatalf("lifecycle = %#v, want startedAt", session.Lifecycle)
		}
		if session.ResultSummary == nil {
			t.Fatal("resultSummary missing")
		}
		return sharedPublicSession{
			SessionID:     session.SessionID,
			Status:        session.Status,
			SourceHash:    session.SourceHash,
			PolicyHash:    session.Policy.EffectiveHash,
			Phase:         session.Phase,
			StartedAt:     session.Lifecycle.StartedAt.UTC(),
			ResultStatus:  session.ResultSummary.ResultStatus,
			ArtifactCount: session.ArtifactCount,
			Links:         session.Links,
		}
	}
	petriPublic := sharedProjection(petriProjection)
	javascriptPublic := sharedProjection(javascriptProjection)
	if petriPublic != javascriptPublic {
		t.Fatalf("shared public session projection differs by orchestrator:\nPETRI: %#v\nJAVASCRIPT: %#v", petriPublic, javascriptPublic)
	}
	if petriProjection.OrchestratorKind != "PETRI" || javascriptProjection.OrchestratorKind != "JAVASCRIPT" {
		t.Fatalf("orchestrator identity lost: PETRI=%q JAVASCRIPT=%q", petriProjection.OrchestratorKind, javascriptProjection.OrchestratorKind)
	}
	if petriPayload.TransitionId == "" || javascriptPayload.Label == "" {
		t.Fatal("typed orchestrator-specific facts missing")
	}
}

func TestReplayDispatchProjection_EquivalentOrchestratorsMatchLiveDispatchSummary(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 20, 0, 0, 0, time.UTC)
	providerRef := ProviderSessionRef{Provider: "mock", Kind: "session_id", ID: "provider-session-parity-1"}

	for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
		t.Run(orchestratorKind, func(t *testing.T) {
			session := SessionReadResult{
				SessionID:        "dispatch-parity-" + strings.ToLower(orchestratorKind),
				Status:           LifecycleStatusSucceeded,
				OrchestratorKind: orchestratorKind,
				Phase:            "execute",
				Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
			}
			live := DispatchSummary{
				ID:                  "dispatch-equivalent-1",
				Status:              DispatchStatusCompleted,
				DispatchKind:        "AGENT",
				Phase:               "execute",
				Label:               "summarize findings",
				RunnerID:            "worker-summary",
				Model:               "mock-model",
				Provider:            "mock",
				ProviderSessionRefs: []ProviderSessionRef{providerRef},
				OutputArtifactIDs:   []string{"artifact-equivalent-1"},
			}
			events := BuildCanonicalRuntimeSessionEvents(
				session,
				ResultReadResult{SessionID: session.SessionID, ResultStatus: ResultStatusFinal},
				RuntimeDispatchEventInput{Dispatches: []DispatchSummary{live}},
			)

			replayed, err := ReplayDispatchProjection(events)
			if err != nil {
				t.Fatalf("ReplayDispatchProjection: %v", err)
			}
			if len(replayed) != 1 || !reflect.DeepEqual(replayed[0], live) {
				t.Fatalf("replayed dispatch = %#v, want live projection %#v", replayed, live)
			}
		})
	}
}

func TestReplayDispatchProjection_EquivalentOrchestratorsPreserveAbsentProviderSession(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 20, 5, 0, 0, time.UTC)
	for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
		t.Run(orchestratorKind, func(t *testing.T) {
			session := SessionReadResult{
				SessionID:        "dispatch-no-provider-" + strings.ToLower(orchestratorKind),
				Status:           LifecycleStatusFailed,
				OrchestratorKind: orchestratorKind,
				Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
			}
			live := DispatchSummary{ID: "dispatch-no-provider", Status: DispatchStatusFailed, DispatchKind: "AGENT"}
			events := BuildCanonicalRuntimeSessionEvents(
				session,
				ResultReadResult{SessionID: session.SessionID, ResultStatus: ResultStatusUnavailable},
				RuntimeDispatchEventInput{Dispatches: []DispatchSummary{live}},
			)
			replayed, err := ReplayDispatchProjection(events)
			if err != nil {
				t.Fatalf("ReplayDispatchProjection: %v", err)
			}
			if len(replayed) != 1 || len(replayed[0].ProviderSessionRefs) != 0 {
				t.Fatalf("replayed dispatch = %#v, want absent provider session refs", replayed)
			}
		})
	}
}

func TestReplaySessionProjection_EquivalentOrchestratorsRestoreArtifactsAndLatestLifecycle(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 21, 0, 0, 0, time.UTC)
	pausedAt := startedAt.Add(time.Minute)
	resumedAt := pausedAt.Add(time.Minute)
	artifact := map[string]any{
		"id":          "artifact-parity-1",
		"kind":        "FINAL_RESULT",
		"visibility":  "PUBLIC",
		"contentHash": "sha256:artifact-parity",
		"sizeBytes":   128,
	}

	for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
		t.Run(orchestratorKind, func(t *testing.T) {
			sessionID := "artifact-parity-" + strings.ToLower(orchestratorKind)
			session := SessionReadResult{
				SessionID:        sessionID,
				Status:           LifecycleStatusRunning,
				OrchestratorKind: orchestratorKind,
				Lifecycle: &LifecycleTimestamps{
					StartedAt: &startedAt,
					PausedAt:  &pausedAt,
					ResumedAt: &resumedAt,
				},
			}
			events := BuildCanonicalRuntimeSessionEvents(session, ResultReadResult{
				SessionID:    sessionID,
				ResultStatus: ResultStatusPartial,
				ArtifactIDs:  []string{"artifact-parity-1"},
			})
			artifactEvent := canonicalTypedInternalEvent(t, "ARTIFACT_CREATED", sessionID, map[string]any{
				"artifact":   artifact,
				"capturedAt": resumedAt.Format(time.RFC3339),
			})
			var artifactEnvelope map[string]any
			if err := json.Unmarshal(artifactEvent, &artifactEnvelope); err != nil {
				t.Fatalf("unmarshal artifact envelope: %v", err)
			}
			context := artifactEnvelope["context"].(map[string]any)
			context["orchestratorKind"] = orchestratorKind
			artifactEvent, _ = json.Marshal(artifactEnvelope)

			internalType := "DISPATCH_REQUESTED"
			internalPayload := map[string]any{"transitionId": "transition-1"}
			if orchestratorKind == "JAVASCRIPT" {
				internalType = "ORCHESTRATOR_CHECKPOINT_WRITTEN"
				internalPayload = map[string]any{"checkpointId": "checkpoint-1"}
			}
			events = append(events, artifactEvent, canonicalTypedInternalEvent(t, internalType, sessionID, internalPayload))

			replayed, _, err := ReplaySessionProjection(events)
			if err != nil {
				t.Fatalf("ReplaySessionProjection: %v", err)
			}
			wantRef := ArtifactRefSummary{ID: "artifact-parity-1", Kind: "FINAL_RESULT", Visibility: "PUBLIC", ContentHash: "sha256:artifact-parity", SizeBytes: 128}
			if replayed.ArtifactCount != 1 || !reflect.DeepEqual(replayed.ArtifactRefs, []ArtifactRefSummary{wantRef}) {
				t.Fatalf("artifact projection = count %d refs %#v, want %#v", replayed.ArtifactCount, replayed.ArtifactRefs, wantRef)
			}
			if replayed.Status != LifecycleStatusRunning || replayed.Lifecycle == nil || !replayed.Lifecycle.ResumedAt.Equal(resumedAt) {
				t.Fatalf("lifecycle = status %q timestamps %#v, want latest RUNNING at %s", replayed.Status, replayed.Lifecycle, resumedAt)
			}
		})
	}
}

func TestReplaySessionProjection_EquivalentOrchestratorsPreserveResultAvailability(t *testing.T) {
	t.Parallel()
	startedAt := time.Date(2026, 7, 11, 22, 0, 0, 0, time.UTC)
	tests := []struct {
		name             string
		sessionStatus    LifecycleStatus
		resultStatus     ResultStatus
		primaryResult    json.RawMessage
		precedingPartial bool
	}{
		{
			name:          "partial result without terminal result",
			sessionStatus: LifecycleStatusRunning,
			resultStatus:  ResultStatusPartial,
			primaryResult: json.RawMessage(`[{"type":"text","text":"partial output"}]`),
		},
		{
			name:          "terminal result",
			sessionStatus: LifecycleStatusSucceeded,
			resultStatus:  ResultStatusFinal,
			primaryResult: json.RawMessage(`[{"type":"text","text":"final output"}]`),
		},
		{
			name:             "terminal result supersedes partial result",
			sessionStatus:    LifecycleStatusSucceeded,
			resultStatus:     ResultStatusFinal,
			primaryResult:    json.RawMessage(`[{"type":"text","text":"final output after partial"}]`),
			precedingPartial: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var sharedResult *ResultReadResult
			for _, orchestratorKind := range []string{"PETRI", "JAVASCRIPT"} {
				sessionID := "result-parity-" + strings.ToLower(orchestratorKind)
				session := SessionReadResult{
					SessionID:        sessionID,
					Status:           test.sessionStatus,
					OrchestratorKind: orchestratorKind,
					Lifecycle:        &LifecycleTimestamps{StartedAt: &startedAt},
				}
				if IsTerminalLifecycleStatus(test.sessionStatus) {
					finishedAt := startedAt.Add(time.Minute)
					session.Lifecycle.FinishedAt = &finishedAt
				}
				live := ResultReadResult{
					SessionID:     sessionID,
					SessionStatus: test.sessionStatus,
					ResultStatus:  test.resultStatus,
					PrimaryResult: test.primaryResult,
				}
				events := BuildCanonicalRuntimeSessionEvents(session, live)
				if test.precedingPartial {
					partialEvent := canonicalTypedInternalEvent(t, "SESSION_RESULT_UPDATED", sessionID, map[string]any{
						"resultStatus":  "PARTIAL",
						"resultSummary": []map[string]any{{"type": "text", "text": "earlier partial output"}},
					})
					events = append(events[:1], append([]json.RawMessage{partialEvent}, events[1:]...)...)
				}

				replayedSession, replayedResult, err := ReplaySessionProjection(events)
				if err != nil {
					t.Fatalf("ReplaySessionProjection(%s): %v", orchestratorKind, err)
				}
				if replayedResult.ResultStatus != test.resultStatus || replayedResult.SessionStatus != test.sessionStatus {
					t.Fatalf("%s result availability = (%q, %q), want (%q, %q)", orchestratorKind, replayedResult.ResultStatus, replayedResult.SessionStatus, test.resultStatus, test.sessionStatus)
				}
				if !jsonEqual(replayedResult.PrimaryResult, test.primaryResult) {
					t.Fatalf("%s primary result = %s, want %s", orchestratorKind, replayedResult.PrimaryResult, test.primaryResult)
				}
				if replayedSession.ResultSummary == nil || replayedSession.ResultSummary.ResultStatus != string(test.resultStatus) {
					t.Fatalf("%s session result summary = %#v, want %q", orchestratorKind, replayedSession.ResultSummary, test.resultStatus)
				}

				comparable := replayedResult
				comparable.SessionID = ""
				if sharedResult == nil {
					sharedResult = &comparable
				} else if !reflect.DeepEqual(*sharedResult, comparable) {
					t.Fatalf("shared result projection differs by orchestrator:\nfirst: %#v\n%s: %#v", *sharedResult, orchestratorKind, comparable)
				}
			}
		})
	}
}

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
