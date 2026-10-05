package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func newFrozenRuntimeControlFixture(t *testing.T) (*registry, string, *runtimeAttempt, *int) {
	t.Helper()
	r := newTestRegistry(t)
	id := scopedWorkerAddress("frozen-factory-worker", "factory-a")
	calls := new(int)
	a := &runtimeAttempt{
		registry: r, workerID: id, dispatchID: "logical-dispatch", attemptID: "physical-attempt",
		key: workersessions.RuntimeAttemptKey{RuntimeID: "factory-a", DispatchID: "logical-dispatch"},
		cancel: func(context.Context) (workers.WorkstationDispatchCancelOutcome, error) {
			*calls++
			return "", errors.New("unexpected cancellation")
		},
	}
	r.sessions[id] = workersessions.Session{ID: publicWorkerID(id), State: workersessions.StateRunning}
	r.runtimeAttemptControls[id] = a
	r.runtimeAttemptOwners = map[workersessions.RuntimeAttemptKey]string{a.key: id}
	r.latestRuntimeDispatchIDs = map[string]string{id: a.dispatchID}
	r.publications[id] = &publication{open: true}
	return r, id, a, calls
}

func replaceFrozenRuntimeOwner(r *registry, id string, a *runtimeAttempt, replacement string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch replacement {
	case "handle":
		r.runtimeAttemptControls[id] = &runtimeAttempt{workerID: id, dispatchID: a.dispatchID, attemptID: "replacement-attempt", key: a.key, cancel: a.cancel}
	case "scope":
		r.runtimeAttemptOwners[a.key] = scopedWorkerAddress(publicWorkerID(id), "factory-b")
	case "dispatch":
		r.latestRuntimeDispatchIDs[id] = "replacement-dispatch"
	}
}

func TestControlFrozenRuntimeRefusesReplacementDuringHistory(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		for _, replacement := range []string{"handle", "scope", "dispatch"} {
			t.Run(string(action)+"/"+replacement, func(t *testing.T) {
				t.Parallel()
				r, id, a, calls := newFrozenRuntimeControlFixture(t)
				sink := &perRuntimeAppendCapture{EventsAppender: r.events}
				r.events = &controlReplacementAppender{EventsAppender: sink, replace: func() {
					replaceFrozenRuntimeOwner(r, id, a, replacement)
				}}
				req := workersessions.ControlRequest{ID: id}
				var result workersessions.ControlResult
				var err error
				if action == workersessions.ControlActionCancel {
					result, err = r.Cancel(context.Background(), req)
				} else {
					result, err = r.Terminate(context.Background(), req)
				}
				if !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || result.DispatchID != a.dispatchID {
					t.Fatalf("stale Runtime stop = %#v, %v", result, err)
				}
				if *calls != 0 || a.controlPending || a.controlAction != "" || r.sessions[id].State != workersessions.StateRunning {
					t.Fatal("stale Runtime control affected execution or retained a pending claim")
				}
				assertFrozenRuntimeHistory(t, sink, a.dispatchID)
			})
		}
	}
}

func assertFrozenRuntimeHistory(t *testing.T, sink *perRuntimeAppendCapture, dispatchID string) {
	t.Helper()
	requests := sink.requestsFor("")
	if len(requests) != 2 {
		t.Fatalf("control bracket records = %d, want request and failed outcome", len(requests))
	}
	for i, request := range requests {
		draft := decodePerRuntimeDraft(t, request)
		var payload workersessions.ControlRecordPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.DispatchID != dispatchID || payload.RequestID != controlFallbackRequestID(payload.Action, payload.WorkerSessionID, dispatchID) {
			t.Fatalf("history rediscovered a replacement target: %#v", payload)
		}
		if i == 1 && payload.Outcome != workersessions.ControlOutcomeFailed {
			t.Fatalf("refused control outcome = %q", payload.Outcome)
		}
	}
}

func TestControlFrozenRuntimeRefusesReplacementAfterWait(t *testing.T) {
	t.Parallel()
	for _, replacement := range []string{"handle", "scope", "dispatch"} {
		t.Run(replacement, func(t *testing.T) {
			t.Parallel()
			r, id, a, calls := newFrozenRuntimeControlFixture(t)
			a.controlPending = true
			a.controlDone = make(chan struct{})
			target, err := r.freezeControlTarget(id)
			if err != nil {
				t.Fatal(err)
			}
			claimed, wait, completed, err := r.claimFrozenRuntimeControl(id, target)
			if claimed || wait != a.controlDone || completed != nil || err != nil {
				t.Fatalf("pending control claim = %t, %v, %v, %v", claimed, wait, completed, err)
			}
			replaceFrozenRuntimeOwner(r, id, a, replacement)
			a.resolveControl(workersessions.ControlActionCancel, "", errors.New("prior control failed"))
			<-wait
			result, complete, err := r.awaitRuntimeAttemptControl(id, workersessions.ControlActionTerminate, target)
			if !complete || !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || *calls != 0 || a.controlPending {
				t.Fatalf("stale control after wait = %#v, %t, %v, calls %d", result, complete, err, *calls)
			}
		})
	}
}

func TestControlFrozenRuntimeNaturalCompletionKeepsNoop(t *testing.T) {
	t.Parallel()
	r, id, a, calls := newFrozenRuntimeControlFixture(t)
	target, err := r.freezeControlTarget(id)
	if err != nil {
		t.Fatal(err)
	}
	a.completing = true
	a.completed = make(chan struct{})
	close(a.completed)
	r.sessions[id] = workersessions.Session{ID: publicWorkerID(id), State: workersessions.StateCompleted}
	delete(r.runtimeAttemptControls, id)
	delete(r.runtimeAttemptOwners, a.key)
	result, complete, err := r.awaitRuntimeAttemptControl(id, workersessions.ControlActionTerminate, target)
	if !complete || err != nil || result.Outcome != workersessions.ControlOutcomeNoop || result.Session.State != workersessions.StateCompleted || *calls != 0 {
		t.Fatalf("natural winner = %#v, %t, %v", result, complete, err)
	}
	otherID := scopedWorkerAddress(publicWorkerID(id), "factory-b")
	r.sessions[otherID] = r.sessions[id]
	if _, _, _, err := r.claimFrozenRuntimeControl(otherID, target); !errors.Is(err, workersessions.ErrInvalidState) {
		t.Fatalf("completed handle granted cross-scope authority: %v", err)
	}
}

func TestControlFrozenDirectRefusesRuntimeAdmissionDuringHistory(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	const id = "unadmitted-worker"
	r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateStarting}
	r.publications[id] = &publication{open: true}
	r.events = &controlReplacementAppender{EventsAppender: r.events, replace: func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.runtimeAttemptControls[id] = &runtimeAttempt{workerID: id}
	}}
	result, err := r.Cancel(context.Background(), workersessions.ControlRequest{ID: id})
	if !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || r.sessions[id].State != workersessions.StateStarting {
		t.Fatalf("control terminalized newly admitted Runtime worker = %#v, %v", result, err)
	}
}

func TestControlFrozenHistoryDoesNotRediscoverDispatch(t *testing.T) {
	t.Parallel()
	r := newTestRegistry(t)
	const id = "worker-history-target"
	s := newSupervision("accepted-attempt", "")
	s.accepted = true
	r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateRunning}
	r.supervisions[id] = s
	r.publications[id] = &publication{open: true}
	sink := &perRuntimeAppendCapture{EventsAppender: r.events}
	r.events = sink
	target, err := r.freezeControlTarget(id)
	if err != nil {
		t.Fatal(err)
	}
	s.dispatchID = "replacement-attempt"
	reservation, err := r.beginFrozenControlHistory(context.Background(), id, workersessions.ControlActionCancel, "", target)
	if err != nil {
		t.Fatal(err)
	}
	result, retry, err := r.cancelControlIteration(context.Background(), workersessions.ControlRequest{ID: id}, workersessions.ControlActionCancel, true, target)
	if retry || !errors.Is(err, workersessions.ErrInvalidState) {
		t.Fatalf("replacement control = %#v, %t, %v", result, retry, err)
	}
	r.finishControlHistory(reservation, result.Outcome, result.DispatchID, result.Session.State)
	assertFrozenRuntimeHistory(t, sink, "accepted-attempt")
}
