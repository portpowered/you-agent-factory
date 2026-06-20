package factorysessionexecution

import (
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

// SeedRuntimeSessionWithRunningDispatch seeds one in-memory JavaScript runtime session
// with a running child dispatch for interrupt-dispatch integration tests.
func SeedRuntimeSessionWithRunningDispatch(
	service *JavaScriptRuntimeService,
	sessionID, dispatchID, label string,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return NewValidationError("dispatchId", "dispatchId is required")
	}

	now := time.Now().UTC()
	session := SessionReadResult{
		SessionID:        id,
		Status:           LifecycleStatusRunning,
		OrchestratorKind: interfaces.OrchestratorKindJavaScript,
		Phase:            "execute",
		Lifecycle:        &LifecycleTimestamps{StartedAt: &now},
		Links:            InspectionLinksForSession(id, true),
		Progress: &ProgressCounts{
			TotalDispatches:    1,
			InFlightDispatches: 1,
		},
	}
	result := ResultReadResult{
		SessionID:     id,
		SessionStatus: LifecycleStatusRunning,
		ResultStatus:  ResultStatusNotReady,
		Availability: &ResultAvailabilityDetail{
			Reason:    "RESULT_NOT_READY",
			Message:   "Session is still running.",
			Retryable: true,
		},
	}
	state := &runtimeSessionState{
		session: session,
		result:  result,
		dispatches: []DispatchSummary{{
			ID:     dispatchID,
			Status: DispatchStatusRunning,
			Phase:  "execute",
			Label:  label,
		}},
		dispatchStatusTransitions: map[string][]DispatchStatus{
			dispatchID: {DispatchStatusQueued, DispatchStatusRunning},
		},
		events: BuildCanonicalRuntimeSessionEvents(session, result),
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	service.sessions[id] = state
	return nil
}

// ApplyRuntimeTerminalOutcomeForTests merges one terminal workflow outcome into a
// seeded JavaScript runtime session for interrupt-dispatch race integration tests.
func ApplyRuntimeTerminalOutcomeForTests(
	service *JavaScriptRuntimeService,
	sessionID string,
	outcome workflowruntime.Outcome,
) error {
	if service == nil {
		return NewValidationError("service", "service is required")
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	state, ok := service.sessions[id]
	if !ok {
		return ErrSessionNotFound
	}

	finishedAt := time.Now().UTC()
	terminal := runtimeSessionState{
		session: cloneSessionRead(state.session),
		result:  cloneResultRead(state.result),
	}
	if outcome.OK {
		applyRuntimeSuccessProjection(&terminal, id, outcome, finishedAt)
	} else if len(outcome.Records) > 0 {
		applyRuntimeExecutionRecordProjection(&terminal, id, outcome.Records, finishedAt)
		projectRuntimeFailure(&terminal.session, &terminal.result, outcome)
	}
	applyTerminalRuntimeProjection(state, terminal, outcome)
	return nil
}
