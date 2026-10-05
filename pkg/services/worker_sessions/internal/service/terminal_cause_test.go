package service

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestTerminalCauseRequiresCommittedAppliedExactStop(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		for _, variant := range []string{"applied", "intent", "failed", "noop", "scope", "epoch", "generation", "attempt", "worker", "action", "state", "malformed", "failure-code"} {
			t.Run(string(action)+"/"+variant, func(t *testing.T) {
				t.Parallel()
				r, _, _ := newDurableStopFixture(t)
				target, _ := r.freezeControlTarget("worker")
				record := stopIntent(workersessions.ControlRequest{ID: "worker"}, action, target)
				expected := record.Target
				record.Revision, record.Operation.Phase = 2, "COMPLETED"
				state := controlTerminalState(action)
				result := workersessions.ControlResult{Session: workersessions.Session{ID: "worker", State: state}, Action: action, Outcome: workersessions.ControlOutcomeApplied}
				mutateCauseEvidence(&record, &result, variant)
				record.Result, _ = json.Marshal(result)
				if variant == "malformed" {
					record.Result = []byte("{")
				}
				cause := committedStopCause([]recordings.WorkerControlOperationRecord{record}, expected, state)
				if variant != "applied" {
					if cause != nil {
						t.Fatalf("invalid evidence granted cause %s", *cause)
					}
					return
				}
				want := "OPERATOR_CANCEL"
				if action == workersessions.ControlActionTerminate {
					want = "OPERATOR_TERMINATE"
				}
				if cause == nil || *cause != want {
					t.Fatalf("cause = %v, want %s", cause, want)
				}
			})
		}
	}
}

func mutateCauseEvidence(record *recordings.WorkerControlOperationRecord, result *workersessions.ControlResult, variant string) {
	switch variant {
	case "intent":
		record.Operation.Phase = "INTENT"
	case "failed":
		record.Operation.Phase = "FAILED"
	case "noop":
		result.Outcome = workersessions.ControlOutcomeNoop
	case "scope":
		record.Target.FactorySessionID = "foreign"
	case "epoch":
		record.Target.OwnerEpoch = "foreign"
	case "generation":
		record.Target.RecordingGenerationID = "foreign"
	case "attempt":
		record.Target.ExpectedAttemptID = "foreign"
	case "worker":
		result.Session.ID = "foreign"
	case "action":
		result.Action = workersessions.ControlActionPause
	case "state":
		result.Session.State = workersessions.StateCompleted
	case "failure-code":
		record.FailureCode = "STOP_FAILED"
	}
}

func TestTerminalCauseNaturalAndLiveStates(t *testing.T) {
	t.Parallel()
	for _, state := range []workersessions.State{workersessions.StateReserved, workersessions.StateStarting, workersessions.StateRunning, workersessions.StatePaused, workersessions.StateCanceled, workersessions.StateTerminated, workersessions.StateCompleted, workersessions.StateFailed} {
		cause := naturalTerminalCause(state)
		if state == workersessions.StateCompleted || state == workersessions.StateFailed {
			if cause == nil || *cause != string(state) {
				t.Fatalf("natural cause for %s = %v", state, cause)
			}
		} else if cause != nil {
			t.Fatalf("state %s invented cause %s", state, *cause)
		}
	}
}

func TestTerminalCauseObservationCloneIsDetached(t *testing.T) {
	t.Parallel()
	cause := "OPERATOR_CANCEL"
	original := workersessions.Observation{TerminalCause: &cause}
	clone := original.Clone()
	*clone.TerminalCause = "OPERATOR_TERMINATE"
	if *original.TerminalCause != "OPERATOR_CANCEL" {
		t.Fatal("clone changed authoritative cause")
	}
}
