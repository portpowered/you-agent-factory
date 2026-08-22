package service

import (
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (r *registry) transitionToRunning(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, exists := r.sessions[id]
	if !exists || session.State != workersessions.StateStarting {
		return false
	}
	session.State = workersessions.StateRunning
	r.sessions[id] = session
	return true
}

func (r *registry) logReconciliation(
	id, attemptID string,
	result workers.WorkstationDispatchResult,
	priorState, resultingState workersessions.State,
	startedAt time.Time,
	deadlineAt time.Time,
) {
	elapsedMS := int64(0)
	if !startedAt.IsZero() {
		elapsedMS = r.clock.Now().Sub(startedAt).Milliseconds()
		if elapsedMS < 0 {
			elapsedMS = 0
		}
	}
	deadline := "not_applicable"
	configuredTimeoutMS := int64(0)
	if !deadlineAt.IsZero() {
		deadline = deadlineAt.UTC().Format(time.RFC3339Nano)
		configuredTimeoutMS = deadlineAt.Sub(startedAt).Milliseconds()
	}
	fields := []any{
		"sessionID", id,
		"attemptID", attemptID,
		"dispatchID", result.DispatchID,
		"reason", string(result.ReconciliationReason),
		"prior_state", string(priorState),
		"resulting_state", string(resultingState),
		"result", string(result.TerminalOutcome),
		"elapsed_ms", elapsedMS,
		"deadline", deadline,
	}
	if !deadlineAt.IsZero() {
		fields = append(fields, "configured_timeout_ms", configuredTimeoutMS)
	}
	r.logger.Info("worker session reconciliation", fields...)
}
