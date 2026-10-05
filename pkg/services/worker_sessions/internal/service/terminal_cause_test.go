package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestTerminalCausePlainStopRejectsUnsafeRecoveredSnapshots(t *testing.T) {
	t.Parallel()
	valid := `{"Session":{"ID":"worker","State":"CANCELED"},"Action":"CANCEL","Outcome":"APPLIED","DispatchID":"logical-dispatch"}`
	for name, payload := range map[string]string{
		"logical-dispatch": valid,
		"unknown":          strings.Replace(valid, `"Action":`, `"private":"private-stop-detail","Action":`, 1),
		"duplicate":        strings.Replace(valid, `"ID":"worker"`, `"ID":"foreign","ID":"worker"`, 1),
		"alias":            strings.Replace(valid, `"ID":"worker"`, `"id":"worker"`, 1),
		"private-result":   strings.Replace(valid, `"State":"CANCELED"`, `"State":"CANCELED","Result":{"Output":"private-stop-detail"}`, 1),
		"private-model":    strings.Replace(valid, `"State":"CANCELED"`, `"State":"CANCELED","Model":"private-stop-detail"`, 1),
		"lineage":          strings.Replace(valid, `"State":"CANCELED"`, `"State":"CANCELED","SuccessorWorkerSessionID":"foreign"`, 1),
		"missing-dispatch": strings.Replace(valid, `"logical-dispatch"`, `""`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, _, _ := newDurableStopFixture(t)
			target, _ := r.freezeControlTarget("worker")
			record := stopIntent(workersessions.ControlRequest{ID: "worker"}, workersessions.ControlActionCancel, target)
			record.Operation.Phase, record.Result = "COMPLETED", []byte(payload)
			cause := committedStopCause([]recordings.WorkerControlOperationRecord{record}, record.Target, workersessions.StateCanceled)
			if name == "logical-dispatch" {
				if cause == nil || *cause != "OPERATOR_CANCEL" {
					t.Fatalf("logical dispatch must preserve exact physical cause: %v", cause)
				}
			} else if cause != nil {
				t.Fatalf("unsafe saved snapshot granted cause %s", *cause)
			}
		})
	}
}

func TestTerminalCausePlainStopRequiresMatchingOperationEnvelope(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*recordings.WorkerControlOperation){
		"version": func(operation *recordings.WorkerControlOperation) { operation.Version = 2 },
		"worker":  func(operation *recordings.WorkerControlOperation) { operation.WorkerSessionID = "foreign" },
		"attempt": func(operation *recordings.WorkerControlOperation) { operation.ExpectedAttemptID = "foreign" },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, _, _ := newDurableStopFixture(t)
			target, _ := r.freezeControlTarget("worker")
			record := stopIntent(workersessions.ControlRequest{ID: "worker"}, workersessions.ControlActionCancel, target)
			record.Operation.Phase = "COMPLETED"
			record.Result = []byte(`{"Session":{"ID":"worker","State":"CANCELED"},"Action":"CANCEL","Outcome":"APPLIED","DispatchID":"attempt"}`)
			mutate(&record.Operation)
			if cause := committedStopCause([]recordings.WorkerControlOperationRecord{record}, record.Target, workersessions.StateCanceled); cause != nil {
				t.Fatalf("incompatible operation claimed cause %s", *cause)
			}
		})
	}
}

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
				result := workersessions.ControlResult{Session: workersessions.Session{ID: "worker", State: state}, Action: action, Outcome: workersessions.ControlOutcomeApplied, DispatchID: "attempt"}
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

func TestTerminalCauseInterruptRequiresCommittedSourceJoin(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"INTENT", "SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED", "FAILED", "corrupt", "stale"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			r, plan, _ := newDurableInterruptFixture(t)
			record, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			target := record.Target
			result := interruptResult(plan.request, workersessions.InterruptPhaseSuccessorAdmission, false)
			result.Source = workersessions.Session{ID: plan.request.SourceWorkerSessionID, State: workersessions.StateCanceled}
			if phase == "SUCCESSOR_ADMITTED" || phase == "COMPLETED" {
				result.Accepted = true
				result.Successor = workersessions.Session{ID: plan.request.SuccessorWorkerSessionID, State: workersessions.StateRunning}
			}
			outcome := durableInterruptOutcome{InterruptResult: result}
			if phase == "FAILED" {
				outcome.FailureCauses = []string{"SUCCESSOR_ADMISSION_FAILED"}
				record.FailureCode = string(result.Phase)
			}
			record.Operation.Phase = phase
			record.Result, _ = json.Marshal(outcome)
			if phase == "corrupt" {
				record.Result = []byte(`{"Source":{"State":"CANCELED"}}`)
			}
			if phase == "stale" {
				record.Target.ExpectedAttemptID = "another-attempt"
			}
			cause := committedStopCause([]recordings.WorkerControlOperationRecord{*record}, target, workersessions.StateCanceled)
			want := phase != "INTENT" && phase != "corrupt" && phase != "stale"
			if want && (cause == nil || *cause != "OPERATOR_CANCEL") || !want && cause != nil {
				t.Fatalf("phase=%s cause=%v want committed source join=%t", phase, cause, want)
			}
		})
	}
}
