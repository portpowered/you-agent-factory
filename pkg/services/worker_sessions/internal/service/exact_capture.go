package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// A public direct start does not require the customer to choose a recording
// identity. Bind its opening and recipe to one deterministic store identity.
func (r *registry) bindDirectRecording(req *workersessions.StartRequest) {
	if r.logs == nil || r.recording == nil || req.Execution.Execution.RecordingID != "" {
		return
	}
	digest := sha256.Sum256([]byte(req.ID))
	req.Execution.Execution.RecordingID = fmt.Sprintf("direct-%x", digest)
}

// Unreconstructible requests remain invocable. Only an actual artifact-store
// failure rejects admission; unsafe settings never become a changed recipe.
func (r *registry) saveDirectRestartRecipe(ctx context.Context, req workersessions.InvokeSessionRequest) error {
	if _, metadata, ok := r.loadObservationState(req.ID); !ok || !metadata.direct {
		return nil
	}
	pub := r.publicationFor(req.ID)
	if pub == nil {
		return nil
	}
	pub.mu.Lock()
	target := pub.capture
	pub.mu.Unlock()
	if target.RecordingID == "" {
		return nil
	}
	target.ExpectedAttemptID = req.Execution.Execution.Dispatch.DispatchID
	if !directRestartRecipeSafe(req.Execution) {
		r.logger.Info("worker session restart recipe unavailable", "sessionID", publicWorkerID(req.ID), "attemptID", target.ExpectedAttemptID, "outcome", "unsafe_input")
		return nil
	}
	err := r.restart.SaveWorkerRestartRecipe(ctx, target, req.Execution)
	if errors.Is(err, recordings.ErrInvalidRecordingRedactionRequest) {
		r.logger.Info("worker session restart recipe unavailable", "sessionID", publicWorkerID(req.ID), "attemptID", target.ExpectedAttemptID, "outcome", "unsafe_input")
		return nil
	}
	return err
}

func directRestartRecipeSafe(execution workers.WorkstationDispatchRequest) bool {
	payload, err := json.Marshal(execution)
	return err == nil && interruptExecutionReplaySafe(execution.Execution) &&
		interruptRecipeSafe(payload, execution.Execution.ProcessEnvironment)
}

// The caller holds pub.mu across opening acknowledgement and this binding.
// Only the admitted capture supplies generation/epoch; later catalog entries
// must not upgrade a stale execution handle's authority.
func (r *registry) bindOpeningCapture(ctx context.Context, id string, payload workers.SessionPayload, pub *publication) error {
	if r.logs == nil {
		return nil
	}
	entry, err := r.logs.reader.LookupWorkerSessionCapture(ctx, publicWorkerID(id))
	if err != nil {
		return fmt.Errorf("%w: capture identity unavailable", recordings.ErrWorkerRecordingOpening)
	}
	if entry.WorkerSessionID != publicWorkerID(id) || entry.RecordingID != payload.RecordingID ||
		entry.FactorySessionID != payload.FactorySessionID || entry.RecordingGenerationID == "" || entry.OwnerEpoch == "" {
		return fmt.Errorf("%w: capture identity does not match opening", recordings.ErrWorkerRecordingOpening)
	}
	pub.capture = recordings.WorkerControlTarget{
		RecordingID: entry.RecordingID, WorkerSessionID: entry.WorkerSessionID,
		FactorySessionID: entry.FactorySessionID, RecordingGenerationID: entry.RecordingGenerationID,
		OwnerEpoch: entry.OwnerEpoch,
	}
	return nil
}

// Publication -> registry -> attempt is the admission lock order. Keep the
// immutable capture pinned through the ownership comparison/control claim,
// then release every lock before callbacks, publication, or joining.
func (r *registry) lockFrozenCapture(id string, target frozenControlTarget) (func(), error) {
	pub := r.publicationFor(id)
	if pub != target.publication {
		return nil, staleControlTargetError()
	}
	if pub == nil {
		return func() {}, nil
	}
	pub.mu.Lock()
	if pub.capture != target.capture {
		pub.mu.Unlock()
		return nil, staleControlTargetError()
	}
	return pub.mu.Unlock, nil
}
