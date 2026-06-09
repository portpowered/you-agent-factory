package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SessionProjectionEventKinds lists canonical event types that project durable
// session lifecycle, result, dispatch, artifact, phase, checkpoint, and budget state.
var SessionProjectionEventKinds = []string{
	"SESSION_STARTED",
	"SESSION_RESULT_UPDATED",
	"SESSION_COMPLETED",
	"ORCHESTRATOR_PHASE_CHANGED",
	"ORCHESTRATOR_CHECKPOINT_WRITTEN",
	"DISPATCH_QUEUED",
	"DISPATCH_INTERRUPTED",
	"DISPATCH_RECONCILED",
	"ARTIFACT_CREATED",
	"JAVASCRIPT_PHASE_CHANGE",
	"JAVASCRIPT_CHECKPOINT_REF",
}

type sessionResultUpdatedProjection struct {
	ResultStatus string `json:"resultStatus"`
}

type factoryEventEnvelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// ValidateResultMatchesSessionRead checks that one result read aligns with the
// session read summary when both are present.
func ValidateResultMatchesSessionRead(session SessionReadResult, result ResultReadResult) error {
	if strings.TrimSpace(session.SessionID) == "" || strings.TrimSpace(result.SessionID) == "" {
		return nil
	}
	if session.SessionID != result.SessionID {
		return fmt.Errorf("result sessionId %q does not match session read %q", result.SessionID, session.SessionID)
	}
	if session.ResultSummary != nil {
		summaryStatus := strings.TrimSpace(session.ResultSummary.ResultStatus)
		if summaryStatus != "" && summaryStatus != string(result.ResultStatus) {
			return fmt.Errorf("result status %q does not match session resultSummary %q", result.ResultStatus, summaryStatus)
		}
	}
	if session.Status != "" && result.SessionStatus != "" && session.Status != result.SessionStatus {
		return fmt.Errorf("result sessionStatus %q does not match session read %q", result.SessionStatus, session.Status)
	}
	return nil
}

// ValidateDispatchListMatchesSessionProgress checks dispatch list counts against
// one session read progress summary when present.
func ValidateDispatchListMatchesSessionProgress(session SessionReadResult, dispatches []DispatchSummary) error {
	if session.Progress == nil {
		return nil
	}
	total := len(dispatches)
	if session.Progress.TotalDispatches > 0 && total > session.Progress.TotalDispatches {
		return fmt.Errorf("dispatch list length %d exceeds session progress total %d", total, session.Progress.TotalDispatches)
	}
	return nil
}

// LatestResultStatusFromEvents returns the resultStatus from the latest
// SESSION_RESULT_UPDATED event when present.
func LatestResultStatusFromEvents(events []json.RawMessage) (ResultStatus, bool) {
	var latest ResultStatus
	found := false
	for _, raw := range events {
		var envelope factoryEventEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			continue
		}
		if strings.TrimSpace(envelope.Type) != "SESSION_RESULT_UPDATED" {
			continue
		}
		var payload sessionResultUpdatedProjection
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			continue
		}
		status := strings.TrimSpace(payload.ResultStatus)
		if status == "" {
			continue
		}
		latest = ResultStatus(status)
		found = true
	}
	return latest, found
}

// ValidateResultMatchesEventProjection checks that one result read matches the
// latest SESSION_RESULT_UPDATED event projection when events are present.
func ValidateResultMatchesEventProjection(result ResultReadResult, events []json.RawMessage) error {
	eventStatus, ok := LatestResultStatusFromEvents(events)
	if !ok {
		return nil
	}
	if eventStatus != result.ResultStatus {
		return fmt.Errorf("result status %q does not match latest SESSION_RESULT_UPDATED event %q", result.ResultStatus, eventStatus)
	}
	return nil
}
