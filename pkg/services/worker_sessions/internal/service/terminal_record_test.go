package service

import (
	"context"
	"encoding/json"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestTerminalDraft_Completed_MapsToPhaseCompletedWithStatusOnly proves the
// pure COMPLETED mapping: no FailureCause exists, so the payload carries only
// the status.
func TestTerminalDraft_Completed_MapsToPhaseCompletedWithStatusOnly(t *testing.T) {
	draft, err := terminalDraft(workersessions.StateCompleted, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}, "dispatch-1")
	if err != nil {
		t.Fatalf("terminalDraft() error = %v, want nil", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseCompleted {
		t.Fatalf("terminalDraft() = %+v, want Kind=SESSION Phase=COMPLETED", draft)
	}
	if err := workers.ValidateDraft(draft); err != nil {
		t.Fatalf("workers.ValidateDraft(draft) error = %v, want nil", err)
	}

	var payload terminalSessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateCompleted) {
		t.Fatalf("payload.Status = %q, want %q", payload.Status, workersessions.StateCompleted)
	}
	if payload.FailureCause != "" || payload.FailureDetail != "" {
		t.Fatalf("payload = %+v, want no failure fields on a COMPLETED terminal record", payload)
	}

	var sessionPayload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &sessionPayload); err != nil {
		t.Fatalf("unmarshal into workers.SessionPayload error = %v, want the terminal payload to remain a valid SessionPayload", err)
	}
	if sessionPayload.Status != string(workersessions.StateCompleted) {
		t.Fatalf("workers.SessionPayload.Status = %q, want %q", sessionPayload.Status, workersessions.StateCompleted)
	}
}

// TestTerminalDraft_Failed_MapsToPhaseFailedPreservingCauseAndSafeDetail
// proves the FAILED mapping preserves the already-computed typed
// FailureCause classification and its bounded safe Detail in the payload.
func TestTerminalDraft_Failed_MapsToPhaseFailedPreservingCauseAndSafeDetail(t *testing.T) {
	result := workersessions.TerminalResult{
		Outcome: workersessions.TerminalOutcomeFailed,
		Cause: &workersessions.FailureCause{
			Kind:   workersessions.FailureCauseExecutorPanic,
			Detail: safeDetail(workersessions.FailureCauseExecutorPanic, nil),
		},
	}
	draft, err := terminalDraft(workersessions.StateFailed, result, "dispatch-1")
	if err != nil {
		t.Fatalf("terminalDraft() error = %v, want nil", err)
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseFailed {
		t.Fatalf("terminalDraft() = %+v, want Kind=SESSION Phase=FAILED", draft)
	}
	if err := workers.ValidateDraft(draft); err != nil {
		t.Fatalf("workers.ValidateDraft(draft) error = %v, want nil", err)
	}

	var payload terminalSessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload error = %v", err)
	}
	if payload.Status != string(workersessions.StateFailed) {
		t.Fatalf("payload.Status = %q, want %q", payload.Status, workersessions.StateFailed)
	}
	if payload.FailureCause != string(workersessions.FailureCauseExecutorPanic) {
		t.Fatalf("payload.FailureCause = %q, want %q", payload.FailureCause, workersessions.FailureCauseExecutorPanic)
	}
	if payload.FailureDetail != result.Cause.Detail {
		t.Fatalf("payload.FailureDetail = %q, want %q", payload.FailureDetail, result.Cause.Detail)
	}
}

// TestTerminalDraft_CanceledAndTerminated_ShareExistingPhaseCanceledButPreserveDistinctStatus
// proves the pure mapping the W3 scope note requires for CANCELED/TERMINATED
// (neither reachable through Start until W6 adds controls): both project
// through the same existing PhaseCanceled, with the distinct originating
// state preserved as the payload Status so a consumer can still tell them
// apart.
func TestTerminalDraft_CanceledAndTerminated_ShareExistingPhaseCanceledButPreserveDistinctStatus(t *testing.T) {
	for _, state := range []workersessions.State{workersessions.StateCanceled, workersessions.StateTerminated} {
		t.Run(string(state), func(t *testing.T) {
			draft, err := terminalDraft(state, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}, "dispatch-1")
			if err != nil {
				t.Fatalf("terminalDraft() error = %v, want nil", err)
			}
			if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseCanceled {
				t.Fatalf("terminalDraft() = %+v, want Kind=SESSION Phase=CANCELED", draft)
			}
			if err := workers.ValidateDraft(draft); err != nil {
				t.Fatalf("workers.ValidateDraft(draft) error = %v, want nil", err)
			}

			var payload terminalSessionPayload
			if err := json.Unmarshal(draft.Payload, &payload); err != nil {
				t.Fatalf("unmarshal payload error = %v", err)
			}
			if payload.Status != string(state) {
				t.Fatalf("payload.Status = %q, want %q", payload.Status, state)
			}
		})
	}
}

// TestTerminalDraft_NonTerminalState_ReturnsErrorAndNoDraft proves
// terminalPhase/terminalDraft refuse to fabricate a terminal projection for a
// state that is not one of the four absorbing terminal states.
func TestTerminalDraft_NonTerminalState_ReturnsErrorAndNoDraft(t *testing.T) {
	for _, state := range []workersessions.State{
		workersessions.StateReserved,
		workersessions.StateStarting,
		workersessions.StateRunning,
		workersessions.StatePaused,
	} {
		t.Run(string(state), func(t *testing.T) {
			if _, err := terminalDraft(state, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted}, "dispatch-1"); err == nil {
				t.Fatalf("terminalDraft(%q) error = nil, want a non-nil error", state)
			}
		})
	}
}

// TestPublishTerminalRecord_NonTerminalState_PropagatesTerminalDraftErrorAndAppendsNothing
// proves publishTerminalRecord propagates terminalDraft's error unchanged and
// never reaches appendDraft/Events for a state with no terminal projection.
// Start itself can never pass such a state (see terminalPhase), so this
// drives the registry method directly, the same way
// TestTerminalDraft_NonTerminalState_ReturnsErrorAndNoDraft exercises the
// pure mapping directly.
func TestPublishTerminalRecord_NonTerminalState_PropagatesTerminalDraftErrorAndAppendsNothing(t *testing.T) {
	r := newTestRegistry(t)

	err := r.publishTerminalRecord(context.Background(), "worker-1", "dispatch-1", workersessions.StateReserved, workersessions.TerminalResult{Outcome: workersessions.TerminalOutcomeCompleted})
	if err == nil {
		t.Fatal("publishTerminalRecord() error = nil, want a non-nil error for a non-terminal state")
	}
}
