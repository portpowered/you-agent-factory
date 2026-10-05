package service

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func naturalTerminalCause(state workersessions.State) *string {
	if state == workersessions.StateCompleted || state == workersessions.StateFailed {
		cause := string(state)
		return &cause
	}
	return nil
}

// The terminal publication supplies the physical attempt, never a later
// provider reference or a logical Factory dispatch. An intent alone does not
// establish causation, and a failed read leaves the optional fact unknown.
func (r *registry) observationTerminalCause(ctx context.Context, id string, state workersessions.State) *string {
	if cause := naturalTerminalCause(state); cause != nil || !state.Terminal() {
		return cause
	}
	pub := r.publicationFor(id)
	if pub == nil {
		return nil
	}
	pub.mu.Lock()
	target := pub.capture
	target.ExpectedAttemptID = pub.terminalAttemptID
	pub.mu.Unlock()
	if target.ExpectedAttemptID == "" || target.RecordingID == "" {
		return nil
	}
	records, err := r.operations.ListWorkerControlOperations(ctx, target)
	if err != nil {
		return nil
	}
	return committedStopCause(records, target, state)
}

// Result snapshots are written only after the authoritative callback joins.
// Require matching applied action/state as well as the complete capture fence:
// a stop that loses to natural completion must never claim an operator cause.
func committedStopCause(records []recordings.WorkerControlOperationRecord, target recordings.WorkerControlTarget, state workersessions.State) *string {
	for _, record := range records {
		if record.Target != target {
			continue
		}
		if committedInterruptStop(record, state) {
			cause := "OPERATOR_CANCEL"
			return &cause
		}
		if cause := committedPlainStopCause(record, state); cause != nil {
			return cause
		}
	}
	return nil
}

func committedPlainStopCause(record recordings.WorkerControlOperationRecord, state workersessions.State) *string {
	if record.Operation.Phase != "COMPLETED" || record.FailureCode != "" {
		return nil
	}
	var result workersessions.ControlResult
	if json.Unmarshal(record.Result, &result) != nil || result.Outcome != workersessions.ControlOutcomeApplied ||
		result.Session.ID != record.Target.WorkerSessionID || result.Session.State != state {
		return nil
	}
	var cause string
	switch {
	case record.Operation.Action == "cancel" && result.Action == workersessions.ControlActionCancel && state == workersessions.StateCanceled:
		cause = "OPERATOR_CANCEL"
	case record.Operation.Action == "terminate" && result.Action == workersessions.ControlActionTerminate && state == workersessions.StateTerminated:
		cause = "OPERATOR_TERMINATE"
	default:
		return nil
	}
	return &cause
}

// Source join is committed before successor admission. An admission failure
// or missing final row does not erase that causal stop; INTENT alone cannot
// establish it. Reuse the strict result decoder to reject malformed snapshots.
func committedInterruptStop(record recordings.WorkerControlOperationRecord, state workersessions.State) bool {
	operation := record.Operation
	if state != workersessions.StateCanceled || operation.Action != "interrupt" || operation.Version != 1 ||
		operation.WorkerSessionID != record.Target.WorkerSessionID || operation.ExpectedAttemptID != record.Target.ExpectedAttemptID {
		return false
	}
	req := workersessions.InterruptRequest{RequestID: operation.RequestID, SourceWorkerSessionID: operation.WorkerSessionID, SuccessorWorkerSessionID: operation.SuccessorWorkerSessionID}
	result, _ := decodeInterruptOutcome(req, record)
	return result.Phase == workersessions.InterruptPhaseSuccessorAdmission && result.Source.State == state
}
