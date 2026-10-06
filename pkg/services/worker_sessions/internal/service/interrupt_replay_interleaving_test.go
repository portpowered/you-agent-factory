package service

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

type interruptInterleavingStore struct {
	*interruptInputStore
	beforeLoad func()
}

func (store *interruptInterleavingStore) LoadWorkerControlOperation(ctx context.Context, key recordings.WorkerControlOperationKey) (recordings.WorkerControlOperationRecord, error) {
	store.beforeLoad()
	return store.interruptInputStore.LoadWorkerControlOperation(ctx, key)
}

// An owner can reserve and journal its operation after another caller's initial
// live lookup. The second caller must join that owner, with tuple conflicts
// checked by normal admission, rather than treating the pending journal as recovery.
func TestInterruptReplayDefersToOwnerReservedDuringJournalRead(t *testing.T) {
	t.Parallel()
	for _, conflict := range []bool{false, true} {
		name := "matching request"
		if conflict {
			name = "conflicting request"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			req := plan.request.Normalize()
			tuple := interruptTuple{sourceID: req.SourceWorkerSessionID, successorID: req.SuccessorWorkerSessionID, message: req.ReplacementMessage, mode: req.ResumeMode}
			if conflict {
				tuple.message = "other replacement"
			}
			replay := &interruptReplay{tuple: tuple, done: make(chan struct{})}
			r.operations = &interruptInterleavingStore{interruptInputStore: store, beforeLoad: func() {
				r.mu.Lock()
				r.interruptReplays = map[string]*interruptReplay{req.RequestID: replay}
				r.mu.Unlock()
				if _, err := r.beginInterruptIntent(t.Context(), plan); err != nil {
					t.Fatal(err)
				}
			}}
			_, found, err := r.replayDurableInterrupt(t.Context(), req)
			if found || err != nil {
				t.Fatalf("pending live operation treated as recovery: found=%v err=%v", found, err)
			}
			reserved, owner, err := r.reserveInterrupt(req)
			if conflict {
				if !errors.Is(err, workersessions.ErrInterruptRequestIDConflict) {
					t.Fatalf("conflicting live request error = %v", err)
				}
			} else if err != nil || owner || reserved != replay {
				t.Fatalf("live replay = %p owner=%v err=%v, want existing owner %p", reserved, owner, err, replay)
			}
		})
	}
}
