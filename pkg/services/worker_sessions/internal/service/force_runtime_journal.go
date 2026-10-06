package service

import (
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// Retain journal authority separately from live execution. Completion removes
// the live handle to release joiners; that must not also reopen retry admission.
func (r *registry) beginFrozenForceJournal(id string, target frozenControlTarget) (func(), error) {
	unlock, err := r.lockFrozenCapture(id, target)
	if err != nil {
		return nil, err
	}
	defer unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.supervisions[id] != target.supervision {
		return nil, staleControlTargetError()
	}
	if target.runtime == nil {
		if r.runtimeAttemptControls[id] != nil {
			return nil, staleControlTargetError()
		}
		return target.supervision.beginForceJournal(), nil
	}
	a := target.runtime
	if err := r.frozenRuntimeOwnerLocked(id, a); err != nil {
		return nil, err
	}
	if existing := r.runtimeForceJournals[a.key]; existing != nil && existing != a {
		return nil, staleControlTargetError()
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.forceJournalPending == 0 {
		a.forceJournalDone = make(chan struct{})
	}
	a.forceJournalPending++
	if r.runtimeForceJournals == nil {
		r.runtimeForceJournals = make(map[workersessions.RuntimeAttemptKey]*runtimeAttempt)
	}
	r.runtimeForceJournals[a.key] = a
	return a.finishForceJournal, nil
}

func (a *runtimeAttempt) finishForceJournal() {
	r := a.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.forceJournalPending--
	if a.forceJournalPending != 0 {
		return
	}
	if !a.controlPersistenceLost && r.runtimeForceJournals[a.key] == a {
		delete(r.runtimeForceJournals, a.key)
	}
	close(a.forceJournalDone)
}

func (target frozenControlTarget) markControlPersistenceLost() {
	target.supervision.markControlPersistenceLost()
	if a := target.runtime; a != nil {
		a.mu.Lock()
		a.controlPersistenceLost = true
		a.mu.Unlock()
	}
}

func (a *runtimeAttempt) awaitForceJournal() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for a.forceJournalPending != 0 {
		wait := a.forceJournalDone
		a.mu.Unlock()
		<-wait
		a.mu.Lock()
	}
	if a.controlPersistenceLost {
		return recordings.ErrWorkerRecordingPersistence
	}
	return nil
}
