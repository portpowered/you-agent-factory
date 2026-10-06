package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const forceJoinDeadline = 10 * time.Second

type forceControlClaim struct {
	control providers.AttemptControl
	wait    <-chan struct{}
	joined  <-chan struct{}
	resolve func(bool, error)
}

func (r *registry) forceTerminate(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, error) {
	failed := workersessions.ControlResult{Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeFailed, Forced: true}
	if ctx != nil && ctx.Err() != nil {
		return failed, ctx.Err()
	}
	if err := req.Validate(); err != nil {
		return failed, err
	}
	value, _ := r.stopOperations.LoadOrStore(req.RequestID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if result, found, err := r.replayForceResult(ctx, req); found {
		return result, err
	}
	if err := r.validateControlTarget(req); err != nil {
		return failed, err
	}
	req.ID = r.workerAddress(req.ID, req.FactorySessionID)
	target, err := r.freezeControlTarget(req.ID)
	if err != nil {
		return failed, err
	}
	failed.DispatchID = target.dispatchID
	if req.ExpectedAttemptID != target.attemptID {
		return failed, staleControlTargetError()
	}
	intent := stopIntent(req, workersessions.ControlActionTerminate, target)
	owned, cancel := context.WithCancelCause(controlContext(ctx))
	timer := r.scheduler.NewTimer(forceJoinDeadline)
	defer timer.Stop()
	defer cancel(context.Canceled)
	go func() {
		select {
		case <-timer.C():
			cancel(context.DeadlineExceeded)
		case <-owned.Done():
		}
	}()
	return r.forceWithCommittedIntent(owned, req, target, intent)
}

func (r *registry) forceWithCommittedIntent(ctx context.Context, req workersessions.ControlRequest, target frozenControlTarget, intent recordings.WorkerControlOperationRecord) (workersessions.ControlResult, error) {
	finishJournal := target.supervision.beginForceJournal()
	defer finishJournal()
	var accepted recordings.WorkerControlOperationRecord
	var storeErr error
	journal := target.capture.RecordingID != ""
	if journal {
		accepted, _, storeErr = r.operations.BeginWorkerControlOperation(ctx, intent)
		if errors.Is(storeErr, recordings.ErrWorkerControlConflict) || errors.Is(storeErr, recordings.ErrInvalidWorkerControlOperation) {
			return r.forceResult(req.ID, target, workersessions.ControlOutcomeFailed), errors.Join(workersessions.ErrInvalidState, storeErr)
		}
		if storeErr == nil && (accepted.Operation.Phase == "COMPLETED" || accepted.Operation.Phase == "FAILED") {
			return recoveredForceResult(accepted, intent, target.dispatchID)
		}
	} else if r.logs != nil {
		storeErr = recordings.ErrWorkerRecordingPersistence
	}
	if storeErr != nil {
		target.supervision.markControlPersistenceLost()
	}
	r.logger.Info("worker session force intent", "sessionID", publicWorkerID(req.ID), "attemptID", target.attemptID, "action", "kill", "durable", journal && storeErr == nil)
	result, stopErr := r.executeFrozenForce(ctx, req.ID, target)
	if journal && storeErr == nil {
		// The effect deadline must not prevent saving its authoritative failure.
		storeErr = r.commitStopResult(context.WithoutCancel(ctx), accepted, result, stopErr)
	}
	if storeErr != nil {
		target.supervision.markControlPersistenceLost()
		r.recordStopPersistenceLoss(context.WithoutCancel(ctx), req.ID, target)
		result.Outcome = workersessions.ControlOutcomeFailed
		stopErr = errors.Join(stopErr, recordings.ErrWorkerRecordingPersistence)
	}
	r.logger.Info("worker session force outcome", "sessionID", publicWorkerID(req.ID), "attemptID", target.attemptID, "action", "kill", "outcome", string(result.Outcome))
	return result, stopErr
}

// Completion may release the attempt join before the journal acknowledges its
// result. Keep successor admission fenced throughout that acknowledgement.
// This gate belongs to the captured supervision, never a replacement owner.
func (s *supervision) beginForceJournal() func() {
	if s == nil {
		return func() {}
	}
	s.mu.Lock()
	if s.forceJournalPending == 0 {
		s.forceJournalDone = make(chan struct{})
	}
	s.forceJournalPending++
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.forceJournalPending--
		if s.forceJournalPending == 0 {
			close(s.forceJournalDone)
		}
		s.mu.Unlock()
	}
}

func (s *supervision) markControlPersistenceLost() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.controlPersistenceLost = true
	s.mu.Unlock()
}

func recoveredForceResult(record, intent recordings.WorkerControlOperationRecord, dispatchID string) (workersessions.ControlResult, error) {
	failed := workersessions.ControlResult{Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeFailed, DispatchID: dispatchID, Forced: true}
	var result workersessions.ControlResult
	if record.Validate() != nil || !record.SameIntent(intent) || !readCommittedStopResult(record.Result, &result) || !result.Forced ||
		result.Action != workersessions.ControlActionTerminate || result.Session.ID != intent.Target.WorkerSessionID || result.DispatchID != dispatchID || !result.Session.State.Valid() {
		return failed, workersessions.ErrInvalidState
	}
	if record.Operation.Phase == "FAILED" {
		if record.FailureCode != "STOP_FAILED" || result.Outcome != workersessions.ControlOutcomeFailed {
			return failed, workersessions.ErrInvalidState
		}
		return result, errRuntimeAttemptControlUnavailable
	}
	if !validCompletedForceResult(record, result) {
		return failed, workersessions.ErrInvalidState
	}
	return result, nil
}

func validCompletedForceResult(record recordings.WorkerControlOperationRecord, result workersessions.ControlResult) bool {
	if record.Operation.Phase != "COMPLETED" || record.FailureCode != "" {
		return false
	}
	switch result.Outcome {
	case workersessions.ControlOutcomeApplied:
		return result.Session.State == workersessions.StateTerminated
	case workersessions.ControlOutcomeNoop:
		return result.Session.Terminal()
	case workersessions.ControlOutcomeUnsupported:
		return true
	default:
		return false
	}
}

func (r *registry) executeFrozenForce(ctx context.Context, id string, target frozenControlTarget) (workersessions.ControlResult, error) {
	for {
		if err := context.Cause(ctx); err != nil {
			return r.forceResult(id, target, workersessions.ControlOutcomeFailed), errors.Join(errRuntimeAttemptControlUnavailable, err)
		}
		claim, err := r.claimFrozenForce(id, target)
		if err != nil {
			return r.forceResult(id, target, workersessions.ControlOutcomeFailed), err
		}
		if claim.wait != nil {
			if err := awaitForceJoin(ctx, claim.wait); err != nil {
				return r.forceResult(id, target, workersessions.ControlOutcomeFailed), err
			}
			continue
		}
		if claim.control == nil {
			outcome := workersessions.ControlOutcomeUnsupported
			if claim.joined != nil {
				if err := awaitForceJoin(ctx, claim.joined); err != nil {
					return r.forceResult(id, target, workersessions.ControlOutcomeFailed), err
				}
				outcome = workersessions.ControlOutcomeNoop
			}
			return r.forceResult(id, target, outcome), nil
		}
		return r.applyForceClaim(ctx, id, target, claim)
	}
}

func (r *registry) applyForceClaim(ctx context.Context, id string, target frozenControlTarget, claim forceControlClaim) (workersessions.ControlResult, error) {
	type outcome struct {
		killed bool
		err    error
	}
	completed := make(chan outcome, 1)
	go func() {
		defer func() {
			if recover() != nil {
				completed <- outcome{err: errRuntimeAttemptControlUnavailable}
			}
		}()
		killed, err := claim.control.ForceKill(ctx)
		completed <- outcome{killed, err}
	}()
	var effect outcome
	select {
	case effect = <-completed:
	case <-ctx.Done():
		effect.err = context.Cause(ctx)
	}
	// A late success after the service deadline is never APPLIED.
	if cause := context.Cause(ctx); cause != nil {
		effect.err = cause
	}
	claim.resolve(effect.killed, effect.err)
	if effect.err != nil {
		return r.forceResult(id, target, workersessions.ControlOutcomeFailed), errRuntimeAttemptControlUnavailable
	}
	if !effect.killed {
		return r.forceResult(id, target, workersessions.ControlOutcomeUnsupported), nil
	}
	if err := awaitForceJoin(ctx, claim.joined); err != nil {
		return r.forceResult(id, target, workersessions.ControlOutcomeFailed), err
	}
	result := r.forceResult(id, target, workersessions.ControlOutcomeApplied)
	if result.Session.State != workersessions.StateTerminated {
		result.Outcome = workersessions.ControlOutcomeFailed
		return result, errRuntimeAttemptControlUnavailable
	}
	return result, nil
}

func awaitForceJoin(ctx context.Context, joined <-chan struct{}) error {
	select {
	case <-joined:
		if context.Cause(ctx) == nil {
			return nil
		}
	case <-ctx.Done():
	}
	return errors.Join(errRuntimeAttemptControlUnavailable, context.Cause(ctx))
}

func (r *registry) forceResult(id string, target frozenControlTarget, outcome workersessions.ControlOutcome) workersessions.ControlResult {
	r.mu.RLock()
	session := cloneSession(r.sessions[id])
	r.mu.RUnlock()
	return workersessions.ControlResult{Session: session, Action: workersessions.ControlActionTerminate, Outcome: outcome, DispatchID: target.dispatchID, Forced: true}
}

// Publication, registry and attempt locks fence the effect claim. Completion
// may retire the capability while it joins, but cannot replace this generation.
func (r *registry) claimFrozenForce(id string, target frozenControlTarget) (forceControlClaim, error) {
	unlock, err := r.lockFrozenCapture(id, target)
	if err != nil {
		return forceControlClaim{}, err
	}
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.supervisions[id] != target.supervision || (target.runtime == nil && r.runtimeAttemptControls[id] != nil) {
		return forceControlClaim{}, staleControlTargetError()
	}
	if target.runtime != nil {
		if err := r.frozenRuntimeOwnerLocked(id, target.runtime); err != nil {
			return forceControlClaim{}, err
		}
		return target.runtime.claimForce(r.sessions[id].Terminal()), nil
	}
	if target.supervision == nil {
		return forceControlClaim{}, nil
	}
	s := target.supervision
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimForce(target, r.sessions[id].Terminal())
}

// The caller holds the owning supervision lock and registry admission lock.
func (s *supervision) claimForce(target frozenControlTarget, terminal bool) (forceControlClaim, error) {
	if s.dispatchID != target.dispatchID || s.providerAttempt != target.providerAttempt {
		return forceControlClaim{}, staleControlTargetError()
	}
	if s.controlPersistenceLost && s.forceSafetyClaimed {
		return forceControlClaim{}, recordings.ErrWorkerRecordingPersistence
	}
	if terminal || s.controlAction != "" {
		return forceControlClaim{joined: s.done}, nil
	}
	if s.interrupting {
		return forceControlClaim{wait: s.interruptDone}, nil
	}
	if s.controlActive {
		return forceControlClaim{wait: s.controlDone}, nil
	}
	if !s.accepted || s.providerAttempt == nil || s.providerAttempt.retired || s.providerAttempt.control == nil {
		return forceControlClaim{}, nil
	}
	s.controlActive, s.forcePending = true, true
	s.forceSafetyClaimed = true
	s.controlDone = make(chan struct{})
	return forceControlClaim{control: s.providerAttempt.control, joined: s.done, resolve: s.resolveForce}, nil
}

func (s *supervision) resolveForce(killed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if killed && err == nil {
		s.forceConfirmed = true
		s.requestedAction = workersessions.ControlActionTerminate
		s.controlAction = workersessions.ControlActionTerminate
	}
	s.forcePending, s.controlActive = false, false
	close(s.controlDone)
}

func (a *runtimeAttempt) claimForce(terminal bool) forceControlClaim {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.completed == nil {
		a.completed = make(chan struct{})
	}
	if terminal || a.completing || a.controlAction != "" {
		return forceControlClaim{joined: a.completed}
	}
	if a.controlPending {
		return forceControlClaim{wait: a.controlDone}
	}
	if a.providerControlRetired || a.providerControl == nil {
		return forceControlClaim{}
	}
	a.controlPending = true
	a.controlDone = make(chan struct{})
	return forceControlClaim{control: a.providerControl, joined: a.completed, resolve: func(killed bool, err error) {
		outcome := workers.WorkstationDispatchCancelOutcomeAlreadyTerminal
		if killed {
			outcome = workers.WorkstationDispatchCancelOutcomeCanceled
		}
		a.mu.Lock()
		a.forceConfirmed = killed && err == nil
		a.mu.Unlock()
		a.resolveControl(workersessions.ControlActionTerminate, outcome, err)
	}}
}
