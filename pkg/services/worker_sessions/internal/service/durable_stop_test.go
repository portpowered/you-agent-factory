package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Only the coordinator is real. This collaborator models the store's sync
// acknowledgement/failure and records detached operation snapshots.
type stopOperationStore struct {
	testutil.UnavailableWorkerControlStore
	begin      func(context.Context, recordings.WorkerControlOperationRecord) error
	advanceErr error
	records    []recordings.WorkerControlOperationRecord
	failures   []recordings.WorkerRecordingFailure
}

func (s *stopOperationStore) BeginWorkerControlOperation(ctx context.Context, record recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, bool, error) {
	if err := record.ValidateIntent(); err != nil {
		return record, false, err
	}
	if s.begin != nil {
		if err := s.begin(ctx, record); err != nil {
			return record, false, err
		}
	}
	if len(s.records) > 0 {
		previous := s.records[len(s.records)-1]
		if !previous.SameIntent(record) {
			return record, false, recordings.ErrWorkerControlConflict
		}
		return previous.Detached(), false, nil
	}
	s.records = append(s.records, record.Detached())
	return record.Detached(), true, nil
}

func (s *stopOperationStore) AdvanceWorkerControlOperation(_ context.Context, record recordings.WorkerControlOperationRecord, expected uint64) (recordings.WorkerControlOperationRecord, error) {
	if s.advanceErr != nil {
		return record, s.advanceErr
	}
	previous := s.records[len(s.records)-1]
	if !previous.CanAdvance(record, expected) {
		return record, recordings.ErrWorkerControlConflict
	}
	s.records = append(s.records, record.Detached())
	return record.Detached(), nil
}

func (s *stopOperationStore) PersistWorkerRecordingFailure(_ context.Context, failure recordings.WorkerRecordingFailure) error {
	s.failures = append(s.failures, failure)
	return nil
}

func newDurableStopFixture(t *testing.T) (*registry, *supervision, *stopOperationStore) {
	t.Helper()
	r := newTestRegistry(t)
	s := newSupervision("attempt", "")
	s.accepted = true
	r.sessions["worker"] = workersessions.Session{ID: "worker", State: workersessions.StateRunning}
	r.supervisions["worker"] = s
	capture := exactCaptureIdentity()
	capture.FactorySessionID = ""
	r.publications["worker"] = &publication{capture: capture}
	store := &stopOperationStore{}
	r.operations = store
	return r, s, store
}

func TestControlDurableStopCommitsIntentBeforeCancelAndResultAfterJoin(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			t.Cleanup(s.signalDone)
			canceled := make(chan struct{})
			s.installCancel(func() { close(canceled) })
			finished := make(chan workersessions.ControlResult, 1)
			ctx, cancel := context.WithCancel(t.Context())
			store.begin = func(ctx context.Context, record recordings.WorkerControlOperationRecord) error {
				cancel()
				return ctx.Err() // the host-owned context remains usable
			}
			go func() {
				result, err := r.cancelControl(ctx, workersessions.ControlRequest{ID: "worker"}, action, true)
				if err != nil {
					t.Error(err)
				}
				finished <- result
			}()
			awaitStopSignal(t, canceled)
			if len(store.records) != 1 || store.records[0].Operation.Phase != "INTENT" {
				t.Fatal("cancellation preceded intent acknowledgement")
			}
			select {
			case <-finished:
				t.Fatal("stop returned before callback joined")
			default:
			}
			r.commitControlTerminal("worker", controlTerminalState(action))
			s.signalDone()
			var result workersessions.ControlResult
			select {
			case result = <-finished:
			case <-time.After(30 * time.Second):
				t.Fatal("stop did not join")
			}
			if result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != controlTerminalState(action) {
				t.Fatalf("joined stop = %#v", result)
			}
			if len(store.records) != 2 || store.records[1].Operation.Phase != "COMPLETED" {
				t.Fatalf("operation history = %#v", store.records)
			}
			repeated, err := r.cancelControl(t.Context(), workersessions.ControlRequest{ID: "worker"}, action, true)
			if err != nil || repeated.Outcome != workersessions.ControlOutcomeNoop || len(store.records) != 2 {
				t.Fatalf("terminal repeat = %#v, %v", repeated, err)
			}
		})
	}
}

func awaitStopSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("stop signal unavailable")
	}
}

func TestControlDurableStopPersistenceLossStillStopsAndJoins(t *testing.T) {
	t.Parallel()
	for _, failure := range []string{"intent", "result"} {
		for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
			t.Run(failure+"/"+string(action), func(t *testing.T) {
				t.Parallel()
				r, s, store := newDurableStopFixture(t)
				private := errors.New("private disk path and credentials")
				if failure == "intent" {
					store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error { return private }
				} else {
					store.advanceErr = private
				}
				calls := 0
				s.installCancel(func() { calls++; r.commitControlTerminal("worker", controlTerminalState(action)); s.signalDone() })
				result, err := r.cancelControl(t.Context(), workersessions.ControlRequest{ID: "worker"}, action, true)
				if calls != 1 || result.Session.State != controlTerminalState(action) || result.Outcome != workersessions.ControlOutcomeFailed || !errors.Is(err, recordings.ErrWorkerRecordingPersistence) {
					t.Fatalf("degraded stop = %#v, calls %d, %v", result, calls, err)
				}
				if strings.Contains(err.Error(), "credentials") || len(store.failures) != 1 || store.failures[0].Code != "CONTROL_OPERATION_PERSISTENCE_FAILED" {
					t.Fatal("persistence failure was not safely classified")
				}
			})
		}
	}
}

func TestControlDurableStopStoreConflictAndReplacementHaveNoEffects(t *testing.T) {
	t.Parallel()
	for _, refusal := range []string{"conflict", "invalid", "replacement"} {
		t.Run(refusal, func(t *testing.T) {
			t.Parallel()
			r, s, store := newDurableStopFixture(t)
			calls := 0
			s.installCancel(func() { calls++ })
			store.begin = func(context.Context, recordings.WorkerControlOperationRecord) error {
				switch refusal {
				case "conflict":
					return recordings.ErrWorkerControlConflict
				case "invalid":
					return recordings.ErrInvalidWorkerControlOperation
				default:
					s.dispatchID = "replacement"
					return nil
				}
			}
			result, err := r.Cancel(t.Context(), workersessions.ControlRequest{ID: "worker"})
			if calls != 0 || !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || r.sessions["worker"].State != workersessions.StateRunning {
				t.Fatalf("refused stop = %#v, calls %d, %v", result, calls, err)
			}
		})
	}
}

func TestControlDurableFactoryStopCapturesPhysicalAttemptAndJoinedResult(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			r, id, attempt, calls := newFrozenRuntimeControlFixture(t)
			store := &stopOperationStore{}
			r.operations = store
			capture := exactCaptureIdentity()
			capture.WorkerSessionID, capture.FactorySessionID = publicWorkerID(id), "factory-a"
			r.publications[id].capture = capture
			attempt.completed = make(chan struct{})
			attempt.cancel = func(context.Context) (workers.WorkstationDispatchCancelOutcome, error) {
				*calls++
				if len(store.records) != 1 || store.records[0].Target.ExpectedAttemptID != attempt.attemptID {
					t.Fatal("Factory cancellation lacked physical-attempt intent")
				}
				r.commitControlTerminal(id, controlTerminalState(action))
				close(attempt.completed)
				return workers.WorkstationDispatchCancelOutcomeCanceled, nil
			}
			result, err := r.cancelControl(t.Context(), workersessions.ControlRequest{ID: id}, action, true)
			if err != nil || *calls != 1 || result.Outcome != workersessions.ControlOutcomeApplied || result.Session.State != controlTerminalState(action) {
				t.Fatalf("Factory stop = %#v, calls %d, %v", result, *calls, err)
			}
			if len(store.records) != 2 || store.records[1].Operation.Phase != "COMPLETED" {
				t.Fatalf("Factory operation history = %#v", store.records)
			}
		})
	}
}

func TestControlDurableStopFailureIsAbsorbingAndSafe(t *testing.T) {
	t.Parallel()
	r, s, store := newDurableStopFixture(t)
	calls := 0
	private := errors.New("execution failed secret=sentinel")
	s.installCancelFailure(func() error { calls++; return private })
	req := workersessions.ControlRequest{ID: "worker", RequestID: "operator-request"}
	if result, err := r.Cancel(t.Context(), req); !errors.Is(err, private) || result.Outcome != workersessions.ControlOutcomeFailed {
		t.Fatalf("failed stop = %#v, %v", result, err)
	}
	if len(store.records) != 2 || store.records[1].Operation.Phase != "FAILED" || store.records[1].FailureCode != "STOP_FAILED" || strings.Contains(string(store.records[1].Result), "sentinel") {
		t.Fatalf("unsafe failure snapshot = %#v", store.records)
	}
	if _, err := r.Cancel(t.Context(), req); !errors.Is(err, workersessions.ErrInvalidState) || calls != 1 {
		t.Fatalf("failed replay repeated effect: calls %d, %v", calls, err)
	}
}
