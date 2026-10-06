package service

import (
	"context"
	"errors"
	"os"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// A completed operation is read-only authority for its original request, even
// when the live attempt has moved on. Never reconstruct a capability from it.
// The request lock serializes this lookup with the original effect and join.
func (r *registry) replayForceResult(ctx context.Context, req workersessions.ControlRequest) (workersessions.ControlResult, bool, error) {
	failed := workersessions.ControlResult{Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeFailed, Forced: true}
	target, live, err := r.forceReplayCapture(ctx, req)
	if errors.Is(err, os.ErrNotExist) || (err == nil && target.RecordingID == "") {
		return failed, false, nil
	}
	if err != nil {
		if live && !errors.Is(err, workersessions.ErrInvalidState) {
			// An unavailable journal must not remove live safety stopping.
			return failed, false, nil
		}
		return failed, true, err
	}
	key := recordings.WorkerControlOperationKey{RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID, FactorySessionID: target.FactorySessionID, RequestID: req.RequestID}
	record, err := r.operations.LoadWorkerControlOperation(ctx, key)
	if err != nil {
		found, readErr := forceReplayReadError(err, live)
		return failed, found, readErr
	}
	intent := stopIntent(req, workersessions.ControlActionTerminate, frozenControlTarget{capture: target})
	if record.Validate() != nil || !record.SameIntent(intent) {
		return failed, true, workersessions.ErrInvalidState
	}
	if record.Operation.Phase == "INTENT" && live {
		// Only fresh live fencing may claim an unfinished operation's effect.
		return failed, false, nil
	}
	var saved workersessions.ControlResult
	if !readCommittedStopResult(record.Result, &saved) {
		return failed, true, workersessions.ErrInvalidState
	}
	result, replayErr := recoveredForceResult(record, intent, saved.DispatchID)
	r.logger.Info("worker session force replay", "sessionID", target.WorkerSessionID, "attemptID", req.ExpectedAttemptID, "action", "kill", "outcome", string(result.Outcome))
	return result, true, replayErr
}

func forceReplayReadError(err error, live bool) (bool, error) {
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if errors.Is(err, recordings.ErrWorkerControlConflict) || errors.Is(err, recordings.ErrInvalidWorkerControlOperation) {
		return true, workersessions.ErrInvalidState
	}
	if live {
		return false, nil
	}
	return true, recordings.ErrWorkerRecordingPersistence
}

func (r *registry) forceReplayCapture(ctx context.Context, req workersessions.ControlRequest) (recordings.WorkerControlTarget, bool, error) {
	address := r.workerAddress(req.ID, req.FactorySessionID)
	r.mu.RLock()
	_, live := r.sessions[address]
	identityExists := r.workerIdentityExistsLocked(publicWorkerID(req.ID))
	r.mu.RUnlock()
	if identityExists && !live {
		return recordings.WorkerControlTarget{}, false, workersessions.ErrSessionNotFound
	}
	if !live {
		address = publicWorkerID(req.ID)
	}
	target, err := r.interruptReplayCapture(ctx, address)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return target, live, err
		}
		return target, live, recordings.ErrWorkerRecordingPersistence
	}
	scope := req.FactorySessionID
	if scope == "" {
		scope = workerAddressScope(req.ID)
	}
	if target.RecordingID != "" && (target.WorkerSessionID != publicWorkerID(req.ID) || (scope != "" && target.FactorySessionID != scope)) {
		return target, live, workersessions.ErrInvalidState
	}
	return target, live, nil
}
