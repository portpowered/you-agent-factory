package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// The host owns input sync, intent sync, source join and the reserved successor.
// Caller cancellation only stops waiting for this sequence.
func (r *registry) runDurableInterrupt(plan interruptPlan) (workersessions.InterruptResult, error) {
	ctx := context.WithoutCancel(r.serverOwnedContext())
	operation, err := r.beginInterruptIntent(ctx, plan)
	if err != nil {
		finishInterruptExecution(plan.supervision, false)
		finishInterruptOperation(plan.supervision)
		result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseValidation, false)
		if operation != nil {
			_ = r.commitInterruptResult(ctx, operation, result, err)
		}
		return result, newInterruptError(result.Phase, result, err)
	}
	result, interruptErr := r.runInterruptExecution(plan, operation)
	if operation != nil {
		if err := r.commitInterruptResult(ctx, operation, result, interruptErr); err != nil {
			return result, newInterruptError(result.Phase, result, recordings.ErrWorkerRecordingPersistence)
		}
	}
	return result, interruptErr
}

func (r *registry) beginInterruptIntent(ctx context.Context, plan interruptPlan) (*recordings.WorkerControlOperationRecord, error) {
	target, err := r.freezeControlTarget(plan.request.SourceWorkerSessionID)
	if err != nil || target.supervision != plan.supervision || target.dispatchID != plan.dispatchID {
		return nil, workersessions.ErrInterruptSourceConflict
	}
	// Component fixtures without a captured opening have no journal identity.
	// Production opening binds the capture before admitting the execution.
	if target.capture.RecordingID == "" {
		if r.logs != nil {
			return nil, recordings.ErrWorkerRecordingPersistence
		}
		return nil, nil
	}
	plan, err = r.capturedInterruptPlan(ctx, plan, target.capture)
	if err != nil {
		return nil, err
	}
	if !interruptInputSafe(plan) {
		return nil, recordings.ErrInvalidRecordingRedactionRequest
	}
	payload, err := encodeInterruptInput(plan)
	if err != nil {
		return nil, err
	}
	intent := stopIntent(workersessions.ControlRequest{RequestID: plan.request.RequestID}, workersessions.ControlActionInterrupt, target)
	digest := sha256.Sum256(payload)
	intent.Operation.InputDigest = hex.EncodeToString(digest[:])
	intent.Operation.SuccessorWorkerSessionID = plan.request.SuccessorWorkerSessionID
	intent.Operation.ResumeMode = "provider"
	key := interruptOperationKey(intent)
	ref, err := r.operations.PersistWorkerControlInput(ctx, key, payload)
	if err != nil {
		return nil, safeInterruptStoreError(err)
	}
	// Reuse is authoritative only after validating the immutable captured bytes.
	stored, err := r.operations.ReadWorkerControlInput(ctx, key, ref)
	if err != nil || !bytes.Equal(stored, payload) {
		return nil, recordings.ErrWorkerRecordingPersistence
	}
	intent.InputArtifactRef = ref
	accepted, err := r.commitInterruptIntent(ctx, intent)
	if err != nil {
		return nil, err
	}
	r.logger.Info("worker session interrupt intent", "sessionID", publicWorkerID(plan.request.SourceWorkerSessionID), "attemptID", plan.dispatchID, "phase", "INTENT", "outcome", "committed")
	if err := r.validateInterruptFence(plan, target); err != nil {
		return &accepted, err
	}
	return &accepted, nil
}

func (r *registry) commitInterruptIntent(ctx context.Context, intent recordings.WorkerControlOperationRecord) (recordings.WorkerControlOperationRecord, error) {
	accepted, created, err := r.operations.BeginWorkerControlOperation(ctx, intent)
	if err != nil {
		// A lost acknowledgement licenses effects only after Load confirms the
		// exact synced INTENT. A conflict or later phase belongs to recovery;
		// neither may authorize repeating cancellation or admission here.
		if !errors.Is(err, recordings.ErrWorkerControlConflict) {
			loaded, loadErr := r.operations.LoadWorkerControlOperation(ctx, interruptOperationKey(intent))
			if loadErr == nil && sameInterruptPhaseRecord(loaded, intent) {
				return loaded, nil
			}
		}
		return recordings.WorkerControlOperationRecord{}, safeInterruptStoreError(err)
	}
	if !created {
		return recordings.WorkerControlOperationRecord{}, workersessions.ErrInterruptSourceConflict
	}
	return accepted, nil
}

func (r *registry) validateInterruptFence(plan interruptPlan, target frozenControlTarget) error {
	unlock, err := r.lockFrozenCapture(plan.request.SourceWorkerSessionID, target)
	if err != nil {
		return workersessions.ErrInterruptSourceConflict
	}
	defer unlock()
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.supervisions[plan.request.SourceWorkerSessionID] != plan.supervision || r.sessions[plan.request.SourceWorkerSessionID].State != workersessions.StateRunning {
		return workersessions.ErrInterruptSourceConflict
	}
	plan.supervision.mu.Lock()
	defer plan.supervision.mu.Unlock()
	if plan.supervision.dispatchID != plan.dispatchID {
		return workersessions.ErrInterruptSourceConflict
	}
	return nil
}

func interruptOperationKey(record recordings.WorkerControlOperationRecord) recordings.WorkerControlOperationKey {
	return recordings.WorkerControlOperationKey{
		RecordingID: record.Target.RecordingID, WorkerSessionID: record.Target.WorkerSessionID,
		FactorySessionID: record.Target.FactorySessionID, RequestID: record.Operation.RequestID,
	}
}

// Refuse input requiring secret substitution: redaction would change the
// replacement actually sent to Providers. Values are inspected only in memory.
func interruptInputSafe(plan interruptPlan) bool {
	fields := []string{plan.request.RequestID, plan.request.SourceWorkerSessionID, plan.request.SuccessorWorkerSessionID, plan.request.ReplacementMessage}
	for _, value := range plan.execution.Execution.EnvVars {
		if interruptFieldsContain(fields, value) {
			return false
		}
	}
	for _, entry := range plan.execution.Execution.ProcessEnvironment {
		name, value, found := strings.Cut(entry, "=")
		if found && interruptSensitiveEnvironmentName(name) && interruptFieldsContain(fields, value) {
			return false
		}
	}
	return true
}

func interruptSensitiveEnvironmentName(name string) bool {
	// Use the Workers-owned classification with an empty value, so this check
	// cannot expose credentials or confuse a literal redaction marker with one
	// produced by the classifier. Recipe privacy must match captured diagnostics.
	projection := workers.ProjectCommandEnvForDiagnostics([]string{name + "="})
	return projection.Values[name] == workers.RedactedCommandEnvValue
}

func interruptFieldsContain(fields []string, value string) bool {
	if value == "" {
		return false
	}
	for _, field := range fields {
		if strings.Contains(field, value) {
			return true
		}
	}
	return false
}

func safeInterruptStoreError(err error) error {
	if errors.Is(err, recordings.ErrWorkerControlConflict) {
		return workersessions.ErrInterruptRequestIDConflict
	}
	return recordings.ErrWorkerRecordingPersistence
}

func (r *registry) advanceInterruptPhase(ctx context.Context, operation *recordings.WorkerControlOperationRecord, phase string) error {
	if operation == nil {
		return nil
	}
	next := operation.Detached()
	next.Revision++
	next.Operation.Phase = phase
	accepted, err := r.operations.AdvanceWorkerControlOperation(ctx, next, operation.Revision)
	if err != nil && !errors.Is(err, recordings.ErrWorkerControlConflict) {
		// Advance may have synced the row before losing its acknowledgement.
		// Load reconciles uncertain journal bytes through the same sync boundary.
		// An explicit conflict is a refusal, never an uncertain acknowledgement.
		// Only the exact attempted snapshot licenses proceeding; never re-append
		// a stage or infer admission from an opening/current session alone.
		loaded, loadErr := r.operations.LoadWorkerControlOperation(ctx, interruptOperationKey(next))
		if loadErr == nil && sameInterruptPhaseRecord(loaded, next) {
			accepted, err = loaded, nil
		}
	}
	if err == nil {
		*operation = accepted
	}
	return err
}

func sameInterruptPhaseRecord(record, expected recordings.WorkerControlOperationRecord) bool {
	return record.Target == expected.Target && record.Revision == expected.Revision &&
		record.Operation == expected.Operation && record.InputArtifactRef == expected.InputArtifactRef &&
		record.FailureCode == expected.FailureCode && bytes.Equal(record.Result, expected.Result)
}

func (r *registry) commitInterruptResult(ctx context.Context, operation *recordings.WorkerControlOperationRecord, result workersessions.InterruptResult, interruptErr error) error {
	phase := "COMPLETED"
	if interruptErr != nil {
		phase = "FAILED"
	}
	return r.commitInterruptPhase(ctx, operation, phase, result, interruptErr)
}

// A phase and its detached facts share one sync acknowledgement. Recovery can
// report a joined source or admitted successor even before the final outcome,
// without promoting those observations into authority to repeat an effect.
func (r *registry) commitInterruptPhase(ctx context.Context, operation *recordings.WorkerControlOperationRecord, phase string, result workersessions.InterruptResult, interruptErr error) error {
	if operation == nil {
		return nil
	}
	// Persist control facts without provider content, terminal diagnostics or
	// execution secrets. Session/history reads retain their canonical owners.
	safe := result.Clone()
	safe.Source = interruptSessionFacts(result.Source)
	safe.Successor = interruptSessionFacts(result.Successor)
	operation.Result, _ = json.Marshal(durableInterruptOutcome{InterruptResult: safe, FailureCauses: interruptFailureCodes(interruptErr)})
	operation.FailureCode = ""
	if interruptErr != nil {
		operation.FailureCode = string(result.Phase)
	}
	return r.advanceInterruptPhase(ctx, operation, phase)
}

// Lineage is part of the accepted snapshot, rather than a lookup of a session
// that may have continued again by the time the operator retries.
func interruptSessionFacts(session workersessions.Session) workersessions.Session {
	return workersessions.Session{
		ID: session.ID, State: session.State,
		PredecessorWorkerSessionID: session.PredecessorWorkerSessionID,
		SuccessorWorkerSessionID:   session.SuccessorWorkerSessionID,
	}
}
