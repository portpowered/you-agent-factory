package factorysessionexecution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
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
