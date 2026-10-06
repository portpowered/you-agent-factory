package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// A request owns its sync/effect/join sequence, without holding registry or
// publication locks. The attempt is fenced again by executeFrozenStop after
// every asynchronous boundary, including journal acknowledgement.
func (r *registry) stopWithCommittedIntent(ctx context.Context, req workersessions.ControlRequest, action workersessions.ControlAction, detach bool, target frozenControlTarget) (workersessions.ControlResult, error) {
	// Reserved/pre-opening sessions have no captured execution to journal.
	if target.capture.RecordingID == "" || target.dispatchID == "" {
		return r.executeFrozenStop(ctx, req, action, detach, target)
	}
	r.mu.RLock()
	terminal := r.sessions[req.ID].Terminal()
	r.mu.RUnlock()
	if terminal && req.RequestID == "" {
		// A terminal repeat needs only the original callback join and ownership
		// fence. It must not overwrite the committed winner or require storage.
		return r.executeFrozenStop(ctx, req, action, detach, target)
	}
	intent := stopIntent(req, action, target)
	value, _ := r.stopOperations.LoadOrStore(intent.Operation.RequestID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	// Once accepted, transport cancellation cannot abandon the owned stop/join.
	owned := controlContext(ctx)
	accepted, _, storeErr := r.operations.BeginWorkerControlOperation(owned, intent)
	if errors.Is(storeErr, recordings.ErrWorkerControlConflict) || errors.Is(storeErr, recordings.ErrInvalidWorkerControlOperation) {
		r.logger.Warn("worker session control intent refused", "sessionID", publicWorkerID(req.ID), "attemptID", target.dispatchID, "action", string(action), "outcome", "conflict")
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed, DispatchID: target.dispatchID}, errors.Join(workersessions.ErrInvalidState, storeErr)
	}
	if storeErr == nil {
		r.logger.Info("worker session control intent", "sessionID", publicWorkerID(req.ID), "attemptID", target.dispatchID, "action", string(action), "phase", accepted.Operation.Phase, "outcome", "committed")
	}
	if storeErr == nil && accepted.Operation.Phase == "FAILED" {
		return workersessions.ControlResult{Action: action, Outcome: workersessions.ControlOutcomeFailed, DispatchID: target.dispatchID}, workersessions.ErrInvalidState
	}
	result, stopErr := r.executeFrozenStop(owned, req, action, detach, target)
	if storeErr == nil && accepted.Operation.Phase != "COMPLETED" {
		storeErr = r.commitStopResult(owned, accepted, result, stopErr)
	}
	if storeErr != nil {
		r.recordStopPersistenceLoss(owned, req.ID, target)
		// Live safety survives journal failure, but no durable success is claimed.
		result.Outcome = workersessions.ControlOutcomeFailed
		return result, errors.Join(stopErr, recordings.ErrWorkerRecordingPersistence)
	}
	return result, stopErr
}

func stopIntent(req workersessions.ControlRequest, action workersessions.ControlAction, target frozenControlTarget) recordings.WorkerControlOperationRecord {
	capture := target.capture
	capture.ExpectedAttemptID = target.dispatchID
	if target.runtime != nil {
		capture.ExpectedAttemptID = target.runtime.attemptID
	}
	operationAction := strings.ToLower(string(action))
	if req.Force {
		capture.ExpectedAttemptID = req.ExpectedAttemptID
		operationAction = "kill"
	}
	// Only the immutable address/action is hashed. No provider content or secret
	// is needed for the bodyless stop request. JSON avoids delimiter collisions.
	input, _ := json.Marshal(struct {
		Target recordings.WorkerControlTarget
		Action string
	}{capture, operationAction})
	digest := sha256.Sum256(input)
	encoded := hex.EncodeToString(digest[:])
	key := req.RequestID
	if key == "" {
		key = "stop/" + encoded
	}
	return recordings.WorkerControlOperationRecord{
		Target: capture, Revision: 1,
		Operation: recordings.WorkerControlOperation{
			Version: 1, RequestID: key, WorkerSessionID: capture.WorkerSessionID,
			ExpectedAttemptID: capture.ExpectedAttemptID, Action: operationAction, Phase: "INTENT", InputDigest: encoded,
		},
	}
}

func (r *registry) commitStopResult(ctx context.Context, record recordings.WorkerControlOperationRecord, result workersessions.ControlResult, stopErr error) error {
	// Persist only safe control facts, never a terminal error's text, output,
	// provider reference or execution recipe. Canonical history owns those reads.
	safe := result
	safe.Session = workersessions.Session{ID: result.Session.ID, State: result.Session.State}
	payload, err := json.Marshal(safe)
	if err != nil {
		return recordings.ErrWorkerRecordingPersistence
	}
	previous := record.Revision
	record.Revision++
	record.Operation.Phase = "COMPLETED"
	if stopErr != nil {
		record.Operation.Phase = "FAILED"
		record.FailureCode = "STOP_FAILED"
	}
	record.Result = payload
	_, err = r.operations.AdvanceWorkerControlOperation(ctx, record, previous)
	return err
}

func (r *registry) recordStopPersistenceLoss(ctx context.Context, id string, target frozenControlTarget) {
	r.logger.Warn("worker session control persistence failed", "sessionID", publicWorkerID(id), "attemptID", target.dispatchID, "outcome", "degraded")
	if writer, ok := r.operations.(recordings.WorkerRecordingFailureWriter); ok {
		_ = writer.PersistWorkerRecordingFailure(ctx, recordings.WorkerRecordingFailure{
			RecordingID: target.capture.RecordingID, WorkerSessionID: target.capture.WorkerSessionID,
			Topic: r.observationTopic(id), Code: "CONTROL_OPERATION_PERSISTENCE_FAILED",
		})
	}
}
