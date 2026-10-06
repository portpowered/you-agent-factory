package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestControlFrozenAttemptRefusesReplacementAfterWait(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		for _, replacement := range []string{"dispatch", "supervision"} {
			t.Run(string(action)+"/"+replacement, func(t *testing.T) {
				t.Parallel()
				r := newTestRegistry(t)
				const id = "frozen-worker"
				s := newSupervision("old-attempt", "")
				s.accepted = true
				s.controlActive = true
				s.controlDone = make(chan struct{})
				close(s.controlDone)
				r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateRunning}
				r.supervisions[id] = s
				target, err := r.freezeControlTarget(id)
				if err != nil {
					t.Fatal(err)
				}
				req := workersessions.ControlRequest{ID: id}
				if _, retry, err := r.cancelControlIteration(context.Background(), req, action, true, target); err != nil || !retry {
					t.Fatalf("waiting control = retry %t, %v", retry, err)
				}
				s.controlActive = false
				if replacement == "dispatch" {
					s.dispatchID = "new-attempt"
				} else {
					s = newSupervision("old-attempt", "")
					s.accepted = true
					r.supervisions[id] = s
				}
				cancelCalls := 0
				s.installCancel(func() { cancelCalls++ })
				result, retry, err := r.cancelControlIteration(context.Background(), req, action, true, target)
				if retry || !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || result.DispatchID != "old-attempt" {
					t.Fatalf("stale control = %#v, retry %t, %v", result, retry, err)
				}
				assertFrozenControlUnaffected(t, r, id, s, cancelCalls)
			})
		}
	}
}

func TestControlFrozenPausedAttemptPreventsContinuationAdmission(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			r, s, reference := newPausedContinuationRegistry(t)
			target, err := r.freezeControlTarget("worker-1")
			if err != nil {
				t.Fatal(err)
			}
			_, claim, err := r.claimFrozenCancellation("worker-1", action, target)
			if err != nil || claim.kind != cancellationAttemptPaused {
				t.Fatalf("paused stop claim = %#v, %v", claim, err)
			}
			if _, _, prepared := r.prepareContinuation("worker-1", s, reference); prepared {
				t.Fatal("resume admitted a continuation after terminal control claimed the paused attempt")
			}
			if s.dispatchID != target.dispatchID || s.attemptsMade != 0 || s.continuing || s.publishing {
				t.Fatal("refused continuation changed the captured attempt")
			}
		})
	}
}

type controlReplacementAppender struct {
	EventsAppender
	replace func()
	once    sync.Once
}

func (a *controlReplacementAppender) Append(ctx context.Context, req events.AppendRequest) (events.AppendResult, error) {
	a.once.Do(a.replace)
	return a.EventsAppender.Append(ctx, req)
}

func TestControlPublicStopFreezesAttemptBeforeHistoryPublication(t *testing.T) {
	t.Parallel()
	for _, action := range []workersessions.ControlAction{workersessions.ControlActionCancel, workersessions.ControlActionTerminate} {
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			r := newTestRegistry(t)
			const id = "worker-history-race"
			s := newSupervision("accepted-attempt", "")
			s.accepted = true
			cancelCalls := 0
			s.installCancel(func() { cancelCalls++ })
			r.sessions[id] = workersessions.Session{ID: id, State: workersessions.StateRunning}
			r.supervisions[id] = s
			r.publications[id] = &publication{open: true}
			r.events = &controlReplacementAppender{EventsAppender: r.events, replace: func() {
				r.mu.Lock()
				defer r.mu.Unlock()
				s.mu.Lock()
				defer s.mu.Unlock()
				s.dispatchID = "replacement-attempt"
			}}
			req := workersessions.ControlRequest{ID: id}
			var result workersessions.ControlResult
			var err error
			if action == workersessions.ControlActionCancel {
				result, err = r.Cancel(context.Background(), req)
			} else {
				result, err = r.Terminate(context.Background(), req)
			}
			if !errors.Is(err, workersessions.ErrInvalidState) || result.Outcome != workersessions.ControlOutcomeFailed || result.DispatchID != "accepted-attempt" {
				t.Fatalf("public stop after attempt replacement = %#v, %v", result, err)
			}
			assertFrozenControlUnaffected(t, r, id, s, cancelCalls)
		})
	}
}

func assertFrozenControlUnaffected(t *testing.T, r *registry, id string, s *supervision, cancelCalls int) {
	t.Helper()
	if cancelCalls != 0 || s.requestedAction != "" || s.controlAction != "" || s.controlActive || r.sessions[id].State != workersessions.StateRunning {
		t.Fatal("stale control affected the replacement execution")
	}
}
