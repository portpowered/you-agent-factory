package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// A committed outcome is read-only authority. Never turn an incomplete stage
// into a new cancellation or admission just because the replay map is absent.
func (r *registry) replayDurableInterrupt(ctx context.Context, req workersessions.InterruptRequest) (workersessions.InterruptResult, bool, error) {
	r.mu.RLock()
	_, liveReplay := r.interruptReplays[req.RequestID]
	r.mu.RUnlock()
	if liveReplay {
		return workersessions.InterruptResult{}, false, nil
	}
	result := r.interruptResultSnapshot(req, workersessions.InterruptPhaseValidation, false)
	target, err := r.interruptReplayCapture(ctx, req.SourceWorkerSessionID)
	if errors.Is(err, os.ErrNotExist) || target.RecordingID == "" && err == nil {
		return workersessions.InterruptResult{}, false, nil
	}
	if err != nil {
		return result, true, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
	}
	key := recordings.WorkerControlOperationKey{RecordingID: target.RecordingID, WorkerSessionID: target.WorkerSessionID, FactorySessionID: target.FactorySessionID, RequestID: req.RequestID}
	record, err := r.operations.LoadWorkerControlOperation(ctx, key)
	if errors.Is(err, os.ErrNotExist) {
		return workersessions.InterruptResult{}, false, nil
	}
	if err != nil {
		return result, true, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
	}
	payload, _ := json.Marshal(req)
	if record.Operation.Action != "interrupt" || record.Operation.SuccessorWorkerSessionID != req.SuccessorWorkerSessionID {
		return result, true, newInterruptError(result.Phase, result, workersessions.ErrInterruptRequestIDConflict)
	}
	target.ExpectedAttemptID = record.Target.ExpectedAttemptID
	if record.Target != target {
		return result, true, newInterruptError(result.Phase, result, workersessions.ErrInterruptSourceConflict)
	}
	if err := r.validateInterruptReplayInput(ctx, key, req, record, payload); err != nil {
		r.logger.Warn("worker session interrupt replay refused", "sessionID", req.SourceWorkerSessionID, "requestID", req.RequestID, "phase", "VALIDATION", "outcome", "persistence_unavailable")
		return result, true, newInterruptError(result.Phase, result, err)
	}
	replayed, replayErr := decodeInterruptOutcome(req, record)
	if record.Operation.Phase == "SOURCE_STOPPED" && errors.Is(replayErr, workersessions.ErrInterruptExecutionUnavailable) {
		if err := r.inspectPendingInterruptSuccessor(ctx, req, record.Target); err != nil {
			replayErr = newInterruptError(replayed.Phase, replayed, err)
		}
	}
	r.logger.Info("worker session interrupt replay", "sessionID", req.SourceWorkerSessionID, "requestID", req.RequestID, "phase", record.Operation.Phase, "outcome", "read_only")
	return replayed, true, replayErr
}

// An opening precedes provider admission. Inspect the reserved identity to
// distinguish a collision from uncertain admission, but never turn either an
// opening or its absence into authority to execute again after owner loss.
func (r *registry) inspectPendingInterruptSuccessor(ctx context.Context, req workersessions.InterruptRequest, target recordings.WorkerControlTarget) error {
	if r.logs == nil {
		return nil
	}
	entry, err := r.logs.reader.LookupWorkerSessionCapture(ctx, req.SuccessorWorkerSessionID)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return recordings.ErrWorkerRecordingPersistence
	}
	if entry.WorkerSessionID != req.SuccessorWorkerSessionID || entry.FactorySessionID != "" || entry.OwnerEpoch != target.OwnerEpoch {
		return workersessions.ErrInterruptSourceConflict
	}
	if entry.RecordingID == "" || entry.RecordingGenerationID == "" {
		return recordings.ErrWorkerRecordingPersistence
	}
	page, err := r.logs.reader.ReadWorkerCapturedActivity(ctx, recordings.WorkerCapturedActivityRequest{WorkerSessionID: req.SuccessorWorkerSessionID, Limit: 1})
	// The capture may grow between these reads; only identity is immutable.
	entry.CommittedPosition = page.Catalog.CommittedPosition
	if err != nil || page.Catalog != entry {
		return recordings.ErrWorkerRecordingPersistence
	}
	return validatePendingInterruptOpening(page, req, target.ExpectedAttemptID)
}

func validatePendingInterruptOpening(page recordings.WorkerCapturedActivityPage, req workersessions.InterruptRequest, sourceAttempt string) error {
	var draft workers.Draft
	var opening workers.SessionPayload
	if readPendingInterruptOpeningJSON(page.Opening.Payload, &draft) != nil || draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted ||
		readPendingInterruptOpeningJSON(draft.Payload, &opening) != nil {
		return recordings.ErrWorkerRecordingPersistence
	}
	if opening.WorkerSessionID != req.SuccessorWorkerSessionID || opening.FactorySessionID != "" || opening.RecordingID != page.Catalog.RecordingID {
		return workersessions.ErrInterruptSourceConflict
	}
	expected := continuationDispatchID(sourceAttempt, req.SuccessorWorkerSessionID)
	if opening.DispatchID != expected || opening.AttemptID != expected || opening.Lineage == nil {
		return workersessions.ErrInterruptSourceConflict
	}
	lineage := opening.Lineage
	if lineage.PredecessorWorkerSessionID != req.SourceWorkerSessionID || lineage.PreviousDispatchID != sourceAttempt || lineage.PreviousAttemptID != sourceAttempt {
		return workersessions.ErrInterruptSourceConflict
	}
	return nil
}

// Opening identities must not be shadowed by duplicate members or Go's
// case-insensitive aliases before recovery compares the frozen target.
func readPendingInterruptOpeningJSON(payload []byte, decoded any) error {
	if !uniqueInterruptJSONFields(payload) {
		return recordings.ErrWorkerRecordingPersistence
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(decoded) != nil || decoder.Decode(new(any)) != io.EOF || !canonicalInterruptJSONFields(payload, decoded) {
		return recordings.ErrWorkerRecordingPersistence
	}
	return nil
}

// The journal digest identifies the original tuple; the immutable artifact is
// its recoverable input. Both must agree before a stored outcome is trusted.
// Read failures are normalized so filesystem/provider details stay private.
func (r *registry) validateInterruptReplayInput(ctx context.Context, key recordings.WorkerControlOperationKey, req workersessions.InterruptRequest, record recordings.WorkerControlOperationRecord, payload []byte) error {
	operation := record.Operation
	if operation.Version != 1 || operation.ResumeMode != "provider" || operation.RequestID != req.RequestID ||
		operation.WorkerSessionID != req.SourceWorkerSessionID || operation.ExpectedAttemptID == "" ||
		operation.ExpectedAttemptID != record.Target.ExpectedAttemptID || record.InputArtifactRef == "" {
		return recordings.ErrWorkerRecordingPersistence
	}
	stored, err := r.operations.ReadWorkerControlInput(ctx, key, record.InputArtifactRef)
	if err != nil {
		return recordings.ErrWorkerRecordingPersistence
	}
	digest := sha256.Sum256(stored)
	if operation.InputDigest != hex.EncodeToString(digest[:]) {
		return recordings.ErrWorkerRecordingPersistence
	}
	return validateCapturedInterruptInput(stored, payload, req, record.Target.ExpectedAttemptID)
}

func (r *registry) interruptReplayCapture(ctx context.Context, id string) (recordings.WorkerControlTarget, error) {
	if pub := r.publicationFor(id); pub != nil {
		pub.mu.Lock()
		capture := pub.capture
		pub.mu.Unlock()
		return capture, nil
	}
	if r.logs == nil {
		return recordings.WorkerControlTarget{}, os.ErrNotExist
	}
	entry, err := r.logs.reader.LookupWorkerSessionCapture(ctx, id)
	if err != nil {
		return recordings.WorkerControlTarget{}, err
	}
	if entry.WorkerSessionID != id || entry.FactorySessionID != "" || entry.RecordingGenerationID == "" || entry.OwnerEpoch == "" {
		return recordings.WorkerControlTarget{}, recordings.ErrWorkerControlConflict
	}
	return recordings.WorkerControlTarget{RecordingID: entry.RecordingID, WorkerSessionID: entry.WorkerSessionID, RecordingGenerationID: entry.RecordingGenerationID, OwnerEpoch: entry.OwnerEpoch}, nil
}

func decodeInterruptOutcome(req workersessions.InterruptRequest, record recordings.WorkerControlOperationRecord) (workersessions.InterruptResult, error) {
	result := interruptResult(req, workersessions.InterruptPhaseValidation, false)
	if record.Operation.Phase != "COMPLETED" && record.Operation.Phase != "FAILED" {
		return decodePendingInterruptOutcome(req, record)
	}
	var outcome durableInterruptOutcome
	if readInterruptOutcome(record.Result, &outcome) != nil || !validInterruptOutcome(req, outcome.InterruptResult) {
		result = interruptResult(req, workersessions.InterruptPhaseValidation, false)
		return result, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
	}
	result = outcome.InterruptResult
	if record.Operation.Phase == "COMPLETED" && committedInterruptSucceeded(req, result) && len(outcome.FailureCauses) == 0 && record.FailureCode == "" {
		return result, nil
	}
	if record.Operation.Phase == "COMPLETED" || record.FailureCode != string(result.Phase) {
		result = interruptResult(req, workersessions.InterruptPhaseValidation, false)
		return result, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
	}
	cause := workersessions.ErrInterruptExecutionUnavailable
	switch result.Phase {
	case workersessions.InterruptPhaseSourceCancellation:
		cause = workersessions.ErrInterruptSourceCancellationFailed
	case workersessions.InterruptPhaseSuccessorAdmission:
		cause = workersessions.ErrInterruptSuccessorAdmissionFailed
	}
	if len(outcome.FailureCauses) > 0 {
		var err error
		cause, err = interruptFailureCause(outcome.FailureCauses)
		if err != nil {
			result = interruptResult(req, workersessions.InterruptPhaseValidation, false)
			return result, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
		}
	}
	return result, newInterruptError(result.Phase, result, cause)
}

func decodePendingInterruptOutcome(req workersessions.InterruptRequest, record recordings.WorkerControlOperationRecord) (workersessions.InterruptResult, error) {
	result := interruptResult(req, workersessions.InterruptPhaseValidation, false)
	// Legacy phase-only records establish no response snapshot.
	if len(record.Result) == 0 || record.Operation.Phase == "INTENT" {
		return result, newInterruptError(result.Phase, result, workersessions.ErrInterruptExecutionUnavailable)
	}
	var outcome durableInterruptOutcome
	if readInterruptOutcome(record.Result, &outcome) != nil || !validInterruptOutcome(req, outcome.InterruptResult) ||
		len(outcome.FailureCauses) != 0 || record.FailureCode != "" || !validPendingInterruptSnapshot(req, record.Operation.Phase, outcome.InterruptResult) {
		return result, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
	}
	result = outcome.InterruptResult
	// SUCCESSOR_ADMITTED already synced the complete accepted response after
	// source join and admission. A missing final bookkeeping row cannot erase
	// that success or license a second admission. Replay stays read-only.
	if record.Operation.Phase == "SUCCESSOR_ADMITTED" {
		return result, nil
	}
	return result, newInterruptError(result.Phase, result, workersessions.ErrInterruptExecutionUnavailable)
}

func validPendingInterruptSnapshot(req workersessions.InterruptRequest, phase string, result workersessions.InterruptResult) bool {
	switch phase {
	case "SOURCE_STOPPED":
		return result.Phase == workersessions.InterruptPhaseSuccessorAdmission && result.Source.State == workersessions.StateCanceled &&
			!result.Accepted && result.Successor.ID == "" && result.Source.SuccessorWorkerSessionID == ""
	case "SUCCESSOR_ADMITTED":
		return committedInterruptSucceeded(req, result)
	default:
		return false
	}
}

func committedInterruptSucceeded(req workersessions.InterruptRequest, result workersessions.InterruptResult) bool {
	return result.Accepted && result.Phase == workersessions.InterruptPhaseSuccessorAdmission &&
		result.Source.ID == req.SourceWorkerSessionID && result.Source.State == workersessions.StateCanceled &&
		result.Successor.ID == req.SuccessorWorkerSessionID && interruptSuccessorAdmittedState(result.Successor.State)
}
