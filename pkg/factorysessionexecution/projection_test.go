package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

func TestNormalizeResultRequest_DefaultsAndValidation(t *testing.T) {
	normalized, err := NormalizeResultRequest(ResultRequest{})
	if err != nil {
		t.Fatalf("NormalizeResultRequest: %v", err)
	}
	if normalized.Mode != ResultModeFinal {
		t.Fatalf("mode = %q, want final", normalized.Mode)
	}

	partial, err := NormalizeResultRequest(ResultRequest{Mode: ResultModePartial, IncludeArtifacts: true})
	if err != nil {
		t.Fatalf("NormalizeResultRequest partial: %v", err)
	}
	if !partial.IncludeArtifacts {
		t.Fatal("includeArtifacts = false, want true")
	}

	_, err = NormalizeResultRequest(ResultRequest{Mode: ResultMode("invalid")})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestNormalizeEventReconnectRequest_RejectsNegativeSequence(t *testing.T) {
	sequence := -1
	_, err := NormalizeEventReconnectRequest(EventReconnectRequest{AfterSequence: &sequence})
	if err == nil {
		t.Fatal("error = nil, want validation error")
	}
}

func TestValidateResultMatchesSessionRead(t *testing.T) {
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Status:    LifecycleStatusRunning,
		ResultSummary: &ResultSummary{
			ResultStatus: string(ResultStatusPartial),
		},
	}
	result := ResultReadResult{
		SessionID:     "dur-sess-001",
		ResultStatus:  ResultStatusPartial,
		SessionStatus: LifecycleStatusRunning,
	}
	if err := ValidateResultMatchesSessionRead(session, result); err != nil {
		t.Fatalf("ValidateResultMatchesSessionRead: %v", err)
	}

	mismatch := result
	mismatch.ResultStatus = ResultStatusFinal
	if err := ValidateResultMatchesSessionRead(session, mismatch); err == nil {
		t.Fatal("error = nil, want mismatch")
	}
}

func TestValidateDispatchListMatchesSessionProgress(t *testing.T) {
	session := SessionReadResult{
		SessionID: "dur-sess-001",
		Progress: &ProgressCounts{
			TotalDispatches: 3,
		},
	}
	dispatches := []DispatchSummary{
		{ID: "disp-1"},
		{ID: "disp-2"},
		{ID: "disp-3"},
	}
	if err := ValidateDispatchListMatchesSessionProgress(session, dispatches); err != nil {
		t.Fatalf("ValidateDispatchListMatchesSessionProgress: %v", err)
	}

	tooMany := append(dispatches, DispatchSummary{ID: "disp-4"})
	if err := ValidateDispatchListMatchesSessionProgress(session, tooMany); err == nil {
		t.Fatal("error = nil, want dispatch count mismatch")
	}
}

func TestValidateResultMatchesEventProjection(t *testing.T) {
	events := []json.RawMessage{
		json.RawMessage(`{"type":"SESSION_RESULT_UPDATED","payload":{"resultStatus":"PARTIAL"}}`),
		json.RawMessage(`{"type":"SESSION_RESULT_UPDATED","payload":{"resultStatus":"FINAL"}}`),
	}
	result := ResultReadResult{
		SessionID:    "dur-sess-001",
		ResultStatus: ResultStatusFinal,
	}
	if err := ValidateResultMatchesEventProjection(result, events); err != nil {
		t.Fatalf("ValidateResultMatchesEventProjection: %v", err)
	}

	mismatch := result
	mismatch.ResultStatus = ResultStatusPartial
	if err := ValidateResultMatchesEventProjection(mismatch, events); err == nil {
		t.Fatal("error = nil, want event mismatch")
	}
}

func TestProjectionServiceMethods_PropagateContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var service interface {
		GetResult(context.Context, string, ResultRequest) (ResultReadResult, error)
	}
	service = stubProjectionCancelAwareService{}
	if _, err := service.GetResult(ctx, "dur-sess-001", ResultRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetResult error = %v, want context.Canceled", err)
	}
}

type stubProjectionCancelAwareService struct{}

func (stubProjectionCancelAwareService) GetResult(ctx context.Context, _ string, _ ResultRequest) (ResultReadResult, error) {
	if err := ctx.Err(); err != nil {
		return ResultReadResult{}, err
	}
	return ResultReadResult{}, nil
}

func TestBuildCanonicalSessionEvents_RunningAndTerminalSessions(t *testing.T) {
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
	artifactRef := workflowresult.FormatArtifactURI("session-live-child", "child-artifact-1")
	records := []workflowruntime.RuntimeRecord{
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:    "dispatch-1",
				Status:        workflowruntime.ChildDispatchStatusQueued,
				Label:         "summarize-findings",
				ExecutionMode: ChildExecutorModeLive,
				ArtifactRef:   artifactRef,
			},
		},
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:    "dispatch-1",
				Status:        workflowruntime.ChildDispatchStatusRunning,
				Label:         "summarize-findings",
				ExecutionMode: ChildExecutorModeLive,
				ArtifactRef:   artifactRef,
			},
		},
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:         "dispatch-1",
				Status:             workflowruntime.ChildDispatchStatusCompleted,
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

func TestProjectRuntimeExecutionRecords_FailedLiveChild_ProjectsFailureDetail(t *testing.T) {
	records := []workflowruntime.RuntimeRecord{
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        workflowruntime.ChildDispatchStatusQueued,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
			},
		},
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:    "dispatch-2",
				Status:        workflowruntime.ChildDispatchStatusRunning,
				Label:         "child-1",
				ExecutionMode: ChildExecutorModeLive,
			},
		},
		{
			Kind: workflowruntime.RecordKindChildDispatch,
			ChildDispatch: &workflowruntime.ChildDispatchRecord{
				DispatchID:        "dispatch-2",
				Status:            workflowruntime.ChildDispatchStatusFailed,
				Label:             "child-1",
				ExecutionMode:     ChildExecutorModeLive,
				FailureReason:     workflowruntime.ChildExecutionFailureReason,
				FailureMessage:    "live child failed: simulated child error",
				FailureErrorClass: "terminal",
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
	if dispatch.FailureDetail == nil || dispatch.FailureDetail.Reason != workflowruntime.ChildExecutionFailureReason {
		t.Fatalf("failureDetail = %#v", dispatch.FailureDetail)
	}
	if dispatch.FailureDetail.Message != "live child failed: simulated child error" {
		t.Fatalf("failure message = %q", dispatch.FailureDetail.Message)
	}
	if dispatch.FailureDetail.ErrorClass != "terminal" {
		t.Fatalf("failure errorClass = %q, want terminal", dispatch.FailureDetail.ErrorClass)
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

	_, err = FilterEventsAfterReconnect(events, EventReconnectRequest{
		AfterEventID: "missing-event",
	}, "s1")
	if !errors.Is(err, ErrReconnectCursorNotFound) {
		t.Fatalf("missing cursor error = %v, want ErrReconnectCursorNotFound", err)
	}
}

func assertCanonicalEventEnvelope(t *testing.T, raw json.RawMessage, eventType, id string) {
	t.Helper()
	var envelope struct {
		SchemaVersion string `json:"schemaVersion"`
		ID            string `json:"id"`
		Type          string `json:"type"`
		Context       struct {
			Sequence int `json:"sequence"`
		} `json:"context"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("Unmarshal event: %v", err)
	}
	if envelope.SchemaVersion != canonicalFactoryEventSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", envelope.SchemaVersion, canonicalFactoryEventSchemaVersion)
	}
	if id != "" && envelope.ID != id {
		t.Fatalf("id = %q, want %q", envelope.ID, id)
	}
	if eventType != "" && envelope.Type != eventType {
		t.Fatalf("type = %q, want %q", envelope.Type, eventType)
	}
	if envelope.Context.Sequence <= 0 {
		t.Fatalf("sequence = %d, want positive", envelope.Context.Sequence)
	}
	if len(envelope.Payload) == 0 {
		t.Fatal("payload missing")
	}
}
func TestReplaySessionProjection_TerminalSessionBracket(t *testing.T) {
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

func TestReplaySessionProjection_PreservesSyncTimeoutAvailability(t *testing.T) {
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
